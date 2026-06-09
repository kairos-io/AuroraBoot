package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/AuroraBoot/pkg/handlers"
	"github.com/kairos-io/AuroraBoot/pkg/store"
	"github.com/labstack/echo/v4"
)

var _ = Describe("BMC target CRUD ImageURL validation", func() {
	var (
		e          *echo.Echo
		bmcTargets *fakeBMCTargetStore
		h          *handlers.DeployHandler
	)

	BeforeEach(func() {
		e = echo.New()
		bmcTargets = &fakeBMCTargetStore{}
		h = handlers.NewDeployHandler(&fakeArtifactStore{}, &fakeDeploymentStore{}, bmcTargets, nil, "", nil, nil)
	})

	create := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/bmc-targets", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		Expect(h.CreateBMCTarget(c)).To(Succeed())
		return rec
	}

	update := func(id, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/bmc-targets/"+id, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(id)
		Expect(h.UpdateBMCTarget(c)).To(Succeed())
		return rec
	}

	It("accepts and persists a valid per-BMC ImageURL on create", func() {
		rec := create(`{"name":"h","endpoint":"https://10.0.0.9","imageUrl":"https://10.0.0.5/os.iso"}`)
		Expect(rec.Code).To(Equal(http.StatusCreated))
		Expect(bmcTargets.targets).To(HaveLen(1))
		Expect(bmcTargets.targets[0].ImageURL).To(Equal("https://10.0.0.5/os.iso"))
	})

	It("rejects an SSRF-blocked ImageURL on create with 400", func() {
		rec := create(`{"name":"h","endpoint":"https://10.0.0.9","imageUrl":"http://169.254.169.254/x.iso"}`)
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
		Expect(rec.Body.String()).To(ContainSubstring("invalid imageUrl"))
		Expect(bmcTargets.targets).To(BeEmpty())
	})

	It("rejects an SSRF-blocked ImageURL on update with 400", func() {
		bmcTargets.targets = []*store.BMCTarget{{ID: "bmc-1", Endpoint: "https://10.0.0.9"}}
		rec := update("bmc-1", `{"name":"h","endpoint":"https://10.0.0.9","imageUrl":"http://169.254.169.254/x.iso"}`)
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
		Expect(rec.Body.String()).To(ContainSubstring("invalid imageUrl"))
		// The stored target is unchanged.
		Expect(bmcTargets.targets[0].ImageURL).To(BeEmpty())
	})

	It("persists an updated ImageURL on update", func() {
		bmcTargets.targets = []*store.BMCTarget{{ID: "bmc-1", Endpoint: "https://10.0.0.9"}}
		rec := update("bmc-1", `{"name":"h","endpoint":"https://10.0.0.9","imageUrl":"https://10.0.0.5/os.iso"}`)
		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(bmcTargets.targets[0].ImageURL).To(Equal("https://10.0.0.5/os.iso"))
	})
})
