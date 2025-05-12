package web

import (
	"fmt"
	"net/http"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/phayes/freeport"
)

var (
	serverURL string
	port      int
)

func TestWeb(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Web Suite")
}

var _ = BeforeSuite(func() {
	// Create temporary directories for artifacts and logs
	var err error
	artifactDir, err = os.MkdirTemp("", "auroraboot-test-artifacts-*")
	Expect(err).NotTo(HaveOccurred())

	logsDir, err = os.MkdirTemp("", "auroraboot-test-logs-*")
	Expect(err).NotTo(HaveOccurred())

	// Get a free port
	port, err = freeport.GetFreePort()
	Expect(err).NotTo(HaveOccurred())

	// Start the web server
	serverURL = fmt.Sprintf("http://localhost:%d", port)
	go func() {
		err := App(fmt.Sprintf(":%d", port), artifactDir, logsDir, AppConfig{EnableLogger: false})
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
	// Clean up the temporary directories
	os.RemoveAll(artifactDir)
	os.RemoveAll(logsDir)
})
