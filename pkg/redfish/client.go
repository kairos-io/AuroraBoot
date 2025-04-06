package redfish

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client represents a RedFish API client
type Client struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
	session    *Session
}

// Session represents a RedFish session
type Session struct {
	Token     string
	ExpiresAt time.Time
	Location  string
}

// NewClient creates a new RedFish client
func NewClient(endpoint, username, password string, verifySSL bool, timeout time.Duration) (*Client, error) {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: !verifySSL,
		},
	}

	client := &Client{
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

// authenticate performs RedFish authentication and establishes a session
func (c *Client) authenticate() error {
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
		ExpiresAt: time.Now().Add(30 * time.Minute), // Default session timeout
	}

	return nil
}

// GetSystemInfo retrieves basic system information
func (c *Client) GetSystemInfo() (*SystemInfo, error) {
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

// SystemInfo represents basic system information
type SystemInfo struct {
	ID             string `json:"Id"`
	Name           string `json:"Name"`
	Model          string `json:"Model"`
	Manufacturer   string `json:"Manufacturer"`
	SerialNumber   string `json:"SerialNumber"`
	MemorySize     int    `json:"MemorySummaryTotalSystemMemoryGiB"`
	ProcessorCount int    `json:"ProcessorSummaryCount"`
}
