package cmd

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

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
	for _, c := range options {
		dat := strings.Split(c, "=")
		if len(dat) != 2 {
			return nil, nil, fmt.Errorf("Invalid arguments for set")
		}
		m[dat[0]] = dat[1]
	}

	y, err := unstructured.ToYAML(m)
	if err != nil {
		return c, r, err
	}

	yaml.Unmarshal(y, c)
	yaml.Unmarshal(y, r)

	if cloudConfig != "" {
		if _, err := os.Stat(cloudConfig); err == nil {
			dat, err := os.ReadFile(cloudConfig)
			if err == nil {
				c.CloudConfig = string(dat)
			}
		} else if isUrl(cloudConfig) {
			d, err := downloadFile(cloudConfig)
			if err != nil {
				return c, r, err
			}
			c.CloudConfig = d
		} else {
			return c, r, fmt.Errorf("file '%s' not found", cloudConfig)
		}
	}

	return c, r, nil
}
