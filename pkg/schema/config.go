package schema

import (
	"path/filepath"

	"github.com/kairos-io/kairos-sdk/types/logger"
)

// Config represent the AuroraBoot
// configuration
type Config struct {
	// CloudConfig to use for generating installation mediums
	CloudConfig string `yaml:"cloud_config"`

	// NoDefaultCloudConfig to skip injecting default cloud config if user doesn't provide one
	NoDefaultCloudConfig bool `yaml:"no_default_cloud_config"`

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

	// Architecture to use for container image pulling (e.g., "amd64", "arm64")
	Arch string `yaml:"arch"`

	// AllowInsecureRegistries allows pulling container images from registries
	// over plain HTTP or presenting untrusted/self-signed TLS certificates.
	// It is a pointer so that HandleDeprecations can distinguish "explicitly
	// set to false" from "absent"; the new key always wins when both it and
	// the deprecated "insecure" key are present.
	AllowInsecureRegistries *bool `yaml:"allow-insecure-registries"`

	// DeprecatedInsecure preserves the old "insecure" config key so configs
	// written against v0.22.0 keep working. Prefer "allow-insecure-registries".
	// Reconciled into AllowInsecureRegistries by HandleDeprecations.
	DeprecatedInsecure bool `yaml:"insecure"`

	// ISO block configuration
	ISO ISO `yaml:"iso"`

	// Netboot block configuration
	NetBoot NetBoot `yaml:"netboot"`

	Disk Disk `yaml:"disk"`

	System System `yaml:"system"`
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
	// ExtendLiveCmdline is appended to the kernel cmdline when booting from the live/installer ISO. Does not affect the installed system.
	ExtendLiveCmdline string `yaml:"extend-live-cmdline"`
}

// HandleDeprecations checks for deprecated ISO options and migrates them.
// iso.data is deprecated in favor of iso.overlay_iso. If iso.data is set,
// its value is moved to iso.overlay_iso (unless overlay_iso is already set)
// and a deprecation warning is logged.
func (i *ISO) HandleDeprecations(log logger.KairosLogger) {
	if i.DataPath == "" {
		return
	}

	if i.OverlayISO == "" {
		log.Logger.Warn().Msg("'iso.data' is deprecated and will be removed in a future release. Use 'iso.overlay_iso' instead.")
		i.OverlayISO = i.DataPath
	} else {
		log.Logger.Warn().Msg("'iso.data' is deprecated and will be removed in a future release. Both 'iso.data' and 'iso.overlay_iso' are set; 'iso.data' will be ignored.")
	}
	i.DataPath = ""
}

// HandleDeprecations reconciles deprecated top-level config keys into their
// current equivalents. The old "insecure" key is migrated to
// "allow-insecure-registries" only when the new key is absent; if the new key
// is already set (to any value, including false) it is left untouched so that
// an explicit "allow-insecure-registries: false" is never overridden.
func (c *Config) HandleDeprecations(log logger.KairosLogger) {
	if c.DeprecatedInsecure {
		log.Logger.Warn().Msg("'insecure' is deprecated and will be removed in a future release. Use 'allow-insecure-registries' instead.")
		if c.AllowInsecureRegistries == nil {
			t := true
			c.AllowInsecureRegistries = &t
		}
		c.DeprecatedInsecure = false
	}
}

// AllowInsecureRegistriesBool returns the resolved boolean value of
// AllowInsecureRegistries, returning false when the field has not been set.
func (c *Config) AllowInsecureRegistriesBool() bool {
	if c.AllowInsecureRegistries == nil {
		return false
	}
	return *c.AllowInsecureRegistries
}

func (c Config) StateDir(s ...string) string {
	d := "/tmp"
	if c.State != "" {
		d = c.State
	}

	return filepath.Join(append([]string{d}, s...)...)
}
