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

		It("detects UEFI from the boot-mode allowable values", func() {
			bmc.bootModeAllowableValues = []string{"Legacy", "UEFI"}
			Expect(d.Connect(ctx)).To(Succeed())
			defer func() { _ = d.Close() }()

			info, err := d.Inspect(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Features).To(HaveKeyWithValue(redfish.FeatureUEFI, true))
		})

		It("does not report UEFI when the allowable boot modes exclude it", func() {
			bmc.bootModeAllowableValues = []string{"Legacy"}
			Expect(d.Connect(ctx)).To(Succeed())
			defer func() { _ = d.Close() }()

			info, err := d.Inspect(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Features).NotTo(HaveKey(redfish.FeatureUEFI))
		})

		It("does not report UEFI when no boot-mode signal is present", func() {
			bmc.bootModeAllowableValues = nil // BMC omits the annotation
			Expect(d.Connect(ctx)).To(Succeed())
			defer func() { _ = d.Close() }()

			info, err := d.Inspect(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Features).NotTo(HaveKey(redfish.FeatureUEFI))
		})

		It("detects SecureBoot only when the system exposes the link", func() {
			bmc.withSecureBoot = true
			Expect(d.Connect(ctx)).To(Succeed())
			defer func() { _ = d.Close() }()

			info, err := d.Inspect(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Features).To(HaveKeyWithValue(redfish.FeatureSecureBoot, true))
		})

		It("does not report SecureBoot when no link is present", func() {
			bmc.withSecureBoot = false
			Expect(d.Connect(ctx)).To(Succeed())
			defer func() { _ = d.Close() }()

			info, err := d.Inspect(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Features).NotTo(HaveKey(redfish.FeatureSecureBoot))
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

	Describe("Deploy progress callback", func() {
		It("invokes the callback in non-decreasing percent order ending at 100", func() {
			Expect(d.Connect(ctx)).To(Succeed())
			defer func() { _ = d.Close() }()

			var steps []string
			var percents []int
			_, err := d.Deploy(ctx, redfish.DeployRequest{
				ImageURL:   testImageURL,
				BootTarget: redfish.BootTargetCd,
				BootMode:   redfish.BootModeUEFI,
				Progress: func(step string, percent int) {
					steps = append(steps, step)
					percents = append(percents, percent)
				},
			})
			Expect(err).NotTo(HaveOccurred())

			// The flow reports each stage.
			Expect(steps).To(ContainElements(
				"discovering", "inserting media", "setting boot", "resetting", "polling task", "completed",
			))

			// Percentages are monotonically non-decreasing and finish at 100.
			Expect(percents).NotTo(BeEmpty())
			for i := 1; i < len(percents); i++ {
				Expect(percents[i]).To(BeNumerically(">=", percents[i-1]),
					"percent must never regress")
			}
			Expect(percents[0]).To(Equal(10))
			Expect(percents[len(percents)-1]).To(Equal(100))
		})

		It("is nil-safe (no callback supplied)", func() {
			Expect(d.Connect(ctx)).To(Succeed())
			defer func() { _ = d.Close() }()

			_, err := d.Deploy(ctx, redfish.DeployRequest{
				ImageURL:   testImageURL,
				BootTarget: redfish.BootTargetCd,
				BootMode:   redfish.BootModeUEFI,
				// Progress nil
			})
			Expect(err).NotTo(HaveOccurred())
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

	Describe("Vendor quirk seam", func() {
		// Regression guard: the generic profile must drive the exact same protocol
		// flow as the default. We run the happy path with Vendor explicitly set to
		// generic and assert the same request shapes the default path produces.
		It("leaves the generic path byte-for-byte unchanged", func() {
			gd := redfish.NewDeployer(redfish.Config{
				Endpoint:  bmc.URL(),
				Username:  testUser,
				Password:  testPassword,
				Vendor:    redfish.VendorGeneric,
				VerifySSL: false,
				Timeout:   30 * time.Second,
			})
			Expect(gd.Connect(ctx)).To(Succeed())
			defer func() { _ = gd.Close() }()

			_, err := gd.Deploy(ctx, redfish.DeployRequest{
				ImageURL:   testImageURL,
				BootTarget: redfish.BootTargetCd,
				BootMode:   redfish.BootModeUEFI,
			})
			Expect(err).NotTo(HaveOccurred())

			// Same InsertMedia URL-pull body as the default path.
			Expect(bmc.insertBody).To(HaveKeyWithValue("Image", testImageURL))
			Expect(bmc.insertBody).To(HaveKeyWithValue("Inserted", true))
			Expect(bmc.insertBody).To(HaveKeyWithValue("WriteProtected", true))
			Expect(bmc.insertBody).To(HaveKeyWithValue("MediaType", "CD"))
			// Discovery hit the System media first (default order), not the Manager.
			Expect(bmc.sawRequest(http.MethodPost,
				"/redfish/v1/Systems/sys-xyz/VirtualMedia/Cd/Actions/VirtualMedia.InsertMedia")).To(BeTrue())
		})

		It("(ilo) discovers Manager-hosted media when the System exposes none", func() {
			bmc.mediaOnManager = true

			ilo := redfish.NewDeployer(redfish.Config{
				Endpoint:  bmc.URL(),
				Username:  testUser,
				Password:  testPassword,
				Vendor:    redfish.VendorHPE,
				VerifySSL: false,
				Timeout:   30 * time.Second,
			})
			Expect(ilo.Connect(ctx)).To(Succeed())
			defer func() { _ = ilo.Close() }()

			result, err := ilo.Deploy(ctx, redfish.DeployRequest{
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

	Describe("Redfish error surfacing", func() {
		It("includes @Message.ExtendedInfo detail in the returned error", func() {
			bmc.insertMediaStatus = http.StatusBadRequest
			bmc.insertMediaExtendedInfo = true

			Expect(d.Connect(ctx)).To(Succeed())
			defer func() { _ = d.Close() }()

			_, err := d.Deploy(ctx, redfish.DeployRequest{
				ImageURL:   testImageURL,
				BootTarget: redfish.BootTargetCd,
				BootMode:   redfish.BootModeUEFI,
			})
			Expect(err).To(HaveOccurred())
			// The actionable ExtendedInfo text and resolution must surface, not just
			// a bare status code.
			Expect(err.Error()).To(ContainSubstring("inserting virtual media"))
			Expect(err.Error()).To(ContainSubstring("The image URL could not be reached by the BMC."))
			Expect(err.Error()).To(ContainSubstring("Base.1.0.ResourceAtUriUnauthorized"))
			Expect(err.Error()).To(ContainSubstring("Verify the image URL is reachable"))
		})
	})

	Describe("Unknown TaskState handling", func() {
		It("fails fast on a garbage TaskState instead of looping forever", func() {
			// The Task GET will return an out-of-enum state; the poll must reject it.
			bmc.taskStates = []string{"Frobnicating"}

			Expect(d.Connect(ctx)).To(Succeed())
			defer func() { _ = d.Close() }()

			// Bound the test independently so a regression (infinite loop) fails
			// loudly rather than hanging the suite.
			tctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			_, err := d.Deploy(tctx, redfish.DeployRequest{
				ImageURL:   testImageURL,
				BootTarget: redfish.BootTargetCd,
				BootMode:   redfish.BootModeUEFI,
			})
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unknown Redfish TaskState"))
			Expect(err.Error()).To(ContainSubstring("Frobnicating"))
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
