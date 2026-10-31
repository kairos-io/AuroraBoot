package config

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/kairos-io/AuroraBoot/deployer"
	"github.com/kairos-io/AuroraBoot/internal"
	"github.com/kairos-io/AuroraBoot/pkg/schema"
	"github.com/kairos-io/kairos/v4/sdk/unstructured"
	"gopkg.in/yaml.v3"
)

const (
	delimLeft  = "[[["
	delimRight = "]]]"

	// maxDownloadSize caps what a config or cloud-config fetched over HTTP is
	// allowed to expand to in memory. The body is read into a string and then
	// handed to text/template, so an unbounded read is an unbounded allocation
	// driven by whoever answers the URL. Real configs are kilobytes; 10 MiB
	// leaves room for a very generous one and still refuses a feed that never
	// ends.
	maxDownloadSize = 10 * 1024 * 1024
)

// downloadTimeout bounds one fetch end to end: connect, headers and body.
// net/http's default client has no timeout at all, so a server that accepts the
// connection and then trickles, or never answers, hangs the whole build with no
// way out but a kill. A variable rather than a constant so a test can lower it.
var downloadTimeout = 30 * time.Second

func isUrl(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme != "" && u.Host != ""
}

func downloadFile(url string) (content string, err error) {
	b := bytes.NewBuffer([]byte{})
	// Get the data
	client := &http.Client{Timeout: downloadTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Check server response
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("bad status: %s", resp.Status)
	}

	// A declared length over the cap is refused before a byte of the body is
	// read. ContentLength is -1 when the length is unknown, which the bounded
	// read below covers instead.
	if resp.ContentLength > maxDownloadSize {
		return "", fmt.Errorf("%s is %d bytes, over the %d byte limit for a config", url, resp.ContentLength, maxDownloadSize)
	}

	// Read one byte past the cap, so a body that stops exactly at the cap is
	// still accepted and anything longer is refused.
	n, err := io.Copy(b, io.LimitReader(resp.Body, maxDownloadSize+1))
	if err != nil {
		return "", err
	}
	if n > maxDownloadSize {
		return "", fmt.Errorf("%s is over the %d byte limit for a config", url, maxDownloadSize)
	}

	return b.String(), nil
}

func render(data string, foo any) (string, error) {
	t, err := template.New("cloudConfig template").Delims(delimLeft, delimRight).Option("missingkey=zero").Parse(data)
	if err != nil {
		internal.Log.Logger.Error().Err(err).Msg("Parsing data")
		internal.Log.Logger.Debug().Err(err).Str("data", data).Str("Left delimiter", delimLeft).Str("Right delimiter", delimRight).Msg("Parsing data")
		return "", err
	}
	b := bytes.NewBuffer([]byte{})
	err = t.Execute(b, foo)
	if err != nil {
		return "", err
	}
	return b.String(), nil
}

// ReadConfig reads and parses configuration from various sources
func ReadConfig(fileConfig, cloudConfig string, options []string) (*schema.Config, *schema.ReleaseArtifact, error) {
	c := &schema.Config{}
	r := &schema.ReleaseArtifact{}

	if fileConfig != "" {
		var err error
		if isUrl(fileConfig) {
			var d string
			d, err = downloadFile(fileConfig)
			if err != nil {
				return c, r, err
			}
			c, r, err = deployer.LoadByte([]byte(d))
			if err != nil {
				return c, r, err
			}
		} else {
			c, r, err = deployer.LoadFile(fileConfig)
			if err != nil {
				return c, r, err
			}
		}
	}

	m := map[string]interface{}{}
	var templateValues map[string]interface{}

	for _, c := range options {
		i := strings.Index(c, "=")
		if i != -1 {
			k := c[:i]
			// Old values to new, clear values
			if strings.ToLower(k) == "disk.raw" {
				k = "disk.efi"
			}
			if strings.ToLower(k) == "disk.mbr" {
				k = "disk.bios"
			}
			v := c[i+1:]
			m[k] = v
		} else {
			return nil, nil, fmt.Errorf("Invalid arguments for set")
		}
	}

	y, err := unstructured.ToYAML(m)
	if err != nil {
		return c, r, err
	}

	// Apply the --set values (as YAML) onto each target, surfacing type
	// mismatches instead of dropping them. One loop rather than three copies so
	// the single error path is exercised by any target's failure.
	for _, target := range []struct {
		dst   any
		label string
	}{
		{c, "config"},
		{r, "release artifact"},
		{&templateValues, "template values"},
	} {
		if err := yaml.Unmarshal(y, target.dst); err != nil {
			return c, r, fmt.Errorf("applying --set values to %s: %w", target.label, err)
		}
	}

	if cloudConfig != "" {
		var err error
		c.CloudConfig, err = ReadCloudConfig(cloudConfig, templateValues)
		if err != nil {
			return nil, nil, err
		}
		internal.Log.Logger.Debug().Str("cc", c.CloudConfig).Msg("Cloud config")
	}

	return c, r, nil
}

// ReadCloudConfig reads and processes cloud configuration from various sources
func ReadCloudConfig(cloudConfig string, templateValues map[string]interface{}) (string, error) {
	result := ""
	if cloudConfig == "-" {
		d, err := io.ReadAll(os.Stdin)
		if err != nil {
			return result, fmt.Errorf("error reading from STDIN")
		}
		result, err = render(string(d), templateValues)
		if err != nil {
			return result, err
		}
	} else if _, err := os.Stat(cloudConfig); err == nil {
		dat, err := os.ReadFile(cloudConfig)
		if err == nil {
			result, err = render(string(dat), templateValues)
			if err != nil {
				return result, err
			}
		}
	} else if isUrl(cloudConfig) {
		d, err := downloadFile(cloudConfig)
		if err != nil {
			return result, err
		}
		result, err = render(d, templateValues)
		if err != nil {
			return result, err
		}
	} else {
		return result, fmt.Errorf("file '%s' not found", cloudConfig)
	}

	if result == "" {
		return result, fmt.Errorf("cloud config set but contents are empty. Check that the content of the file is correct or the path is the proper one")
	}

	return result, nil
}
