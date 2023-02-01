package deployer

import "path/filepath"

type Config struct {
	CloudConfig string `yaml:"cloud_config"`

	DisableNetboot bool
	DisableISOboot bool

	State string
}

func (c Config) StateDir(s ...string) string {
	d := "/tmp"
	if c.State != "" {
		d = c.State
	}

	return filepath.Join(append([]string{d}, s...)...)
}
