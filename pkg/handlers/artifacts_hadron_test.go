package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/AuroraBoot/pkg/builder"
	"github.com/kairos-io/AuroraBoot/pkg/handlers"
	"github.com/kairos-io/AuroraBoot/pkg/store"
	"github.com/labstack/echo/v4"
)

var _ = Describe("ArtifactHandler — Hadron kind", func() {
	var (
		e       *echo.Echo
		fb      *fakeBuilder
		handler *handlers.ArtifactHandler
	)

	BeforeEach(func() {
		e = echo.New()
		fb = &fakeBuilder{}
		handler = handlers.NewArtifactHandler(fb, nil, nil, nil, "", "reg-token", "http://localhost:8080")
	})

	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		Expect(handler.Create(c)).To(Succeed())
		return rec
	}

	It("routes kind=hadron to the hadron pipeline and captures the spec", func() {
		body := `{
			"kind": "hadron",
			"name": "example",
			"hadron": {
				"baseImage": "ghcr.io/kairos-io/hadron:main",
				"firmware": ["ghcr.io/kairos-io/hadron-firmware/linux-firmware-amdgpu:20260622"],
				"layers":   ["ghcr.io/kairos-io/git:latest"],
				"platforms": ["linux/amd64", "linux/arm64"],
				"outputRef": "example.com/team/os:v1",
				"push": true
			}
		}`
		rec := post(body)
		Expect(rec.Code).To(Equal(http.StatusCreated))

		var status builder.BuildStatus
		Expect(json.Unmarshal(rec.Body.Bytes(), &status)).To(Succeed())
		Expect(status.ID).NotTo(BeEmpty())

		opts := fb.lastOpts
		Expect(opts.Kind).To(Equal(store.ArtifactKindHadron))
		Expect(opts.Hadron.BaseImage).To(Equal("ghcr.io/kairos-io/hadron:main"))
		Expect(opts.Hadron.Firmware).To(ConsistOf("ghcr.io/kairos-io/hadron-firmware/linux-firmware-amdgpu:20260622"))
		Expect(opts.Hadron.Layers).To(ConsistOf("ghcr.io/kairos-io/git:latest"))
		Expect(opts.Hadron.Platforms).To(Equal([]string{"linux/amd64", "linux/arm64"}))
		Expect(opts.Hadron.OutputRef).To(Equal("example.com/team/os:v1"))
		Expect(opts.Hadron.Push).To(BeTrue())

		// Kairos-specific opts must be untouched by the hadron path.
		Expect(opts.CloudConfig).To(BeEmpty())
		Expect(opts.Provisioning.AutoInstall).To(BeFalse())
	})

	It("rejects kind=hadron without a hadron block", func() {
		body := `{"kind":"hadron","name":"broken"}`
		rec := post(body)
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
	})

	It("rejects a hadron spec with no output destination", func() {
		body := `{
			"kind":"hadron",
			"hadron":{"baseImage":"ghcr.io/kairos-io/hadron:main","outputRef":"r/x:v"}
		}`
		rec := post(body)
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
		var resp map[string]string
		Expect(json.Unmarshal(rec.Body.Bytes(), &resp)).To(Succeed())
		Expect(resp["error"]).To(ContainSubstring("push or produceTarball"))
	})

	It("rejects a hadron spec with a shell-metacharacter image ref", func() {
		body := `{
			"kind":"hadron",
			"hadron":{
				"baseImage":"ghcr.io/kairos-io/hadron:main",
				"layers":["ghcr.io/x` + "`" + `whoami` + "`" + `:latest"],
				"outputRef":"r/x:v",
				"produceTarball":true
			}
		}`
		rec := post(body)
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
	})
})
