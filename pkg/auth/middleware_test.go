package auth_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/AuroraBoot/pkg/auth"
	"github.com/kairos-io/AuroraBoot/pkg/store"
	"github.com/labstack/echo/v4"
)

// fakeNodeStore implements store.NodeStore for testing.
type fakeNodeStore struct {
	nodes []*store.ManagedNode
}

func (f *fakeNodeStore) Register(_ context.Context, n *store.ManagedNode) error {
	f.nodes = append(f.nodes, n)
	return nil
}
func (f *fakeNodeStore) GetByID(_ context.Context, id string) (*store.ManagedNode, error) {
	for _, n := range f.nodes {
		if n.ID == id {
			return n, nil
		}
	}
	return nil, fmt.Errorf("not found")
}
func (f *fakeNodeStore) GetByMachineID(_ context.Context, mid string) (*store.ManagedNode, error) {
	for _, n := range f.nodes {
		if n.MachineID == mid {
			return n, nil
		}
	}
	return nil, fmt.Errorf("not found")
}
func (f *fakeNodeStore) GetByAPIKey(_ context.Context, key string) (*store.ManagedNode, error) {
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

var _ = Describe("AdminMiddleware", func() {
	var (
		e          *echo.Echo
		middleware echo.MiddlewareFunc
	)

	BeforeEach(func() {
		e = echo.New()
		middleware = auth.AdminMiddleware("secret-password")
	})

	It("should allow requests with valid Bearer token", func() {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer secret-password")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := middleware(func(c echo.Context) error {
			return c.String(http.StatusOK, "ok")
		})
		err := handler(c)
		Expect(err).NotTo(HaveOccurred())
		Expect(rec.Code).To(Equal(http.StatusOK))
	})

	It("should reject requests with invalid Bearer token", func() {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer wrong-password")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := middleware(func(c echo.Context) error {
			return c.String(http.StatusOK, "ok")
		})
		err := handler(c)
		Expect(err).NotTo(HaveOccurred())
		Expect(rec.Code).To(Equal(http.StatusUnauthorized))
	})

	It("should reject requests with no Authorization header", func() {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := middleware(func(c echo.Context) error {
			return c.String(http.StatusOK, "ok")
		})
		err := handler(c)
		Expect(err).NotTo(HaveOccurred())
		Expect(rec.Code).To(Equal(http.StatusUnauthorized))
	})
})

var _ = Describe("NodeAPIKeyMiddleware", func() {
	var (
		e         *echo.Echo
		nodeStore *fakeNodeStore
	)

	BeforeEach(func() {
		e = echo.New()
		nodeStore = &fakeNodeStore{
			nodes: []*store.ManagedNode{
				{ID: "node-1", APIKey: "valid-api-key"},
			},
		}
	})

	It("should allow requests with a valid API key and set nodeID", func() {
		middleware := auth.NodeAPIKeyMiddleware(nodeStore)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer valid-api-key")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		var capturedNodeID string
		handler := middleware(func(c echo.Context) error {
			capturedNodeID = c.Get(auth.ContextKeyNodeID).(string)
			return c.String(http.StatusOK, "ok")
		})
		err := handler(c)
		Expect(err).NotTo(HaveOccurred())
		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(capturedNodeID).To(Equal("node-1"))
	})

	It("should reject requests with an invalid API key", func() {
		middleware := auth.NodeAPIKeyMiddleware(nodeStore)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer invalid-key")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := middleware(func(c echo.Context) error {
			return c.String(http.StatusOK, "ok")
		})
		err := handler(c)
		Expect(err).NotTo(HaveOccurred())
		Expect(rec.Code).To(Equal(http.StatusUnauthorized))
	})

	It("should reject requests with no Authorization header", func() {
		middleware := auth.NodeAPIKeyMiddleware(nodeStore)
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := middleware(func(c echo.Context) error {
			return c.String(http.StatusOK, "ok")
		})
		err := handler(c)
		Expect(err).NotTo(HaveOccurred())
		Expect(rec.Code).To(Equal(http.StatusUnauthorized))
	})
})

var _ = Describe("RegistrationTokenAuth", func() {
	var (
		e          *echo.Echo
		middleware echo.MiddlewareFunc
	)

	BeforeEach(func() {
		e = echo.New()
		// Pointer so the rotation test below can mutate what the middleware
		// sees on subsequent requests.
		tok := "reg-token-123"
		middleware = auth.RegistrationTokenAuth(&tok)
	})

	It("should allow requests with a valid registration token", func() {
		body := `{"registrationToken":"reg-token-123","hostname":"test-node"}`
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := middleware(func(c echo.Context) error {
			return c.String(http.StatusOK, "ok")
		})
		err := handler(c)
		Expect(err).NotTo(HaveOccurred())
		Expect(rec.Code).To(Equal(http.StatusOK))
	})

	It("should reject requests with an invalid registration token", func() {
		body := `{"registrationToken":"wrong-token","hostname":"test-node"}`
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := middleware(func(c echo.Context) error {
			return c.String(http.StatusOK, "ok")
		})
		err := handler(c)
		Expect(err).NotTo(HaveOccurred())
		Expect(rec.Code).To(Equal(http.StatusUnauthorized))
	})

	It("should reject requests with no registration token", func() {
		body := `{"hostname":"test-node"}`
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := middleware(func(c echo.Context) error {
			return c.String(http.StatusOK, "ok")
		})
		err := handler(c)
		Expect(err).NotTo(HaveOccurred())
		Expect(rec.Code).To(Equal(http.StatusUnauthorized))
	})

	It("should reject requests with invalid JSON", func() {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not json"))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)

		handler := middleware(func(c echo.Context) error {
			return c.String(http.StatusOK, "ok")
		})
		err := handler(c)
		Expect(err).NotTo(HaveOccurred())
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
	})
})
