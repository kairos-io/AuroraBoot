package schema_test

import (
	"github.com/kairos-io/AuroraBoot/pkg/schema"
	"github.com/kairos-io/kairos-sdk/types/logger"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ISO HandleDeprecations", func() {
	var (
		iso schema.ISO
		log logger.KairosLogger
	)

	BeforeEach(func() {
		iso = schema.ISO{}
		log = logger.NewKairosLogger("test", "error", false)
	})

	It("does nothing when neither iso.data nor iso.overlay_iso are set", func() {
		iso.HandleDeprecations(log)
		Expect(iso.DataPath).To(BeEmpty())
		Expect(iso.OverlayISO).To(BeEmpty())
	})

	It("migrates iso.data to iso.overlay_iso when only iso.data is set", func() {
		iso.DataPath = "/some/path"
		iso.HandleDeprecations(log)
		Expect(iso.OverlayISO).To(Equal("/some/path"))
		Expect(iso.DataPath).To(BeEmpty())
	})

	It("keeps iso.overlay_iso and clears iso.data when both are set", func() {
		iso.DataPath = "/old/path"
		iso.OverlayISO = "/new/path"
		iso.HandleDeprecations(log)
		Expect(iso.OverlayISO).To(Equal("/new/path"))
		Expect(iso.DataPath).To(BeEmpty())
	})

	It("preserves iso.overlay_iso when only iso.overlay_iso is set", func() {
		iso.OverlayISO = "/overlay/path"
		iso.HandleDeprecations(log)
		Expect(iso.OverlayISO).To(Equal("/overlay/path"))
		Expect(iso.DataPath).To(BeEmpty())
	})

	It("does not affect other ISO fields", func() {
		iso.DataPath = "/some/path"
		iso.OverlayRootfs = "/rootfs"
		iso.Name = "test-iso"
		iso.HandleDeprecations(log)
		Expect(iso.OverlayRootfs).To(Equal("/rootfs"))
		Expect(iso.Name).To(Equal("test-iso"))
	})
})

var _ = Describe("Config HandleDeprecations", func() {
	var (
		cfg schema.Config
		log logger.KairosLogger
	)

	BeforeEach(func() {
		cfg = schema.Config{}
		log = logger.NewKairosLogger("test", "error", false)
	})

	It("does nothing when neither key is set", func() {
		cfg.HandleDeprecations(log)
		Expect(cfg.AllowInsecureRegistriesBool()).To(BeFalse())
		Expect(cfg.DeprecatedInsecure).To(BeFalse())
	})

	It("migrates the deprecated insecure key to allow-insecure-registries", func() {
		cfg.DeprecatedInsecure = true
		cfg.HandleDeprecations(log)
		Expect(cfg.AllowInsecureRegistriesBool()).To(BeTrue())
		Expect(cfg.DeprecatedInsecure).To(BeFalse())
	})

	It("keeps allow-insecure-registries when only that key is set", func() {
		t := true
		cfg.AllowInsecureRegistries = &t
		cfg.HandleDeprecations(log)
		Expect(cfg.AllowInsecureRegistriesBool()).To(BeTrue())
		Expect(cfg.DeprecatedInsecure).To(BeFalse())
	})

	It("does not clobber allow-insecure-registries when both keys are set to true", func() {
		t := true
		cfg.AllowInsecureRegistries = &t
		cfg.DeprecatedInsecure = true
		cfg.HandleDeprecations(log)
		Expect(cfg.AllowInsecureRegistriesBool()).To(BeTrue())
		Expect(cfg.DeprecatedInsecure).To(BeFalse())
	})

	It("does not override an explicit false allow-insecure-registries when insecure is true", func() {
		f := false
		cfg.AllowInsecureRegistries = &f
		cfg.DeprecatedInsecure = true
		cfg.HandleDeprecations(log)
		Expect(cfg.AllowInsecureRegistriesBool()).To(BeFalse())
		Expect(cfg.DeprecatedInsecure).To(BeFalse())
	})
})

var _ = Describe("Config Validate", func() {
	var cfg schema.Config

	BeforeEach(func() {
		cfg = schema.Config{}
	})

	It("passes with no disk options set", func() {
		Expect(cfg.Validate()).To(Succeed())
	})

	It("passes for a plain EFI raw disk build", func() {
		cfg.Disk.EFI = true
		Expect(cfg.Validate()).To(Succeed())
	})

	It("passes for partition-image output on its own", func() {
		cfg.Disk.Partitions = true
		Expect(cfg.Validate()).To(Succeed())
	})

	It("passes for partition-image output combined with efi", func() {
		cfg.Disk.Partitions = true
		cfg.Disk.EFI = true
		Expect(cfg.Validate()).To(Succeed())
	})

	It("rejects partition-image output combined with gce", func() {
		cfg.Disk.Partitions = true
		cfg.Disk.GCE = true
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("partitions"))
		Expect(err.Error()).To(ContainSubstring("gce"))
	})

	It("rejects partition-image output combined with vhd", func() {
		cfg.Disk.Partitions = true
		cfg.Disk.VHD = true
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("partitions"))
		Expect(err.Error()).To(ContainSubstring("vhd"))
	})
})
