package client_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/AuroraBoot/pkg/client"
)

type receivedPart struct {
	field    string
	filename string
	content  string
}

var _ = Describe("ArtifactsService", func() {
	Describe("UploadOverlay", func() {
		var (
			server      *httptest.Server
			gotMethod   string
			gotPath     string
			gotAuth     string
			gotParts    []receivedPart
			status      int
			replyBody   string
			parseErrMsg string
		)

		BeforeEach(func() {
			gotMethod, gotPath, gotAuth, parseErrMsg = "", "", "", ""
			gotParts = nil
			status = http.StatusOK
			replyBody = `{"id":"6f1c2a0e-3b7d-4c1e-9a52-0d8e4f7b1c3a"}`
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod = r.Method
				gotPath = r.URL.Path
				gotAuth = r.Header.Get("Authorization")
				mr, err := r.MultipartReader()
				if err != nil {
					parseErrMsg = err.Error()
				} else {
					for {
						p, err := mr.NextPart()
						if err == io.EOF {
							break
						}
						if err != nil {
							parseErrMsg = err.Error()
							break
						}
						b, _ := io.ReadAll(p)
						gotParts = append(gotParts, receivedPart{
							field:    p.FormName(),
							filename: p.FileName(),
							content:  string(b),
						})
					}
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				_, _ = io.WriteString(w, replyBody)
			}))
		})

		AfterEach(func() {
			server.Close()
		})

		It("posts every file as a multipart part named files and returns the ID", func() {
			cli := client.New(server.URL, client.WithAdminPassword("s3cret"))

			id, err := cli.Artifacts.UploadOverlay(context.Background(),
				client.OverlayFile{Name: "motd", Content: strings.NewReader("hello\n")},
				client.OverlayFile{Name: "overlay.tar.gz", Content: strings.NewReader("\x1f\x8bbinary\x00data")},
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(id).To(Equal("6f1c2a0e-3b7d-4c1e-9a52-0d8e4f7b1c3a"))
			Expect(parseErrMsg).To(BeEmpty())
			Expect(gotMethod).To(Equal(http.MethodPost))
			Expect(gotPath).To(Equal("/api/v1/artifacts/upload-overlay"))
			Expect(gotAuth).To(Equal("Bearer s3cret"))
			Expect(gotParts).To(Equal([]receivedPart{
				{field: "files", filename: "motd", content: "hello\n"},
				{field: "files", filename: "overlay.tar.gz", content: "\x1f\x8bbinary\x00data"},
			}))
		})

		It("returns an APIError carrying the status for a non-2xx response", func() {
			status = http.StatusBadRequest
			replyBody = `{"error":"invalid multipart form"}`
			cli := client.New(server.URL, client.WithAdminPassword("s3cret"))

			id, err := cli.Artifacts.UploadOverlay(context.Background(),
				client.OverlayFile{Name: "motd", Content: strings.NewReader("hello\n")},
			)

			Expect(id).To(BeEmpty())
			var apiErr *client.APIError
			Expect(err).To(BeAssignableToTypeOf(apiErr))
			apiErr = err.(*client.APIError)
			Expect(apiErr.StatusCode).To(Equal(http.StatusBadRequest))
			Expect(apiErr.ErrorMsg).To(Equal("invalid multipart form"))
		})

		It("refuses to upload without any file", func() {
			cli := client.New(server.URL, client.WithAdminPassword("s3cret"))

			_, err := cli.Artifacts.UploadOverlay(context.Background())

			Expect(err).To(HaveOccurred())
			Expect(gotMethod).To(BeEmpty())
		})
	})

	Describe("CreateArtifactRequest", func() {
		It("sends the overlay by ID and carries no server path fields", func() {
			req := client.CreateArtifactRequest{
				OverlayID: "6f1c2a0e-3b7d-4c1e-9a52-0d8e4f7b1c3a",
				Signing: client.ArtifactSigning{
					UKIKeySetID:         "ks-1",
					UKISecureBootEnroll: "if-safe",
				},
			}

			b, err := json.Marshal(req)
			Expect(err).NotTo(HaveOccurred())

			var body map[string]interface{}
			Expect(json.Unmarshal(b, &body)).To(Succeed())
			Expect(body).To(HaveKeyWithValue("overlayId", "6f1c2a0e-3b7d-4c1e-9a52-0d8e4f7b1c3a"))
			Expect(body).NotTo(HaveKey("overlayRootfs"))
			Expect(body["signing"]).To(Equal(map[string]interface{}{
				"ukiKeySetId":         "ks-1",
				"ukiSecureBootEnroll": "if-safe",
			}))
		})

		It("omits overlayId when no overlay is set", func() {
			b, err := json.Marshal(client.CreateArtifactRequest{})
			Expect(err).NotTo(HaveOccurred())
			Expect(string(b)).NotTo(ContainSubstring("overlayId"))
		})
	})

	Describe("Artifact", func() {
		It("decodes the record's overlayId", func() {
			var a client.Artifact
			Expect(json.Unmarshal([]byte(`{"id":"a1","overlayId":"6f1c2a0e-3b7d-4c1e-9a52-0d8e4f7b1c3a"}`), &a)).To(Succeed())
			Expect(a.OverlayID).To(Equal("6f1c2a0e-3b7d-4c1e-9a52-0d8e4f7b1c3a"))
		})
	})
})
