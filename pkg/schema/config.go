package schema

import "path/filepath"

// Config represent the AuroraBoot
// configuration
type Config struct {
	// CloudConfig to use for generating installation mediums
	CloudConfig string `yaml:"cloud_config"`

	// Disable Netboot
	DisableNetboot bool `yaml:"disable_netboot"`

	// Disable HTTP Server
	DisableHTTPServer bool `yaml:"disable_http_server"`

	// Disable manual ISO boot
	DisableISOboot bool `yaml:"disable_iso"`

	// PixieCore HTTPServer Port
	NetBootHTTPPort string `yaml:"netboot_http_port"`

	// PixieCore Listen addr
	NetBootListenAddr string `yaml:"netboot_listen_addr"`

	State string `yaml:"state_dir"`

	ListenAddr string `yaml:"listen_addr"`

	// ISO block configuration
	ISO ISO `yaml:"iso"`

	// Netboot block configuration
	NetBoot NetBoot `yaml:"netboot"`

	Disk Disk `yaml:"disk"`

	System System `yaml:"system"`

	// Artifact branding
	ArtifactName string `yaml:"artifact_name"` // Custom prefix for artifacts (default: "kairos")
}

type System struct {
	Memory  string `yaml:"memory"`
	Cores   string `yaml:"cores"`
	Qemubin string `yaml:"qemu_bin"`
	KVM     bool   `yaml:"kvm"`
}

type Disk struct {
	EFI       bool   `yaml:"efi"`
	GCE       bool   `yaml:"gce"`
	VHD       bool   `yaml:"vhd"`
	BIOS      bool   `yaml:"bios"`
	Size      string `yaml:"size"`
	StateSize string `yaml:"state_size"`
}

type NetBoot struct {
	Cmdline string `yaml:"cmdline"`
}

type ISO struct {
	DataPath      string `yaml:"data"`
	Name          string `yaml:"name"` // Final artifact base name
	OverrideName  string `yaml:"override_name"`
	IncludeDate   bool   `yaml:"include_date"`
	OverlayISO    string `yaml:"overlay_iso"`
	OverlayRootfs string `yaml:"overlay_rootfs"`
	OverlayUEFI   string `yaml:"overlay_uefi"`
}

func (c Config) StateDir(s ...string) string {
	d := "/tmp"
	if c.State != "" {
		d = c.State
	}

	return filepath.Join(append([]string{d}, s...)...)
}

func (c Config) GetArtifactName() string {
	if c.ArtifactName != "" {
		return c.ArtifactName
	}
	return "kairos" // Default artifact name
}
