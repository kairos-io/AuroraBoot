package handlers_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/AuroraBoot/pkg/handlers"
	"github.com/kairos-io/AuroraBoot/pkg/store"
	"github.com/labstack/echo/v4"
)

var _ = Describe("ArtifactHandler.Upload", func() {
	var (
		e            *echo.Echo
		fb           *fakeBuilder
		as           *fakeArtifactStore
		artifactsDir string
		handler      *handlers.ArtifactHandler
	)

	const (
		buildID = "build-42"
		token   = "abcdef0123456789"
	)

	BeforeEach(func() {
		e = echo.New()
		fb = &fakeBuilder{}
		var err error
		artifactsDir, err = os.MkdirTemp("", "upload-test-")
		Expect(err).NotTo(HaveOccurred())
		as = &fakeArtifactStore{
			records: []*store.ArtifactRecord{
				{ID: buildID, Phase: store.ArtifactBuilding, UploadToken: token},
			},
		}
		handler = handlers.NewArtifactHandler(fb, as, nil, nil, artifactsDir, "reg-token", "http://localhost:8080")
	})

	AfterEach(func() {
		_ = os.RemoveAll(artifactsDir)
	})

	upload := func(id, filename, bearer string, body []byte) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut,
			"/api/v1/artifacts/"+id+"/upload/"+filename, bytes.NewReader(body))
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		rec := httptest.NewRecorder()
		c := e.NewContext(req, rec)
		c.SetParamNames("id", "*")
		c.SetParamValues(id, filename)
		Expect(handler.Upload(c)).To(Succeed())
		return rec
	}

	It("writes the body to artifactsDir/<id>/<filename> and returns 201", func() {
		rec := upload(buildID, "kairos.iso", token, []byte("iso-bytes"))
		Expect(rec.Code).To(Equal(http.StatusCreated))

		got, err := os.ReadFile(filepath.Join(artifactsDir, buildID, "kairos.iso"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(got)).To(Equal("iso-bytes"))
	})

	It("returns 401 when the Authorization header is missing", func() {
		rec := upload(buildID, "kairos.iso", "", []byte("x"))
		Expect(rec.Code).To(Equal(http.StatusUnauthorized))
	})

	It("returns 401 when the token does not match the record", func() {
		rec := upload(buildID, "kairos.iso", "wrong-token", []byte("x"))
		Expect(rec.Code).To(Equal(http.StatusUnauthorized))
	})

	It("returns 404 when the build id has no store record", func() {
		rec := upload("does-not-exist", "kairos.iso", token, []byte("x"))
		Expect(rec.Code).To(Equal(http.StatusNotFound))
	})

	It("rejects filenames that would escape the build directory", func() {
		rec := upload(buildID, "../evil", token, []byte("x"))
		Expect(rec.Code).To(Equal(http.StatusBadRequest))

		rec = upload(buildID, "/etc/passwd", token, []byte("x"))
		Expect(rec.Code).To(Equal(http.StatusBadRequest))
	})

	It("rejects artifact ids that would escape artifactsDir", func() {
		// Ids feed into filesystem paths (buildDir, tmpPath, dst) so any
		// traversal in the URL segment would let a caller mkdir or rename
		// outside artifactsDir. GetByID would reject unknown ids on its own,
		// but validate at the boundary too - defense in depth.
		for _, badID := range []string{"..", "../other", "a/b", `a\b`, "/abs"} {
			rec := upload(badID, "kairos.iso", token, []byte("x"))
			Expect(rec.Code).To(Equal(http.StatusBadRequest),
				"id %q should be rejected before store lookup", badID)
		}
	})

	It("overwrites atomically when the same filename uploads twice", func() {
		Expect(upload(buildID, "kairos.iso", token, []byte("first")).Code).To(Equal(http.StatusCreated))
		Expect(upload(buildID, "kairos.iso", token, []byte("second")).Code).To(Equal(http.StatusCreated))

		got, err := os.ReadFile(filepath.Join(artifactsDir, buildID, "kairos.iso"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(got)).To(Equal("second"))
	})

	It("appends uploaded filenames to the store record without duplicating on retry", func() {
		Expect(upload(buildID, "kairos.iso", token, []byte("a")).Code).To(Equal(http.StatusCreated))
		Expect(upload(buildID, "kairos.raw", token, []byte("b")).Code).To(Equal(http.StatusCreated))
		// Retry of the same file: exporter Job backoff may re-run.
		Expect(upload(buildID, "kairos.iso", token, []byte("a")).Code).To(Equal(http.StatusCreated))

		rec, err := as.GetByID(nil, buildID)
		Expect(err).NotTo(HaveOccurred())
		Expect(rec.ArtifactFiles).To(ConsistOf("kairos.iso", "kairos.raw"))
	})

	It("uses constant-time comparison so a length-mismatch token cannot short-circuit", func() {
		// This mostly documents intent; the security property is that both
		// "wrong" and "too short" fail with the same 401 code and no timing
		// signal reachable from the test harness. Assert both paths respond
		// identically.
		shortRec := upload(buildID, "kairos.iso", "short", []byte("x"))
		wrongRec := upload(buildID, "kairos.iso", strings.Repeat("z", len(token)), []byte("x"))
		Expect(shortRec.Code).To(Equal(http.StatusUnauthorized))
		Expect(wrongRec.Code).To(Equal(http.StatusUnauthorized))
	})
})
