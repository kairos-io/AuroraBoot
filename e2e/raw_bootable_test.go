package e2e_test

import (
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/spectrocloud/peg/matcher"
	"os"
	"path/filepath"
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
			time.Sleep(5 * time.Minute)
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
		// This checks both that the disk is bootable and with secureboot enabled
		By("Have secureboot enabled", func() {
			output, err := vm.Sudo("dmesg | grep -i secure")
			Expect(err).ToNot(HaveOccurred(), output)
			Expect(output).To(ContainSubstring("Secure boot enabled"))
		})
	})
})
