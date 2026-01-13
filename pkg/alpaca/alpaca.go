package alpaca

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/unklstewy/ads-bscope/pkg/config"
)

// Client represents an ASCOM Alpaca telescope client.
// It implements the Alpaca REST API for telescope control.
// Reference: https://ascom-standards.org/Developer/Alpaca.htm
type Client struct {
	// config contains all telescope configuration from the config system
	config config.TelescopeConfig

	// clientID is a unique identifier for this client instance
	// Generated at client creation to comply with Alpaca specification
	clientID int

	// httpClient is the HTTP client used for API requests
	httpClient *http.Client

	// connected tracks if we're currently connected to the telescope
	connected bool
}

// NewClient creates a new Alpaca telescope client from configuration.
// The configuration should be loaded from config file or database.
func NewClient(cfg config.TelescopeConfig) *Client {
	return &Client{
		config:   cfg,
		clientID: generateClientID(),
		httpClient: &http.Client{
			Timeout: 30 * time.Second, // Increased timeout for slow simulations
		},
		connected: false,
	}
}

// generateClientID creates a unique client ID for this Alpaca session.
// The Alpaca specification requires each client to have a unique ID.
// Uses Unix timestamp to ensure uniqueness across sessions.
func generateClientID() int {
	return int(time.Now().Unix())
}

// Connect establishes a connection to the telescope.
// Must be called before any other telescope operations.
// Implements: PUT /api/v1/telescope/{device_number}/connected
func (c *Client) Connect() error {
	// Set connected=true via Alpaca API
	params := url.Values{}
	params.Add("Connected", "true")
	params.Add("ClientID", strconv.Itoa(c.clientID))
	params.Add("ClientTransactionID", strconv.Itoa(c.getTransactionID()))

	resp, err := c.put("connected", params)
	if err != nil {
		return fmt.Errorf("failed to connect to telescope: %w", err)
	}

	c.connected = true
	return resp.Error()
}

// Disconnect closes the connection to the telescope.
// Implements: PUT /api/v1/telescope/{device_number}/connected
func (c *Client) Disconnect() error {
	if !c.connected {
		return nil
	}

	// Set connected=false via Alpaca API
	params := url.Values{}
	params.Add("Connected", "false")
	params.Add("ClientID", strconv.Itoa(c.clientID))
	params.Add("ClientTransactionID", strconv.Itoa(c.getTransactionID()))

	resp, err := c.put("connected", params)
	if err != nil {
		return fmt.Errorf("failed to disconnect from telescope: %w", err)
	}

	c.connected = false
	return resp.Error()
}

// IsConnected returns the current connection status.
// Implements: GET /api/v1/telescope/{device_number}/connected
func (c *Client) IsConnected() (bool, error) {
	resp, err := c.get("connected")
	if err != nil {
		return false, fmt.Errorf("failed to get connection status: %w", err)
	}

	if err := resp.Error(); err != nil {
		return false, err
	}

	connected, ok := resp.Value.(bool)
	if !ok {
		return false, fmt.Errorf("unexpected response type for connected status")
	}

	return connected, nil
}

// SlewToAltAz slews the telescope to the specified altitude and azimuth coordinates.
// altitude: angle above horizon in degrees (0-90)
// azimuth: angle from north clockwise in degrees (0-360)
// This is used for Alt/Az mounted telescopes.
// Implements: PUT /api/v1/telescope/{device_number}/slewtoaltaz
func (c *Client) SlewToAltAz(altitude, azimuth float64) error {
	if !c.connected {
		return fmt.Errorf("telescope not connected")
	}

	// Verify mount type
	if strings.ToLower(c.config.MountType) != "altaz" {
		return fmt.Errorf("telescope mount type is %s, not altaz", c.config.MountType)
	}

	params := url.Values{}
	params.Add("Azimuth", fmt.Sprintf("%.6f", azimuth))
	params.Add("Altitude", fmt.Sprintf("%.6f", altitude))
	params.Add("ClientID", strconv.Itoa(c.clientID))
	params.Add("ClientTransactionID", strconv.Itoa(c.getTransactionID()))

	resp, err := c.put("slewtoaltaz", params)
	if err != nil {
		return fmt.Errorf("failed to slew telescope: %w", err)
	}

	return resp.Error()
}

// SlewToCoordinates slews the telescope to the specified equatorial coordinates.
// ra: right ascension in decimal hours (0-24)
// dec: declination in decimal degrees (-90 to +90)
// This is used for equatorially mounted telescopes.
// Implements: PUT /api/v1/telescope/{device_number}/slewtocoordinates
func (c *Client) SlewToCoordinates(ra, dec float64) error {
	if !c.connected {
		return fmt.Errorf("telescope not connected")
	}

	// Verify mount type
	if strings.ToLower(c.config.MountType) != "equatorial" {
		return fmt.Errorf("telescope mount type is %s, not equatorial", c.config.MountType)
	}

	params := url.Values{}
	params.Add("RightAscension", fmt.Sprintf("%.6f", ra))
	params.Add("Declination", fmt.Sprintf("%.6f", dec))
	params.Add("ClientID", strconv.Itoa(c.clientID))
	params.Add("ClientTransactionID", strconv.Itoa(c.getTransactionID()))

	resp, err := c.put("slewtocoordinates", params)
	if err != nil {
		return fmt.Errorf("failed to slew telescope: %w", err)
	}

	return resp.Error()
}

// IsSlewing returns true if the telescope is currently slewing.
// Implements: GET /api/v1/telescope/{device_number}/slewing
func (c *Client) IsSlewing() (bool, error) {
	if !c.connected {
		return false, fmt.Errorf("telescope not connected")
	}

	resp, err := c.get("slewing")
	if err != nil {
		return false, fmt.Errorf("failed to get slewing status: %w", err)
	}

	if err := resp.Error(); err != nil {
		return false, err
	}

	slewing, ok := resp.Value.(bool)
	if !ok {
		return false, fmt.Errorf("unexpected response type for slewing status")
	}

	return slewing, nil
}

// AbortSlew immediately stops any telescope motion.
// Implements: PUT /api/v1/telescope/{device_number}/abortslew
func (c *Client) AbortSlew() error {
	if !c.connected {
		return fmt.Errorf("telescope not connected")
	}

	params := url.Values{}
	params.Add("ClientID", strconv.Itoa(c.clientID))
	params.Add("ClientTransactionID", strconv.Itoa(c.getTransactionID()))

	resp, err := c.put("abortslew", params)
	if err != nil {
		return fmt.Errorf("failed to abort slew: %w", err)
	}

	return resp.Error()
}

// GetAltitude returns the telescope's current altitude.
// Implements: GET /api/v1/telescope/{device_number}/altitude
func (c *Client) GetAltitude() (float64, error) {
	if !c.connected {
		return 0, fmt.Errorf("telescope not connected")
	}

	resp, err := c.get("altitude")
	if err != nil {
		return 0, fmt.Errorf("failed to get altitude: %w", err)
	}

	if err := resp.Error(); err != nil {
		return 0, err
	}

	altitude, ok := resp.Value.(float64)
	if !ok {
		return 0, fmt.Errorf("unexpected response type for altitude")
	}

	return altitude, nil
}

// GetAzimuth returns the telescope's current azimuth.
// Implements: GET /api/v1/telescope/{device_number}/azimuth
func (c *Client) GetAzimuth() (float64, error) {
	if !c.connected {
		return 0, fmt.Errorf("telescope not connected")
	}

	resp, err := c.get("azimuth")
	if err != nil {
		return 0, fmt.Errorf("failed to get azimuth: %w", err)
	}

	if err := resp.Error(); err != nil {
		return 0, err
	}

	azimuth, ok := resp.Value.(float64)
	if !ok {
		return 0, fmt.Errorf("unexpected response type for azimuth")
	}

	return azimuth, nil
}

// MoveAxis moves the telescope at a constant rate on a specified axis.
// This is ideal for tracking moving targets like aircraft.
// axis: 0 = Azimuth (primary), 1 = Altitude (secondary)
// rate: speed in degrees per second (positive = CW/up, negative = CCW/down)
// For Seestar S30: max rate is 6.0 deg/sec
// Set rate to 0 to stop movement on that axis.
// Implements: PUT /api/v1/telescope/{device_number}/moveaxis
func (c *Client) MoveAxis(axis int, rate float64) error {
	if !c.connected {
		return fmt.Errorf("telescope not connected")
	}

	// Validate axis
	if axis < 0 || axis > 1 {
		return fmt.Errorf("invalid axis %d: must be 0 (azimuth) or 1 (altitude)", axis)
	}

	// Validate rate against configured slew rate
	if rate > c.config.SlewRate || rate < -c.config.SlewRate {
		return fmt.Errorf("rate %.2f exceeds configured slew rate %.2f deg/sec", rate, c.config.SlewRate)
	}

	params := url.Values{}
	params.Add("Axis", strconv.Itoa(axis))
	params.Add("Rate", fmt.Sprintf("%.6f", rate))
	params.Add("ClientID", strconv.Itoa(c.clientID))
	params.Add("ClientTransactionID", strconv.Itoa(c.getTransactionID()))

	resp, err := c.put("moveaxis", params)
	if err != nil {
		return fmt.Errorf("failed to move axis: %w", err)
	}

	return resp.Error()
}

// StopAxes stops movement on both axes by setting their rates to 0.
// Convenience method for stopping all telescope motion.
func (c *Client) StopAxes() error {
	if !c.connected {
		return fmt.Errorf("telescope not connected")
	}

	// Stop both axes
	if err := c.MoveAxis(0, 0); err != nil {
		return fmt.Errorf("failed to stop azimuth axis: %w", err)
	}
	if err := c.MoveAxis(1, 0); err != nil {
		return fmt.Errorf("failed to stop altitude axis: %w", err)
	}

	return nil
}

// GetAtPark returns true if the telescope is at the park position.
// Implements: GET /api/v1/telescope/{device_number}/atpark
func (c *Client) GetAtPark() (bool, error) {
	resp, err := c.get("atpark")
	if err != nil {
		return false, fmt.Errorf("failed to get park status: %w", err)
	}

	if err := resp.Error(); err != nil {
		return false, err
	}

	atPark, ok := resp.Value.(bool)
	if !ok {
		return false, fmt.Errorf("unexpected response type for atpark status")
	}

	return atPark, nil
}

// Unpark unparks the telescope, preparing it for slewing.
// Must be called before any slewing operations if the telescope is parked.
// Implements: PUT /api/v1/telescope/{device_number}/unpark
func (c *Client) Unpark() error {
	if !c.connected {
		return fmt.Errorf("telescope not connected")
	}

	params := url.Values{}
	params.Add("ClientID", strconv.Itoa(c.clientID))
	params.Add("ClientTransactionID", strconv.Itoa(c.getTransactionID()))

	resp, err := c.put("unpark", params)
	if err != nil {
		return fmt.Errorf("failed to unpark telescope: %w", err)
	}

	return resp.Error()
}

// Park parks the telescope at its configured park position.
// Implements: PUT /api/v1/telescope/{device_number}/park
func (c *Client) Park() error {
	if !c.connected {
		return fmt.Errorf("telescope not connected")
	}

	params := url.Values{}
	params.Add("ClientID", strconv.Itoa(c.clientID))
	params.Add("ClientTransactionID", strconv.Itoa(c.getTransactionID()))

	resp, err := c.put("park", params)
	if err != nil {
		return fmt.Errorf("failed to park telescope: %w", err)
	}

	return resp.Error()
}

// SetTracking enables or disables telescope tracking.
// For Alt-Az mounts, this typically has no effect.
// For equatorial mounts, enables sidereal tracking.
// Implements: PUT /api/v1/telescope/{device_number}/tracking
func (c *Client) SetTracking(enabled bool) error {
	if !c.connected {
		return fmt.Errorf("telescope not connected")
	}

	params := url.Values{}
	params.Add("Tracking", strconv.FormatBool(enabled))
	params.Add("ClientID", strconv.Itoa(c.clientID))
	params.Add("ClientTransactionID", strconv.Itoa(c.getTransactionID()))

	resp, err := c.put("tracking", params)
	if err != nil {
		return fmt.Errorf("failed to set tracking: %w", err)
	}

	return resp.Error()
}

// GetTracking returns the current tracking state.
// Implements: GET /api/v1/telescope/{device_number}/tracking
func (c *Client) GetTracking() (bool, error) {
	if !c.connected {
		return false, fmt.Errorf("telescope not connected")
	}

	resp, err := c.get("tracking")
	if err != nil {
		return false, fmt.Errorf("failed to get tracking status: %w", err)
	}

	if err := resp.Error(); err != nil {
		return false, err
	}

	tracking, ok := resp.Value.(bool)
	if !ok {
		return false, fmt.Errorf("unexpected response type for tracking status")
	}

	return tracking, nil
}

// GetConfig returns the current telescope configuration.
// This allows external code to check mount type and other settings.
func (c *Client) GetConfig() config.TelescopeConfig {
	return c.config
}

// getTransactionID generates a unique transaction ID for each API call.
// Uses Unix timestamp modulo 2^31-1 to keep within 32-bit signed integer range.
// Alpaca spec requires transaction IDs to fit in a 32-bit signed integer.
func (c *Client) getTransactionID() int {
	// Use Unix timestamp in seconds modulo a large prime to avoid overflow
	// while maintaining uniqueness across restarts
	now := time.Now().Unix()
	ms := int64(time.Now().Nanosecond() / 1000000)
	return int((now + ms) % 2147483647)
}

// get performs an HTTP GET request to an Alpaca endpoint.
func (c *Client) get(endpoint string) (*alpacaResponse, error) {
	// Build URL
	apiURL := fmt.Sprintf("%s/api/v1/telescope/%d/%s",
		c.config.BaseURL, c.config.DeviceNumber, endpoint)

	// Add query parameters
	params := url.Values{}
	params.Add("ClientID", strconv.Itoa(c.clientID))
	params.Add("ClientTransactionID", strconv.Itoa(c.getTransactionID()))

	fullURL := fmt.Sprintf("%s?%s", apiURL, params.Encode())

	// Make request
	resp, err := c.httpClient.Get(fullURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var alpacaResp alpacaResponse
	if err := json.Unmarshal(body, &alpacaResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &alpacaResp, nil
}

// put performs an HTTP PUT request to an Alpaca endpoint.
func (c *Client) put(endpoint string, params url.Values) (*alpacaResponse, error) {
	// Build URL
	apiURL := fmt.Sprintf("%s/api/v1/telescope/%d/%s",
		c.config.BaseURL, c.config.DeviceNumber, endpoint)

	// Create PUT request with form-encoded body
	req, err := http.NewRequest("PUT", apiURL, strings.NewReader(params.Encode()))
	if err != nil {
		return nil, err
	}

	// Set Content-Type header for form data
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Make request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var alpacaResp alpacaResponse
	if err := json.Unmarshal(body, &alpacaResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &alpacaResp, nil
}

// alpacaResponse represents the standard Alpaca API response format.
type alpacaResponse struct {
	// Value contains the response data (type varies by endpoint)
	Value interface{} `json:"Value"`

	// ClientTransactionID echoes back the client's transaction ID
	ClientTransactionID int `json:"ClientTransactionID"`

	// ServerTransactionID is the server's transaction ID
	ServerTransactionID int `json:"ServerTransactionID"`

	// ErrorNumber is non-zero if an error occurred
	ErrorNumber int `json:"ErrorNumber"`

	// ErrorMessage describes the error if ErrorNumber is non-zero
	ErrorMessage string `json:"ErrorMessage"`
}

// Error returns an error if the Alpaca response indicates failure.
func (r *alpacaResponse) Error() error {
	if r.ErrorNumber != 0 {
		return fmt.Errorf("alpaca error %d: %s", r.ErrorNumber, r.ErrorMessage)
	}
	return nil
}

// parseAlpacaResponse parses an Alpaca JSON response from an io.Reader.
// Helper function for other clients to reuse.
func parseAlpacaResponse(body io.Reader, resp *alpacaResponse) error {
	data, err := io.ReadAll(body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if err := json.Unmarshal(data, resp); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	return nil
}
