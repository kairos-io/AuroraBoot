package worker_test

import (
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/kairos-io/AuroraBoot/internal/web"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/phayes/freeport"
)

var (
	serverURL   string
	artifactDir string
)

func TestWorker(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Worker Suite")
}

var _ = BeforeSuite(func() {
	// Create a temporary directory for artifacts
	var err error
	artifactDir, err = os.MkdirTemp("", "auroraboot-test-*")
	Expect(err).NotTo(HaveOccurred())

	// Get a free port
	port, err := freeport.GetFreePort()
	Expect(err).NotTo(HaveOccurred())

	// Start the web server
	serverURL = fmt.Sprintf("http://localhost:%d", port)
	go func() {
		err := web.App(web.AppConfig{
			EnableLogger: false,
			ListenAddr:   fmt.Sprintf(":%d", port),
			OutDir:       artifactDir,
			BuildsDir:    artifactDir,
		})
		Expect(err).NotTo(HaveOccurred())
	}()

	// Wait for server to be ready
	Eventually(func() error {
		resp, err := http.Get(serverURL + "/api/v1/builds")
		if err != nil {
			return err
		}
		resp.Body.Close()
		return nil
	}, "5s", "100ms").Should(Succeed())
})

var _ = AfterSuite(func() {
	// Clean up the temporary directory
	os.RemoveAll(artifactDir)
})
