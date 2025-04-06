package redfish

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// DeploymentStatus represents the status of an ISO deployment
type DeploymentStatus struct {
	State     string    `json:"State"`
	Message   string    `json:"Message"`
	Progress  int       `json:"Progress"`
	StartTime time.Time `json:"StartTime"`
	EndTime   time.Time `json:"EndTime,omitempty"`
}

// DeployISO deploys an ISO image to the target system
func (c *Client) DeployISO(isoPath string) (*DeploymentStatus, error) {
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

// uploadISO uploads the ISO file to the server's temporary storage
func (c *Client) uploadISO(isoPath string) error {
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

// configureBoot configures the system to boot from the uploaded ISO
func (c *Client) configureBoot() error {
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

// startDeployment initiates the deployment process
func (c *Client) startDeployment() (*DeploymentStatus, error) {
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

// GetDeploymentStatus retrieves the current status of the deployment
func (c *Client) GetDeploymentStatus() (*DeploymentStatus, error) {
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
