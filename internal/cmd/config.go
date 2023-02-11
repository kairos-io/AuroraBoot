package cmd

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/template"

	"github.com/rs/zerolog/log"

	"github.com/kairos-io/AuroraBoot/deployer"
	"github.com/kairos-io/kairos/sdk/unstructured"
	"gopkg.in/yaml.v1"
)

func isUrl(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme != "" && u.Host != ""
}

func downloadFile(url string) (content string, err error) {

	b := bytes.NewBuffer([]byte{})
	// Get the data
	resp, err := http.Get(url)
	if err != nil {
		return b.String(), err
	}
	defer resp.Body.Close()

	// Check server response
	if resp.StatusCode != http.StatusOK {
		return b.String(), fmt.Errorf("bad status: %s", resp.Status)
	}

	// Writer the body to file
	_, err = io.Copy(b, resp.Body)
	if err != nil {
		return b.String(), err
	}

	return b.String(), nil
}

func render(data string, foo any) string {
	t := template.New("cloudConfig template").Delims("[[", "]]").Option("missingkey=zero")
	t, _ = t.Parse(data)
	b := bytes.NewBuffer([]byte{})
	t.Execute(b, foo)
	return b.String()
}

func ReadConfig(fileConfig, cloudConfig string, options []string) (*deployer.Config, *deployer.ReleaseArtifact, error) {
	c := &deployer.Config{}
	r := &deployer.ReleaseArtifact{}

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

	yaml.Unmarshal(y, c)
	yaml.Unmarshal(y, r)
	yaml.Unmarshal(y, &templateValues)

	if cloudConfig != "" {
		if _, err := os.Stat(cloudConfig); err == nil {
			dat, err := os.ReadFile(cloudConfig)
			if err == nil {
				c.CloudConfig = render(string(dat), templateValues)
			}
		} else if isUrl(cloudConfig) {
			d, err := downloadFile(cloudConfig)
			if err != nil {
				return c, r, err
			}
			c.CloudConfig = render(d, templateValues)
		} else {
			return c, r, fmt.Errorf("file '%s' not found", cloudConfig)
		}
	}
	log.Print(c.CloudConfig)
	return c, r, nil
}
