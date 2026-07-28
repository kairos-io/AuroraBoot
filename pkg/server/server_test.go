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
func (f *fakeNodeStore) UpdateHeartbeat(_ context.Context, _ string, _ string, _ map[string]string, _ []store.NodeAddress, _ string) error {
	return nil
}
func (f *fakeNodeStore) UpdatePhase(_ context.Context, _ string, _ string) error { return nil }
func (f *fakeNodeStore) SetGroup(_ context.Context, _ string, _ string) error    { return nil }
func (f *fakeNodeStore) SetLabels(_ context.Context, _ string, _ map[string]string) error {
	return nil
}
func (f *fakeNodeStore) Delete(_ context.Context, _ string) error { return nil }

type fakeCommandStore struct {
	mu   sync.Mutex
	cmds []*store.NodeCommand
}

func (f *fakeCommandStore) Create(_ context.Context, _ *store.NodeCommand) error { return nil }
func (f *fakeCommandStore) GetByID(_ context.Context, _ string) (*store.NodeCommand, error) {
	return nil, fmt.Errorf("not found")
}
func (f *fakeCommandStore) GetPending(_ context.Context, nodeID string) ([]*store.NodeCommand, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*store.NodeCommand
	for _, c := range f.cmds {
		if c.ManagedNodeID == nodeID && c.Phase == store.CommandPending {
			out = append(out, c)
		}
	}
	return out, nil
}
func (f *fakeCommandStore) MarkDelivered(_ context.Context, _ []string) error { return nil }
func (f *fakeCommandStore) ClaimForDelivery(_ context.Context, id string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.cmds {
		if c.ID == id && c.Phase == store.CommandPending {
			c.Phase = store.CommandDelivered
			return true, nil
		}
	}
	return false, nil
}
func (f *fakeCommandStore) UpdateStatus(_ context.Context, _ string, _ string, _ string) error {
	return nil
}
func (f *fakeCommandStore) UpdateStatusForNode(_ context.Context, id string, nodeID string, phase string, result string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.cmds {
		if c.ID == id && c.ManagedNodeID == nodeID {
			c.Phase = phase
			c.Result = result
			return nil
		}
	}
	return store.ErrCommandNotFound
}
func (f *fakeCommandStore) ListByNode(_ context.Context, nodeID string) ([]*store.NodeCommand, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*store.NodeCommand
	for _, c := range f.cmds {
		if c.ManagedNodeID == nodeID {
			out = append(out, c)
		}
	}
	return out, nil
}
func (f *fakeCommandStore) Delete(_ context.Context, _ string) error         { return nil }
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
		cs *fakeCommandStore
	)

	BeforeEach(func() {
		ns = &fakeNodeStore{}
		cs = &fakeCommandStore{}
		echoApp := server.New(server.Config{
			NodeStore:     ns,
			CommandStore:  cs,
			GroupStore:    &fakeGroupStore{},
			Builder:       &fakeBuilder{},
			AdminPassword: "admin-pass",
			RegToken:      "reg-token",
			AuroraBootURL: "http://localhost:8080",
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

	// Node-impersonation / BOLA: an agent authenticates with its own API key
	// but the path carries a :nodeID. These specs prove a node can only act on
	// its own resources — never another node's — even with a valid key.
	Describe("Node impersonation (BOLA)", func() {
		BeforeEach(func() {
			ns.nodes = []*store.ManagedNode{
				{ID: "node-A", APIKey: "key-A"},
				{ID: "node-B", APIKey: "key-B"},
			}
		})

		doReq := func(method, path, key, body string) *http.Response {
			var r *http.Request
			if body != "" {
				r, _ = http.NewRequest(method, e.URL+path, strings.NewReader(body))
				r.Header.Set("Content-Type", "application/json")
			} else {
				r, _ = http.NewRequest(method, e.URL+path, nil)
			}
			if key != "" {
				r.Header.Set("Authorization", "Bearer "+key)
			}
			resp, err := http.DefaultClient.Do(r)
			Expect(err).NotTo(HaveOccurred())
			return resp
		}

		It("lets a node heartbeat for itself (200)", func() {
			resp := doReq(http.MethodPost, "/api/v1/nodes/node-A/heartbeat", "key-A", `{"agentVersion":"1.0"}`)
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})

		It("blocks a node from heartbeating as another node (403)", func() {
			resp := doReq(http.MethodPost, "/api/v1/nodes/node-B/heartbeat", "key-A", `{"agentVersion":"1.0"}`)
			Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
		})

		It("lets a node read its own commands (200)", func() {
			resp := doReq(http.MethodGet, "/api/v1/nodes/node-A/commands", "key-A", "")
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})

		It("blocks a node from reading/consuming another node's commands (403)", func() {
			// Seed a pending command for node-B; node-A must not be able to
			// read it (and thereby mark it delivered).
			cs.cmds = []*store.NodeCommand{
				{ID: "cmd-B", ManagedNodeID: "node-B", Command: "upgrade", Phase: store.CommandPending},
			}
			resp := doReq(http.MethodGet, "/api/v1/nodes/node-B/commands", "key-A", "")
			Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
			// The command must remain pending — not consumed by the attacker.
			Expect(cs.cmds[0].Phase).To(Equal(store.CommandPending))
		})

		It("lets a node update the status of its own command (200)", func() {
			cs.cmds = []*store.NodeCommand{
				{ID: "cmd-A", ManagedNodeID: "node-A", Phase: store.CommandDelivered},
			}
			resp := doReq(http.MethodPut, "/api/v1/nodes/node-A/commands/cmd-A/status", "key-A", `{"phase":"Completed","result":"ok"}`)
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(cs.cmds[0].Phase).To(Equal(store.CommandCompleted))
		})

		It("blocks a node from updating another node's command via a forged path (403)", func() {
			// Path nodeID mismatches the key's identity — RequireNodeMatch
			// rejects before the handler runs.
			cs.cmds = []*store.NodeCommand{
				{ID: "cmd-B", ManagedNodeID: "node-B", Phase: store.CommandDelivered},
			}
			resp := doReq(http.MethodPut, "/api/v1/nodes/node-B/commands/cmd-B/status", "key-A", `{"phase":"Completed","result":"pwned"}`)
			Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
			Expect(cs.cmds[0].Phase).To(Equal(store.CommandDelivered))
		})

		It("blocks a node from updating a foreign command id under its own path (403)", func() {
			// Path nodeID matches the key (passes RequireNodeMatch), but the
			// commandID belongs to node-B. The node-scoped store update matches
			// zero rows and must surface as 403, not a silent 200.
			cs.cmds = []*store.NodeCommand{
				{ID: "cmd-B", ManagedNodeID: "node-B", Phase: store.CommandDelivered},
			}
			resp := doReq(http.MethodPut, "/api/v1/nodes/node-A/commands/cmd-B/status", "key-A", `{"phase":"Completed","result":"pwned"}`)
			Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
			Expect(cs.cmds[0].Phase).To(Equal(store.CommandDelivered))
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

	Describe("Extension routes", func() {
		It("GET /api/v1/artifacts/:id/bundle-extensions is reachable behind admin auth", func() {
			req, _ := http.NewRequest(http.MethodGet, e.URL+"/api/v1/artifacts/a-1/bundle-extensions", nil)
			req.Header.Set("Authorization", "Bearer admin-pass")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			// Reaches the handler; with nil bundle store the empty-list branch is taken.
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})

		It("/api/v1/artifacts/:id/bundle-extensions requires auth", func() {
			resp, err := http.Get(e.URL + "/api/v1/artifacts/a-1/bundle-extensions")
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})

		It("POST /api/v1/artifacts/:id/bundle-resolve is reachable", func() {
			req, _ := http.NewRequest(http.MethodPost, e.URL+"/api/v1/artifacts/a-1/bundle-resolve", nil)
			req.Header.Set("Authorization", "Bearer admin-pass")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			// Stores are nil here; ResolveBundle short-circuits with 500. Routing works.
			Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError))
		})

		It("does NOT register /api/v1/extensions when ExtensionBuilder is nil", func() {
			req, _ := http.NewRequest(http.MethodGet, e.URL+"/api/v1/extensions", nil)
			req.Header.Set("Authorization", "Bearer admin-pass")
			resp, err := http.DefaultClient.Do(req)
			Expect(err).NotTo(HaveOccurred())
			// 404 (no route) or the SPA fallback — both prove the admin route isn't bound.
			Expect(resp.StatusCode).To(BeNumerically(">=", 400))
		})
	})
})
