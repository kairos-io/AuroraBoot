package auth_test

import (
	"net/http"
	"net/http/httptest"

	"github.com/kairos-io/AuroraBoot/pkg/auth"
	"github.com/labstack/echo/v4"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Rate limiting", func() {
	var e *echo.Echo

	BeforeEach(func() { e = echo.New() })

	okHandler := func(c echo.Context) error { return c.String(http.StatusOK, "ok") }

	Describe("NodeRateLimiter", func() {
		// fireAsNode sends n requests through the limiter with the given node ID set
		// in context (as the node-auth middleware would have), returning the status
		// codes. The remote address is left at httptest's default so the IP plays no
		// part — the key under test is the node ID. An empty nodeID models an admin
		// request, which sets no node ID.
		fireAsNode := func(mw echo.MiddlewareFunc, nodeID string, n int) []int {
			codes := make([]int, 0, n)
			handler := mw(okHandler)
			for i := 0; i < n; i++ {
				req := httptest.NewRequest(http.MethodGet, "/", nil)
				rec := httptest.NewRecorder()
				c := e.NewContext(req, rec)
				if nodeID != "" {
					c.Set(auth.ContextKeyNodeID, nodeID)
				}
				_ = handler(c)
				codes = append(codes, rec.Code)
			}
			return codes
		}

		It("limits a node once its burst is spent", func() {
			mw := auth.NodeRateLimiter(1, 2) // burst 2, ~1 rps refill (negligible in-test)
			codes := fireAsNode(mw, "node-a", 5)
			Expect(codes[0]).To(Equal(http.StatusOK))
			Expect(codes[1]).To(Equal(http.StatusOK))
			Expect(codes[2]).To(Equal(http.StatusTooManyRequests))
		})

		It("keys per node — one node's burst does not affect another", func() {
			mw := auth.NodeRateLimiter(1, 2)
			_ = fireAsNode(mw, "node-a", 5) // exhaust node-a
			codes := fireAsNode(mw, "node-b", 2)
			Expect(codes).To(Equal([]int{http.StatusOK, http.StatusOK}))
		})

		It("never limits admin requests (no node ID in context)", func() {
			mw := auth.NodeRateLimiter(1, 2)
			codes := fireAsNode(mw, "", 10) // admin: AuthNodeID == ""
			for _, code := range codes {
				Expect(code).To(Equal(http.StatusOK))
			}
		})

		It("passes through when disabled (non-positive rps)", func() {
			mw := auth.NodeRateLimiter(0, 0)
			codes := fireAsNode(mw, "node-a", 50)
			for _, code := range codes {
				Expect(code).To(Equal(http.StatusOK))
			}
		})
	})

	Describe("RegistrationRateLimiter", func() {
		// fireFromIP sends n requests through the limiter from the given client IP,
		// returning the status codes. No node ID is set — registration has no node
		// identity yet, so the key is the client IP.
		fireFromIP := func(mw echo.MiddlewareFunc, ip string, n int) []int {
			codes := make([]int, 0, n)
			handler := mw(okHandler)
			for i := 0; i < n; i++ {
				req := httptest.NewRequest(http.MethodPost, "/", nil)
				req.RemoteAddr = ip + ":12345"
				rec := httptest.NewRecorder()
				c := e.NewContext(req, rec)
				_ = handler(c)
				codes = append(codes, rec.Code)
			}
			return codes
		}

		It("limits an IP once its burst is spent", func() {
			mw := auth.RegistrationRateLimiter(0.5, 2)
			codes := fireFromIP(mw, "203.0.113.5", 5)
			Expect(codes[0]).To(Equal(http.StatusOK))
			Expect(codes[1]).To(Equal(http.StatusOK))
			Expect(codes[2]).To(Equal(http.StatusTooManyRequests))
		})

		It("keys per IP — one IP's flood does not affect another", func() {
			mw := auth.RegistrationRateLimiter(0.5, 2)
			_ = fireFromIP(mw, "203.0.113.5", 5) // exhaust one IP
			codes := fireFromIP(mw, "198.51.100.9", 2)
			Expect(codes).To(Equal([]int{http.StatusOK, http.StatusOK}))
		})

		It("passes through when disabled (non-positive rps)", func() {
			mw := auth.RegistrationRateLimiter(0, 0)
			codes := fireFromIP(mw, "203.0.113.5", 50)
			for _, code := range codes {
				Expect(code).To(Equal(http.StatusOK))
			}
		})
	})
})
