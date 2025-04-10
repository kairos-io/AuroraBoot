package redfish

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// HPEClient implements HPE iLO-specific RedFish operations
type HPEClient struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
	session    *Session
}

// NewHPEClient creates a new HPE iLO RedFish client
func NewHPEClient(endpoint, username, password string, verifySSL bool, timeout time.Duration) (*HPEClient, error) {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: !verifySSL,
		},
	}

	client := &HPEClient{
		baseURL:  endpoint,
		username: username,
		password: password,
		httpClient: &http.Client{
			Transport: transport,
			Timeout:   timeout,
		},
	}

	if err := client.authenticate(); err != nil {
		return nil, fmt.Errorf("authentication failed: %w", err)
	}

	return client, nil
}

// authenticate performs HPE iLO-specific RedFish authentication
func (c *HPEClient) authenticate() error {
	// HPE iLO uses a different authentication endpoint
	req, err := http.NewRequest("POST", fmt.Sprintf("%s/redfish/v1/SessionService/Sessions", c.baseURL), nil)
	if err != nil {
		return fmt.Errorf("creating auth request: %w", err)
	}

	req.SetBasicAuth(c.username, c.password)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("auth request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("authentication failed with status: %d", resp.StatusCode)
	}

	c.session = &Session{
		Token:     resp.Header.Get("X-Auth-Token"),
		Location:  resp.Header.Get("Location"),
		ExpiresAt: time.Now().Add(30 * time.Minute),
	}

	return nil
}

// GetSystemInfo retrieves HPE iLO-specific system information
func (c *HPEClient) GetSystemInfo() (*SystemInfo, error) {
	// HPE iLO uses a different path for system information
	req, err := http.NewRequest("GET", fmt.Sprintf("%s/redfish/v1/Systems/1", c.baseURL), nil)
	if err != nil {
		return nil, fmt.Errorf("creating system info request: %w", err)
	}

	req.Header.Set("X-Auth-Token", c.session.Token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("system info request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get system info: %d", resp.StatusCode)
	}

	var sysInfo SystemInfo
	if err := json.NewDecoder(resp.Body).Decode(&sysInfo); err != nil {
		return nil, fmt.Errorf("decoding system info: %w", err)
	}

	return &sysInfo, nil
}

// DeployISO implements HPE iLO-specific ISO deployment
func (c *HPEClient) DeployISO(isoPath string) (*DeploymentStatus, error) {
	// First, upload the ISO to the server's virtual media
	if err := c.uploadISO(isoPath); err != nil {
		return nil, fmt.Errorf("uploading ISO: %w", err)
	}

	// Configure the system to boot from the ISO
	if err := c.configureBoot(); err != nil {
		return nil, fmt.Errorf("configuring boot: %w", err)
	}

	// Start the deployment
	status, err := c.startDeployment()
	if err != nil {
		return nil, fmt.Errorf("starting deployment: %w", err)
	}

	return status, nil
}

// uploadISO uploads the ISO file to HPE iLO's virtual media
func (c *HPEClient) uploadISO(isoPath string) error {
	file, err := os.Open(isoPath)
	if err != nil {
		return fmt.Errorf("opening ISO file: %w", err)
	}
	defer file.Close()

	// HPE iLO uses a different endpoint for virtual media
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/redfish/v1/Systems/1/VirtualMedia/2/Actions/VirtualMedia.InsertMedia", c.baseURL),
		nil)
	if err != nil {
		return fmt.Errorf("creating virtual media request: %w", err)
	}

	req.Header.Set("X-Auth-Token", c.session.Token)
	req.Header.Set("Content-Type", "application/json")

	// HPE iLO requires a specific payload for virtual media
	payload := map[string]interface{}{
		"Image":          isoPath,
		"WriteProtected": true,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshaling virtual media payload: %w", err)
	}

	req.Body = io.NopCloser(bytes.NewBuffer(jsonData))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("virtual media request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("virtual media insertion failed with status: %d", resp.StatusCode)
	}

	return nil
}

// configureBoot configures HPE iLO-specific boot settings
func (c *HPEClient) configureBoot() error {
	// HPE iLO uses a different endpoint for boot configuration
	req, err := http.NewRequest("PATCH",
		fmt.Sprintf("%s/redfish/v1/Systems/1", c.baseURL),
		nil)
	if err != nil {
		return fmt.Errorf("creating boot config request: %w", err)
	}

	// HPE iLO requires a specific payload for boot configuration
	bootConfig := map[string]interface{}{
		"Boot": map[string]interface{}{
			"BootSourceOverrideEnabled": "Once",
			"BootSourceOverrideTarget":  "Cd",
			"BootSourceOverrideMode":    "UEFI",
		},
	}

	jsonData, err := json.Marshal(bootConfig)
	if err != nil {
		return fmt.Errorf("marshaling boot config: %w", err)
	}

	req.Body = io.NopCloser(bytes.NewBuffer(jsonData))
	req.Header.Set("X-Auth-Token", c.session.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("boot config request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("boot configuration failed with status: %d", resp.StatusCode)
	}

	return nil
}

// startDeployment initiates the HPE iLO deployment process
func (c *HPEClient) startDeployment() (*DeploymentStatus, error) {
	// HPE iLO uses a different endpoint for system reset
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/redfish/v1/Systems/1/Actions/ComputerSystem.Reset", c.baseURL),
		nil)
	if err != nil {
		return nil, fmt.Errorf("creating deployment request: %w", err)
	}

	// HPE iLO requires a specific payload for system reset
	resetPayload := map[string]interface{}{
		"ResetType": "ForceRestart",
	}

	jsonData, err := json.Marshal(resetPayload)
	if err != nil {
		return nil, fmt.Errorf("marshaling reset payload: %w", err)
	}

	req.Body = io.NopCloser(bytes.NewBuffer(jsonData))
	req.Header.Set("X-Auth-Token", c.session.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("deployment request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("deployment failed with status: %d", resp.StatusCode)
	}

	return &DeploymentStatus{
		State:     "Started",
		Message:   "Deployment initiated",
		Progress:  0,
		StartTime: time.Now(),
	}, nil
}

// GetDeploymentStatus retrieves the current status of the HPE iLO deployment
func (c *HPEClient) GetDeploymentStatus() (*DeploymentStatus, error) {
	// HPE iLO uses a different endpoint for system status
	req, err := http.NewRequest("GET",
		fmt.Sprintf("%s/redfish/v1/Systems/1", c.baseURL),
		nil)
	if err != nil {
		return nil, fmt.Errorf("creating status request: %w", err)
	}

	req.Header.Set("X-Auth-Token", c.session.Token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("status request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status check failed with status: %d", resp.StatusCode)
	}

	var systemData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&systemData); err != nil {
		return nil, fmt.Errorf("decoding system data: %w", err)
	}

	// Extract power state from HPE iLO response
	powerState, ok := systemData["PowerState"].(string)
	if !ok {
		powerState = "Unknown"
	}

	// Determine deployment status based on power state
	status := &DeploymentStatus{
		State:     "InProgress",
		Message:   "Deployment in progress",
		Progress:  50,
		StartTime: time.Now(),
	}

	if powerState == "On" {
		status.State = "Completed"
		status.Message = "Deployment completed"
		status.Progress = 100
	} else if powerState == "Off" {
		status.State = "Failed"
		status.Message = "System powered off during deployment"
		status.Progress = 0
	}

	return status, nil
}
