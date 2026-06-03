package redfish_test

import (
	"context"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/AuroraBoot/pkg/redfish"
)

const (
	testUser     = "admin"
	testPassword = "s3cr3t-p@ss"
	testImageURL = "http://serve.example/kairos.iso"
)

var _ = Describe("Deployer", func() {
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

	Describe("Connect and Close", func() {
		It("creates a session with a JSON credential body and deletes it on Close", func() {
			Expect(d.Connect(ctx)).To(Succeed())

			// Session was created with a JSON body carrying the credentials.
			Expect(bmc.sessionBody).To(HaveKeyWithValue("UserName", testUser))
			Expect(bmc.sessionBody).To(HaveKeyWithValue("Password", testPassword))

			Expect(d.Close()).To(Succeed())
			Expect(bmc.sawRequest(http.MethodDelete, bmc.sessionLocation)).To(BeTrue(),
				"Close must DELETE the session resource")
		})

		It("rejects a second Connect", func() {
			Expect(d.Connect(ctx)).To(Succeed())
			defer func() { _ = d.Close() }()
			Expect(d.Connect(ctx)).To(MatchError(ContainSubstring("already connected")))
		})
	})

	Describe("Inspect", func() {
		It("populates memory and CPU from the nested summaries (no 0/0 bug)", func() {
			Expect(d.Connect(ctx)).To(Succeed())
			defer func() { _ = d.Close() }()

			info, err := d.Inspect(ctx)
			Expect(err).NotTo(HaveOccurred())

			// These come from MemorySummary.TotalSystemMemoryGiB / ProcessorSummary.Count.
			Expect(info.MemoryGiB).To(Equal(64))
			Expect(info.ProcessorCount).To(Equal(8))
			Expect(info.Manufacturer).To(Equal("ACME"))
			Expect(info.Model).To(Equal("ProLiant-Test"))
			Expect(info.SerialNumber).To(Equal("SN-0001"))
			// Discovery used the opaque member ID, not a hardcoded Systems/1.
			Expect(info.ID).To(Equal("sys-xyz"))
		})
	})

	Describe("Deploy happy path", func() {
		It("inserts media, sets one-time boot, resets with a ResetType, polls the task, and tears down the session", func() {
			Expect(d.Connect(ctx)).To(Succeed())

			result, err := d.Deploy(ctx, redfish.DeployRequest{
				ImageURL:   testImageURL,
				BootTarget: redfish.BootTargetCd,
				BootMode:   redfish.BootModeUEFI,
			})
			Expect(err).NotTo(HaveOccurred())

			// Discovery used opaque IDs, never hardcoded Systems/1 or VirtualMedia/1.
			Expect(result.SystemID).To(Equal("sys-xyz"))
			Expect(result.MediaID).To(Equal("Cd"))
			Expect(bmc.sawRequest(http.MethodGet, "/redfish/v1/Systems/sys-xyz")).To(BeTrue())

			// InsertMedia URL-pull body.
			Expect(bmc.insertBody).To(HaveKeyWithValue("Image", testImageURL))
			Expect(bmc.insertBody).To(HaveKeyWithValue("Inserted", true))
			Expect(bmc.insertBody).To(HaveKeyWithValue("WriteProtected", true))

			// One-time boot override PATCH: Once / Cd / UEFI.
			Expect(bmc.bootPatchBody).To(HaveKey("Boot"))
			boot, ok := bmc.bootPatchBody["Boot"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(boot).To(HaveKeyWithValue("BootSourceOverrideEnabled", "Once"))
			Expect(boot).To(HaveKeyWithValue("BootSourceOverrideTarget", "Cd"))
			Expect(boot).To(HaveKeyWithValue("BootSourceOverrideMode", "UEFI"))

			// Reset carried an explicit ResetType.
			Expect(bmc.resetBody).To(HaveKey("ResetType"))
			Expect(bmc.resetBody["ResetType"]).NotTo(BeEmpty())

			// Async Task was polled to a terminal Completed state.
			Expect(result.TaskCompleted).To(BeTrue())
			Expect(result.TaskState).To(Equal("Completed"))

			// Session is deleted on Close even on the success path.
			Expect(d.Close()).To(Succeed())
			Expect(bmc.sawRequest(http.MethodDelete, bmc.sessionLocation)).To(BeTrue())
		})
	})

	Describe("Deploy error path", func() {
		It("still tears the session down when InsertMedia fails mid-flow", func() {
			bmc.insertMediaStatus = http.StatusInternalServerError

			Expect(d.Connect(ctx)).To(Succeed())

			_, err := d.Deploy(ctx, redfish.DeployRequest{
				ImageURL:   testImageURL,
				BootTarget: redfish.BootTargetCd,
				BootMode:   redfish.BootModeUEFI,
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("inserting virtual media"))

			// deferred Close still runs on the error path.
			Expect(d.Close()).To(Succeed())
			Expect(bmc.sawRequest(http.MethodDelete, bmc.sessionLocation)).To(BeTrue(),
				"the session must be DELETEd even when the deploy errors")
		})

		It("scrubs the password from returned errors", func() {
			bmc.insertMediaStatus = http.StatusInternalServerError

			Expect(d.Connect(ctx)).To(Succeed())
			defer func() { _ = d.Close() }()

			_, err := d.Deploy(ctx, redfish.DeployRequest{ImageURL: testImageURL})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).NotTo(ContainSubstring(testPassword))
		})

		It("requires an ImageURL", func() {
			Expect(d.Connect(ctx)).To(Succeed())
			defer func() { _ = d.Close() }()

			_, err := d.Deploy(ctx, redfish.DeployRequest{})
			Expect(err).To(MatchError(ContainSubstring("ImageURL is required")))
		})
	})

	Describe("VirtualMedia discovery on the Manager", func() {
		It("finds CD media on the Manager when the System has none", func() {
			bmc.mediaOnManager = true

			Expect(d.Connect(ctx)).To(Succeed())
			defer func() { _ = d.Close() }()

			result, err := d.Deploy(ctx, redfish.DeployRequest{
				ImageURL:   testImageURL,
				BootTarget: redfish.BootTargetCd,
				BootMode:   redfish.BootModeUEFI,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.MediaID).To(Equal("Cd"))
			Expect(bmc.sawRequest(http.MethodPost,
				"/redfish/v1/Managers/mgr-1/VirtualMedia/Cd/Actions/VirtualMedia.InsertMedia")).To(BeTrue())
		})
	})

	Describe("TLS verification default", func() {
		// gofish reuses http.DefaultTransport's TLSClientConfig pointer; an
		// insecure connect elsewhere in the suite can flip InsecureSkipVerify on
		// that shared config and leak into this spec. Reset it so the assertion
		// reflects our VerifySSL handling, not cross-spec global state.
		It("fails against a self-signed BMC when VerifySSL is left on", func() {
			if tr, ok := http.DefaultTransport.(*http.Transport); ok && tr.TLSClientConfig != nil {
				tr.TLSClientConfig.InsecureSkipVerify = false
			}

			secure := redfish.NewDeployer(redfish.Config{
				Endpoint:  bmc.URL(),
				Username:  testUser,
				Password:  testPassword,
				VerifySSL: true, // explicit: verification stays on
				Timeout:   5 * time.Second,
			})
			err := secure.Connect(ctx)
			Expect(err).To(HaveOccurred(), "TLS verification must reject the self-signed cert by default")
			_ = secure.Close()
		})
	})
})
