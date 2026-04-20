package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/AuroraBoot/pkg/handlers"
	"github.com/labstack/echo/v4"
)

var _ = Describe("SettingsHandler", func() {
	var (
		e        *echo.Echo
		token    string
		handler  *handlers.SettingsHandler
	)

	BeforeEach(func() {
		e = echo.New()
		token = "initial-token"
		handler = handlers.NewSettingsHandler(&token, "")
	})

	Describe("GetRegistrationToken", func() {
		It("should return the current token", func() {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/settings/registration-token", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := handler.GetRegistrationToken(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusOK))

			var resp map[string]string
			Expect(json.Unmarshal(rec.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp["registrationToken"]).To(Equal("initial-token"))
		})
	})

	Describe("RotateRegistrationToken", func() {
		It("should rotate the token", func() {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/settings/registration-token/rotate", nil)
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)

			err := handler.RotateRegistrationToken(c)
			Expect(err).NotTo(HaveOccurred())
			Expect(rec.Code).To(Equal(http.StatusOK))

			var resp map[string]string
			Expect(json.Unmarshal(rec.Body.Bytes(), &resp)).To(Succeed())
			Expect(resp["registrationToken"]).NotTo(Equal("initial-token"))
			Expect(resp["registrationToken"]).NotTo(BeEmpty())
			// Token should also be updated in the pointer
			Expect(token).To(Equal(resp["registrationToken"]))
		})
	})
})
