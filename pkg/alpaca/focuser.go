package alpaca

import (
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/unklstewy/ads-bscope/pkg/config"
)

// FocuserClient represents an ASCOM Alpaca focuser client.
// Used for setting focus to infinity for aircraft/terrestrial tracking.
// Reference: https://ascom-standards.org/Developer/Alpaca.htm
type FocuserClient struct {
	// config contains telescope configuration (includes focuser settings)
	config config.TelescopeConfig

	// clientID is a unique identifier for this client instance
	clientID int

	// httpClient is the HTTP client used for API requests (shared with telescope)
	telescope *Client

	// connected tracks if we're currently connected to the focuser
	connected bool
}

// NewFocuserClient creates a new Alpaca focuser client from telescope client.
func NewFocuserClient(telescopeClient *Client) *FocuserClient {
	return &FocuserClient{
		config:    telescopeClient.config,
		clientID:  telescopeClient.clientID,
		telescope: telescopeClient,
		connected: false,
	}
}

// Connect establishes a connection to the focuser.
// Implements: PUT /api/v1/focuser/{device_number}/connected
func (f *FocuserClient) Connect() error {
	params := url.Values{}
	params.Add("Connected", "true")
	params.Add("ClientID", strconv.Itoa(f.clientID))
	params.Add("ClientTransactionID", strconv.Itoa(f.getTransactionID()))

	resp, err := f.put("connected", params)
	if err != nil {
		return fmt.Errorf("failed to connect to focuser: %w", err)
	}

	f.connected = true
	return resp.Error()
}

// Disconnect closes the connection to the focuser.
// Implements: PUT /api/v1/focuser/{device_number}/connected
func (f *FocuserClient) Disconnect() error {
	if !f.connected {
		return nil
	}

	params := url.Values{}
	params.Add("Connected", "false")
	params.Add("ClientID", strconv.Itoa(f.clientID))
	params.Add("ClientTransactionID", strconv.Itoa(f.getTransactionID()))

	resp, err := f.put("connected", params)
	if err != nil {
		return fmt.Errorf("failed to disconnect from focuser: %w", err)
	}

	f.connected = false
	return resp.Error()
}

// IsConnected returns the current connection status.
// Implements: GET /api/v1/focuser/{device_number}/connected
func (f *FocuserClient) IsConnected() (bool, error) {
	resp, err := f.get("connected")
	if err != nil {
		return false, fmt.Errorf("failed to get focuser connection status: %w", err)
	}

	if err := resp.Error(); err != nil {
		return false, err
	}

	connected, ok := resp.Value.(bool)
	if !ok {
		return false, fmt.Errorf("unexpected response type for focuser connected status")
	}

	return connected, nil
}

// GetPosition returns the current focuser position in steps.
// Implements: GET /api/v1/focuser/{device_number}/position
func (f *FocuserClient) GetPosition() (int, error) {
	if !f.connected {
		return 0, fmt.Errorf("focuser not connected")
	}

	resp, err := f.get("position")
	if err != nil {
		return 0, fmt.Errorf("failed to get focuser position: %w", err)
	}

	if err := resp.Error(); err != nil {
		return 0, err
	}

	// Position comes back as float64 from JSON
	posFloat, ok := resp.Value.(float64)
	if !ok {
		return 0, fmt.Errorf("unexpected response type for focuser position")
	}

	return int(posFloat), nil
}

// Move moves the focuser to an absolute position.
// position: target position in steps
// Implements: PUT /api/v1/focuser/{device_number}/move
func (f *FocuserClient) Move(position int) error {
	if !f.connected {
		return fmt.Errorf("focuser not connected")
	}

	params := url.Values{}
	params.Add("Position", strconv.Itoa(position))
	params.Add("ClientID", strconv.Itoa(f.clientID))
	params.Add("ClientTransactionID", strconv.Itoa(f.getTransactionID()))

	resp, err := f.put("move", params)
	if err != nil {
		return fmt.Errorf("failed to move focuser: %w", err)
	}

	return resp.Error()
}

// IsMoving returns true if the focuser is currently moving.
// Implements: GET /api/v1/focuser/{device_number}/ismoving
func (f *FocuserClient) IsMoving() (bool, error) {
	if !f.connected {
		return false, fmt.Errorf("focuser not connected")
	}

	resp, err := f.get("ismoving")
	if err != nil {
		return false, fmt.Errorf("failed to get focuser moving status: %w", err)
	}

	if err := resp.Error(); err != nil {
		return false, err
	}

	moving, ok := resp.Value.(bool)
	if !ok {
		return false, fmt.Errorf("unexpected response type for focuser moving status")
	}

	return moving, nil
}

// Halt immediately stops focuser movement.
// Critical for aborting focus operations.
// Implements: PUT /api/v1/focuser/{device_number}/halt
func (f *FocuserClient) Halt() error {
	if !f.connected {
		return fmt.Errorf("focuser not connected")
	}

	params := url.Values{}
	params.Add("ClientID", strconv.Itoa(f.clientID))
	params.Add("ClientTransactionID", strconv.Itoa(f.getTransactionID()))

	resp, err := f.put("halt", params)
	if err != nil {
		return fmt.Errorf("failed to halt focuser: %w", err)
	}

	return resp.Error()
}

// MoveToInfinity moves the focuser to the configured infinity position.
// Uses the InfinityFocusPosition from config (typically 1700-1850 for Seestar).
// Waits for the move to complete.
func (f *FocuserClient) MoveToInfinity() error {
	if !f.connected {
		return fmt.Errorf("focuser not connected")
	}

	target := f.config.InfinityFocusPosition
	if target <= 0 {
		return fmt.Errorf("infinity focus position not configured")
	}

	// Check current position
	current, err := f.GetPosition()
	if err != nil {
		return fmt.Errorf("failed to get current position: %w", err)
	}

	// Already at infinity?
	tolerance := 10 // steps
	if current >= target-tolerance && current <= target+tolerance {
		return nil // Already close enough
	}

	// Move to infinity
	if err := f.Move(target); err != nil {
		return fmt.Errorf("failed to initiate move to infinity: %w", err)
	}

	// Wait for movement to complete (with timeout)
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			moving, err := f.IsMoving()
			if err != nil {
				return fmt.Errorf("failed to check moving status: %w", err)
			}
			if !moving {
				// Verify final position
				final, err := f.GetPosition()
				if err != nil {
					return fmt.Errorf("failed to verify final position: %w", err)
				}
				if final >= target-tolerance && final <= target+tolerance {
					return nil
				}
				return fmt.Errorf("focuser stopped at unexpected position %d (target: %d)", final, target)
			}

		case <-timeout:
			return fmt.Errorf("timeout waiting for focuser to reach infinity position")
		}
	}
}

// getTransactionID generates a unique transaction ID for each API call.
func (f *FocuserClient) getTransactionID() int {
	return int(time.Now().UnixNano() / 1000000)
}

// get performs an HTTP GET request to a focuser endpoint.
func (f *FocuserClient) get(endpoint string) (*alpacaResponse, error) {
	// Build URL using focuser device number
	apiURL := fmt.Sprintf("%s/api/v1/focuser/%d/%s",
		f.config.BaseURL, f.config.FocuserDeviceNumber, endpoint)

	// Add query parameters
	params := url.Values{}
	params.Add("ClientID", strconv.Itoa(f.clientID))
	params.Add("ClientTransactionID", strconv.Itoa(f.getTransactionID()))

	fullURL := fmt.Sprintf("%s?%s", apiURL, params.Encode())

	// Use telescope's HTTP client
	resp, err := f.telescope.httpClient.Get(fullURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Parse response
	var alpacaResp alpacaResponse
	if err := parseAlpacaResponse(resp.Body, &alpacaResp); err != nil {
		return nil, err
	}

	return &alpacaResp, nil
}

// put performs an HTTP PUT request to a focuser endpoint.
func (f *FocuserClient) put(endpoint string, params url.Values) (*alpacaResponse, error) {
	// Build URL using focuser device number
	apiURL := fmt.Sprintf("%s/api/v1/focuser/%d/%s",
		f.config.BaseURL, f.config.FocuserDeviceNumber, endpoint)

	// Make request with form data using telescope's HTTP client
	resp, err := f.telescope.httpClient.PostForm(apiURL, params)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Parse response (reuse telescope client's parsing logic)
	var alpacaResp alpacaResponse
	if err := parseAlpacaResponse(resp.Body, &alpacaResp); err != nil {
		return nil, err
	}

	return &alpacaResp, nil
}
