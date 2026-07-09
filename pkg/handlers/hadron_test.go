package handlers_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/AuroraBoot/internal/secrets"
	"github.com/kairos-io/AuroraBoot/pkg/hadron"
	"github.com/kairos-io/AuroraBoot/pkg/handlers"
	"github.com/labstack/echo/v4"
)

var _ = Describe("HadronHandler", func() {
	var (
		e        *echo.Echo
		catalog  *hadron.Catalog
		settings *fakeSettingsStore
		cipher   *secrets.Cipher
		h        *handlers.HadronHandler
	)

	newCipher := func() *secrets.Cipher {
		key := make([]byte, 32)
		_, err := rand.Read(key)
		Expect(err).NotTo(HaveOccurred())
		c, err := secrets.NewCipher(key)
		Expect(err).NotTo(HaveOccurred())
		return c
	}

	BeforeEach(func() {
		e = echo.New()
		catalog = hadron.NewCatalog()
		settings = newFakeSettingsStore()
		cipher = newCipher()
		h = handlers.NewHadronHandler(catalog, settings, cipher)
	})

	do := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		switch method + " " + path {
		case "GET /api/v1/hadron/registry-credentials":
			Expect(h.ListRegistryCredentials(c)).To(Succeed())
		case "PUT /api/v1/hadron/registry-credentials":
			Expect(h.PutRegistryCredentials(c)).To(Succeed())
		case "GET /api/v1/hadron/base-versions":
			Expect(h.GetBaseVersions(c)).To(Succeed())
		}
		return rec
	}

	Describe("Registry credentials", func() {
		It("returns [] when no credentials are stored", func() {
			rec := do(http.MethodGet, "/api/v1/hadron/registry-credentials", "")
			Expect(rec.Code).To(Equal(http.StatusOK))
			var out []map[string]any
			Expect(json.Unmarshal(rec.Body.Bytes(), &out)).To(Succeed())
			Expect(out).To(BeEmpty())
		})

		It("persists credentials with an encrypted password and never returns plaintext", func() {
			body := `[{"registry":"registry.example.com","username":"alice","password":"s3cret"}]`
			rec := do(http.MethodPut, "/api/v1/hadron/registry-credentials", body)
			Expect(rec.Code).To(Equal(http.StatusOK))
			var out []map[string]any
			Expect(json.Unmarshal(rec.Body.Bytes(), &out)).To(Succeed())
			Expect(out).To(HaveLen(1))
			Expect(out[0]["registry"]).To(Equal("registry.example.com"))
			Expect(out[0]["username"]).To(Equal("alice"))
			Expect(out[0]["hasPassword"]).To(BeTrue())
			// Password never leaks to the wire.
			Expect(out[0]).NotTo(HaveKey("password"))

			// Underlying setting is encrypted (not equal to plaintext).
			raw, found, _ := settings.Get(context.Background(), handlers.SettingHadronRegistryCreds)
			Expect(found).To(BeTrue())
			Expect(raw).NotTo(ContainSubstring("s3cret"))
		})

		It("rejects an entry missing registry or username", func() {
			body := `[{"registry":"registry.example.com","username":"","password":"x"}]`
			req := httptest.NewRequest(http.MethodPut, "/api/v1/hadron/registry-credentials", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			c := e.NewContext(req, rec)
			Expect(h.PutRegistryCredentials(c)).To(Succeed())
			Expect(rec.Code).To(Equal(http.StatusBadRequest))
		})

		It("keepPassword preserves the existing encrypted value on unrelated edits", func() {
			// Seed.
			rec := do(http.MethodPut, "/api/v1/hadron/registry-credentials", `[{"registry":"registry.example.com","username":"alice","password":"s3cret"}]`)
			Expect(rec.Code).To(Equal(http.StatusOK))
			before, _, _ := settings.Get(context.Background(), handlers.SettingHadronRegistryCreds)

			// Same (registry, username) tuple, keepPassword set — server must
			// carry over the previously-encrypted ciphertext byte-identically
			// (no re-encrypt on a no-password edit).
			body := `[{"registry":"registry.example.com","username":"alice","keepPassword":true}]`
			rec = do(http.MethodPut, "/api/v1/hadron/registry-credentials", body)
			Expect(rec.Code).To(Equal(http.StatusOK))
			after, _, _ := settings.Get(context.Background(), handlers.SettingHadronRegistryCreds)
			// Ciphertext byte-identical → keepPassword worked without a re-encrypt.
			Expect(after).To(Equal(before))
		})

		It("AuthProvider decrypts and returns credentials", func() {
			body := `[
				{"registry":"registry.example.com","username":"alice","password":"s3cret"},
				{"registry":"other.example.com","username":"bob","password":""}
			]`
			rec := do(http.MethodPut, "/api/v1/hadron/registry-credentials", body)
			Expect(rec.Code).To(Equal(http.StatusOK))

			creds, err := h.AuthProvider()(context.Background())
			Expect(err).NotTo(HaveOccurred())
			Expect(creds).To(HaveLen(2))
			// Alice returns the decrypted secret; Bob has no password.
			var alice, bob hadron.RegistryCredential
			for _, c := range creds {
				switch c.Username {
				case "alice":
					alice = c
				case "bob":
					bob = c
				}
			}
			Expect(alice.Password).To(Equal("s3cret"))
			Expect(bob.Password).To(BeEmpty())
		})

		It("AuthProvider fails closed on a corrupted ciphertext", func() {
			// Store a garbage payload under the setting.
			settings.Set(context.Background(), handlers.SettingHadronRegistryCreds, `[{"registry":"r","username":"u","password":"!!!not-base64!!!"}]`)
			creds, err := h.AuthProvider()(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(creds).To(BeNil())
		})
	})
})
