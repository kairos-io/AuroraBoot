package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/AuroraBoot/pkg/handlers"
	"github.com/kairos-io/AuroraBoot/pkg/store"
	"github.com/labstack/echo/v4"
)

// fakeExporter stands in for the docker create/export/import/save pipeline. It
// records how many exports are in flight at once, which is the property the
// per-artifact queue in ExportImage exists to bound.
type fakeExporter struct {
	mu       sync.Mutex
	inFlight int
	maxSeen  int
	calls    int

	// body is written to the response before returning, when non-empty.
	body []byte
	// err is returned after writing body, when non-nil.
	err error
	// hold is how long each export stays "running". It has to be long enough
	// that concurrent callers would visibly overlap if nothing serialized
	// them, otherwise the maxSeen assertion passes on the unqueued code too.
	hold time.Duration
	// block, when non-nil, is waited on while the export is "running", so a
	// test can hold an export open and observe what a second request does.
	block chan struct{}
	// started is closed by the first call, letting a test wait until an
	// export is actually in flight rather than sleeping.
	started chan struct{}
}

func (f *fakeExporter) export(ctx context.Context, containerImage string, w io.Writer) error {
	f.mu.Lock()
	f.calls++
	f.inFlight++
	if f.inFlight > f.maxSeen {
		f.maxSeen = f.inFlight
	}
	if f.started != nil {
		select {
		case <-f.started:
		default:
			close(f.started)
		}
	}
	f.mu.Unlock()

	defer func() {
		f.mu.Lock()
		f.inFlight--
		f.mu.Unlock()
	}()

	if f.hold > 0 {
		select {
		case <-time.After(f.hold):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if len(f.body) > 0 {
		if _, err := w.Write(f.body); err != nil {
			return err
		}
	}
	return f.err
}

func (f *fakeExporter) stats() (calls, maxSeen int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls, f.maxSeen
}

var _ = Describe("ArtifactHandler ExportImage", func() {
	var (
		e   *echo.Echo
		as  *fakeArtifactStore
		rec *store.ArtifactRecord
	)

	BeforeEach(func() {
		e = echo.New()
		rec = &store.ArtifactRecord{ID: "art-1", ContainerImage: "quay.io/kairos/fake:v1"}
		as = &fakeArtifactStore{records: []*store.ArtifactRecord{rec}}
	})

	// call drives GET /api/v1/artifacts/:id/image against the handler.
	call := func(h *handlers.ArtifactHandler, ctx context.Context, id string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/artifacts/"+id+"/image", nil)
		if ctx != nil {
			req = req.WithContext(ctx)
		}
		w := httptest.NewRecorder()
		c := e.NewContext(req, w)
		c.SetParamNames("id")
		c.SetParamValues(id)
		_ = h.ExportImage(c)
		return w
	}

	Describe("concurrent requests for the same artifact", func() {
		It("runs one export at a time and still answers every caller", func() {
			// Each export stays busy for hold, and all callers are released
			// from one barrier, so an unqueued handler would show several
			// exports in flight at once.
			fe := &fakeExporter{body: []byte("flat-tar-bytes"), hold: 60 * time.Millisecond}
			h := handlers.NewArtifactHandler(&fakeBuilder{}, as, nil, nil, "", "reg", "http://localhost:8080").
				WithTestImageExporter(fe.export)

			const callers = 6
			results := make([]*httptest.ResponseRecorder, callers)
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i := 0; i < callers; i++ {
				wg.Add(1)
				go func(i int) {
					defer GinkgoRecover()
					defer wg.Done()
					<-start
					results[i] = call(h, nil, "art-1")
				}(i)
			}
			close(start)
			wg.Wait()

			calls, maxSeen := fe.stats()
			Expect(calls).To(Equal(callers))
			Expect(maxSeen).To(Equal(1), "exports of one artifact must not overlap")

			for i, r := range results {
				Expect(r.Code).To(Equal(http.StatusOK), fmt.Sprintf("caller %d", i))
				Expect(r.Body.String()).To(Equal("flat-tar-bytes"), fmt.Sprintf("caller %d", i))
			}
		})

		It("does not make different artifacts wait on each other", func() {
			other := &store.ArtifactRecord{ID: "art-2", ContainerImage: "quay.io/kairos/fake:v2"}
			as.records = append(as.records, other)

			// Both exports block until released. If the queue were global
			// rather than per artifact, the second would never start and the
			// wait below would time out.
			bothIn := make(chan struct{})
			release := make(chan struct{})
			var mu sync.Mutex
			seen := 0
			exporter := func(ctx context.Context, image string, w io.Writer) error {
				mu.Lock()
				seen++
				if seen == 2 {
					close(bothIn)
				}
				mu.Unlock()
				<-release
				_, err := w.Write([]byte("tar"))
				return err
			}
			h := handlers.NewArtifactHandler(&fakeBuilder{}, as, nil, nil, "", "reg", "http://localhost:8080").
				WithTestImageExporter(exporter)

			var wg sync.WaitGroup
			for _, id := range []string{"art-1", "art-2"} {
				wg.Add(1)
				go func(id string) {
					defer GinkgoRecover()
					defer wg.Done()
					call(h, nil, id)
				}(id)
			}

			Eventually(bothIn, 5*time.Second).Should(BeClosed())
			close(release)
			wg.Wait()
		})

		It("releases the per-artifact lock so the map does not grow", func() {
			fe := &fakeExporter{body: []byte("tar")}
			h := handlers.NewArtifactHandler(&fakeBuilder{}, as, nil, nil, "", "reg", "http://localhost:8080").
				WithTestImageExporter(fe.export)

			for i := 0; i < 3; i++ {
				Expect(call(h, nil, "art-1").Code).To(Equal(http.StatusOK))
			}
			Expect(h.ExportLockCountForTest()).To(Equal(0))
		})

		It("gives up with a retryable status when the caller disconnects while queued", func() {
			fe := &fakeExporter{body: []byte("tar"), block: make(chan struct{}), started: make(chan struct{})}
			h := handlers.NewArtifactHandler(&fakeBuilder{}, as, nil, nil, "", "reg", "http://localhost:8080").
				WithTestImageExporter(fe.export)

			holder := make(chan struct{})
			go func() {
				defer GinkgoRecover()
				defer close(holder)
				call(h, nil, "art-1")
			}()
			Eventually(fe.started, 5*time.Second).Should(BeClosed())

			ctx, cancel := context.WithCancel(context.Background())
			queued := make(chan *httptest.ResponseRecorder, 1)
			go func() {
				defer GinkgoRecover()
				queued <- call(h, ctx, "art-1")
			}()

			// The second caller is behind the first in the queue. Hang up.
			time.Sleep(100 * time.Millisecond)
			cancel()

			var w *httptest.ResponseRecorder
			Eventually(queued, 5*time.Second).Should(Receive(&w))
			Expect(w.Code).To(BeNumerically(">=", http.StatusInternalServerError),
				"a queue give-up must look retryable to the agent, not like a permanent 4xx")

			close(fe.block)
			Eventually(holder, 5*time.Second).Should(BeClosed())

			calls, _ := fe.stats()
			Expect(calls).To(Equal(1), "the request that hung up must not start an export")
		})
	})

	Describe("failure before any output", func() {
		It("answers 500 with a JSON error instead of an empty 200", func() {
			fe := &fakeExporter{err: fmt.Errorf("docker import failed: exit status 1")}
			h := handlers.NewArtifactHandler(&fakeBuilder{}, as, nil, nil, "", "reg", "http://localhost:8080").
				WithTestImageExporter(fe.export)

			w := call(h, nil, "art-1")
			Expect(w.Code).To(Equal(http.StatusInternalServerError))
			Expect(w.Header().Get("Content-Disposition")).To(BeEmpty(),
				"download headers must not be committed for a failed export")

			var body map[string]string
			Expect(json.Unmarshal(w.Body.Bytes(), &body)).To(Succeed())
			Expect(body["error"]).To(ContainSubstring("docker import failed"))
		})
	})

	Describe("successful export", func() {
		It("sets the download headers and streams the tar", func() {
			fe := &fakeExporter{body: []byte("tar-bytes")}
			h := handlers.NewArtifactHandler(&fakeBuilder{}, as, nil, nil, "", "reg", "http://localhost:8080").
				WithTestImageExporter(fe.export)

			w := call(h, nil, "art-1")
			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(w.Header().Get("Content-Type")).To(Equal("application/octet-stream"))
			Expect(w.Header().Get("Content-Disposition")).To(Equal("attachment; filename=art-1.tar"))
			Expect(w.Body.String()).To(Equal("tar-bytes"))
		})
	})

	Describe("an export that succeeds without producing bytes", func() {
		It("still answers 200 with the download headers", func() {
			fe := &fakeExporter{}
			h := handlers.NewArtifactHandler(&fakeBuilder{}, as, nil, nil, "", "reg", "http://localhost:8080").
				WithTestImageExporter(fe.export)

			w := call(h, nil, "art-1")
			Expect(w.Code).To(Equal(http.StatusOK))
			Expect(w.Header().Get("Content-Disposition")).To(Equal("attachment; filename=art-1.tar"))
			Expect(w.Body.Len()).To(Equal(0))
		})
	})

	Describe("lookup failures", func() {
		It("404s an unknown artifact without touching docker", func() {
			fe := &fakeExporter{body: []byte("tar")}
			h := handlers.NewArtifactHandler(&fakeBuilder{}, as, nil, nil, "", "reg", "http://localhost:8080").
				WithTestImageExporter(fe.export)

			Expect(call(h, nil, "nope").Code).To(Equal(http.StatusNotFound))
			calls, _ := fe.stats()
			Expect(calls).To(Equal(0))
		})

		It("404s an artifact that has no container image", func() {
			as.records = append(as.records, &store.ArtifactRecord{ID: "art-iso-only"})
			fe := &fakeExporter{body: []byte("tar")}
			h := handlers.NewArtifactHandler(&fakeBuilder{}, as, nil, nil, "", "reg", "http://localhost:8080").
				WithTestImageExporter(fe.export)

			Expect(call(h, nil, "art-iso-only").Code).To(Equal(http.StatusNotFound))
			calls, _ := fe.stats()
			Expect(calls).To(Equal(0))
		})
	})

	Describe("docker object names", func() {
		// The reported HTTP 500 storm came from `docker create --name
		// auroraboot-export-<id>`: the name was a constant per artifact, so
		// the second concurrent node hit "container name already in use".
		It("are unique per export and valid for docker", func() {
			validName := regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)
			validTag := regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*:[a-zA-Z0-9_][a-zA-Z0-9._-]{0,127}$`)

			seen := map[string]bool{}
			for i := 0; i < 32; i++ {
				cid, tag, err := handlers.ExportObjectNamesForTest()
				Expect(err).NotTo(HaveOccurred())
				Expect(validName.MatchString(cid)).To(BeTrue(), cid)
				Expect(validTag.MatchString(tag)).To(BeTrue(), tag)
				Expect(seen[cid]).To(BeFalse(), "container name repeated: "+cid)
				Expect(seen[tag]).To(BeFalse(), "image tag repeated: "+tag)
				seen[cid] = true
				seen[tag] = true
			}
		})
	})
})
