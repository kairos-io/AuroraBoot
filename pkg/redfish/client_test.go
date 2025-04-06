package redfish

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redfish/v1/SessionService/Sessions" {
			w.Header().Set("X-Auth-Token", "test-token")
			w.Header().Set("Location", "/redfish/v1/SessionService/Sessions/1")
			w.WriteHeader(http.StatusCreated)
		}
	}))
	defer server.Close()

	// Test client creation
	client, err := NewClient(server.URL, "user", "pass", true, 30*time.Second)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	if client.session == nil {
		t.Error("Session was not created")
	}

	if client.session.Token != "test-token" {
		t.Errorf("Expected token 'test-token', got '%s'", client.session.Token)
	}
}

func TestGetSystemInfo(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redfish/v1/SessionService/Sessions" {
			w.Header().Set("X-Auth-Token", "test-token")
			w.Header().Set("Location", "/redfish/v1/SessionService/Sessions/1")
			w.WriteHeader(http.StatusCreated)
		} else if r.URL.Path == "/redfish/v1/Systems/1" {
			w.Header().Set("X-Auth-Token", "test-token")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{
				"Id": "1",
				"Name": "Test System",
				"Model": "Test Model",
				"Manufacturer": "Test Manufacturer",
				"SerialNumber": "TEST123",
				"MemorySummaryTotalSystemMemoryGiB": 16,
				"ProcessorSummaryCount": 4
			}`))
		}
	}))
	defer server.Close()

	// Create client
	client, err := NewClient(server.URL, "user", "pass", true, 30*time.Second)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Test getting system info
	info, err := client.GetSystemInfo()
	if err != nil {
		t.Fatalf("Failed to get system info: %v", err)
	}

	// Verify system info
	if info.ID != "1" {
		t.Errorf("Expected ID '1', got '%s'", info.ID)
	}
	if info.Name != "Test System" {
		t.Errorf("Expected Name 'Test System', got '%s'", info.Name)
	}
	if info.MemorySize != 16 {
		t.Errorf("Expected MemorySize 16, got %d", info.MemorySize)
	}
	if info.ProcessorCount != 4 {
		t.Errorf("Expected ProcessorCount 4, got %d", info.ProcessorCount)
	}
}

func TestDeployISO(t *testing.T) {
	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redfish/v1/SessionService/Sessions" {
			w.Header().Set("X-Auth-Token", "test-token")
			w.Header().Set("Location", "/redfish/v1/SessionService/Sessions/1")
			w.WriteHeader(http.StatusCreated)
		} else if r.URL.Path == "/redfish/v1/Systems/1/VirtualMedia/1/Upload" {
			w.WriteHeader(http.StatusAccepted)
		} else if r.URL.Path == "/redfish/v1/Systems/1" {
			w.WriteHeader(http.StatusOK)
		} else if r.URL.Path == "/redfish/v1/Systems/1/Actions/ComputerSystem.Reset" {
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	// Create client
	client, err := NewClient(server.URL, "user", "pass", true, 30*time.Second)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Create a temporary ISO file for testing
	tmpFile := "test.iso"
	if err := os.WriteFile(tmpFile, []byte("test iso content"), 0644); err != nil {
		t.Fatalf("Failed to create test ISO: %v", err)
	}
	defer os.Remove(tmpFile)

	// Test ISO deployment
	status, err := client.DeployISO(tmpFile)
	if err != nil {
		t.Fatalf("Failed to deploy ISO: %v", err)
	}

	if status.State != "Started" {
		t.Errorf("Expected state 'Started', got '%s'", status.State)
	}
}
