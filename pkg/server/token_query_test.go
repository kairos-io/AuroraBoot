package server_test

import (
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/AuroraBoot/pkg/server"
	"github.com/kairos-io/AuroraBoot/pkg/store"
	"github.com/labstack/echo/v4"
)

// The admin password in a query string is no credential on any route, walked
// from the router so a route added later is covered too.
var _ = Describe("Admin password in the query string", func() {
	It("is refused on every route", func() {
		e := server.New(server.Config{
			NodeStore:        &fakeNodeStore{nodes: []*store.ManagedNode{{ID: "node-1", APIKey: "node-key"}}},
			CommandStore:     &fakeCommandStore{},
			GroupStore:       &fakeGroupStore{},
			ArtifactStore:    &fakeArtifactStore{},
			Builder:          &fakeBuilder{},
			ExtensionBuilder: unusedExtensionBuilder{},
			DeploymentStore:  unusedDeploymentStore{},
			AdminPassword:    "admin-pass",
			RegToken:         "reg-token",
			AuroraBootURL:    "http://localhost:8080",
			ArtifactsDir:     GinkgoT().TempDir(),
			KeysDir:          GinkgoT().TempDir(),
		})

		public := map[string]bool{
			"GET /healthz": true, "GET /readyz": true,
			"GET /api/v1/openapi.json": true, "GET /api/v1/openapi.yaml": true,
			"GET /api/docs": true, "GET /api/v1/install-agent": true,
		}
		for _, r := range e.Routes() {
			route := r.Method + " " + r.Path
			if public[route] || r.Method == echo.RouteNotFound || r.Path == "/*" {
				continue
			}
			req := httptest.NewRequest(r.Method, concreteRoutePath(r.Path)+"?token=admin-pass", nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			Expect(rec.Code).To(BeNumerically(">=", 400), route)
		}
	})
})

// concreteRoutePath fills a route template's parameters.
func concreteRoutePath(template string) string {
	out := []byte{}
	for i := 0; i < len(template); i++ {
		switch template[i] {
		case ':':
			for i < len(template) && template[i] != '/' {
				i++
			}
			out = append(out, 'x')
			if i < len(template) {
				out = append(out, '/')
			}
		case '*':
			out = append(out, "file"...)
		default:
			out = append(out, template[i])
		}
	}
	return string(out)
}
