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

var _ = Describe("ArtifactHandler", func() {
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

	Describe("Create", func() {
		It("should create a build", func() {
			body := `{"baseImage":"quay.io/kairos/ubuntu:24.04","outputs":{"iso":true}}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := handler.Create(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusCreated))

			var status builder.BuildStatus
			Expect(json.Unmarshal(rec.Body.Bytes(), &status)).To(Succeed())
			Expect(status.Phase).To(Equal(builder.BuildPending))
			Expect(status.ID).NotTo(BeEmpty())
		})
	})

	Describe("List", func() {
		It("should list all builds", func() {
			fb.builds = []*builder.BuildStatus{
				{ID: "build-1", Phase: builder.BuildReady},
			}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/artifacts", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := handler.List(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusOK))

			var statuses []*builder.BuildStatus
			Expect(json.Unmarshal(rec.Body.Bytes(), &statuses)).To(Succeed())
			Expect(statuses).To(HaveLen(1))
		})

		It("should return empty list when no builds exist", func() {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/artifacts", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := handler.List(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusOK))

			var statuses []*builder.BuildStatus
			Expect(json.Unmarshal(rec.Body.Bytes(), &statuses)).To(Succeed())
			Expect(statuses).To(HaveLen(0))
		})
	})

	Describe("DELETE /artifacts/:id", func() {
		var (
			as             *fakeArtifactStore
			handlerWithStore *handlers.ArtifactHandler
		)

		BeforeEach(func() {
			as = &fakeArtifactStore{
				records: []*store.ArtifactRecord{
					{ID: "art-1", Phase: store.ArtifactReady, BaseImage: "img1"},
				},
			}
			handlerWithStore = handlers.NewArtifactHandler(fb, as, nil, nil, "", "reg-token", "http://localhost:8080")
		})

		It("should delete the artifact and return 204", func() {
			req := httptest.NewRequest(http.MethodDelete, "/api/v1/artifacts/art-1", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id")
			c.SetParamValues("art-1")

			err := handlerWithStore.Delete(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusNoContent))

			// Verify artifact is gone.
			_, lookupErr := as.GetByID(nil, "art-1")
			Expect(lookupErr).To(HaveOccurred())
		})

		It("should return 404 for non-existent artifact", func() {
			req := httptest.NewRequest(http.MethodDelete, "/api/v1/artifacts/nonexistent", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id")
			c.SetParamValues("nonexistent")

			err := handlerWithStore.Delete(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusNotFound))
		})
	})

	Describe("DELETE /artifacts/failed (ClearFailed)", func() {
		var (
			as             *fakeArtifactStore
			handlerWithStore *handlers.ArtifactHandler
		)

		BeforeEach(func() {
			as = &fakeArtifactStore{
				records: []*store.ArtifactRecord{
					{ID: "art-ok", Phase: store.ArtifactReady, BaseImage: "img1"},
					{ID: "art-err1", Phase: store.ArtifactError, BaseImage: "img2"},
					{ID: "art-err2", Phase: store.ArtifactError, BaseImage: "img3"},
				},
			}
			handlerWithStore = handlers.NewArtifactHandler(fb, as, nil, nil, "", "reg-token", "http://localhost:8080")
		})

		It("should delete all Error-phase artifacts and return 204", func() {
			req := httptest.NewRequest(http.MethodDelete, "/api/v1/artifacts/failed", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := handlerWithStore.ClearFailed(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusNoContent))

			remaining, _ := as.List(nil)
			Expect(remaining).To(HaveLen(1))
			Expect(remaining[0].ID).To(Equal("art-ok"))
		})
	})

	Describe("Get", func() {
		It("should return a build by ID", func() {
			fb.builds = []*builder.BuildStatus{
				{ID: "build-1", Phase: builder.BuildReady},
			}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/artifacts/build-1", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id")
			c.SetParamValues("build-1")

			err := handler.Get(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusOK))
		})

		It("should return 404 for missing build", func() {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/artifacts/missing", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id")
			c.SetParamValues("missing")

			err := handler.Get(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusNotFound))
		})
	})
})
