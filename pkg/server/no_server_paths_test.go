package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/AuroraBoot/internal/netbootmgr"
	gormstore "github.com/kairos-io/AuroraBoot/internal/store/gorm"
	"github.com/kairos-io/AuroraBoot/pkg/server"
	"github.com/kairos-io/AuroraBoot/pkg/store"
)

// Records hold server paths (key set directories, built file paths); no GET
// response may reveal them. The walk reads every GET route from the router,
// so a route added later is covered too.
var _ = Describe("Server paths in API responses", func() {
	It("appear in no GET response", func() {
		root := GinkgoT().TempDir()
		keysDir := filepath.Join(root, "keys")
		artifactsDir := filepath.Join(root, "artifacts")
		db, err := gormstore.New(filepath.Join(root, "auroraboot.db"))
		Expect(err).NotTo(HaveOccurred())
		ctx := context.Background()

		keySets := &gormstore.SecureBootKeySetStoreAdapter{S: db}
		artifacts := &gormstore.ArtifactStoreAdapter{S: db}
		extensions := &gormstore.ExtensionStoreAdapter{S: db}
		Expect(keySets.Create(ctx, &store.SecureBootKeySet{ID: "ks-1", Name: "prod",
			KeysDir: filepath.Join(keysDir, "prod"), TPMPCRKeyPath: filepath.Join(keysDir, "prod", "tpm2-pcr-private.pem")})).To(Succeed())
		Expect(artifacts.Create(ctx, &store.ArtifactRecord{ID: "art-1", Phase: store.ArtifactReady, OverlayID: "ov-1",
			ArtifactFiles: []string{filepath.Join(artifactsDir, "art-1", "kairos.iso"), "data/artifacts/art-1/kairos.raw"}})).To(Succeed())
		Expect(extensions.Create(ctx, &store.ExtensionRecord{ID: "ext-1", Name: "tools", Type: "sysext", Phase: "Ready",
			SigningKeySetID: "ks-1", RawFilename: "tools.sysext.raw"})).To(Succeed())

		e := server.New(server.Config{
			NodeStore:                    &gormstore.NodeStoreAdapter{S: db},
			CommandStore:                 &gormstore.CommandStoreAdapter{S: db},
			GroupStore:                   &gormstore.GroupStoreAdapter{S: db},
			ArtifactStore:                artifacts,
			SecureBootKeySetStore:        keySets,
			ExtensionStore:               extensions,
			ArtifactExtensionBundleStore: &gormstore.ArtifactExtensionBundleStoreAdapter{S: db},
			NodeExtensionStore:           &gormstore.NodeExtensionStoreAdapter{S: db},
			ExtensionBuilder:             unusedExtensionBuilder{},
			DeploymentStore:              &gormstore.DeploymentStoreAdapter{S: db},
			BMCTargetStore:               &gormstore.BMCTargetStoreAdapter{S: db},
			SettingsStore:                &gormstore.SettingsStoreAdapter{S: db},
			NetbootManager:               netbootmgr.NewManager("http://localhost:8080", nil),
			Builder:                      &fakeBuilder{},
			AdminPassword:                "admin-pass",
			RegToken:                     "reg-token",
			AuroraBootURL:                "http://localhost:8080",
			ArtifactsDir:                 artifactsDir,
			KeysDir:                      keysDir,
		})

		ids := map[string]string{"/artifacts/:id": "art-1", "/secureboot-keys/:id": "ks-1", "/extensions/:id": "ext-1"}
		walked := 0
		for _, r := range e.Routes() {
			if r.Method != http.MethodGet || strings.HasPrefix(r.Path, "/api/v1/ws") {
				continue
			}
			path := r.Path
			for template, id := range ids {
				path = strings.Replace(path, template, strings.Replace(template, ":id", id, 1), 1)
			}
			req := httptest.NewRequest(http.MethodGet, concreteRoutePath(path), nil)
			req.Header.Set("Authorization", "Bearer admin-pass")
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			Expect(rec.Code).To(BeNumerically("<", 500), r.Path)
			Expect(rec.Body.String()).NotTo(ContainSubstring(root), r.Path)
			Expect(rec.Body.String()).NotTo(ContainSubstring("data/"), r.Path)
			walked++
		}
		Expect(walked).To(BeNumerically(">", 20))
	})
})
