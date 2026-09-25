package ops

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/kairos-io/kairos/v4/sdk/collector"
	sdkConfig "github.com/kairos-io/kairos/v4/sdk/types/config"
	extensiontypes "github.com/kairos-io/kairos/v4/sdk/types/extensions"
	"github.com/kairos-io/kairos/v4/sdk/types/logger"

	"github.com/kairos-io/AuroraBoot/pkg/constants"
	"github.com/kairos-io/AuroraBoot/pkg/extensions"
	"github.com/kairos-io/AuroraBoot/pkg/schema"
	sdkutils "github.com/kairos-io/kairos/v4/sdk/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/twpayne/go-vfs/v5/vfst"
	"gopkg.in/yaml.v3"
)

var _ = Describe("materializeISOExtensions", func() {
	It("does nothing when no extensions are configured", func() {
		called := false
		materializeExtensionArtifacts = func(context.Context, []string, []extensions.Request, string, string, bool) ([]string, error) {
			called = true
			return nil, nil
		}
		DeferCleanup(func() { materializeExtensionArtifacts = extensions.Materialize })

		Expect(materializeISOExtensions(context.Background(), schema.ISO{}, "amd64", "/unused", false)).To(Succeed())
		Expect(called).To(BeFalse())
	})

	It("uses the target architecture and ISO root directory", func() {
		tmp := GinkgoT().TempDir()
		requests := []extensions.Request{{Name: "foo", Version: "v1"}}
		materializeExtensionArtifacts = func(_ context.Context, catalogs []string, got []extensions.Request, arch, destination string, insecure bool) ([]string, error) {
			Expect(catalogs).To(Equal([]string{"catalog.yaml"}))
			Expect(got).To(Equal(requests))
			Expect(arch).To(Equal("arm64"))
			Expect(destination).To(Equal(tmp))
			Expect(insecure).To(BeTrue())
			path := filepath.Join(destination, "foo.sysext.raw")
			return []string{path}, os.WriteFile(path, []byte("raw"), 0o644)
		}
		DeferCleanup(func() { materializeExtensionArtifacts = extensions.Materialize })

		iso := schema.ISO{ExtensionsCatalogs: []string{"catalog.yaml"}, Extensions: requests}
		Expect(materializeISOExtensions(context.Background(), iso, "arm64", tmp, true)).To(Succeed())
		Expect(filepath.Join(tmp, "foo.sysext.raw")).To(BeAnExistingFile())
	})

	// Regression: a classic (non-UKI) install ignores a raw image that only
	// sits on the live media. The installer stages only what install.extensions
	// declares, so without this declaration the node boots with no extension.
	It("declares every materialized extension under install.extensions", func() {
		tmp := GinkgoT().TempDir()
		materializeExtensionArtifacts = func(_ context.Context, _ []string, _ []extensions.Request, _, destination string, _ bool) ([]string, error) {
			return []string{
				filepath.Join(destination, "tailscale.sysext.raw"),
				filepath.Join(destination, "drbd.sysext.raw"),
			}, nil
		}
		DeferCleanup(func() { materializeExtensionArtifacts = extensions.Materialize })

		iso := schema.ISO{Extensions: []extensions.Request{{Name: "tailscale"}, {Name: "drbd"}}}
		Expect(materializeISOExtensions(context.Background(), iso, "amd64", tmp, false)).To(Succeed())

		declaration, err := os.ReadFile(filepath.Join(tmp, isoExtensionsConfig))
		Expect(err).ToNot(HaveOccurred())
		// The collector skips a config without the header, which would drop
		// the declaration without a word.
		Expect(collector.HasValidHeader(string(declaration))).To(BeTrue())

		var parsed struct {
			Install struct {
				Extensions extensiontypes.Extensions `yaml:"extensions"`
			} `yaml:"install"`
		}
		Expect(yaml.Unmarshal(declaration, &parsed)).To(Succeed())
		Expect(parsed.Install.Extensions).To(Equal(extensiontypes.Extensions{
			{Name: "/run/initramfs/live/tailscale.sysext.raw"},
			{Name: "/run/initramfs/live/drbd.sysext.raw"},
		}))
	})

	// What the installer sees is the merge of every config in the live media
	// root, so check the declaration through the agent's own collector: it
	// has to add to what the user declared in config.yaml, not replace it.
	It("adds to the install.extensions of the user's config.yaml", func() {
		tmp := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(tmp, "config.yaml"),
			[]byte("#cloud-config\ninstall:\n  auto: true\n  extensions:\n    - name: fwupd\n"), 0o644)).To(Succeed())
		materializeExtensionArtifacts = func(_ context.Context, _ []string, _ []extensions.Request, _, destination string, _ bool) ([]string, error) {
			return []string{filepath.Join(destination, "tailscale.sysext.raw")}, nil
		}
		DeferCleanup(func() { materializeExtensionArtifacts = extensions.Materialize })

		iso := schema.ISO{Extensions: []extensions.Request{{Name: "tailscale"}}}
		Expect(materializeISOExtensions(context.Background(), iso, "amd64", tmp, false)).To(Succeed())

		merged, err := collector.Scan(&collector.Options{ScanDir: []string{tmp}, NoLogs: true}, nil)
		Expect(err).ToNot(HaveOccurred())
		rendered, err := merged.String()
		Expect(err).ToNot(HaveOccurred())
		var parsed struct {
			Install struct {
				Auto       bool                      `yaml:"auto"`
				Extensions extensiontypes.Extensions `yaml:"extensions"`
			} `yaml:"install"`
		}
		Expect(yaml.Unmarshal([]byte(rendered), &parsed)).To(Succeed())
		Expect(parsed.Install.Auto).To(BeTrue())
		Expect(parsed.Install.Extensions).To(ConsistOf(
			extensiontypes.Extension{Name: "fwupd"},
			extensiontypes.Extension{Name: "/run/initramfs/live/tailscale.sysext.raw"},
		))
	})

	It("writes no declaration when no extensions are configured", func() {
		tmp := GinkgoT().TempDir()
		Expect(materializeISOExtensions(context.Background(), schema.ISO{}, "amd64", tmp, false)).To(Succeed())
		Expect(filepath.Join(tmp, isoExtensionsConfig)).ToNot(BeAnExistingFile())
	})
})

var _ = Describe("applyGrubTemplate", Label("iso"), func() {
	const templateWithPlaceholders = "linux ($root)/boot/kernel cdroot root=live:CDLABEL=COS_LIVE {{LIVE_CONSOLE}}{{NOMODESET}} install-mode\nlinux ($root)/boot/kernel cdroot{{EXTEND_CMDLINE}}\nmenuentry debug { linux console=tty0 }\n"

	It("replaces NOMODESET and EXTEND_CMDLINE with provided values", func() {
		result := applyGrubTemplate([]byte(templateWithPlaceholders), " nomodeset", " rd.debug rd.shell", "")
		Expect(string(result)).To(ContainSubstring(" nomodeset"))
		Expect(string(result)).To(ContainSubstring(" rd.debug rd.shell"))
		Expect(string(result)).ToNot(ContainSubstring("{{NOMODESET}}"))
		Expect(string(result)).ToNot(ContainSubstring("{{EXTEND_CMDLINE}}"))
	})

	It("replaces EXTEND_CMDLINE with empty string when not provided", func() {
		result := applyGrubTemplate([]byte(templateWithPlaceholders), "", "", "")
		Expect(string(result)).ToNot(ContainSubstring("{{EXTEND_CMDLINE}}"))
		Expect(string(result)).To(ContainSubstring("install-mode\nlinux ($root)/boot/kernel cdroot\n"))
	})

	It("replaces NOMODESET with empty string when not provided", func() {
		result := applyGrubTemplate([]byte(templateWithPlaceholders), "", " rd.debug", "")
		Expect(string(result)).ToNot(ContainSubstring("{{NOMODESET}}"))
		Expect(string(result)).To(ContainSubstring(" rd.debug"))
	})

	It("uses the default live consoles when no override is provided", func() {
		result := applyGrubTemplate(constants.GrubLiveBiosCfg, "", "", "")
		Expect(string(result)).To(ContainSubstring("console=ttyS0 console=tty1"))
		Expect(string(result)).ToNot(ContainSubstring("{{LIVE_CONSOLE}}"))
	})

	It("replaces live consoles while preserving the debug console", func() {
		result := applyGrubTemplate(constants.GrubLiveBiosCfg, "", "", "console=ttyUSB0,115200")
		Expect(string(result)).ToNot(ContainSubstring("console=ttyS0 console=tty1"))
		Expect(strings.Count(string(result), "console=ttyUSB0,115200")).To(Equal(6))
		Expect(string(result)).To(ContainSubstring("console=tty0 rd.debug"))
	})

	It("strips carriage returns and newlines from a live console override", func() {
		result := applyGrubTemplate([]byte("linux {{LIVE_CONSOLE}} end"), "", "", "console=ttyS1\r\nconsole=tty1")
		Expect(string(result)).To(Equal("linux console=ttyS1console=tty1 end"))
	})
})

var _ = Describe("getEfiGrubFilesForArch", Label("iso"), func() {
	It("prepends the openSUSE riscv64 path before SDK paths", func() {
		paths := getEfiGrubFilesForArch("riscv64")
		sdkPaths := sdkutils.GetEfiGrubFiles("riscv64")

		Expect(paths[0]).To(Equal("/usr/share/efi/riscv64/grub.efi"))
		Expect(paths).To(Equal(append([]string{"/usr/share/efi/riscv64/grub.efi"}, sdkPaths...)))
	})

	It("prepends gcdx64.efi.signed on amd64 so the ISO uses the CD grub, not the disk one", func() {
		// gcdx64.efi.signed has iso9660 baked in and its prefix set to
		// /boot/grub, which is what a live ISO needs. grubx64.efi.signed
		// is built for disk installs and its /EFI/ubuntu prefix breaks on
		// firmware that sets $root to the ISO9660 partition.
		paths := getEfiGrubFilesForArch("amd64")
		Expect(paths[0]).To(Equal("/usr/lib/grub/x86_64-efi-signed/gcdx64.efi.signed"))
		Expect(paths).To(ContainElement("/usr/lib/grub/x86_64-efi-signed/grubx64.efi.signed"))
		Expect(indexOf(paths, "/usr/lib/grub/x86_64-efi-signed/gcdx64.efi.signed")).To(
			BeNumerically("<", indexOf(paths, "/usr/lib/grub/x86_64-efi-signed/grubx64.efi.signed")),
		)
	})

	It("prepends gcdaa64.efi.signed on arm64 for the same reason", func() {
		paths := getEfiGrubFilesForArch("arm64")
		Expect(paths[0]).To(Equal("/usr/lib/grub/arm64-efi-signed/gcdaa64.efi.signed"))
		Expect(paths).To(ContainElement("/usr/lib/grub/arm64-efi-signed/grubaa64.efi.signed"))
		Expect(indexOf(paths, "/usr/lib/grub/arm64-efi-signed/gcdaa64.efi.signed")).To(
			BeNumerically("<", indexOf(paths, "/usr/lib/grub/arm64-efi-signed/grubaa64.efi.signed")),
		)
	})

	It("keeps the full SDK path list after the CD-grub prepends", func() {
		amd64Paths := getEfiGrubFilesForArch("amd64")
		for _, p := range sdkutils.GetEfiGrubFiles("amd64") {
			Expect(amd64Paths).To(ContainElement(p))
		}
	})
})

func indexOf(list []string, target string) int {
	for i, s := range list {
		if s == target {
			return i
		}
	}
	return -1
}

var _ = Describe("writeCdGrubEfiCfg", Label("iso"), func() {
	// gcdx64.efi.signed (and its arm64 equivalent) is compiled with prefix
	// /boot/grub. When the CD-media grub is loaded it looks for its config at
	// ($root)/boot/grub/grub.cfg. Without a file at that path the ISO drops
	// to the grub rescue prompt on firmwares where $root is the ISO device.
	It("writes grub.cfg at /boot/grub/grub.cfg with the chainloader stub", func() {
		fs, cleanup, err := vfst.NewTestFS(nil)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(cleanup)

		b := &BuildISOAction{cfg: &BuildConfig{Config: sdkConfig.Config{
			Fs:     fs,
			Logger: logger.NewNullLogger(),
		}}}

		root := "/iso"
		Expect(fs.Mkdir(root, 0o755)).To(Succeed())

		Expect(b.writeCdGrubEfiCfg(root)).To(Succeed())

		out, err := fs.ReadFile(filepath.Join(root, "boot", "grub", constants.GrubCfg))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(out)).To(Equal(constants.GrubEfiCfg))
	})
})

var _ = Describe("cleanupGrubName", Label("iso"), func() {
	DescribeTable("strips signature suffixes",
		func(in, want string) {
			Expect(cleanupGrubName(in)).To(Equal(want))
		},
		Entry("plain grubx64.efi", "grubx64.efi", "grubx64.efi"),
		Entry("Ubuntu grubx64.efi.signed", "grubx64.efi.signed", "grubx64.efi"),
		Entry("Ubuntu grubaa64.efi.signed", "grubaa64.efi.signed", "grubaa64.efi"),
		Entry("Ubuntu grubaa64.efi.dualsigned", "grubaa64.efi.dualsigned", "grubaa64.efi"),
		Entry("Ubuntu grubaa64.efi.signed.latest", "grubaa64.efi.signed.latest", "grubaa64.efi"),
	)

	DescribeTable("renames the CD grub to the name shim chainloads",
		func(in, want string) {
			// Ubuntu's shim is compiled to load grub{x64,aa64}.efi from the
			// same directory. When we ship the CD variant of grub, its
			// filename is gcd*.efi.signed. We must rename it so shim finds
			// it under Secure Boot.
			Expect(cleanupGrubName(in)).To(Equal(want))
		},
		Entry("gcdx64.efi.signed becomes grubx64.efi", "gcdx64.efi.signed", "grubx64.efi"),
		Entry("gcdaa64.efi.signed becomes grubaa64.efi", "gcdaa64.efi.signed", "grubaa64.efi"),
		Entry("plain gcdx64.efi (unsigned build) also renames", "gcdx64.efi", "grubx64.efi"),
	)
})
