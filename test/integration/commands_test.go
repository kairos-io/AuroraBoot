package integration_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Commands", func() {
	var (
		node1ID, node2ID, node3ID string
		groupID                    string
	)

	BeforeEach(func() {
		// Use a unique group name per test to avoid UNIQUE constraint failures.
		uniqueSuffix := fmt.Sprintf("%d", time.Now().UnixNano())

		// Create a group.
		groupResp := adminPost(testServerURL, "/api/v1/groups", testAdminPassword, map[string]interface{}{
			"name":        "cmd-test-group-" + uniqueSuffix,
			"description": "Group for command tests",
		})
		Expect(groupResp.StatusCode).To(Equal(http.StatusCreated))
		var group map[string]interface{}
		decodeJSON(groupResp, &group)
		groupID = group["id"].(string)

		// Register 3 nodes with unique machine IDs.
		node1ID, _ = registerNode(testServerURL, testRegToken, "machine-cmd-1-"+uniqueSuffix, "host-cmd-1")
		node2ID, _ = registerNode(testServerURL, testRegToken, "machine-cmd-2-"+uniqueSuffix, "host-cmd-2")
		node3ID, _ = registerNode(testServerURL, testRegToken, "machine-cmd-3-"+uniqueSuffix, "host-cmd-3")

		// Assign nodes 1 and 2 to the group.
		resp := adminPut(testServerURL, "/api/v1/nodes/"+node1ID+"/group", testAdminPassword, map[string]string{"groupID": groupID})
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		resp.Body.Close()

		resp = adminPut(testServerURL, "/api/v1/nodes/"+node2ID+"/group", testAdminPassword, map[string]string{"groupID": groupID})
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		resp.Body.Close()

		// Set labels on nodes.
		resp = adminPut(testServerURL, "/api/v1/nodes/"+node1ID+"/labels", testAdminPassword, map[string]interface{}{
			"labels": map[string]string{"env": "prod", "role": "worker"},
		})
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		resp.Body.Close()

		resp = adminPut(testServerURL, "/api/v1/nodes/"+node2ID+"/labels", testAdminPassword, map[string]interface{}{
			"labels": map[string]string{"env": "prod", "role": "master"},
		})
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		resp.Body.Close()

		resp = adminPut(testServerURL, "/api/v1/nodes/"+node3ID+"/labels", testAdminPassword, map[string]interface{}{
			"labels": map[string]string{"env": "staging", "role": "worker"},
		})
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		resp.Body.Close()
	})

	It("should send command to single node", func() {
		cmdBody := map[string]interface{}{
			"command": "exec",
			"args":    map[string]string{"cmd": "uname -a"},
		}
		resp := adminPost(testServerURL, "/api/v1/nodes/"+node1ID+"/commands", testAdminPassword, cmdBody)
		Expect(resp.StatusCode).To(Equal(http.StatusCreated))
		var cmd map[string]interface{}
		decodeJSON(resp, &cmd)
		Expect(cmd["id"]).NotTo(BeEmpty())
		Expect(cmd["command"]).To(Equal("exec"))
		Expect(cmd["managedNodeID"]).To(Equal(node1ID))
		Expect(cmd["phase"]).To(Equal("Pending"))
	})

	It("should send command to all nodes in a group", func() {
		cmdBody := map[string]interface{}{
			"command": "upgrade",
			"args":    map[string]string{"version": "1.0.0"},
		}
		resp := adminPost(testServerURL, "/api/v1/groups/"+groupID+"/commands", testAdminPassword, cmdBody)
		Expect(resp.StatusCode).To(Equal(http.StatusCreated))

		var cmds []map[string]interface{}
		decodeJSON(resp, &cmds)

		// Nodes 1 and 2 are in the group, so we should get 2 commands.
		Expect(cmds).To(HaveLen(2))

		nodeIDs := []string{}
		for _, c := range cmds {
			nodeIDs = append(nodeIDs, c["managedNodeID"].(string))
			Expect(c["command"]).To(Equal("upgrade"))
		}
		Expect(nodeIDs).To(ContainElements(node1ID, node2ID))
	})

	It("should send command by label selector", func() {
		cmdBody := map[string]interface{}{
			"selector": map[string]interface{}{
				"labels": map[string]string{"role": "worker"},
			},
			"command": "reset",
			"args":    map[string]string{},
		}
		resp := adminPost(testServerURL, "/api/v1/nodes/commands", testAdminPassword, cmdBody)
		Expect(resp.StatusCode).To(Equal(http.StatusCreated))

		var cmds []map[string]interface{}
		decodeJSON(resp, &cmds)

		// Nodes 1 and 3 have role=worker.
		Expect(len(cmds)).To(BeNumerically(">=", 2))

		nodeIDs := []string{}
		for _, c := range cmds {
			nodeIDs = append(nodeIDs, c["managedNodeID"].(string))
		}
		Expect(nodeIDs).To(ContainElements(node1ID, node3ID))
	})

	It("should send command by group + label (AND)", func() {
		cmdBody := map[string]interface{}{
			"selector": map[string]interface{}{
				"groupID": groupID,
				"labels":  map[string]string{"role": "worker"},
			},
			"command": "exec",
			"args":    map[string]string{"cmd": "date"},
		}
		resp := adminPost(testServerURL, "/api/v1/nodes/commands", testAdminPassword, cmdBody)
		Expect(resp.StatusCode).To(Equal(http.StatusCreated))

		var cmds []map[string]interface{}
		decodeJSON(resp, &cmds)

		// Only node1 is in group AND has role=worker.
		Expect(cmds).To(HaveLen(1))
		Expect(cmds[0]["managedNodeID"]).To(Equal(node1ID))
	})

	It("should reject command without command field", func() {
		cmdBody := map[string]interface{}{
			"args": map[string]string{"cmd": "uname"},
		}
		resp := adminPost(testServerURL, "/api/v1/nodes/"+node1ID+"/commands", testAdminPassword, cmdBody)
		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		var result map[string]string
		json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		Expect(result["error"]).To(ContainSubstring("command"))
	})
})
