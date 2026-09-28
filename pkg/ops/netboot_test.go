package ops

import (
	"context"
	"os"
	"path/filepath"

	"github.com/diskfs/go-diskfs"
	"github.com/diskfs/go-diskfs/disk"
	"github.com/diskfs/go-diskfs/filesystem"
	"github.com/diskfs/go-diskfs/filesystem/iso9660"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// writeISO builds a real ISO 9660 image holding files, keyed by their path
// inside the image. Rock Ridge is on because that is how the Kairos ISOs are
// built, and it is what keeps the names readable rather than "FOO.RAW;1".
func writeISO(files map[string]string) string {
	path := filepath.Join(GinkgoT().TempDir(), "test.iso")

	img, err := diskfs.Create(path, 8*1024*1024, diskfs.SectorSize(2048))
	Expect(err).NotTo(HaveOccurred())
	fs, err := img.CreateFilesystem(disk.FilesystemSpec{Partition: 0, FSType: filesystem.TypeISO9660, VolumeLabel: "TEST"})
	Expect(err).NotTo(HaveOccurred())

	for name, content := range files {
		if dir := filepath.Dir(name); dir != "/" && dir != "." {
			Expect(fs.Mkdir(dir)).To(Succeed())
		}
		f, err := fs.OpenFile(name, os.O_CREATE|os.O_RDWR)
		Expect(err).NotTo(HaveOccurred())
		_, err = f.Write([]byte(content))
		Expect(err).NotTo(HaveOccurred())
		Expect(f.Close()).To(Succeed())
	}

	Expect(fs.(*iso9660.FileSystem).Finalize(iso9660.FinalizeOptions{RockRidge: true})).To(Succeed())
	Expect(img.Close()).To(Succeed())
	return path
}

// netbootISO is the three paths every netboot extraction reads, so a spec only
// has to describe what it adds on top of them.
func netbootISO(extra map[string]string) string {
	files := map[string]string{
		"/rootfs.squashfs": "squashfs",
		"/boot/kernel":     "kernel",
		"/boot/initrd":     "initrd",
	}
	for name, content := range extra {
		files[name] = content
	}
	return writeISO(files)
}

func declaration(names ...string) string {
	body := "#cloud-config\ninstall:\n  extensions:\n"
	for _, name := range names {
		body += "    - name: " + name + "\n"
	}
	return body
}

var _ = Describe("ExtractNetboot", Label("iso"), func() {
	var dst string

	BeforeEach(func() {
		dst = GinkgoT().TempDir()
	})

	extract := func(src string) error {
		return ExtractNetboot(
			func() string { return src },
			func() string { return dst },
			"kairos",
		)(context.Background())
	}

	It("extracts the squashfs, the kernel and the initrd", func() {
		Expect(extract(netbootISO(nil))).To(Succeed())

		Expect(os.ReadFile(filepath.Join(dst, "kairos.squashfs"))).To(BeEquivalentTo("squashfs"))
		Expect(os.ReadFile(filepath.Join(dst, "kairos-kernel"))).To(BeEquivalentTo("kernel"))
		Expect(os.ReadFile(filepath.Join(dst, "kairos-initrd"))).To(BeEquivalentTo("initrd"))
	})

	// Regression for kairos-io/kairos#5040: a bundled extension is materialized
	// into the ISO root, not into rootfs.squashfs, so before this it was dropped
	// on the way to the netboot artifacts and a netboot tree built from an
	// artifact spec with extensions silently carried none.
	It("carries the bundled extension images named by the declaration", func() {
		src := netbootISO(map[string]string{
			"/extensions.yaml":  declaration("/run/initramfs/live/work.sysext.raw", "/run/initramfs/live/tools.sysext.raw"),
			"/work.sysext.raw":  "work image",
			"/tools.sysext.raw": "tools image",
		})

		Expect(extract(src)).To(Succeed())

		Expect(os.ReadFile(filepath.Join(dst, "work.sysext.raw"))).To(BeEquivalentTo("work image"))
		Expect(os.ReadFile(filepath.Join(dst, "tools.sysext.raw"))).To(BeEquivalentTo("tools image"))
	})

	It("carries nothing when the ISO declares no extensions", func() {
		Expect(extract(netbootISO(map[string]string{"/work.sysext.raw": "not declared"}))).To(Succeed())

		Expect(filepath.Join(dst, "work.sysext.raw")).NotTo(BeAnExistingFile())
		entries, err := os.ReadDir(dst)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(3))
	})

	// A user's own extensions.yaml can arrive through --overlay-iso naming an
	// image the installed system fetches for itself. Nothing on the ISO root
	// answers to that name, and it is not this build's to carry.
	It("skips a declared extension that is not a file on the live media", func() {
		src := netbootISO(map[string]string{
			"/extensions.yaml": declaration("oci://ghcr.io/example/tools.sysext.raw", "https://example.com/more.sysext.raw"),
		})

		Expect(extract(src)).To(Succeed())

		entries, err := os.ReadDir(dst)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(3))
	})

	// AuroraBoot writes the declaration and the images together, so the two
	// disagreeing means the ISO is inconsistent. Failing is what keeps a netboot
	// tree from quietly lacking an extension the ISO install gets.
	It("fails when the declaration names an image the ISO root does not carry", func() {
		src := netbootISO(map[string]string{
			"/extensions.yaml": declaration("/run/initramfs/live/missing.sysext.raw"),
		})

		err := extract(src)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("missing.sysext.raw"))
		Expect(err.Error()).To(ContainSubstring("the ISO root does not carry it"))
	})
})
