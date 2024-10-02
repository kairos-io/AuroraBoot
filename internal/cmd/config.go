package cmd

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"text/template"

	"github.com/kairos-io/AuroraBoot/pkg/schema"
	"github.com/rs/zerolog/log"

	"github.com/kairos-io/AuroraBoot/deployer"
	"github.com/kairos-io/kairos/sdk/unstructured"
	"gopkg.in/yaml.v1"
)

const delimLeft = "[[["
const delimright = "]]]"

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

func render(data string, foo any) (string, error) {
	t, err := template.New("cloudConfig template").Delims(delimLeft, delimright).Option("missingkey=zero").Parse(data)
	if err != nil {
		log.Logger.Error().Err(err).Msg("Parsing data")
		log.Logger.Debug().Err(err).Str("data", data).Str("Left delimiter", delimLeft).Str("Right delimiter", delimright).Msg("Parsing data")
		return "", err
	}
	b := bytes.NewBuffer([]byte{})
	err = t.Execute(b, foo)
	if err != nil {
		return "", err
	}
	return b.String(), nil
}

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
		if cloudConfig == "-" {
			d, err := ioutil.ReadAll(os.Stdin)
			if err != nil {
				return c, r, fmt.Errorf("error reading from STDIN")
			}
			c.CloudConfig, err = render(string(d), templateValues)
			if err != nil {
				return nil, nil, err
			}
		} else {
			if _, err := os.Stat(cloudConfig); err == nil {
				dat, err := os.ReadFile(cloudConfig)
				if err == nil {
					c.CloudConfig, err = render(string(dat), templateValues)
					if err != nil {
						return nil, nil, err
					}
				}
			} else if isUrl(cloudConfig) {
				d, err := downloadFile(cloudConfig)
				if err != nil {
					return c, r, err
				}
				c.CloudConfig, err = render(d, templateValues)
				if err != nil {
					return nil, nil, err
				}
			} else {
				return c, r, fmt.Errorf("file '%s' not found", cloudConfig)
			}
		}
		if c.CloudConfig == "" {
			return nil, nil, fmt.Errorf("cloud config set but contents are empty. Check that the content of the file is correct or the path is the proper one")
		}
		log.Debug().Str("cc", c.CloudConfig).Msg("Cloud config")
	}
	
	return c, r, nil
}
