package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	"github.com/kairos-io/AuroraBoot/pkg/builder"
	"github.com/kairos-io/AuroraBoot/pkg/handlers"
	"github.com/kairos-io/AuroraBoot/pkg/store"
	"github.com/labstack/echo/v4"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ExtensionHandler.Create", func() {
	var (
		e       *echo.Echo
		fb      *fakeExtensionBuilder
		es      *fakeExtensionStore
		bs      *fakeBundleStore
		handler *handlers.ExtensionHandler
	)

	BeforeEach(func() {
		e = echo.New()
		fb = &fakeExtensionBuilder{}
		es = newFakeExtensionStore()
		bs = newFakeBundleStore()
		handler = handlers.NewExtensionHandler(fb, es, bs, nil, "")
	})

	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/extensions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		Expect(handler.Create(c)).To(Succeed())
		return rec
	}

	It("creates a sysext build and returns 201 with a Pending status", func() {
		rec := post(`{"name":"tailscale-agent","type":"sysext","arch":"amd64","version":"v1.74.0",
			"source":{"mode":"image","baseImage":"ubuntu:24.04"}}`)
		Expect(rec.Code).To(Equal(http.StatusCreated))

		var status builder.ExtensionBuildStatus
		Expect(json.Unmarshal(rec.Body.Bytes(), &status)).To(Succeed())
		Expect(status.Phase).To(Equal(builder.BuildPending))
		Expect(status.ID).ToNot(BeEmpty())
		Expect(fb.lastOpts.Type).To(Equal("sysext"))
		Expect(fb.lastOpts.Source.Mode).To(Equal("image"))
		Expect(fb.lastOpts.Source.BaseImage).To(Equal("ubuntu:24.04"))
	})
})

var _ = Describe("ExtensionHandler.Create — hierarchies validation", func() {
	var (
		e       *echo.Echo
		fb      *fakeExtensionBuilder
		handler *handlers.ExtensionHandler
	)

	BeforeEach(func() {
		e = echo.New()
		fb = &fakeExtensionBuilder{}
		handler = handlers.NewExtensionHandler(fb, newFakeExtensionStore(), newFakeBundleStore(), nil, "")
	})

	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/extensions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		Expect(handler.Create(c)).To(Succeed())
		return rec
	}

	base := `"name":"x","type":"sysext","arch":"amd64","source":{"mode":"image","baseImage":"ubuntu:24.04"}`

	It("rejects a path without a leading slash", func() {
		rec := post(`{` + base + `,"hierarchies":["opt"]}`)
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
		Expect(rec.Body.String()).To(ContainSubstring("hierarchies[0]"))
	})

	It("rejects a path containing ..", func() {
		rec := post(`{` + base + `,"hierarchies":["/opt/../etc"]}`)
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
		Expect(rec.Body.String()).To(ContainSubstring(".."))
	})

	It("rejects exactly /usr", func() {
		rec := post(`{` + base + `,"hierarchies":["/usr"]}`)
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
		Expect(rec.Body.String()).To(ContainSubstring("/usr"))
	})

	It("rejects exactly /", func() {
		rec := post(`{` + base + `,"hierarchies":["/"]}`)
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
	})

	It("rejects a path longer than 256 chars", func() {
		long := "/" + strings.Repeat("a", 256)
		rec := post(`{` + base + `,"hierarchies":["` + long + `"]}`)
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
		Expect(rec.Body.String()).To(ContainSubstring("256"))
	})

	It("normalizes: trims trailing slashes, dedupes, sorts alphabetically", func() {
		rec := post(`{` + base + `,"hierarchies":["/srv/","/opt","/srv","/opt/"]}`)
		Expect(rec.Code).To(Equal(http.StatusCreated))
		Expect(fb.lastOpts.Hierarchies).To(Equal([]string{"/opt", "/srv"}))
	})

	It("accepts nil hierarchies for a confext", func() {
		rec := post(`{"name":"fb","type":"confext","arch":"amd64","source":{"mode":"image","baseImage":"alpine:3"}}`)
		Expect(rec.Code).To(Equal(http.StatusCreated))
		Expect(fb.lastOpts.Hierarchies).To(BeNil())
	})
})

var _ = Describe("ExtensionHandler.Create — source/mode validation", func() {
	var (
		e       *echo.Echo
		handler *handlers.ExtensionHandler
	)

	BeforeEach(func() {
		e = echo.New()
		handler = handlers.NewExtensionHandler(&fakeExtensionBuilder{}, newFakeExtensionStore(), newFakeBundleStore(), nil, "")
	})

	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/extensions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		Expect(handler.Create(c)).To(Succeed())
		return rec
	}

	common := `"name":"x","type":"sysext","arch":"amd64"`

	It("rejects an unsupported source.mode", func() {
		rec := post(`{` + common + `,"source":{"mode":"voodoo"}}`)
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
		Expect(rec.Body.String()).To(ContainSubstring("mode"))
	})

	It("requires source.baseImage for mode=image", func() {
		rec := post(`{` + common + `,"source":{"mode":"image"}}`)
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
		Expect(rec.Body.String()).To(ContainSubstring("baseImage"))
	})

	It("requires source.artifactId for mode=artifact", func() {
		rec := post(`{` + common + `,"source":{"mode":"artifact"}}`)
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
		Expect(rec.Body.String()).To(ContainSubstring("artifactId"))
	})

	It("requires source.dockerfile for mode=dockerfile", func() {
		rec := post(`{` + common + `,"source":{"mode":"dockerfile"}}`)
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
		Expect(rec.Body.String()).To(ContainSubstring("dockerfile"))
	})

	It("rejects extraSteps with a FROM line", func() {
		rec := post(`{` + common + `,"source":{"mode":"artifact","artifactId":"a-1","extraSteps":"FROM ubuntu:24.04\nRUN ls"}}`)
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
		Expect(rec.Body.String()).To(ContainSubstring("FROM"))
	})

	It("rejects extraSteps with a FROM line preceded by whitespace and case-insensitive", func() {
		rec := post(`{` + common + `,"source":{"mode":"artifact","artifactId":"a-1","extraSteps":"  from ubuntu:24.04"}}`)
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
	})

	It("accepts extraSteps without any FROM line", func() {
		rec := post(`{` + common + `,"source":{"mode":"artifact","artifactId":"a-1","extraSteps":"RUN curl -fsSL https://tailscale.com/install.sh | sh"}}`)
		Expect(rec.Code).To(Equal(http.StatusCreated))
	})

	It("rejects unsupported arch", func() {
		rec := post(`{"name":"x","type":"sysext","arch":"i386","source":{"mode":"image","baseImage":"ubuntu:24.04"}}`)
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
		Expect(rec.Body.String()).To(ContainSubstring("arch"))
	})
})

var _ = Describe("ExtensionHandler — Get / List / PATCH / GetLogs / Cancel", func() {
	var (
		e       *echo.Echo
		fb      *fakeExtensionBuilder
		es      *fakeExtensionStore
		handler *handlers.ExtensionHandler
		ctx     = context.Background()
	)

	BeforeEach(func() {
		e = echo.New()
		fb = &fakeExtensionBuilder{}
		es = newFakeExtensionStore()
		handler = handlers.NewExtensionHandler(fb, es, newFakeBundleStore(), nil, "")
		_ = es.Create(ctx, &store.ExtensionRecord{ID: "e-1", Name: "ts", Type: "sysext", Phase: "Ready", Arch: "amd64", Logs: "step 1\nstep 2"})
	})

	withParam := func(method, path, id, body string) (echo.Context, *httptest.ResponseRecorder) {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(id)
		return c, rec
	}

	It("Get returns the extension record", func() {
		c, rr := withParam(http.MethodGet, "/api/v1/extensions/e-1", "e-1", "")
		Expect(handler.Get(c)).To(Succeed())
		Expect(rr.Code).To(Equal(http.StatusOK))
		Expect(rr.Body.String()).To(ContainSubstring(`"name":"ts"`))
	})

	It("Get returns 404 when the extension does not exist", func() {
		c, rr := withParam(http.MethodGet, "/api/v1/extensions/missing", "missing", "")
		Expect(handler.Get(c)).To(Succeed())
		Expect(rr.Code).To(Equal(http.StatusNotFound))
	})

	It("List returns every record", func() {
		_ = es.Create(ctx, &store.ExtensionRecord{ID: "e-2", Name: "x", Type: "confext", Phase: "Ready"})
		req := httptest.NewRequest(http.MethodGet, "/api/v1/extensions", nil)
		rr := httptest.NewRecorder()
		c := e.NewContext(req, rr)
		Expect(handler.List(c)).To(Succeed())
		Expect(rr.Code).To(Equal(http.StatusOK))
		Expect(rr.Body.String()).To(ContainSubstring("e-1"))
		Expect(rr.Body.String()).To(ContainSubstring("e-2"))
	})

	It("PATCH renames the extension", func() {
		c, rr := withParam(http.MethodPatch, "/api/v1/extensions/e-1", "e-1", `{"name":"tailscale-renamed"}`)
		Expect(handler.Update(c)).To(Succeed())
		Expect(rr.Code).To(Equal(http.StatusOK))
		got, _ := es.GetByID(ctx, "e-1")
		Expect(got.Name).To(Equal("tailscale-renamed"))
	})

	It("PATCH rejects an empty body", func() {
		c, rr := withParam(http.MethodPatch, "/api/v1/extensions/e-1", "e-1", `{}`)
		Expect(handler.Update(c)).To(Succeed())
		Expect(rr.Code).To(Equal(http.StatusBadRequest))
	})

	It("GetLogs returns the appended log buffer as text/plain", func() {
		c, rr := withParam(http.MethodGet, "/api/v1/extensions/e-1/logs", "e-1", "")
		Expect(handler.GetLogs(c)).To(Succeed())
		Expect(rr.Code).To(Equal(http.StatusOK))
		Expect(rr.Body.String()).To(Equal("step 1\nstep 2"))
	})

	It("Cancel delegates to the builder", func() {
		c, rr := withParam(http.MethodPost, "/api/v1/extensions/e-1/cancel", "e-1", "")
		Expect(handler.Cancel(c)).To(Succeed())
		Expect(rr.Code).To(Equal(http.StatusNoContent))
		Expect(fb.cancels).To(Equal([]string{"e-1"}))
	})
})

var _ = Describe("ExtensionHandler.Delete — bundle-blocks-delete", func() {
	var (
		e       *echo.Echo
		es      *fakeExtensionStore
		bs      *fakeBundleStore
		handler *handlers.ExtensionHandler
		ctx     = context.Background()
	)

	BeforeEach(func() {
		e = echo.New()
		es = newFakeExtensionStore()
		bs = newFakeBundleStore()
		handler = handlers.NewExtensionHandler(&fakeExtensionBuilder{}, es, bs, nil, "")
		_ = es.Create(ctx, &store.ExtensionRecord{ID: "e-1", Name: "tailscale-agent", Type: "sysext", Phase: "Ready"})
	})

	doDelete := func(id string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/extensions/"+id, nil)
		rr := httptest.NewRecorder()
		c := e.NewContext(req, rr)
		c.SetParamNames("id")
		c.SetParamValues(id)
		Expect(handler.Delete(c)).To(Succeed())
		return rr
	}

	It("allows deletion when no bundle references the name", func() {
		rec := doDelete("e-1")
		Expect(rec.Code).To(Equal(http.StatusNoContent))
		_, err := es.GetByID(ctx, "e-1")
		Expect(err).To(HaveOccurred())
	})

	It("returns 409 when a bundle references the extension by name", func() {
		_ = bs.ReplaceForArtifact(ctx, "a-1", []store.ArtifactExtensionBundle{
			{ArtifactID: "a-1", ExtensionName: "tailscale-agent", ExtensionType: "sysext"},
		})
		rec := doDelete("e-1")
		Expect(rec.Code).To(Equal(http.StatusConflict))
		Expect(rec.Body.String()).To(ContainSubstring("a-1"))
		// Record must NOT be deleted.
		_, err := es.GetByID(ctx, "e-1")
		Expect(err).ToNot(HaveOccurred())
	})

	It("returns 404 for a missing extension", func() {
		rec := doDelete("nope")
		Expect(rec.Code).To(Equal(http.StatusNotFound))
	})
})

var _ = Describe("ExtensionHandler.Download", func() {
	var (
		e       *echo.Echo
		tmp     string
		handler *handlers.ExtensionHandler
	)

	BeforeEach(func() {
		e = echo.New()
		tmp = GinkgoT().TempDir()
		extDir := filepath.Join(tmp, "extensions", "e-1")
		Expect(os.MkdirAll(extDir, 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(extDir, "tailscale-agent.sysext.raw"), []byte("RAW-BYTES"), 0o644)).To(Succeed())
		handler = handlers.NewExtensionHandler(&fakeExtensionBuilder{}, newFakeExtensionStore(), newFakeBundleStore(), nil, tmp)
	})

	do := func(id, filename string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/extensions/"+id+"/download/"+filename, nil)
		rr := httptest.NewRecorder()
		c := e.NewContext(req, rr)
		c.SetParamNames("id", "filename")
		c.SetParamValues(id, filename)
		Expect(handler.Download(c)).To(Succeed())
		return rr
	}

	It("serves the .raw file", func() {
		rec := do("e-1", "tailscale-agent.sysext.raw")
		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Body.String()).To(Equal("RAW-BYTES"))
	})

	It("returns 404 for a missing file", func() {
		rec := do("e-1", "missing.raw")
		Expect(rec.Code).To(Equal(http.StatusNotFound))
	})

	It("rejects path traversal in filename", func() {
		rec := do("e-1", "../../etc/passwd")
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
	})

	It("rejects path traversal in id", func() {
		rec := do("../e-1", "foo.raw")
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
	})
})
