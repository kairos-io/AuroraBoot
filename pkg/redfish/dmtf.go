package redfish

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// DMTFClient implements DMTF RedFish operations
type DMTFClient struct {
	*GenericClient
}

// NewDMTFClient creates a new DMTF RedFish client
func NewDMTFClient(endpoint, username, password string, verifySSL bool, timeout time.Duration) (*DMTFClient, error) {
	genericClient, err := NewGenericClient(endpoint, username, password, verifySSL, timeout)
	if err != nil {
		return nil, fmt.Errorf("creating generic client: %w", err)
	}

	return &DMTFClient{
		GenericClient: genericClient,
	}, nil
}

// GetDeploymentStatus retrieves the current status of the DMTF deployment
func (c *DMTFClient) GetDeploymentStatus() (*DeploymentStatus, error) {
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

	var systemData struct {
		PowerState string `json:"PowerState"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&systemData); err != nil {
		return nil, fmt.Errorf("decoding system data: %w", err)
	}

	status := &DeploymentStatus{
		StartTime: time.Now(),
	}

	switch systemData.PowerState {
	case "On":
		status.State = "Completed"
		status.Message = "Deployment completed"
		status.Progress = 100
	case "Off":
		status.State = "Failed"
		status.Message = "System powered off during deployment"
		status.Progress = 0
	default:
		status.State = "InProgress"
		status.Message = "Deployment in progress"
		status.Progress = 50
	}

	return status, nil
}
