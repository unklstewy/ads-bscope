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
			Timeout: 10 * time.Second,
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

// GetConfig returns the current telescope configuration.
// This allows external code to check mount type and other settings.
func (c *Client) GetConfig() config.TelescopeConfig {
	return c.config
}

// getTransactionID generates a unique transaction ID for each API call.
// Uses Unix timestamp in nanoseconds for uniqueness.
func (c *Client) getTransactionID() int {
	return int(time.Now().UnixNano() / 1000000) // Milliseconds
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

	// Make request with form data
	resp, err := c.httpClient.PostForm(apiURL, params)
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
