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

// SuperMicroClient implements vendor-specific RedFish operations for SuperMicro hardware
type SuperMicroClient struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
	session    *Session
}

// NewSuperMicroClient creates a new SuperMicro RedFish client
func NewSuperMicroClient(endpoint, username, password string, verifySSL bool, timeout time.Duration) (*SuperMicroClient, error) {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: !verifySSL,
		},
	}

	client := &SuperMicroClient{
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

// authenticate performs SuperMicro-specific RedFish authentication
func (c *SuperMicroClient) authenticate() error {
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

// GetSystemInfo retrieves SuperMicro-specific system information
func (c *SuperMicroClient) GetSystemInfo() (*SystemInfo, error) {
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

// DeployISO implements SuperMicro-specific ISO deployment
func (c *SuperMicroClient) DeployISO(isoPath string) (*DeploymentStatus, error) {
	// First, upload the ISO to the server's temporary storage
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

// uploadISO uploads the ISO file to SuperMicro's temporary storage
func (c *SuperMicroClient) uploadISO(isoPath string) error {
	file, err := os.Open(isoPath)
	if err != nil {
		return fmt.Errorf("opening ISO file: %w", err)
	}
	defer file.Close()

	// Create multipart form data
	body := &bytes.Buffer{}
	writer := io.MultiWriter(body)

	if _, err := io.Copy(writer, file); err != nil {
		return fmt.Errorf("copying ISO data: %w", err)
	}

	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/redfish/v1/Systems/1/VirtualMedia/1/Upload", c.baseURL),
		body)
	if err != nil {
		return fmt.Errorf("creating upload request: %w", err)
	}

	req.Header.Set("X-Auth-Token", c.session.Token)
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("upload request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("upload failed with status: %d", resp.StatusCode)
	}

	return nil
}

// configureBoot configures SuperMicro-specific boot settings
func (c *SuperMicroClient) configureBoot() error {
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

	req, err := http.NewRequest("PATCH",
		fmt.Sprintf("%s/redfish/v1/Systems/1", c.baseURL),
		bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("creating boot config request: %w", err)
	}

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

// startDeployment initiates the SuperMicro deployment process
func (c *SuperMicroClient) startDeployment() (*DeploymentStatus, error) {
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/redfish/v1/Systems/1/Actions/ComputerSystem.Reset", c.baseURL),
		nil)
	if err != nil {
		return nil, fmt.Errorf("creating deployment request: %w", err)
	}

	req.Header.Set("X-Auth-Token", c.session.Token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("deployment request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("deployment failed with status: %d", resp.StatusCode)
	}

	return &DeploymentStatus{
		State:     "Started",
		Message:   "Deployment initiated",
		Progress:  0,
		StartTime: time.Now(),
	}, nil
}

// GetDeploymentStatus retrieves the current status of the SuperMicro deployment
func (c *SuperMicroClient) GetDeploymentStatus() (*DeploymentStatus, error) {
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

	var status DeploymentStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, fmt.Errorf("decoding status: %w", err)
	}

	return &status, nil
}
