package alpaca

import (
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/unklstewy/ads-bscope/pkg/config"
)

// FilterPosition represents the Seestar S30 filter wheel positions
type FilterPosition int

const (
	FilterUVIRCut   FilterPosition = 0 // UV/IR Cut - standard tracking filter
	FilterDuoBand   FilterPosition = 1 // Duo-Band - narrowband imaging
	FilterDarkField FilterPosition = 2 // Dark Field - safety shutter/calibration
	FilterSolar     FilterPosition = 3 // Solar - sun observation (DO NOT USE FOR AIRCRAFT)
)

// FilterNames maps positions to human-readable names (Seestar internal IDs)
var FilterNames = map[FilterPosition]string{
	FilterUVIRCut:   "UV/IR Cut",
	FilterDuoBand:   "Duo-Band",
	FilterDarkField: "Dark Field",
	FilterSolar:     "Solar",
}

// FilterWheelClient represents an ASCOM Alpaca filter wheel client.
// Used for Seestar S30 filter management during aircraft tracking.
type FilterWheelClient struct {
	// config contains telescope configuration
	config config.TelescopeConfig

	// clientID is a unique identifier for this client instance
	clientID int

	// telescope is the parent telescope client (for HTTP access)
	telescope *Client

	// connected tracks if we're currently connected to the filter wheel
	connected bool

	// currentPosition tracks the last known filter position
	currentPosition FilterPosition
}

// NewFilterWheelClient creates a new Alpaca filter wheel client.
func NewFilterWheelClient(telescopeClient *Client) *FilterWheelClient {
	return &FilterWheelClient{
		config:          telescopeClient.config,
		clientID:        telescopeClient.clientID,
		telescope:       telescopeClient,
		connected:       false,
		currentPosition: FilterUVIRCut, // Default to UV/IR Cut
	}
}

// Connect establishes a connection to the filter wheel.
// Implements: PUT /api/v1/filterwheel/{device_number}/connected
func (fw *FilterWheelClient) Connect() error {
	params := url.Values{}
	params.Add("Connected", "true")
	params.Add("ClientID", strconv.Itoa(fw.clientID))
	params.Add("ClientTransactionID", strconv.Itoa(fw.getTransactionID()))

	resp, err := fw.put("connected", params)
	if err != nil {
		return fmt.Errorf("failed to connect to filter wheel: %w", err)
	}

	fw.connected = true

	// Get initial position
	pos, err := fw.GetPosition()
	if err == nil {
		fw.currentPosition = FilterPosition(pos)
	}

	return resp.Error()
}

// Disconnect closes the connection to the filter wheel.
// Implements: PUT /api/v1/filterwheel/{device_number}/connected
func (fw *FilterWheelClient) Disconnect() error {
	if !fw.connected {
		return nil
	}

	params := url.Values{}
	params.Add("Connected", "false")
	params.Add("ClientID", strconv.Itoa(fw.clientID))
	params.Add("ClientTransactionID", strconv.Itoa(fw.getTransactionID()))

	resp, err := fw.put("connected", params)
	if err != nil {
		return fmt.Errorf("failed to disconnect from filter wheel: %w", err)
	}

	fw.connected = false
	return resp.Error()
}

// GetPosition returns the current filter wheel position (0-3).
// Implements: GET /api/v1/filterwheel/{device_number}/position
func (fw *FilterWheelClient) GetPosition() (int, error) {
	if !fw.connected {
		return 0, fmt.Errorf("filter wheel not connected")
	}

	resp, err := fw.get("position")
	if err != nil {
		return 0, fmt.Errorf("failed to get filter position: %w", err)
	}

	if err := resp.Error(); err != nil {
		return 0, err
	}

	// Position comes back as float64 from JSON
	posFloat, ok := resp.Value.(float64)
	if !ok {
		return 0, fmt.Errorf("unexpected response type for filter position")
	}

	position := int(posFloat)
	fw.currentPosition = FilterPosition(position)

	return position, nil
}

// SetPosition moves the filter wheel to the specified position (0-3).
// Implements: PUT /api/v1/filterwheel/{device_number}/position
func (fw *FilterWheelClient) SetPosition(position FilterPosition) error {
	if !fw.connected {
		return fmt.Errorf("filter wheel not connected")
	}

	// Validate position (Seestar has 4 slots: 0-3)
	if position < 0 || position > 3 {
		return fmt.Errorf("invalid filter position %d: must be 0-3", position)
	}

	params := url.Values{}
	params.Add("Position", strconv.Itoa(int(position)))
	params.Add("ClientID", strconv.Itoa(fw.clientID))
	params.Add("ClientTransactionID", strconv.Itoa(fw.getTransactionID()))

	resp, err := fw.put("position", params)
	if err != nil {
		return fmt.Errorf("failed to set filter position: %w", err)
	}

	if err := resp.Error(); err != nil {
		return err
	}

	fw.currentPosition = position
	return nil
}

// GetCurrentFilter returns the current filter position and name.
func (fw *FilterWheelClient) GetCurrentFilter() (FilterPosition, string, error) {
	pos, err := fw.GetPosition()
	if err != nil {
		return 0, "", err
	}

	filter := FilterPosition(pos)
	name := FilterNames[filter]
	return filter, name, nil
}

// SetTrackingFilter sets the optimal filter for aircraft tracking (UV/IR Cut).
// This should be called before starting an aircraft intercept.
func (fw *FilterWheelClient) SetTrackingFilter() error {
	if fw.currentPosition == FilterUVIRCut {
		return nil // Already at correct filter
	}

	if err := fw.SetPosition(FilterUVIRCut); err != nil {
		return fmt.Errorf("failed to set tracking filter: %w", err)
	}

	return fw.WaitForMovement()
}

// SetDarkFilter sets the dark field filter (software lens cap).
// Use this for safety when near the sun or for dark frame calibration.
func (fw *FilterWheelClient) SetDarkFilter() error {
	if fw.currentPosition == FilterDarkField {
		return nil // Already dark
	}

	if err := fw.SetPosition(FilterDarkField); err != nil {
		return fmt.Errorf("failed to set dark filter: %w", err)
	}

	return fw.WaitForMovement()
}

// WaitForMovement waits for the filter wheel to stop moving.
// The Alpaca spec doesn't provide an IsMoving endpoint for filter wheels,
// so we use a short delay (filter wheels are typically fast).
func (fw *FilterWheelClient) WaitForMovement() error {
	// Seestar filter wheel is fast - typical movement is <1 second
	time.Sleep(500 * time.Millisecond)

	// Verify position
	pos, err := fw.GetPosition()
	if err != nil {
		return fmt.Errorf("failed to verify filter position: %w", err)
	}

	if FilterPosition(pos) != fw.currentPosition {
		return fmt.Errorf("filter wheel at unexpected position %d", pos)
	}

	return nil
}

// IsSafeForSolarProximity checks if current filter is safe near the sun.
// Returns false if using Solar filter (which is for solar observation only).
// For aircraft near the sun, Dark Field is recommended for safety.
func (fw *FilterWheelClient) IsSafeForSolarProximity() bool {
	// Solar filter is ONLY for direct sun observation
	// All other filters (including Dark Field) are safe
	return fw.currentPosition != FilterSolar
}

// getTransactionID generates a unique transaction ID for each API call.
func (fw *FilterWheelClient) getTransactionID() int {
	return int(time.Now().UnixNano() / 1000000)
}

// get performs an HTTP GET request to a filter wheel endpoint.
func (fw *FilterWheelClient) get(endpoint string) (*alpacaResponse, error) {
	// Build URL using filter wheel device number (typically 0)
	apiURL := fmt.Sprintf("%s/api/v1/filterwheel/%d/%s",
		fw.config.BaseURL, fw.config.FilterWheelDeviceNumber, endpoint)

	// Add query parameters
	params := url.Values{}
	params.Add("ClientID", strconv.Itoa(fw.clientID))
	params.Add("ClientTransactionID", strconv.Itoa(fw.getTransactionID()))

	fullURL := fmt.Sprintf("%s?%s", apiURL, params.Encode())

	// Use telescope's HTTP client
	resp, err := fw.telescope.httpClient.Get(fullURL)
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

// put performs an HTTP PUT request to a filter wheel endpoint.
func (fw *FilterWheelClient) put(endpoint string, params url.Values) (*alpacaResponse, error) {
	// Build URL using filter wheel device number (typically 0)
	apiURL := fmt.Sprintf("%s/api/v1/filterwheel/%d/%s",
		fw.config.BaseURL, fw.config.FilterWheelDeviceNumber, endpoint)

	// Make request with form data using telescope's HTTP client
	resp, err := fw.telescope.httpClient.PostForm(apiURL, params)
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
