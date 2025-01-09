package e2e_test

import (
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/spectrocloud/peg/matcher"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// NOTE: Once you run a test in 1 raw image, because the image is the installed system, any changes are now permanent
// So you cannot run different tests for 1 raw image that are destructive, like on first boot we will recover and expand
// the system with partitions, so that raw image now has changed.
// All tests in here should be sequential taking into account that the auto-reset is run on teh single raw image
var _ = Describe("raw bootable artifacts", Label("raw-bootable"), func() {
	var vm VM
	var err error

	BeforeEach(func() {
		_, ok := os.Stat(os.Getenv("RAW_IMAGE"))
		Expect(ok).To(BeNil(), "RAW_IMAGE should exist")
		vm, err = startVM()
		Expect(err).ToNot(HaveOccurred())
		vm.EventuallyConnects(1200)
	})

	AfterEach(func() {
		if CurrentSpecReport().Failed() {
			gatherLogs(vm)
			serial, _ := os.ReadFile(filepath.Join(vm.StateDir, "serial.log"))
			_ = os.MkdirAll("logs", os.ModePerm|os.ModeDir)
			_ = os.WriteFile(filepath.Join("logs", "serial.log"), serial, os.ModePerm)
			fmt.Println(string(serial))
		}

		err := vm.Destroy(nil)
		Expect(err).ToNot(HaveOccurred())
	})
	It("Should boot as expected", func() {
		// At first raw images boot on recovery and they reset the system and creates the partitions
		// so it can take a while to boot in the active partition
		// lets wait a bit checking
		By("Waiting for recovery reset to finish", func() {
			Eventually(func() string {
				output, _ := vm.Sudo("kairos-agent state")
				return output
			}, 5*time.Minute, 1*time.Second).Should(
				Or(
					ContainSubstring("active_boot"),
				))
		})

		// This checks both that the disk is bootable and with secureboot enabled
		if os.Getenv("SECUREBOOT") == "true" {
			By("Have secureboot enabled", func() {
				output, err := vm.Sudo("dmesg | grep -i secure")
				Expect(err).ToNot(HaveOccurred(), output)
				Expect(output).To(ContainSubstring("Secure boot enabled"))
			})
		}

		By("checking corresponding state", func() {
			currentVersion, err := vm.Sudo(getVersionCmd)
			Expect(err).ToNot(HaveOccurred(), currentVersion)

			stateAssertVM(vm, "boot", "active_boot")
			stateAssertVM(vm, "oem.mounted", "true")
			stateAssertVM(vm, "oem.found", "true")
			stateAssertVM(vm, "persistent.mounted", "true")
			stateAssertVM(vm, "state.mounted", "true")
			stateAssertVM(vm, "oem.type", "ext4")
			stateAssertVM(vm, "persistent.type", "ext4")
			stateAssertVM(vm, "state.type", "ext4")
			stateAssertVM(vm, "oem.mount_point", "/oem")
			stateAssertVM(vm, "persistent.mount_point", "/usr/local")
			stateAssertVM(vm, "persistent.name", "/dev/vda")
			stateAssertVM(vm, "state.mount_point", "/run/initramfs/cos-state")
			stateAssertVM(vm, "oem.read_only", "false")
			stateAssertVM(vm, "persistent.read_only", "false")
			stateAssertVM(vm, "state.read_only", "true")
			stateAssertVM(vm, "kairos.version", strings.ReplaceAll(strings.ReplaceAll(currentVersion, "\r", ""), "\n", ""))
			stateContains(vm, "system.os.name", "alpine", "opensuse", "ubuntu", "debian")
			stateContains(vm, "kairos.flavor", "alpine", "opensuse", "ubuntu", "debian")
		})
	})
})
