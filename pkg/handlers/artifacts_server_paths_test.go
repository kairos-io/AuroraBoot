package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/AuroraBoot/pkg/handlers"
	"github.com/kairos-io/AuroraBoot/pkg/store"
	"github.com/labstack/echo/v4"
)

// The HTTP API never takes a filesystem path on the server. Overlays are
// referenced by the ID upload-overlay returns, signing goes through a key
// set, and the fields that would name a server path are refused outright so
// a client sending them learns why instead of building without them.
var _ = Describe("ArtifactHandler: no server paths in the API", func() {
	var (
		e            *echo.Echo
		fb           *fakeBuilder
		as           *fakeArtifactStore
		handler      *handlers.ArtifactHandler
		artifactsDir string
	)

	BeforeEach(func() {
		e = echo.New()
		fb = &fakeBuilder{}
		as = &fakeArtifactStore{}
		artifactsDir = GinkgoT().TempDir()
		handler = handlers.NewArtifactHandler(fb, as, nil, nil, nil, nil, artifactsDir, "reg-token", "http://localhost:8080")
	})

	create := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		Expect(handler.Create(e.NewContext(req, rec))).To(Succeed())
		return rec
	}

	errorOf := func(rec *httptest.ResponseRecorder) string {
		var resp map[string]string
		Expect(json.Unmarshal(rec.Body.Bytes(), &resp)).To(Succeed())
		return resp["error"]
	}

	makeOverlay := func() string {
		id := uuid.New().String()
		Expect(os.MkdirAll(filepath.Join(artifactsDir, "overlays", id), 0o755)).To(Succeed())
		return id
	}

	Describe("Create refuses removed path fields", func() {
		DescribeTable("returns 400 naming the replacement and starts no build",
			func(fragment, wantField, wantHint string) {
				rec := create(`{"baseImage":"ubuntu:24.04","outputs":{"iso":true},` + fragment + `}`)
				Expect(rec.Code).To(Equal(http.StatusBadRequest))
				msg := errorOf(rec)
				Expect(msg).To(ContainSubstring(wantField))
				Expect(msg).To(ContainSubstring(wantHint))
				Expect(fb.builds).To(BeEmpty())
				Expect(as.records).To(BeEmpty())
			},
			Entry("overlayRootfs", `"overlayRootfs":"/etc"`, "overlayRootfs", "use overlayId"),
			Entry("buildContextDir", `"buildContextDir":"/etc"`, "buildContextDir", "no longer accepted"),
			Entry("outputDir", `"outputDir":"/tmp/out"`, "outputDir", "no longer accepted"),
			Entry("signing.ukiSecureBootKey", `"signing":{"ukiSecureBootKey":"/k/db.key"}`, "signing.ukiSecureBootKey", "use signing.ukiKeySetId"),
			Entry("signing.ukiSecureBootCert", `"signing":{"ukiSecureBootCert":"/k/db.pem"}`, "signing.ukiSecureBootCert", "use signing.ukiKeySetId"),
			Entry("signing.ukiTpmPcrKey", `"signing":{"ukiTpmPcrKey":"/k/tpm.pem"}`, "signing.ukiTpmPcrKey", "use signing.ukiKeySetId"),
			Entry("signing.ukiPublicKeysDir", `"signing":{"ukiPublicKeysDir":"/k"}`, "signing.ukiPublicKeysDir", "use signing.ukiKeySetId"),
		)

		It("keeps accepting ukiSecureBootEnroll without a key set", func() {
			rec := create(`{"baseImage":"ubuntu:24.04","outputs":{"iso":true},"signing":{"ukiSecureBootEnroll":"if-safe"}}`)
			Expect(rec.Code).To(Equal(http.StatusCreated))
			Expect(fb.lastOpts.Signing.UKISecureBootEnroll).To(Equal("if-safe"))
			Expect(fb.lastOpts.Signing.UKISecureBootKey).To(BeEmpty())
		})
	})

	Describe("Create resolves overlayId", func() {
		DescribeTable("returns 400 for an overlayId that is not an uploaded overlay",
			func(id string) {
				Expect(os.MkdirAll(filepath.Join(artifactsDir, "x"), 0o755)).To(Succeed())
				body, err := json.Marshal(map[string]any{
					"baseImage": "ubuntu:24.04",
					"outputs":   map[string]bool{"iso": true},
					"overlayId": id,
				})
				Expect(err).NotTo(HaveOccurred())
				rec := create(string(body))
				Expect(rec.Code).To(Equal(http.StatusBadRequest))
				Expect(errorOf(rec)).To(ContainSubstring("overlayId"))
				Expect(fb.builds).To(BeEmpty())
			},
			Entry("parent traversal", "../x"),
			Entry("absolute path", "/etc"),
			Entry("arbitrary string", "not-a-uuid"),
			Entry("UUID in braces", "{"+uuid.New().String()+"}"),
			Entry("URN-form UUID", "urn:uuid:"+uuid.New().String()),
			Entry("well-formed UUID with no uploaded overlay", uuid.New().String()),
		)

		It("returns 400 when the overlay ID names a file, not a directory", func() {
			id := uuid.New().String()
			Expect(os.MkdirAll(filepath.Join(artifactsDir, "overlays"), 0o755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(artifactsDir, "overlays", id), []byte("x"), 0o644)).To(Succeed())
			rec := create(`{"baseImage":"ubuntu:24.04","outputs":{"iso":true},"overlayId":"` + id + `"}`)
			Expect(rec.Code).To(Equal(http.StatusBadRequest))
			Expect(fb.builds).To(BeEmpty())
		})

		It("passes the resolved directory to the builder and records the ID", func() {
			id := makeOverlay()
			rec := create(`{"baseImage":"ubuntu:24.04","outputs":{"iso":true},"overlayId":"` + id + `"}`)
			Expect(rec.Code).To(Equal(http.StatusCreated))

			Expect(fb.lastOpts.OverlayRootfs).To(Equal(filepath.Join(artifactsDir, "overlays", id)))
			Expect(fb.lastOpts.OverlayID).To(Equal(id))
			Expect(as.records).To(HaveLen(1))
			Expect(as.records[0].OverlayID).To(Equal(id))
		})

		It("leaves the overlay unset when no overlayId is sent", func() {
			rec := create(`{"baseImage":"ubuntu:24.04","outputs":{"iso":true}}`)
			Expect(rec.Code).To(Equal(http.StatusCreated))
			Expect(fb.lastOpts.OverlayRootfs).To(BeEmpty())
			Expect(fb.lastOpts.OverlayID).To(BeEmpty())
		})
	})

	Describe("UploadOverlay", func() {
		It("returns the overlay ID and no server path", func() {
			buf, contentType := uploadOverlayMultipart([]byte("hello"), "motd")
			req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts/upload-overlay", buf)
			req.Header.Set(echo.HeaderContentType, contentType)
			rec := httptest.NewRecorder()
			Expect(handler.UploadOverlay(e.NewContext(req, rec))).To(Succeed())
			Expect(rec.Code).To(Equal(http.StatusOK))

			var resp map[string]any
			Expect(json.Unmarshal(rec.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp).To(HaveLen(1))
			Expect(resp).To(HaveKey("id"))
			id, ok := resp["id"].(string)
			Expect(ok).To(BeTrue())
			Expect(uuid.Parse(id)).Error().NotTo(HaveOccurred())
			Expect(rec.Body.String()).NotTo(ContainSubstring(artifactsDir))

			data, err := os.ReadFile(filepath.Join(artifactsDir, "overlays", id, "motd"))
			Expect(err).NotTo(HaveOccurred())
			Expect(data).To(Equal([]byte("hello")))
		})
	})

	Describe("overlay cleanup", func() {
		var victim string

		BeforeEach(func() {
			// A directory outside the overlays dir that a crafted stored
			// value would point at.
			victim = filepath.Join(artifactsDir, "victim")
			Expect(os.MkdirAll(victim, 0o755)).To(Succeed())
			Expect(os.MkdirAll(filepath.Join(artifactsDir, "overlays"), 0o755)).To(Succeed())
		})

		deleteRecord := func(id string) {
			req := httptest.NewRequest(http.MethodDelete, "/api/v1/artifacts/"+id, nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			c.SetParamNames("id")
			c.SetParamValues(id)
			Expect(handler.Delete(c)).To(Succeed())
			Expect(rec.Code).To(Equal(http.StatusNoContent))
		}

		clearFailed := func() {
			req := httptest.NewRequest(http.MethodDelete, "/api/v1/artifacts/failed", nil)
			rec := httptest.NewRecorder()
			Expect(handler.ClearFailed(e.NewContext(req, rec))).To(Succeed())
			Expect(rec.Code).To(Equal(http.StatusNoContent))
		}

		It("Delete removes the overlay named by a stored UUID", func() {
			id := makeOverlay()
			as.records = []*store.ArtifactRecord{{ID: "art-1", Phase: store.ArtifactReady, OverlayID: id}}
			deleteRecord("art-1")
			Expect(filepath.Join(artifactsDir, "overlays", id)).NotTo(BeADirectory())
		})

		It("ClearFailed removes the overlay named by a stored UUID", func() {
			id := makeOverlay()
			as.records = []*store.ArtifactRecord{{ID: "art-err", Phase: store.ArtifactError, OverlayID: id}}
			clearFailed()
			Expect(filepath.Join(artifactsDir, "overlays", id)).NotTo(BeADirectory())
		})

		DescribeTable("Delete refuses a stored overlay ID that is not a UUID",
			func(stored string) {
				stored = strings.ReplaceAll(stored, "{dir}", artifactsDir)
				as.records = []*store.ArtifactRecord{{ID: "art-1", Phase: store.ArtifactReady, OverlayID: stored}}
				deleteRecord("art-1")
				Expect(victim).To(BeADirectory())
				Expect(filepath.Join(artifactsDir, "overlays")).To(BeADirectory())
				Expect(artifactsDir).To(BeADirectory())
			},
			Entry("parent traversal", "../victim"),
			Entry("empty-looking dot", "."),
			Entry("parent dir", ".."),
			Entry("absolute path", "/tmp"),
			Entry("path under the artifacts dir that climbs out of it", "{dir}/overlays/../victim"),
			Entry("artifacts dir itself", "{dir}"),
		)

		DescribeTable("ClearFailed refuses a stored overlay ID that is not a UUID",
			func(stored string) {
				stored = strings.ReplaceAll(stored, "{dir}", artifactsDir)
				as.records = []*store.ArtifactRecord{{ID: "art-err", Phase: store.ArtifactError, OverlayID: stored}}
				clearFailed()
				Expect(victim).To(BeADirectory())
				Expect(filepath.Join(artifactsDir, "overlays")).To(BeADirectory())
				Expect(artifactsDir).To(BeADirectory())
			},
			Entry("parent traversal", "../victim"),
			Entry("empty-looking dot", "."),
			Entry("parent dir", ".."),
			Entry("absolute path", "/tmp"),
			Entry("path under the artifacts dir that climbs out of it", "{dir}/overlays/../victim"),
			Entry("artifacts dir itself", "{dir}"),
		)
	})
})

var _ = Describe("ExtensionHandler.Create: no server paths in the API", func() {
	It("returns 400 for source.buildContextDir and starts no build", func() {
		e := echo.New()
		fb := &fakeExtensionBuilder{}
		handler := handlers.NewExtensionHandler(fb, newFakeExtensionStore(), newFakeBundleStore(), nil, nil, "")

		body := `{"name":"x","type":"sysext","arch":"amd64","source":{"mode":"dockerfile","dockerfile":"FROM scratch","buildContextDir":"/etc"}}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/extensions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		Expect(handler.Create(e.NewContext(req, rec))).To(Succeed())

		Expect(rec.Code).To(Equal(http.StatusBadRequest))
		var resp map[string]string
		Expect(json.Unmarshal(rec.Body.Bytes(), &resp)).To(Succeed())
		Expect(resp["error"]).To(ContainSubstring("buildContextDir"))
		Expect(resp["error"]).To(ContainSubstring("no longer accepted"))
		Expect(fb.lastOpts.ID).To(BeEmpty())
	})
})
