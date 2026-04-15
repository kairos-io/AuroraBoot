package server_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/AuroraBoot/pkg/builder"
	"github.com/kairos-io/AuroraBoot/pkg/server"
	"github.com/kairos-io/AuroraBoot/pkg/store"
)

// Minimal fake implementations for server integration tests.

type fakeNodeStore struct {
	mu    sync.Mutex
	nodes []*store.ManagedNode
}

func (f *fakeNodeStore) Register(_ context.Context, n *store.ManagedNode) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nodes = append(f.nodes, n)
	return nil
}
func (f *fakeNodeStore) GetByID(_ context.Context, id string) (*store.ManagedNode, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, n := range f.nodes {
		if n.ID == id {
			return n, nil
		}
	}
	return nil, fmt.Errorf("not found")
}
func (f *fakeNodeStore) GetByMachineID(_ context.Context, mid string) (*store.ManagedNode, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, n := range f.nodes {
		if n.MachineID == mid {
			return n, nil
		}
	}
	return nil, fmt.Errorf("not found")
}
func (f *fakeNodeStore) GetByAPIKey(_ context.Context, key string) (*store.ManagedNode, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, n := range f.nodes {
		if n.APIKey == key {
			return n, nil
		}
	}
	return nil, fmt.Errorf("not found")
}
func (f *fakeNodeStore) List(_ context.Context) ([]*store.ManagedNode, error) {
	return f.nodes, nil
}
func (f *fakeNodeStore) ListByGroup(_ context.Context, _ string) ([]*store.ManagedNode, error) {
	return nil, nil
}
func (f *fakeNodeStore) ListByLabels(_ context.Context, _ map[string]string) ([]*store.ManagedNode, error) {
	return nil, nil
}
func (f *fakeNodeStore) ListBySelector(_ context.Context, _ store.CommandSelector) ([]*store.ManagedNode, error) {
	return nil, nil
}
func (f *fakeNodeStore) UpdateHeartbeat(_ context.Context, _ string, _ string, _ map[string]string) error {
	return nil
}
func (f *fakeNodeStore) UpdatePhase(_ context.Context, _ string, _ string) error { return nil }
func (f *fakeNodeStore) SetGroup(_ context.Context, _ string, _ string) error    { return nil }
func (f *fakeNodeStore) SetLabels(_ context.Context, _ string, _ map[string]string) error {
	return nil
}
func (f *fakeNodeStore) Delete(_ context.Context, _ string) error { return nil }

type fakeCommandStore struct{}

func (f *fakeCommandStore) Create(_ context.Context, _ *store.NodeCommand) error     { return nil }
func (f *fakeCommandStore) GetByID(_ context.Context, _ string) (*store.NodeCommand, error) {
	return nil, fmt.Errorf("not found")
}
func (f *fakeCommandStore) GetPending(_ context.Context, _ string) ([]*store.NodeCommand, error) {
	return nil, nil
}
func (f *fakeCommandStore) MarkDelivered(_ context.Context, _ []string) error { return nil }
func (f *fakeCommandStore) UpdateStatus(_ context.Context, _ string, _ string, _ string) error {
	return nil
}
func (f *fakeCommandStore) ListByNode(_ context.Context, _ string) ([]*store.NodeCommand, error) {
	return nil, nil
}
func (f *fakeCommandStore) Delete(_ context.Context, _ string) error        { return nil }
func (f *fakeCommandStore) DeleteTerminal(_ context.Context, _ string) error { return nil }

type fakeGroupStore struct{}

func (f *fakeGroupStore) Create(_ context.Context, _ *store.NodeGroup) error { return nil }
func (f *fakeGroupStore) GetByID(_ context.Context, _ string) (*store.NodeGroup, error) {
	return nil, fmt.Errorf("not found")
}
func (f *fakeGroupStore) GetByName(_ context.Context, _ string) (*store.NodeGroup, error) {
	return nil, fmt.Errorf("not found")
}
func (f *fakeGroupStore) List(_ context.Context) ([]*store.NodeGroup, error) { return nil, nil }
func (f *fakeGroupStore) Update(_ context.Context, _ *store.NodeGroup) error { return nil }
func (f *fakeGroupStore) Delete(_ context.Context, _ string) error           { return nil }

type fakeBuilder struct{}

func (f *fakeBuilder) Build(_ context.Context, opts builder.BuildOptions) (*builder.BuildStatus, error) {
	return &builder.BuildStatus{ID: opts.ID, Phase: builder.BuildPending}, nil
}
func (f *fakeBuilder) Status(_ context.Context, _ string) (*builder.BuildStatus, error) {
	return nil, fmt.Errorf("not found")
}
func (f *fakeBuilder) List(_ context.Context) ([]*builder.BuildStatus, error) { return nil, nil }
func (f *fakeBuilder) Cancel(_ context.Context, _ string) error               { return nil }

var _ = Describe("Server", func() {
	var (
		e  *httptest.Server
		ns *fakeNodeStore
	)

	BeforeEach(func() {
		ns = &fakeNodeStore{}
		echoApp := server.New(server.Config{
			NodeStore:     ns,
			CommandStore:  &fakeCommandStore{},
			GroupStore:    &fakeGroupStore{},
			Builder:       &fakeBuilder{},
			AdminPassword: "admin-pass",
			RegToken:      "reg-token",
			DaedalusURL:   "http://localhost:8080",
		})
		e = httptest.NewServer(echoApp)
	})

	AfterEach(func() {
		e.Close()
	})

	Describe("Public endpoints", func() {
		It("should serve the install script without auth", func() {
			resp, err := http.Get(e.URL + "/api/v1/install-agent")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})
	})

	Describe("Admin auth", func() {
		It("should reject unauthenticated admin requests", func() {
			resp, err := http.Get(e.URL + "/api/v1/nodes")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})

		It("should allow authenticated admin requests", func() {
			req, _ := http.NewRequest(http.MethodGet, e.URL+"/api/v1/nodes", nil)
			req.Header.Set("Authorization", "Bearer admin-pass")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})
	})

	Describe("Registration auth", func() {
		It("should reject registration without token", func() {
			body := `{"machineID":"m1","hostname":"h1"}`
			resp, err := http.Post(e.URL+"/api/v1/nodes/register", "application/json", strings.NewReader(body))
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})

		It("should allow registration with valid token", func() {
			body := `{"registrationToken":"reg-token","machineID":"m1","hostname":"h1"}`
			resp, err := http.Post(e.URL+"/api/v1/nodes/register", "application/json", strings.NewReader(body))
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusCreated))
		})
	})

	Describe("Node API key auth", func() {
		It("should reject agent requests without API key", func() {
			body := `{"agentVersion":"1.0"}`
			req, _ := http.NewRequest(http.MethodPost, e.URL+"/api/v1/nodes/node-1/heartbeat", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})

		It("should allow agent requests with valid API key", func() {
			ns.nodes = []*store.ManagedNode{
				{ID: "node-1", APIKey: "agent-key"},
			}
			body := `{"agentVersion":"1.0"}`
			req, _ := http.NewRequest(http.MethodPost, e.URL+"/api/v1/nodes/node-1/heartbeat", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer agent-key")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})
	})

	Describe("SPA fallback", func() {
		It("should serve index.html for HTML requests to unknown paths", func() {
			req, _ := http.NewRequest(http.MethodGet, e.URL+"/some/spa/route", nil)
			req.Header.Set("Accept", "text/html")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})
	})
})
