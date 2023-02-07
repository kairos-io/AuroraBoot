package deployer

import "path/filepath"

// Config represent the AuroraBoot
// configuration
type Config struct {
	// CloudConfig to use for generating installation mediums
	CloudConfig string `yaml:"cloud_config"`

	// Disable Netboot
	DisableNetboot bool `yaml:"disable_netboot"`

	// Disable manual ISO boot
	DisableISOboot bool `yaml:"disable_iso"`

	State string `yaml:"state_dir"`

	ListenAddr string `yaml:"listen_addr"`
}

func (c Config) StateDir(s ...string) string {
	d := "/tmp"
	if c.State != "" {
		d = c.State
	}

	return filepath.Join(append([]string{d}, s...)...)
}
