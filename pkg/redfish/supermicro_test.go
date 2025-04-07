package redfish

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSuperMicroClient(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock authentication endpoint
		if r.URL.Path == "/redfish/v1/SessionService/Sessions" && r.Method == "POST" {
			w.Header().Set("X-Auth-Token", "test-token")
			w.Header().Set("Location", "/redfish/v1/SessionService/Sessions/1")
			w.WriteHeader(http.StatusCreated)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Test client creation
	client, err := NewSuperMicroClient(server.URL, "admin", "password", true, 10*time.Second)
	require.NoError(t, err)
	assert.NotNil(t, client)
	assert.Equal(t, server.URL, client.baseURL)
	assert.Equal(t, "admin", client.username)
	assert.Equal(t, "password", client.password)
	assert.NotNil(t, client.httpClient)
	assert.NotNil(t, client.session)
	assert.Equal(t, "test-token", client.session.Token)
}

func TestSuperMicroClient_GetSystemInfo(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock authentication endpoint
		if r.URL.Path == "/redfish/v1/SessionService/Sessions" && r.Method == "POST" {
			w.Header().Set("X-Auth-Token", "test-token")
			w.Header().Set("Location", "/redfish/v1/SessionService/Sessions/1")
			w.WriteHeader(http.StatusCreated)
			return
		}

		// Mock system info endpoint
		if r.URL.Path == "/redfish/v1/Systems/1" && r.Method == "GET" {
			// Check for auth token
			if r.Header.Get("X-Auth-Token") != "test-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			// Return mock system info
			sysInfo := SystemInfo{
				ID:             "1",
				Name:           "Test System",
				Model:          "X11DPi-NT",
				Manufacturer:   "SuperMicro",
				SerialNumber:   "123456789",
				MemorySize:     128,
				ProcessorCount: 24,
			}
			json.NewEncoder(w).Encode(sysInfo)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Create client
	client, err := NewSuperMicroClient(server.URL, "admin", "password", true, 10*time.Second)
	require.NoError(t, err)

	// Test GetSystemInfo
	sysInfo, err := client.GetSystemInfo()
	require.NoError(t, err)
	assert.NotNil(t, sysInfo)
	assert.Equal(t, "SuperMicro", sysInfo.Manufacturer)
	assert.Equal(t, "X11DPi-NT", sysInfo.Model)
	assert.Equal(t, "123456789", sysInfo.SerialNumber)
	assert.Equal(t, 128, sysInfo.MemorySize)
	assert.Equal(t, 24, sysInfo.ProcessorCount)
}

func TestSuperMicroClient_DeployISO(t *testing.T) {
	// Create a temporary ISO file
	tempDir, err := os.MkdirTemp("", "supermicro-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	isoPath := filepath.Join(tempDir, "test.iso")
	err = os.WriteFile(isoPath, []byte("test iso content"), 0644)
	require.NoError(t, err)

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock authentication endpoint
		if r.URL.Path == "/redfish/v1/SessionService/Sessions" && r.Method == "POST" {
			w.Header().Set("X-Auth-Token", "test-token")
			w.Header().Set("Location", "/redfish/v1/SessionService/Sessions/1")
			w.WriteHeader(http.StatusCreated)
			return
		}

		// Mock virtual media endpoint
		if r.URL.Path == "/redfish/v1/Systems/1/VirtualMedia/2/Actions/VirtualMedia.InsertMedia" && r.Method == "POST" {
			// Check for auth token
			if r.Header.Get("X-Auth-Token") != "test-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			// Parse request body
			var payload map[string]interface{}
			err := json.NewDecoder(r.Body).Decode(&payload)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			// Check payload
			if payload["Image"] == nil || payload["WriteProtected"] == nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			w.WriteHeader(http.StatusOK)
			return
		}

		// Mock boot configuration endpoint
		if r.URL.Path == "/redfish/v1/Systems/1" && r.Method == "PATCH" {
			// Check for auth token
			if r.Header.Get("X-Auth-Token") != "test-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			// Parse request body
			var bootConfig map[string]interface{}
			err := json.NewDecoder(r.Body).Decode(&bootConfig)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			// Check boot config
			boot, ok := bootConfig["Boot"].(map[string]interface{})
			if !ok || boot["BootSourceOverrideEnabled"] != "Once" ||
				boot["BootSourceOverrideTarget"] != "Cd" ||
				boot["BootSourceOverrideMode"] != "UEFI" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			w.WriteHeader(http.StatusOK)
			return
		}

		// Mock system reset endpoint
		if r.URL.Path == "/redfish/v1/Systems/1/Actions/ComputerSystem.Reset" && r.Method == "POST" {
			// Check for auth token
			if r.Header.Get("X-Auth-Token") != "test-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			// Parse request body
			var resetPayload map[string]interface{}
			err := json.NewDecoder(r.Body).Decode(&resetPayload)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			// Check reset type
			if resetPayload["ResetType"] != "ForceRestart" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}

			w.WriteHeader(http.StatusOK)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Create client
	client, err := NewSuperMicroClient(server.URL, "admin", "password", true, 10*time.Second)
	require.NoError(t, err)

	// Test DeployISO
	status, err := client.DeployISO(isoPath)
	require.NoError(t, err)
	assert.NotNil(t, status)
	assert.Equal(t, "Started", status.State)
	assert.Equal(t, "Deployment initiated", status.Message)
	assert.Equal(t, float64(0), status.Progress)
}

func TestSuperMicroClient_GetDeploymentStatus(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock authentication endpoint
		if r.URL.Path == "/redfish/v1/SessionService/Sessions" && r.Method == "POST" {
			w.Header().Set("X-Auth-Token", "test-token")
			w.Header().Set("Location", "/redfish/v1/SessionService/Sessions/1")
			w.WriteHeader(http.StatusCreated)
			return
		}

		// Mock system status endpoint
		if r.URL.Path == "/redfish/v1/Systems/1" && r.Method == "GET" {
			// Check for auth token
			if r.Header.Get("X-Auth-Token") != "test-token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			// Return mock system data with different power states
			powerState := r.URL.Query().Get("powerState")
			if powerState == "" {
				powerState = "On" // Default to On
			}

			systemData := map[string]interface{}{
				"PowerState": powerState,
			}
			json.NewEncoder(w).Encode(systemData)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Test GetDeploymentStatus with different power states
	testCases := []struct {
		name         string
		powerState   string
		wantState    string
		wantMsg      string
		wantProgress float64
	}{
		{
			name:         "Power On",
			powerState:   "On",
			wantState:    "Completed",
			wantMsg:      "Deployment completed",
			wantProgress: 100,
		},
		{
			name:         "Power Off",
			powerState:   "Off",
			wantState:    "Failed",
			wantMsg:      "System powered off during deployment",
			wantProgress: 0,
		},
		{
			name:         "Unknown Power State",
			powerState:   "Unknown",
			wantState:    "InProgress",
			wantMsg:      "Deployment in progress",
			wantProgress: 50,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a new server for each test case
			statusServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/redfish/v1/SessionService/Sessions" && r.Method == "POST" {
					w.Header().Set("X-Auth-Token", "test-token")
					w.Header().Set("Location", "/redfish/v1/SessionService/Sessions/1")
					w.WriteHeader(http.StatusCreated)
					return
				}

				if r.URL.Path == "/redfish/v1/Systems/1" && r.Method == "GET" {
					systemData := map[string]interface{}{
						"PowerState": tc.powerState,
					}
					json.NewEncoder(w).Encode(systemData)
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			defer statusServer.Close()

			// Create client
			statusClient, err := NewSuperMicroClient(statusServer.URL, "admin", "password", true, 10*time.Second)
			require.NoError(t, err)

			// Test GetDeploymentStatus
			status, err := statusClient.GetDeploymentStatus()
			require.NoError(t, err)
			assert.NotNil(t, status)
			assert.Equal(t, tc.wantState, status.State)
			assert.Equal(t, tc.wantMsg, status.Message)
			assert.Equal(t, tc.wantProgress, status.Progress)
		})
	}
}
