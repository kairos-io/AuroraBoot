package client_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kairos-io/AuroraBoot/pkg/client"
)

var _ = Describe("GroupsService", func() {
	Describe("SendCommand", func() {
		var (
			server  *httptest.Server
			gotPath string
			gotBody map[string]any
			cli     *client.Client
		)

		BeforeEach(func() {
			gotPath, gotBody = "", nil
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				raw, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(raw, &gotBody)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`[{"id":"c1","managedNodeID":"n1","command":"upgrade","phase":"Pending","batchID":"b1","failFast":true}]`))
			}))
			DeferCleanup(server.Close)
			cli = client.New(server.URL, client.WithAdminPassword("s3cret"))
		})

		It("asks the group endpoint for a fail-fast fan-out", func() {
			cmds, err := cli.Groups.SendCommand(context.Background(), "grp-1", client.GroupCommandRequest{
				Command:  "upgrade",
				Args:     map[string]string{"source": "oci:quay.io/kairos/opensuse:v4"},
				FailFast: true,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(gotPath).To(Equal("/api/v1/groups/grp-1/commands"))
			Expect(gotBody).To(HaveKeyWithValue("command", "upgrade"))
			Expect(gotBody).To(HaveKeyWithValue("failFast", true))
			Expect(gotBody).To(HaveKey("args"))
			Expect(cmds).To(HaveLen(1))
			Expect(cmds[0].FailFast).To(BeTrue())
		})

		It("leaves failFast out of the body when it is not asked for", func() {
			_, err := cli.Groups.SendCommand(context.Background(), "grp-1", client.GroupCommandRequest{Command: "reboot"})
			Expect(err).NotTo(HaveOccurred())
			Expect(gotBody).NotTo(HaveKey("failFast"))
		})
	})
})
