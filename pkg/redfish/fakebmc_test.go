package redfish_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
)

// fakeBMC is a spec-accurate, in-memory Redfish service used to drive the
// gofish-backed Deployer in tests. gofish talks to it over real HTTP, so every
// document it serves (ServiceRoot, collections, ComputerSystem, VirtualMedia,
// Task) must be valid Redfish/odata. The fake records the requests it received
// so assertions can verify the protocol flow (session body, no hardcoded IDs,
// InsertMedia URL-pull body, boot override PATCH, ResetType, session DELETE).
type fakeBMC struct {
	server *httptest.Server

	mu       sync.Mutex
	requests []recordedRequest

	// behaviour toggles
	insertMediaStatus int  // HTTP status the InsertMedia action returns (default 202)
	mediaOnManager    bool // serve VirtualMedia on the Manager, not the System
	taskStates        []string
	taskIdx           int

	// insertMediaExtendedInfo, when true, makes a failing InsertMedia return a
	// Redfish error body carrying @Message.ExtendedInfo (Message/MessageId/
	// Resolution) so the error-surfacing path can be asserted.
	insertMediaExtendedInfo bool

	// captured bodies
	sessionBody     map[string]any
	insertBody      map[string]any
	bootPatchBody   map[string]any
	resetBody       map[string]any
	sessionLocation string
}

type recordedRequest struct {
	Method string
	Path   string
}

const fakeAuthToken = "fake-x-auth-token-123"

func newFakeBMC() *fakeBMC {
	f := &fakeBMC{
		insertMediaStatus: http.StatusAccepted,
		// Drive the Task to a terminal Completed state on the second poll so the
		// poll loop is genuinely exercised.
		taskStates:      []string{"Running", "Completed"},
		sessionLocation: "/redfish/v1/SessionService/Sessions/sess-abc",
	}
	f.server = httptest.NewTLSServer(http.HandlerFunc(f.handle))
	return f
}

func (f *fakeBMC) Close() { f.server.Close() }

func (f *fakeBMC) URL() string { return f.server.URL }

// record stores the method+path of an incoming request.
func (f *fakeBMC) record(r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, recordedRequest{Method: r.Method, Path: r.URL.Path})
}

// sawRequest reports whether a request with the given method and path prefix was
// received.
func (f *fakeBMC) sawRequest(method, pathPrefix string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, req := range f.requests {
		if req.Method == method && strings.HasPrefix(req.Path, pathPrefix) {
			return true
		}
	}
	return false
}

func decodeBody(r *http.Request) map[string]any {
	b, err := io.ReadAll(r.Body)
	if err != nil || len(b) == 0 {
		return nil
	}
	m := map[string]any{}
	_ = json.Unmarshal(b, &m)
	return m
}

func (f *fakeBMC) handle(w http.ResponseWriter, r *http.Request) {
	f.record(r)
	w.Header().Set("Content-Type", "application/json")

	switch {
	case r.URL.Path == "/redfish/v1/" || r.URL.Path == "/redfish/v1":
		f.writeJSON(w, f.serviceRoot())

	// --- session lifecycle ---
	case r.URL.Path == "/redfish/v1/SessionService/Sessions" && r.Method == http.MethodPost:
		f.mu.Lock()
		f.sessionBody = decodeBody(r)
		f.mu.Unlock()
		w.Header().Set("X-Auth-Token", fakeAuthToken)
		w.Header().Set("Location", f.sessionLocation)
		w.WriteHeader(http.StatusCreated)
		f.writeJSON(w, map[string]any{
			"@odata.id":   f.sessionLocation,
			"@odata.type": "#Session.v1_3_0.Session",
			"Id":          "sess-abc",
			"Name":        "User Session",
			"UserName":    "admin",
		})
	case r.URL.Path == f.sessionLocation && r.Method == http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)

	// --- systems ---
	case r.URL.Path == "/redfish/v1/Systems":
		f.writeJSON(w, f.collection("Systems Collection", "/redfish/v1/Systems/sys-xyz"))
	case r.URL.Path == "/redfish/v1/Systems/sys-xyz" && r.Method == http.MethodGet:
		f.writeJSON(w, f.computerSystem())
	case r.URL.Path == "/redfish/v1/Systems/sys-xyz" && r.Method == http.MethodPatch:
		f.mu.Lock()
		f.bootPatchBody = decodeBody(r)
		f.mu.Unlock()
		w.WriteHeader(http.StatusOK)
		f.writeJSON(w, f.computerSystem())
	case r.URL.Path == "/redfish/v1/Systems/sys-xyz/Actions/ComputerSystem.Reset" && r.Method == http.MethodPost:
		f.mu.Lock()
		f.resetBody = decodeBody(r)
		f.mu.Unlock()
		w.Header().Set("Location", "/redfish/v1/TaskService/Tasks/task-1")
		w.WriteHeader(http.StatusAccepted)
		f.writeJSON(w, f.task("Running"))

	// --- managers ---
	case r.URL.Path == "/redfish/v1/Managers":
		f.writeJSON(w, f.collection("Managers Collection", "/redfish/v1/Managers/mgr-1"))
	case r.URL.Path == "/redfish/v1/Managers/mgr-1" && r.Method == http.MethodGet:
		f.writeJSON(w, f.manager())

	// --- virtual media ---
	case r.URL.Path == "/redfish/v1/Systems/sys-xyz/VirtualMedia":
		f.writeJSON(w, f.collection("VirtualMedia Collection", "/redfish/v1/Systems/sys-xyz/VirtualMedia/Cd"))
	case r.URL.Path == "/redfish/v1/Systems/sys-xyz/VirtualMedia/Cd" && r.Method == http.MethodGet:
		f.writeJSON(w, f.virtualMedia("/redfish/v1/Systems/sys-xyz/VirtualMedia/Cd"))
	case r.URL.Path == "/redfish/v1/Managers/mgr-1/VirtualMedia":
		f.writeJSON(w, f.collection("VirtualMedia Collection", "/redfish/v1/Managers/mgr-1/VirtualMedia/Cd"))
	case r.URL.Path == "/redfish/v1/Managers/mgr-1/VirtualMedia/Cd" && r.Method == http.MethodGet:
		f.writeJSON(w, f.virtualMedia("/redfish/v1/Managers/mgr-1/VirtualMedia/Cd"))
	case strings.HasSuffix(r.URL.Path, "/VirtualMedia/Cd/Actions/VirtualMedia.InsertMedia") && r.Method == http.MethodPost:
		f.mu.Lock()
		f.insertBody = decodeBody(r)
		status := f.insertMediaStatus
		f.mu.Unlock()
		if status >= 400 {
			w.WriteHeader(status)
			errBody := map[string]any{
				"code":    "Base.1.0.GeneralError",
				"message": "InsertMedia failed",
			}
			if f.insertMediaExtendedInfo {
				errBody["@Message.ExtendedInfo"] = []map[string]any{
					{
						"Message":    "The image URL could not be reached by the BMC.",
						"MessageId":  "Base.1.0.ResourceAtUriUnauthorized",
						"Resolution": "Verify the image URL is reachable from the BMC network.",
					},
				}
			}
			f.writeJSON(w, map[string]any{"error": errBody})
			return
		}
		w.Header().Set("Location", "/redfish/v1/TaskService/Tasks/task-insert")
		w.WriteHeader(http.StatusAccepted)
		f.writeJSON(w, f.task("Running"))
	case strings.HasSuffix(r.URL.Path, "/VirtualMedia/Cd/Actions/VirtualMedia.EjectMedia") && r.Method == http.MethodPost:
		w.WriteHeader(http.StatusNoContent)

	// --- tasks ---
	case strings.HasPrefix(r.URL.Path, "/redfish/v1/TaskService/Tasks/") && r.Method == http.MethodGet:
		f.writeJSON(w, f.task(f.nextTaskState()))

	default:
		http.Error(w, "not found: "+r.URL.Path, http.StatusNotFound)
	}
}

func (f *fakeBMC) writeJSON(w http.ResponseWriter, v any) {
	_ = json.NewEncoder(w).Encode(v)
}

// nextTaskState advances the canned Task state machine so the Deployer's poll
// loop observes progress towards a terminal state.
func (f *fakeBMC) nextTaskState() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.taskIdx >= len(f.taskStates) {
		return f.taskStates[len(f.taskStates)-1]
	}
	s := f.taskStates[f.taskIdx]
	f.taskIdx++
	return s
}

func (f *fakeBMC) serviceRoot() map[string]any {
	return map[string]any{
		"@odata.id":      "/redfish/v1/",
		"@odata.type":    "#ServiceRoot.v1_5_0.ServiceRoot",
		"Id":             "RootService",
		"Name":           "Root Service",
		"RedfishVersion": "1.6.0",
		"Systems":        map[string]any{"@odata.id": "/redfish/v1/Systems"},
		"Managers":       map[string]any{"@odata.id": "/redfish/v1/Managers"},
		"Tasks":          map[string]any{"@odata.id": "/redfish/v1/TaskService"},
		"SessionService": map[string]any{"@odata.id": "/redfish/v1/SessionService"},
		"Links": map[string]any{
			"Sessions": map[string]any{"@odata.id": "/redfish/v1/SessionService/Sessions"},
		},
	}
}

func (f *fakeBMC) collection(name, member string) map[string]any {
	return map[string]any{
		"@odata.id":           "/redfish/v1/collection",
		"@odata.type":         "#Collection.Collection",
		"Name":                name,
		"Members@odata.count": 1,
		"Members": []map[string]any{
			{"@odata.id": member},
		},
	}
}

func (f *fakeBMC) computerSystem() map[string]any {
	cs := map[string]any{
		"@odata.id":    "/redfish/v1/Systems/sys-xyz",
		"@odata.type":  "#ComputerSystem.v1_5_0.ComputerSystem",
		"Id":           "sys-xyz",
		"Name":         "Test System",
		"Manufacturer": "ACME",
		"Model":        "ProLiant-Test",
		"SerialNumber": "SN-0001",
		"PowerState":   "On",
		// Nested summaries: the historical bug read these as flat fields and got
		// 0/0. They must be populated from the nested objects.
		"MemorySummary": map[string]any{
			"TotalSystemMemoryGiB": 64,
		},
		"ProcessorSummary": map[string]any{
			"Count": 8,
		},
		"Boot": map[string]any{
			"BootSourceOverrideEnabled": "Disabled",
			"BootSourceOverrideTarget":  "None",
		},
		"Actions": map[string]any{
			"#ComputerSystem.Reset": map[string]any{
				"target": "/redfish/v1/Systems/sys-xyz/Actions/ComputerSystem.Reset",
				"ResetType@Redfish.AllowableValues": []string{
					"On", "ForceOff", "ForceRestart", "GracefulRestart", "GracefulShutdown",
				},
			},
		},
	}
	if !f.mediaOnManager {
		cs["VirtualMedia"] = map[string]any{"@odata.id": "/redfish/v1/Systems/sys-xyz/VirtualMedia"}
	}
	return cs
}

func (f *fakeBMC) manager() map[string]any {
	m := map[string]any{
		"@odata.id":   "/redfish/v1/Managers/mgr-1",
		"@odata.type": "#Manager.v1_3_0.Manager",
		"Id":          "mgr-1",
		"Name":        "Test BMC",
	}
	if f.mediaOnManager {
		m["VirtualMedia"] = map[string]any{"@odata.id": "/redfish/v1/Managers/mgr-1/VirtualMedia"}
	}
	return m
}

func (f *fakeBMC) virtualMedia(id string) map[string]any {
	return map[string]any{
		"@odata.id":      id,
		"@odata.type":    "#VirtualMedia.v1_3_0.VirtualMedia",
		"Id":             "Cd",
		"Name":           "Virtual CD",
		"MediaTypes":     []string{"CD", "DVD"},
		"Image":          nil,
		"Inserted":       false,
		"WriteProtected": true,
		"Actions": map[string]any{
			"#VirtualMedia.InsertMedia": map[string]any{
				"target": id + "/Actions/VirtualMedia.InsertMedia",
			},
			"#VirtualMedia.EjectMedia": map[string]any{
				"target": id + "/Actions/VirtualMedia.EjectMedia",
			},
		},
	}
}

func (f *fakeBMC) task(state string) map[string]any {
	return map[string]any{
		"@odata.id":   "/redfish/v1/TaskService/Tasks/task-1",
		"@odata.type": "#Task.v1_4_0.Task",
		"Id":          "task-1",
		"Name":        "Deploy Task",
		"TaskState":   state,
		"TaskStatus":  "OK",
		"Messages": []map[string]any{
			{"Message": "Operation " + state},
		},
	}
}
