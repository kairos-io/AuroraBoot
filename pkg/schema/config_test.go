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
		iso.OverlayUEFI = "/uefi"
		iso.Name = "test-iso"
		iso.HandleDeprecations(log)
		Expect(iso.OverlayRootfs).To(Equal("/rootfs"))
		Expect(iso.OverlayUEFI).To(Equal("/uefi"))
		Expect(iso.Name).To(Equal("test-iso"))
	})
})
