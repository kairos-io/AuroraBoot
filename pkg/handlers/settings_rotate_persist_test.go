package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/AuroraBoot/pkg/auth"
	"github.com/kairos-io/AuroraBoot/pkg/handlers"
	"github.com/labstack/echo/v4"
)

// Reproduces kairos-io/kairos#5679: when the token file cannot be written,
// rotate still answers 200 and swaps the in-memory token. This test asserts
// that CURRENT (buggy) behavior; it flips once the handler returns 500 and
// keeps the old token.
var _ = Describe("SettingsHandler.RotateRegistrationToken persistence failure", func() {
	It("returns 200 and changes the token although the file cannot be written", func() {
		e := echo.New()
		token := auth.NewRegistrationToken("initial-token")
		missing := filepath.Join(GinkgoT().TempDir(), "no-such-dir", "token")
		handler := handlers.NewSettingsHandler(token, missing)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/settings/registration-token/rotate", nil)
		rec := httptest.NewRecorder()
		Expect(handler.RotateRegistrationToken(e.NewContext(req, rec))).To(Succeed())

		// BUG: should be 500 with the old token kept.
		Expect(rec.Code).To(Equal(http.StatusOK))
		var resp map[string]string
		Expect(json.Unmarshal(rec.Body.Bytes(), &resp)).To(Succeed())
		Expect(resp["registrationToken"]).NotTo(Equal("initial-token"))
		Expect(token.Get()).To(Equal(resp["registrationToken"]))
		Expect(token.Get()).NotTo(Equal("initial-token"))
	})
})
