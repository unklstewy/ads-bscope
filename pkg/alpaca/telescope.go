package alpaca

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// TelescopeClient represents a connection to an ASCOM Alpaca telescope
type TelescopeClient struct {
	baseURL      string
	deviceNumber int
	clientID     int
	txnCounter   int
	httpClient   *http.Client
}

// TelescopeStatus represents the current status of the telescope
type TelescopeStatus struct {
	Connected      bool    `json:"connected"`
	Tracking       bool    `json:"tracking"`
	Slewing        bool    `json:"slewing"`
	AtPark         bool    `json:"atPark"`
	Altitude       float64 `json:"altitude"`       // Degrees above horizon
	Azimuth        float64 `json:"azimuth"`        // Degrees from north
	RightAscension float64 `json:"rightAscension"` // Hours
	Declination    float64 `json:"declination"`    // Degrees
}

// TelescopeCapabilities represents the telescope's capabilities
type TelescopeCapabilities struct {
	Description      string   `json:"description"`
	DriverInfo       string   `json:"driverInfo"`
	InterfaceVersion int      `json:"interfaceVersion"`
	CanSetTracking   bool     `json:"canSetTracking"`
	CanSlew          bool     `json:"canSlew"`
	CanSlewAltAz     bool     `json:"canSlewAltAz"`
	SupportedActions []string `json:"supportedActions"`
}

// AlpacaResponse represents a standard Alpaca API response
type AlpacaResponse struct {
	Value                interface{} `json:"Value"`
	ClientTransactionID  int         `json:"ClientTransactionID"`
	ServerTransactionID  int         `json:"ServerTransactionID"`
	ErrorNumber          int         `json:"ErrorNumber"`
	ErrorMessage         string      `json:"ErrorMessage"`
}

// NewTelescopeClient creates a new Alpaca telescope client
func NewTelescopeClient(baseURL string, deviceNumber int) *TelescopeClient {
	return &TelescopeClient{
		baseURL:      baseURL,
		deviceNumber: deviceNumber,
		clientID:     1,
		txnCounter:   0,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// getTransactionID returns a unique transaction ID
func (c *TelescopeClient) getTransactionID() int {
	c.txnCounter++
	return c.txnCounter
}

// get performs a GET request to the Alpaca API
func (c *TelescopeClient) get(endpoint string) (*AlpacaResponse, error) {
	url := fmt.Sprintf("%s/api/v1/telescope/%d/%s", c.baseURL, c.deviceNumber, endpoint)
	
	// Add query parameters
	params := url
	if endpoint != "" {
		params += fmt.Sprintf("?ClientID=%d&ClientTransactionID=%d", c.clientID, c.getTransactionID())
	}
	
	resp, err := c.httpClient.Get(params)
	if err != nil {
		return nil, fmt.Errorf("alpaca GET request failed: %w", err)
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	
	var alpacaResp AlpacaResponse
	if err := json.Unmarshal(body, &alpacaResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}
	
	if alpacaResp.ErrorNumber != 0 {
		return nil, fmt.Errorf("alpaca error %d: %s", alpacaResp.ErrorNumber, alpacaResp.ErrorMessage)
	}
	
	return &alpacaResp, nil
}

// put performs a PUT request to the Alpaca API
func (c *TelescopeClient) put(endpoint string, params map[string]string) (*AlpacaResponse, error) {
	urlStr := fmt.Sprintf("%s/api/v1/telescope/%d/%s", c.baseURL, c.deviceNumber, endpoint)
	
	// Build form data
	formData := url.Values{}
	formData.Set("ClientID", strconv.Itoa(c.clientID))
	formData.Set("ClientTransactionID", strconv.Itoa(c.getTransactionID()))
	for k, v := range params {
		formData.Set(k, v)
	}
	
	resp, err := c.httpClient.PostForm(urlStr, formData)
	if err != nil {
		return nil, fmt.Errorf("alpaca PUT request failed: %w", err)
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	
	// Handle empty response (some ASCOM commands return no content on success)
	if len(body) == 0 {
		// Check HTTP status code
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			// Success with no content - return empty response
			return &AlpacaResponse{ErrorNumber: 0}, nil
		}
		return nil, fmt.Errorf("empty response with status %d", resp.StatusCode)
	}
	
	var alpacaResp AlpacaResponse
	if err := json.Unmarshal(body, &alpacaResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w (body: %s)", err, string(body))
	}
	
	if alpacaResp.ErrorNumber != 0 {
		return nil, fmt.Errorf("alpaca error %d: %s", alpacaResp.ErrorNumber, alpacaResp.ErrorMessage)
	}
	
	return &alpacaResp, nil
}

// IsConnected checks if the telescope is connected
func (c *TelescopeClient) IsConnected() (bool, error) {
	resp, err := c.get("connected")
	if err != nil {
		return false, err
	}
	
	connected, ok := resp.Value.(bool)
	if !ok {
		return false, fmt.Errorf("unexpected response type for connected")
	}
	
	return connected, nil
}

// GetStatus retrieves the current telescope status
func (c *TelescopeClient) GetStatus() (*TelescopeStatus, error) {
	// Get all status fields
	tracking, err := c.getBool("tracking")
	if err != nil {
		return nil, fmt.Errorf("failed to get tracking: %w", err)
	}
	
	slewing, err := c.getBool("slewing")
	if err != nil {
		return nil, fmt.Errorf("failed to get slewing: %w", err)
	}
	
	atPark, err := c.getBool("atpark")
	if err != nil {
		// Some telescopes don't support parking
		atPark = false
	}
	
	altitude, err := c.getFloat64("altitude")
	if err != nil {
		return nil, fmt.Errorf("failed to get altitude: %w", err)
	}
	
	azimuth, err := c.getFloat64("azimuth")
	if err != nil {
		return nil, fmt.Errorf("failed to get azimuth: %w", err)
	}
	
	ra, err := c.getFloat64("rightascension")
	if err != nil {
		ra = 0 // Default if not available
	}
	
	dec, err := c.getFloat64("declination")
	if err != nil {
		dec = 0 // Default if not available
	}
	
	connected, _ := c.IsConnected()
	
	return &TelescopeStatus{
		Connected:      connected,
		Tracking:       tracking,
		Slewing:        slewing,
		AtPark:         atPark,
		Altitude:       altitude,
		Azimuth:        azimuth,
		RightAscension: ra,
		Declination:    dec,
	}, nil
}

// SlewToAltAz slews the telescope to the specified altitude and azimuth
// Uses async slew to return immediately without blocking
func (c *TelescopeClient) SlewToAltAz(altitude, azimuth float64) error {
	params := map[string]string{
		"Altitude": fmt.Sprintf("%.6f", altitude),
		"Azimuth":  fmt.Sprintf("%.6f", azimuth),
	}
	
	// Use async endpoint to avoid blocking until slew completes
	_, err := c.put("slewtoaltazasync", params)
	return err
}

// AbortSlew stops any current slewing operation
func (c *TelescopeClient) AbortSlew() error {
	_, err := c.put("abortslew", nil)
	return err
}

// SetTracking enables or disables telescope tracking
func (c *TelescopeClient) SetTracking(enabled bool) error {
	params := map[string]string{
		"Tracking": strconv.FormatBool(enabled),
	}
	
	_, err := c.put("tracking", params)
	return err
}

// GetCapabilities retrieves the telescope's capabilities
func (c *TelescopeClient) GetCapabilities() (*TelescopeCapabilities, error) {
	description, _ := c.getString("description")
	driverInfo, _ := c.getString("driverinfo")
	interfaceVersion, _ := c.getInt("interfaceversion")
	canSetTracking, _ := c.getBool("cansettracking")
	canSlew, _ := c.getBool("canslew")
	canSlewAltAz, _ := c.getBool("canslewaltaz")
	
	supportedActionsResp, _ := c.get("supportedactions")
	var supportedActions []string
	if supportedActionsResp != nil {
		if actions, ok := supportedActionsResp.Value.([]interface{}); ok {
			for _, action := range actions {
				if str, ok := action.(string); ok {
					supportedActions = append(supportedActions, str)
				}
			}
		}
	}
	
	return &TelescopeCapabilities{
		Description:      description,
		DriverInfo:       driverInfo,
		InterfaceVersion: interfaceVersion,
		CanSetTracking:   canSetTracking,
		CanSlew:          canSlew,
		CanSlewAltAz:     canSlewAltAz,
		SupportedActions: supportedActions,
	}, nil
}

// Helper methods
func (c *TelescopeClient) getBool(endpoint string) (bool, error) {
	resp, err := c.get(endpoint)
	if err != nil {
		return false, err
	}
	
	value, ok := resp.Value.(bool)
	if !ok {
		return false, fmt.Errorf("unexpected response type for %s", endpoint)
	}
	
	return value, nil
}

func (c *TelescopeClient) getFloat64(endpoint string) (float64, error) {
	resp, err := c.get(endpoint)
	if err != nil {
		return 0, err
	}
	
	// JSON numbers can be float64 or int
	switch v := resp.Value.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	default:
		return 0, fmt.Errorf("unexpected response type for %s: %T", endpoint, resp.Value)
	}
}

func (c *TelescopeClient) getString(endpoint string) (string, error) {
	resp, err := c.get(endpoint)
	if err != nil {
		return "", err
	}
	
	value, ok := resp.Value.(string)
	if !ok {
		return "", fmt.Errorf("unexpected response type for %s", endpoint)
	}
	
	return value, nil
}

func (c *TelescopeClient) getInt(endpoint string) (int, error) {
	resp, err := c.get(endpoint)
	if err != nil {
		return 0, err
	}
	
	switch v := resp.Value.(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	case int64:
		return int(v), nil
	default:
		return 0, fmt.Errorf("unexpected response type for %s: %T", endpoint, resp.Value)
	}
}
