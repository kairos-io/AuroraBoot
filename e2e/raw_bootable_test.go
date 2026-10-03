package e2e_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"os"
	"strings"
	"time"
)

// NOTE: Once you run a test in 1 raw image, because the image is the installed system, any changes are now permanent
// So you cannot run different tests for 1 raw image that are destructive, like on first boot we will recover and expand
// the system with partitions, so that raw image now has changed.
// All tests in here should be sequential taking into account that the auto-reset is run on teh single raw image
var _ = Describe("raw bootable artifacts", Label("raw-bootable"), func() {
	var vm testVM
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
			saveSerialLog(vm)
			gatherLogs(vm)
		}

		err := vm.Destroy(nil)
		Expect(err).ToNot(HaveOccurred())
	})
	It("Should boot as expected", func() {
		bootActive := os.Getenv("BOOT_ACTIVE") == "true"
		// Boot-active images ship COS_STATE and must reach active on the very first boot, without a reset reboot
		if bootActive {
			By("Booting straight into active", func() {
				stateAssertVM(vm, "boot", "active_boot")
				boots, err := vm.Sudo("journalctl --list-boots --no-pager -q | wc -l")
				Expect(err).ToNot(HaveOccurred(), boots)
				Expect(strings.TrimSpace(boots)).To(Equal("1"))
				out, err := vm.Sudo("ls /oem")
				Expect(err).ToNot(HaveOccurred(), out)
				Expect(out).To(ContainSubstring("01_layout.yaml"))
				Expect(out).ToNot(ContainSubstring("01_reset.yaml"))
			})
			By("Shipping passive.img only when STATE has room for it", func() {
				out, err := vm.Sudo("test -f /run/initramfs/cos-state/cOS/passive.img")
				if os.Getenv("EXPECT_PASSIVE") == "true" {
					Expect(err).ToNot(HaveOccurred(), out)
				} else {
					Expect(err).To(HaveOccurred(), out)
				}
			})
			if os.Getenv("NO_RECOVERY") == "true" {
				By("Having no recovery partition", func() {
					out, err := vm.Sudo("blkid -L COS_RECOVERY")
					Expect(err).To(HaveOccurred(), out)
				})
			}
		}
		// At first raw images boot on recovery and they reset the system and creates the partitions
		// so it can take a while to boot in the active partition
		// lets wait a bit checking
		//
		// The reset ends in a reboot, so this poll spans the moment the SSH
		// connection goes away. RootCommand reports that as an error and the
		// poll carries on; vm.Sudo would panic from a peg goroutine and end
		// the run with no spec failure and no logs.
		By("Waiting for recovery reset to finish", func() {
			Eventually(func() string {
				output, _ := vm.RootCommand("kairos-agent state")
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
			stateAssertVM(vm, "oem.type", "ext2")
			// Boot-active images create state and persistent as ext4, like an install does
			partitionFs := "ext2"
			if bootActive {
				partitionFs = "ext4"
			}
			stateAssertVM(vm, "persistent.type", partitionFs)
			stateAssertVM(vm, "state.type", partitionFs)
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
