package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"

	"github.com/kairos-io/AuroraBoot/pkg/auth"
	"github.com/kairos-io/AuroraBoot/pkg/store"
	"github.com/labstack/echo/v4"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// fakeCommandStore is a minimal store.CommandStore for the authorization tests;
// only ListByNode is functional.
type fakeCommandStore struct {
	byNode map[string][]*store.NodeCommand
}

func (f *fakeCommandStore) ListByNode(_ context.Context, nodeID string) ([]*store.NodeCommand, error) {
	return f.byNode[nodeID], nil
}
func (f *fakeCommandStore) Create(context.Context, *store.NodeCommand) error { return nil }
func (f *fakeCommandStore) GetByID(context.Context, string) (*store.NodeCommand, error) {
	return nil, nil
}
func (f *fakeCommandStore) GetPending(context.Context, string) ([]*store.NodeCommand, error) {
	return nil, nil
}
func (f *fakeCommandStore) MarkDelivered(context.Context, []string) error          { return nil }
func (f *fakeCommandStore) ClaimForDelivery(context.Context, string) (bool, error) { return false, nil }
func (f *fakeCommandStore) UpdateStatus(context.Context, string, string, string) error {
	return nil
}
func (f *fakeCommandStore) UpdateStatusForNode(context.Context, string, string, string, string) error {
	return nil
}
func (f *fakeCommandStore) Delete(context.Context, string) error         { return nil }
func (f *fakeCommandStore) DeleteTerminal(context.Context, string) error { return nil }

var _ = Describe("ArtifactImageMiddleware", func() {
	const (
		adminPass = "admin-pass"
		artID     = "art-1"
	)

	var (
		e  *echo.Echo
		ns *fakeNodeStore
		cs *fakeCommandStore
		mw echo.MiddlewareFunc
	)

	BeforeEach(func() {
		e = echo.New()
		ns = &fakeNodeStore{nodes: []*store.ManagedNode{{ID: "node-1", APIKey: "node-1-key"}}}
		cs = &fakeCommandStore{byNode: map[string][]*store.NodeCommand{}}
		mw = auth.ArtifactImageMiddleware(adminPass, ns, cs)
	})

	// do runs GET /api/v1/artifacts/:id/image through the middleware and returns
	// the status code. header sets a Bearer token; query sets ?token=.
	do := func(header, query string) int {
		target := "/api/v1/artifacts/" + artID + "/image"
		if query != "" {
			target += "?token=" + query
		}
		req := httptest.NewRequest(http.MethodGet, target, nil)
		if header != "" {
			req.Header.Set("Authorization", "Bearer "+header)
		}
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id")
		c.SetParamValues(artID)
		handler := mw(func(c echo.Context) error { return c.String(http.StatusOK, "image") })
		_ = handler(c)
		return rec.Code
	}

	assign := func(cmd string, source string) {
		cs.byNode["node-1"] = []*store.NodeCommand{
			{ManagedNodeID: "node-1", Command: cmd, Args: map[string]string{"source": source}},
		}
	}

	It("allows the admin via the Authorization header", func() {
		Expect(do(adminPass, "")).To(Equal(http.StatusOK))
	})

	It("allows the admin via ?token= (UI download links)", func() {
		Expect(do("", adminPass)).To(Equal(http.StatusOK))
	})

	It("401s with no credentials", func() {
		Expect(do("", "")).To(Equal(http.StatusUnauthorized))
	})

	It("401s an unknown token (neither admin nor a node key)", func() {
		Expect(do("bogus", "")).To(Equal(http.StatusUnauthorized))
	})

	It("allows a node assigned this artifact by an upgrade command", func() {
		assign(store.CmdUpgrade, "artifact:"+artID)
		Expect(do("node-1-key", "")).To(Equal(http.StatusOK))
	})

	It("allows a node assigned this artifact by an upgrade-recovery command", func() {
		assign(store.CmdUpgradeRecovery, "artifact:"+artID)
		Expect(do("node-1-key", "")).To(Equal(http.StatusOK))
	})

	It("rejects a node API key supplied via ?token= (node keys must use the Authorization header)", func() {
		// Even when the node IS assigned the artifact, a node key in the URL is not
		// accepted — it would leak through logs/proxies/history. Header only.
		assign(store.CmdUpgrade, "artifact:"+artID)
		Expect(do("", "node-1-key")).To(Equal(http.StatusUnauthorized))
	})

	It("403s a node with no command for this artifact", func() {
		Expect(do("node-1-key", "")).To(Equal(http.StatusForbidden))
	})

	It("403s a node assigned a DIFFERENT artifact", func() {
		assign(store.CmdUpgrade, "artifact:other")
		Expect(do("node-1-key", "")).To(Equal(http.StatusForbidden))
	})

	It("403s a node whose only command naming this artifact is not an upgrade", func() {
		assign("exec", "artifact:"+artID)
		Expect(do("node-1-key", "")).To(Equal(http.StatusForbidden))
	})
})
