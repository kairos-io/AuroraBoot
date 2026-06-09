package redfish_test

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/AuroraBoot/pkg/redfish"
)

var _ = Describe("Deployer.Finalize", func() {
	var (
		bmc *fakeBMC
		ctx context.Context
		d   *redfish.Deployer
	)

	BeforeEach(func() {
		ctx = context.Background()
		bmc = newFakeBMC()
		d = redfish.NewDeployer(redfish.Config{
			Endpoint:  bmc.URL(),
			Username:  testUser,
			Password:  testPassword,
			VerifySSL: false, // fake BMC uses a self-signed TLS cert
			Timeout:   30 * time.Second,
		})
	})

	AfterEach(func() {
		bmc.Close()
	})

	It("ejects the virtual media and best-effort boots to disk", func() {
		Expect(d.Connect(ctx)).To(Succeed())
		defer func() { _ = d.Close() }()

		Expect(d.Finalize(ctx, redfish.FinalizeRequest{})).To(Succeed())

		// The load-bearing eject fired.
		Expect(bmc.ejectCalled).To(BeTrue())
		// The opportunistic boot-to-disk PATCH cleared the boot override.
		Expect(bmc.bootPatchBody).To(HaveKey("Boot"))
		boot, ok := bmc.bootPatchBody["Boot"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(boot).To(HaveKeyWithValue("BootSourceOverrideEnabled", "Disabled"))
	})

	It("still succeeds when the boot-to-disk PATCH errors (boot is best-effort)", func() {
		// Both the clear-override PATCH and the fallback Hdd PATCH return 500; the
		// eject must still fire and Finalize must still succeed.
		bmc.bootPatchStatus = 500

		Expect(d.Connect(ctx)).To(Succeed())
		defer func() { _ = d.Close() }()

		Expect(d.Finalize(ctx, redfish.FinalizeRequest{})).To(Succeed())
		Expect(bmc.ejectCalled).To(BeTrue())
	})

	It("requires a connection", func() {
		// No Connect: Finalize must refuse rather than nil-panic.
		Expect(d.Finalize(ctx, redfish.FinalizeRequest{})).To(MatchError(ContainSubstring("not connected")))
	})

	It("honours a cancelled context before touching the BMC", func() {
		Expect(d.Connect(ctx)).To(Succeed())
		defer func() { _ = d.Close() }()

		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		Expect(d.Finalize(cancelled, redfish.FinalizeRequest{})).To(MatchError(context.Canceled))
		Expect(bmc.ejectCalled).To(BeFalse())
	})
})
