package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

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
		handler = handlers.NewArtifactHandler(fb, nil, nil, nil, nil, nil, "", "reg-token", "http://localhost:8080")
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

		It("returns 400 (not 500) when build inputs fail validation", func() {
			// The real builder rejects shell-metacharacter values like this
			// before any build starts and returns an ErrInvalidBuildOptions-wrapped
			// error; the handler must surface that as a client error.
			fb.buildErr = fmt.Errorf("%w: invalid kairos version %q", builder.ErrInvalidBuildOptions, "latest && id")

			body := `{"baseImage":"quay.io/kairos/ubuntu:24.04","kairosVersion":"latest && id","outputs":{"iso":true}}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := handler.Create(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusBadRequest))

			var resp map[string]string
			Expect(json.Unmarshal(rec.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp["error"]).To(ContainSubstring("invalid build options"))

			// The rejected request must not have been queued.
			Expect(fb.builds).To(BeEmpty())
		})

		It("returns 500 for a genuine server failure", func() {
			fb.buildErr = fmt.Errorf("disk full")

			body := `{"baseImage":"quay.io/kairos/ubuntu:24.04","outputs":{"iso":true}}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := handler.Create(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusInternalServerError))
		})
	})

	// The allowed_commands block in the generated phonehome cloud-config is
	// AuroraBoot's only lever for gating destructive remote commands, so we
	// pin its behavior in integration tests through the public Create endpoint.
	Describe("Create — phonehome allowed_commands", func() {
		post := func(body string) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			Expect(handler.Create(c)).To(Succeed())
			Expect(rec.Code).To(Equal(http.StatusCreated))
		}

		It("substitutes safe defaults when allowedCommands is omitted", func() {
			post(`{"baseImage":"quay.io/kairos/ubuntu:24.04","outputs":{"iso":true}}`)
			Expect(fb.lastOpts.Provisioning.AllowedCommands).To(ConsistOf("upgrade", "upgrade-recovery", "reboot", "unregister"))
			Expect(fb.lastOpts.CloudConfig).To(ContainSubstring("phonehome:"))
			Expect(fb.lastOpts.CloudConfig).To(ContainSubstring("allowed_commands:"))
			Expect(fb.lastOpts.CloudConfig).To(ContainSubstring("- upgrade"))
			Expect(fb.lastOpts.CloudConfig).To(ContainSubstring("- reboot"))
		})

		It("passes through a custom allowedCommands list verbatim", func() {
			post(`{"baseImage":"quay.io/kairos/ubuntu:24.04","outputs":{"iso":true},"provisioning":{"registerAuroraBoot":true,"allowedCommands":["exec","reboot"]}}`)
			Expect(fb.lastOpts.Provisioning.AllowedCommands).To(Equal([]string{"exec", "reboot"}))
			Expect(fb.lastOpts.CloudConfig).To(ContainSubstring("- exec"))
			Expect(fb.lastOpts.CloudConfig).To(ContainSubstring("- reboot"))
			// The destructive command the user did NOT pick must not leak in.
			Expect(fb.lastOpts.CloudConfig).NotTo(ContainSubstring("- reset"))
		})

		It("emits an empty list when the operator opts into observe-only", func() {
			post(`{"baseImage":"quay.io/kairos/ubuntu:24.04","outputs":{"iso":true},"provisioning":{"registerAuroraBoot":true,"allowedCommands":[]}}`)
			Expect(fb.lastOpts.Provisioning.AllowedCommands).To(HaveLen(0))
			Expect(fb.lastOpts.Provisioning.AllowedCommands).NotTo(BeNil())
			Expect(fb.lastOpts.CloudConfig).To(ContainSubstring("allowed_commands: []"))
		})

		It("omits the phonehome stanza entirely when registerAuroraBoot is false", func() {
			post(`{"baseImage":"quay.io/kairos/ubuntu:24.04","outputs":{"iso":true},"provisioning":{"registerAuroraBoot":false}}`)
			Expect(fb.lastOpts.CloudConfig).NotTo(ContainSubstring("phonehome:"))
			Expect(fb.lastOpts.CloudConfig).NotTo(ContainSubstring("allowed_commands"))
		})
	})

	Describe("Create — kubernetes provider cloud-config", func() {
		post := func(body string) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			Expect(handler.Create(c)).To(Succeed())
			Expect(rec.Code).To(Equal(http.StatusCreated))
		}

		It("enables k3s in cloud-config for the standard variant", func() {
			post(`{"baseImage":"ubuntu:24.04","variant":"standard","kubernetesDistro":"k3s","outputs":{"iso":true}}`)
			Expect(fb.lastOpts.CloudConfig).To(ContainSubstring("k3s:"))
			Expect(fb.lastOpts.CloudConfig).To(ContainSubstring("enabled: true"))
		})

		It("enables k0s in cloud-config for the standard variant", func() {
			post(`{"baseImage":"ubuntu:24.04","variant":"standard","kubernetesDistro":"k0s","outputs":{"iso":true}}`)
			Expect(fb.lastOpts.CloudConfig).To(ContainSubstring("k0s:"))
			Expect(fb.lastOpts.CloudConfig).To(ContainSubstring("enabled: true"))
		})

		It("disables k3s in cloud-config when kubernetesEnabled is false", func() {
			post(`{"baseImage":"ubuntu:24.04","variant":"standard","kubernetesDistro":"k3s","kubernetesEnabled":false,"outputs":{"iso":true}}`)
			Expect(fb.lastOpts.CloudConfig).To(ContainSubstring("k3s:"))
			Expect(fb.lastOpts.CloudConfig).To(ContainSubstring("enabled: false"))
		})

		It("defaults kubernetesEnabled to true when omitted", func() {
			post(`{"baseImage":"ubuntu:24.04","variant":"standard","kubernetesDistro":"k3s","outputs":{"iso":true}}`)
			Expect(fb.lastOpts.CloudConfig).To(ContainSubstring("enabled: true"))
		})

		It("omits kubernetes provider stanzas for the core variant", func() {
			post(`{"baseImage":"ubuntu:24.04","variant":"core","kubernetesDistro":"k3s","outputs":{"iso":true}}`)
			Expect(fb.lastOpts.CloudConfig).NotTo(ContainSubstring("k3s:"))
			Expect(fb.lastOpts.CloudConfig).NotTo(ContainSubstring("k0s:"))
		})

		It("merges extra k3s YAML without duplicating the top-level key", func() {
			post(`{"baseImage":"ubuntu:24.04","variant":"standard","kubernetesDistro":"k3s","outputs":{"iso":true},"cloudConfig":"k3s:\n  enabled: true\n  cluster-cidr: 10.42.0.0/16"}`)
			Expect(strings.Count(fb.lastOpts.CloudConfig, "k3s:")).To(Equal(1))
			Expect(fb.lastOpts.CloudConfig).To(ContainSubstring("cluster-cidr"))
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
			as               *fakeArtifactStore
			handlerWithStore *handlers.ArtifactHandler
		)

		BeforeEach(func() {
			as = &fakeArtifactStore{
				records: []*store.ArtifactRecord{
					{ID: "art-1", Phase: store.ArtifactReady, BaseImage: "img1"},
				},
			}
			handlerWithStore = handlers.NewArtifactHandler(fb, as, nil, nil, nil, nil, "", "reg-token", "http://localhost:8080")
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
			as               *fakeArtifactStore
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
			handlerWithStore = handlers.NewArtifactHandler(fb, as, nil, nil, nil, nil, "", "reg-token", "http://localhost:8080")
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

var _ = Describe("ArtifactHandler — bundle endpoints", func() {
	var (
		e       *echo.Echo
		fb      *fakeBuilder
		es      *fakeExtensionStore
		bs      *fakeBundleStore
		handler *handlers.ArtifactHandler
		ctx     = context.Background()
	)

	BeforeEach(func() {
		e = echo.New()
		fb = &fakeBuilder{}
		es = newFakeExtensionStore()
		bs = newFakeBundleStore()
		handler = handlers.NewArtifactHandler(fb, nil, nil, nil, es, bs, "", "reg-token", "http://localhost:8080")
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

	It("GET /bundle-extensions returns an empty list when none configured", func() {
		c, rec := withParam(http.MethodGet, "/api/v1/artifacts/a-1/bundle-extensions", "a-1", "")
		Expect(handler.ListBundleExtensions(c)).To(Succeed())
		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Body.String()).To(MatchRegexp(`^\[\]\s*$`))
	})

	It("GET /bundle-extensions returns the stored entries", func() {
		Expect(bs.ReplaceForArtifact(ctx, "a-1", []store.ArtifactExtensionBundle{
			{ArtifactID: "a-1", ExtensionName: "tailscale-agent", ExtensionType: "sysext", Order: 0},
		})).To(Succeed())
		c, rec := withParam(http.MethodGet, "/api/v1/artifacts/a-1/bundle-extensions", "a-1", "")
		Expect(handler.ListBundleExtensions(c)).To(Succeed())
		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Body.String()).To(ContainSubstring("tailscale-agent"))
	})
})

var _ = Describe("ArtifactHandler.SetBundleExtensions", func() {
	var (
		e       *echo.Echo
		es      *fakeExtensionStore
		bs      *fakeBundleStore
		ars     *fakeArtifactStore
		handler *handlers.ArtifactHandler
		ctx     = context.Background()
	)

	BeforeEach(func() {
		e = echo.New()
		es = newFakeExtensionStore()
		bs = newFakeBundleStore()
		ars = newFakeArtifactStore()
		_ = ars.Create(ctx, &store.ArtifactRecord{ID: "a-1", Arch: "amd64", Phase: "Ready"})
		_ = es.Create(ctx, &store.ExtensionRecord{ID: "e-1", Name: "tailscale-agent", Type: "sysext", Arch: "amd64", Phase: "Ready"})
		_ = es.Create(ctx, &store.ExtensionRecord{ID: "e-2", Name: "armthing", Type: "sysext", Arch: "arm64", Phase: "Ready"})
		handler = handlers.NewArtifactHandler(&fakeBuilder{}, ars, nil, nil, es, bs, "", "tok", "http://x")
	})

	put := func(id, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/artifacts/"+id+"/bundle-extensions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(id)
		Expect(handler.SetBundleExtensions(c)).To(Succeed())
		return rec
	}

	It("accepts entries whose arch matches the artifact", func() {
		rec := put("a-1", `[{"extensionName":"tailscale-agent","extensionType":"sysext"}]`)
		Expect(rec.Code).To(Equal(http.StatusOK))
		got, _ := bs.ListForArtifact(ctx, "a-1")
		Expect(got).To(HaveLen(1))
	})

	It("rejects entries whose arch does NOT match the artifact", func() {
		rec := put("a-1", `[{"extensionName":"armthing","extensionType":"sysext"}]`)
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
		Expect(rec.Body.String()).To(ContainSubstring("arch"))
	})

	It("rejects entries whose name has no matching Ready extension", func() {
		rec := put("a-1", `[{"extensionName":"does-not-exist","extensionType":"sysext"}]`)
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
		Expect(rec.Body.String()).To(ContainSubstring("does-not-exist"))
	})

	It("replaces the entire set atomically", func() {
		put("a-1", `[{"extensionName":"tailscale-agent","extensionType":"sysext"}]`)
		_ = es.Create(ctx, &store.ExtensionRecord{ID: "e-3", Name: "newthing", Type: "sysext", Arch: "amd64", Phase: "Ready"})
		rec := put("a-1", `[{"extensionName":"newthing","extensionType":"sysext"}]`)
		Expect(rec.Code).To(Equal(http.StatusOK))
		got, _ := bs.ListForArtifact(ctx, "a-1")
		Expect(got).To(HaveLen(1))
		Expect(got[0].ExtensionName).To(Equal("newthing"))
	})

	It("returns 404 when the artifact does not exist", func() {
		rec := put("missing", `[]`)
		Expect(rec.Code).To(Equal(http.StatusNotFound))
	})

	It("validates pinnedVersion against a Ready extension of that version", func() {
		_ = es.Create(ctx, &store.ExtensionRecord{ID: "e-4", Name: "tailscale-agent", Type: "sysext", Arch: "amd64", Version: "v1.74", Phase: "Ready"})
		rec := put("a-1", `[{"extensionName":"tailscale-agent","extensionType":"sysext","pinnedVersion":"v1.74"}]`)
		Expect(rec.Code).To(Equal(http.StatusOK))
		rec = put("a-1", `[{"extensionName":"tailscale-agent","extensionType":"sysext","pinnedVersion":"v99"}]`)
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
	})
})

var _ = Describe("ArtifactHandler.Create — extensionHierarchies", func() {
	var (
		e       *echo.Echo
		ars     *fakeArtifactStore
		handler *handlers.ArtifactHandler
	)

	BeforeEach(func() {
		e = echo.New()
		ars = newFakeArtifactStore()
		handler = handlers.NewArtifactHandler(&fakeBuilder{}, ars, nil, nil, nil, nil, "", "tok", "http://x")
	})

	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		Expect(handler.Create(c)).To(Succeed())
		return rec
	}

	It("persists sysext + confext hierarchies onto the artifact record", func() {
		rec := post(`{"baseImage":"ubuntu:24.04","arch":"amd64","outputs":{"iso":true},
			"extensionHierarchies":{"sysext":["/opt","/srv"],"confext":[]}}`)
		Expect(rec.Code).To(Equal(http.StatusCreated))

		Expect(ars.lastSaved).ToNot(BeNil())
		Expect(ars.lastSaved.ExtensionHierarchies.Sysext).To(Equal([]string{"/opt", "/srv"}))
		Expect(ars.lastSaved.ExtensionHierarchies.Confext).To(BeEmpty())
	})

	It("validates each sysext hierarchy entry", func() {
		rec := post(`{"baseImage":"ubuntu:24.04","arch":"amd64","outputs":{"iso":true},
			"extensionHierarchies":{"sysext":["/usr"]}}`)
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
		Expect(rec.Body.String()).To(ContainSubstring("/usr"))
	})

	It("validates each confext hierarchy entry — /etc is implicit", func() {
		rec := post(`{"baseImage":"ubuntu:24.04","arch":"amd64","outputs":{"iso":true},
			"extensionHierarchies":{"sysext":[], "confext":["/etc"]}}`)
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
		Expect(rec.Body.String()).To(ContainSubstring("/etc"))
	})

	It("accepts an absent field", func() {
		rec := post(`{"baseImage":"ubuntu:24.04","arch":"amd64","outputs":{"iso":true}}`)
		Expect(rec.Code).To(Equal(http.StatusCreated))
		Expect(ars.lastSaved.ExtensionHierarchies.Sysext).To(BeNil())
	})
})

var _ = Describe("ArtifactHandler.Create — bundledExtensions", func() {
	var (
		e       *echo.Echo
		ars     *fakeArtifactStore
		es      *fakeExtensionStore
		bs      *fakeBundleStore
		handler *handlers.ArtifactHandler
		ctx     = context.Background()
	)

	BeforeEach(func() {
		e = echo.New()
		ars = newFakeArtifactStore()
		es = newFakeExtensionStore()
		bs = newFakeBundleStore()
		_ = es.Create(ctx, &store.ExtensionRecord{ID: "e-1", Name: "tailscale-agent", Type: "sysext", Arch: "amd64", Phase: "Ready"})
		handler = handlers.NewArtifactHandler(&fakeBuilder{}, ars, nil, nil, es, bs, "", "tok", "http://x")
	})

	post := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		Expect(handler.Create(c)).To(Succeed())
		return rec
	}

	It("persists bundledExtensions when arch matches", func() {
		rec := post(`{"baseImage":"ubuntu:24.04","arch":"amd64","outputs":{"iso":true},
			"bundledExtensions":[{"name":"tailscale-agent","type":"sysext"}]}`)
		Expect(rec.Code).To(Equal(http.StatusCreated))
		entries, _ := bs.ListForArtifact(ctx, ars.lastSaved.ID)
		Expect(entries).To(HaveLen(1))
	})

	It("rejects bundled entries whose arch differs from the artifact", func() {
		rec := post(`{"baseImage":"ubuntu:24.04","arch":"arm64","outputs":{"iso":true},
			"bundledExtensions":[{"name":"tailscale-agent","type":"sysext"}]}`)
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
		Expect(ars.lastSaved).To(BeNil())
	})
})

var _ = Describe("ArtifactHandler.Create — extension hierarchies cloud-config bake", func() {
	var (
		e       *echo.Echo
		fb      *fakeBuilder
		ars     *fakeArtifactStore
		handler *handlers.ArtifactHandler
	)

	BeforeEach(func() {
		e = echo.New()
		fb = &fakeBuilder{}
		ars = newFakeArtifactStore()
		handler = handlers.NewArtifactHandler(fb, ars, nil, nil, nil, nil, "", "tok", "http://x")
	})

	post := func(body string) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		Expect(handler.Create(c)).To(Succeed())
		Expect(rec.Code).To(Equal(http.StatusCreated))
	}

	It("bakes a SYSTEMD_SYSEXT_HIERARCHIES drop-in when sysext hierarchies are listed", func() {
		post(`{"baseImage":"ubuntu:24.04","arch":"amd64","outputs":{"iso":true},
			"extensionHierarchies":{"sysext":["/opt","/srv"]}}`)
		Expect(fb.lastOpts.CloudConfig).To(ContainSubstring("99-aurora-hierarchies.conf"))
		Expect(fb.lastOpts.CloudConfig).To(ContainSubstring("SYSTEMD_SYSEXT_HIERARCHIES=/usr:/opt:/srv"))
		Expect(fb.lastOpts.CloudConfig).ToNot(ContainSubstring("SYSTEMD_CONFEXT_HIERARCHIES"))
	})

	It("bakes a SYSTEMD_CONFEXT_HIERARCHIES drop-in when confext hierarchies are listed", func() {
		post(`{"baseImage":"ubuntu:24.04","arch":"amd64","outputs":{"iso":true},
			"extensionHierarchies":{"sysext":[],"confext":["/srv/configs"]}}`)
		Expect(fb.lastOpts.CloudConfig).To(ContainSubstring("SYSTEMD_CONFEXT_HIERARCHIES=/etc:/srv/configs"))
		Expect(fb.lastOpts.CloudConfig).ToNot(ContainSubstring("SYSTEMD_SYSEXT_HIERARCHIES"))
	})

	It("omits both drop-ins when no extension hierarchies are listed", func() {
		post(`{"baseImage":"ubuntu:24.04","arch":"amd64","outputs":{"iso":true}}`)
		Expect(fb.lastOpts.CloudConfig).ToNot(ContainSubstring("99-aurora-hierarchies.conf"))
	})
})

var _ = Describe("ArtifactHandler.ResolveBundle", func() {
	var (
		e       *echo.Echo
		es      *fakeExtensionStore
		bs      *fakeBundleStore
		ars     *fakeArtifactStore
		handler *handlers.ArtifactHandler
		ctx     = context.Background()
	)

	BeforeEach(func() {
		e = echo.New()
		es = newFakeExtensionStore()
		bs = newFakeBundleStore()
		ars = newFakeArtifactStore()
		_ = ars.Create(ctx, &store.ArtifactRecord{ID: "a-1", Arch: "amd64", Phase: "Ready"})
		// Two ready extensions named "tailscale-agent": newer wins for unpinned resolution.
		_ = es.Create(ctx, &store.ExtensionRecord{
			ID: "e-old", Name: "tailscale-agent", Type: "sysext", Arch: "amd64", Version: "v1.72",
			Phase: "Ready", CreatedAt: time.Now().Add(-1 * time.Hour),
			RawFilename: "tailscale-agent.sysext.raw",
		})
		_ = es.Create(ctx, &store.ExtensionRecord{
			ID: "e-new", Name: "tailscale-agent", Type: "sysext", Arch: "amd64", Version: "v1.74",
			Phase: "Ready", CreatedAt: time.Now(),
			RawFilename: "tailscale-agent.sysext.raw",
		})
		_ = bs.ReplaceForArtifact(ctx, "a-1", []store.ArtifactExtensionBundle{
			{ArtifactID: "a-1", ExtensionName: "tailscale-agent", ExtensionType: "sysext"},
		})
		handler = handlers.NewArtifactHandler(&fakeBuilder{}, ars, nil, nil, es, bs, "", "tok", "http://aurora.local")
	})

	post := func(id string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/artifacts/"+id+"/bundle-resolve", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(id)
		Expect(handler.ResolveBundle(c)).To(Succeed())
		return rec
	}

	It("emits a source URL pointing at the extensions download endpoint", func() {
		rec := post("a-1")
		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Body.String()).To(ContainSubstring("/api/v1/extensions/"))
		Expect(rec.Body.String()).To(ContainSubstring("/download/tailscale-agent.sysext.raw"))
		Expect(rec.Body.String()).To(ContainSubstring("http://aurora.local"))
	})

	It("returns 404 when the artifact is unknown", func() {
		rec := post("missing")
		Expect(rec.Code).To(Equal(http.StatusNotFound))
	})

	It("returns 400 when a bundle entry cannot resolve", func() {
		_ = bs.ReplaceForArtifact(ctx, "a-1", []store.ArtifactExtensionBundle{
			{ArtifactID: "a-1", ExtensionName: "does-not-exist", ExtensionType: "sysext"},
		})
		rec := post("a-1")
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
		Expect(rec.Body.String()).To(ContainSubstring("does-not-exist"))
	})
})
