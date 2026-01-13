package alpaca

import (
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/unklstewy/ads-bscope/pkg/config"
)

// SwitchID represents the Seestar S30 switch identifiers
type SwitchID int

const (
	SwitchDewHeater SwitchID = 0 // Internal lens heater (only switch on Seestar)
)

// SwitchClient represents an ASCOM Alpaca switch client.
// Used for Seestar S30 dew heater control.
// The Seestar S30 only exposes one switch: the internal dew heater.
type SwitchClient struct {
	// config contains telescope configuration
	config config.TelescopeConfig

	// clientID is a unique identifier for this client instance
	clientID int

	// telescope is the parent telescope client (for HTTP access)
	telescope *Client

	// connected tracks if we're currently connected to the switch
	connected bool

	// dewHeaterState tracks the dew heater state
	dewHeaterState bool
}

// NewSwitchClient creates a new Alpaca switch client.
func NewSwitchClient(telescopeClient *Client) *SwitchClient {
	return &SwitchClient{
		config:         telescopeClient.config,
		clientID:       telescopeClient.clientID,
		telescope:      telescopeClient,
		connected:      false,
		dewHeaterState: false,
	}
}

// Connect establishes a connection to the switch.
// Implements: PUT /api/v1/switch/{device_number}/connected
func (s *SwitchClient) Connect() error {
	params := url.Values{}
	params.Add("Connected", "true")
	params.Add("ClientID", strconv.Itoa(s.clientID))
	params.Add("ClientTransactionID", strconv.Itoa(s.getTransactionID()))

	resp, err := s.put("connected", params)
	if err != nil {
		return fmt.Errorf("failed to connect to switch: %w", err)
	}

	s.connected = true

	// Get initial dew heater state
	state, err := s.GetDewHeater()
	if err == nil {
		s.dewHeaterState = state
	}

	return resp.Error()
}

// Disconnect closes the connection to the switch.
// Implements: PUT /api/v1/switch/{device_number}/connected
func (s *SwitchClient) Disconnect() error {
	if !s.connected {
		return nil
	}

	params := url.Values{}
	params.Add("Connected", "false")
	params.Add("ClientID", strconv.Itoa(s.clientID))
	params.Add("ClientTransactionID", strconv.Itoa(s.getTransactionID()))

	resp, err := s.put("connected", params)
	if err != nil {
		return fmt.Errorf("failed to disconnect from switch: %w", err)
	}

	s.connected = false
	return resp.Error()
}

// GetDewHeater returns the current dew heater state (on/off).
// Implements: GET /api/v1/switch/{device_number}/getswitch
func (s *SwitchClient) GetDewHeater() (bool, error) {
	if !s.connected {
		return false, fmt.Errorf("switch not connected")
	}

	// Build URL with ID parameter for dew heater (ID=0)
	apiURL := fmt.Sprintf("%s/api/v1/switch/%d/getswitch",
		s.config.BaseURL, s.config.SwitchDeviceNumber)

	params := url.Values{}
	params.Add("Id", "0") // Dew heater is ID 0
	params.Add("ClientID", strconv.Itoa(s.clientID))
	params.Add("ClientTransactionID", strconv.Itoa(s.getTransactionID()))

	fullURL := fmt.Sprintf("%s?%s", apiURL, params.Encode())

	// Use telescope's HTTP client
	resp, err := s.telescope.httpClient.Get(fullURL)
	if err != nil {
		return false, fmt.Errorf("failed to get dew heater state: %w", err)
	}
	defer resp.Body.Close()

	// Parse response
	var alpacaResp alpacaResponse
	if err := parseAlpacaResponse(resp.Body, &alpacaResp); err != nil {
		return false, err
	}

	if err := alpacaResp.Error(); err != nil {
		return false, err
	}

	// Value comes back as float64 or bool from JSON
	switch v := alpacaResp.Value.(type) {
	case bool:
		s.dewHeaterState = v
		return v, nil
	case float64:
		state := v > 0.5 // > 0.5 = on
		s.dewHeaterState = state
		return state, nil
	default:
		return false, fmt.Errorf("unexpected response type for dew heater state")
	}
}

// SetDewHeater sets the dew heater state (on/off).
// Implements: PUT /api/v1/switch/{device_number}/setswitch
func (s *SwitchClient) SetDewHeater(enabled bool) error {
	if !s.connected {
		return fmt.Errorf("switch not connected")
	}

	params := url.Values{}
	params.Add("Id", "0") // Dew heater is ID 0
	params.Add("State", strconv.FormatBool(enabled))
	params.Add("ClientID", strconv.Itoa(s.clientID))
	params.Add("ClientTransactionID", strconv.Itoa(s.getTransactionID()))

	resp, err := s.put("setswitch", params)
	if err != nil {
		return fmt.Errorf("failed to set dew heater: %w", err)
	}

	if err := resp.Error(); err != nil {
		return err
	}

	s.dewHeaterState = enabled
	return nil
}

// EnableDewHeater turns on the dew heater.
// Use this in humid conditions or for long tracking sessions.
func (s *SwitchClient) EnableDewHeater() error {
	if s.dewHeaterState {
		return nil // Already on
	}

	return s.SetDewHeater(true)
}

// DisableDewHeater turns off the dew heater.
func (s *SwitchClient) DisableDewHeater() error {
	if !s.dewHeaterState {
		return nil // Already off
	}

	return s.SetDewHeater(false)
}

// GetSwitchDescription returns the description of a switch.
// For Seestar S30, only ID 0 (dew heater) is valid.
// Implements: GET /api/v1/switch/{device_number}/getswitchdescription
func (s *SwitchClient) GetSwitchDescription(id int) (string, error) {
	if !s.connected {
		return "", fmt.Errorf("switch not connected")
	}

	if id != 0 {
		return "", fmt.Errorf("invalid switch ID %d: Seestar only has ID 0 (dew heater)", id)
	}

	apiURL := fmt.Sprintf("%s/api/v1/switch/%d/getswitchdescription",
		s.config.BaseURL, s.config.SwitchDeviceNumber)

	params := url.Values{}
	params.Add("Id", strconv.Itoa(id))
	params.Add("ClientID", strconv.Itoa(s.clientID))
	params.Add("ClientTransactionID", strconv.Itoa(s.getTransactionID()))

	fullURL := fmt.Sprintf("%s?%s", apiURL, params.Encode())

	resp, err := s.telescope.httpClient.Get(fullURL)
	if err != nil {
		return "", fmt.Errorf("failed to get switch description: %w", err)
	}
	defer resp.Body.Close()

	var alpacaResp alpacaResponse
	if err := parseAlpacaResponse(resp.Body, &alpacaResp); err != nil {
		return "", err
	}

	if err := alpacaResp.Error(); err != nil {
		return "", err
	}

	desc, ok := alpacaResp.Value.(string)
	if !ok {
		return "", fmt.Errorf("unexpected response type for switch description")
	}

	return desc, nil
}

// GetMaxSwitch returns the maximum switch ID (always 0 for Seestar).
// Implements: GET /api/v1/switch/{device_number}/maxswitch
func (s *SwitchClient) GetMaxSwitch() (int, error) {
	if !s.connected {
		return 0, fmt.Errorf("switch not connected")
	}

	resp, err := s.get("maxswitch")
	if err != nil {
		return 0, fmt.Errorf("failed to get max switch: %w", err)
	}

	if err := resp.Error(); err != nil {
		return 0, err
	}

	maxSwitch, ok := resp.Value.(float64)
	if !ok {
		return 0, fmt.Errorf("unexpected response type for max switch")
	}

	return int(maxSwitch), nil
}

// getTransactionID generates a unique transaction ID for each API call.
func (s *SwitchClient) getTransactionID() int {
	return int(time.Now().UnixNano() / 1000000)
}

// get performs an HTTP GET request to a switch endpoint.
func (s *SwitchClient) get(endpoint string) (*alpacaResponse, error) {
	// Build URL using switch device number (typically 0)
	apiURL := fmt.Sprintf("%s/api/v1/switch/%d/%s",
		s.config.BaseURL, s.config.SwitchDeviceNumber, endpoint)

	// Add query parameters
	params := url.Values{}
	params.Add("ClientID", strconv.Itoa(s.clientID))
	params.Add("ClientTransactionID", strconv.Itoa(s.getTransactionID()))

	fullURL := fmt.Sprintf("%s?%s", apiURL, params.Encode())

	// Use telescope's HTTP client
	resp, err := s.telescope.httpClient.Get(fullURL)
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

// put performs an HTTP PUT request to a switch endpoint.
func (s *SwitchClient) put(endpoint string, params url.Values) (*alpacaResponse, error) {
	// Build URL using switch device number (typically 0)
	apiURL := fmt.Sprintf("%s/api/v1/switch/%d/%s",
		s.config.BaseURL, s.config.SwitchDeviceNumber, endpoint)

	// Make request with form data using telescope's HTTP client
	resp, err := s.telescope.httpClient.PostForm(apiURL, params)
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
