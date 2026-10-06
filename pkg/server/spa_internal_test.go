package server

import (
	"net/http"
	"net/http/httptest"
	"testing/fstest"

	"github.com/labstack/echo/v4"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("serving the web UI", func() {
	get := func(assets fstest.MapFS, path string) *httptest.ResponseRecorder {
		e := echo.New()
		setupSPA(e, assets)
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Accept", "text/html")
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec
	}

	It("serves index.html when the UI is built in", func() {
		rec := get(fstest.MapFS{"dist/index.html": {Data: []byte("<html>ui</html>")}}, "/")
		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Body.String()).To(ContainSubstring("<html>ui</html>"))
	})

	It("serves index.html for HTML requests to unknown SPA routes", func() {
		rec := get(fstest.MapFS{"dist/index.html": {Data: []byte("<html>ui</html>")}}, "/some/spa/route")
		Expect(rec.Code).To(Equal(http.StatusOK))
		Expect(rec.Body.String()).To(ContainSubstring("<html>ui</html>"))
	})

	It("says the binary has no web UI when it was built without one", func() {
		rec := get(fstest.MapFS{}, "/")
		Expect(rec.Code).To(Equal(http.StatusNotFound))
		Expect(rec.Body.String()).To(ContainSubstring("built without the web UI"))
		Expect(rec.Body.String()).To(ContainSubstring("release binaries"))
		Expect(rec.Body.String()).ToNot(ContainSubstring("make"))
	})
})
