// Package flightaware provides a client for the FlightAware AeroAPI v4.
//
// The AeroAPI provides access to flight tracking data, flight plans, aircraft
// information, and more. This client focuses on flight plan retrieval for
// enhanced prediction algorithms.
//
// API Documentation: https://www.flightaware.com/aeroapi/portal/documentation
// Rate Limits: Free tier allows 500 requests/month, paid tiers offer higher limits.
package flightaware

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

const (
	// BaseURL is the FlightAware AeroAPI v4 base URL
	BaseURL = "https://aeroapi.flightaware.com/aeroapi"

	// DefaultTimeout for API requests
	DefaultTimeout = 10 * time.Second
)

// Client represents a FlightAware AeroAPI client.
type Client struct {
	apiKey      string
	httpClient  *http.Client
	rateLimiter *rate.Limiter
	baseURL     string
}

// Config contains configuration for the FlightAware client.
type Config struct {
	APIKey          string
	RequestsPerHour int
	Timeout         time.Duration
}

// NewClient creates a new FlightAware AeroAPI client.
//
// The client includes:
// - Rate limiting to prevent exceeding API quotas
// - Configurable timeout for requests
// - Automatic retry logic for transient failures (TODO)
func NewClient(cfg Config) *Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}

	if cfg.RequestsPerHour == 0 {
		// Default: 500 requests/month ≈ 0.7 requests/hour, use 1 req/hour as safe default
		cfg.RequestsPerHour = 1
	}

	// Convert requests per hour to rate limiter (allows burst of 1)
	requestsPerSecond := float64(cfg.RequestsPerHour) / 3600.0
	limiter := rate.NewLimiter(rate.Limit(requestsPerSecond), 1)

	return &Client{
		apiKey: cfg.APIKey,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		rateLimiter: limiter,
		baseURL:     BaseURL,
	}
}

// FlightPlan represents a filed flight plan from AeroAPI.
type FlightPlan struct {
	// Identifiers
	ICAO       string `json:"ident"`        // Aircraft identifier (callsign)
	FAFlightID string `json:"fa_flight_id"` // FlightAware flight ID

	// Route information
	Departure struct {
		Code string `json:"code_icao"` // ICAO airport code (e.g., "KCLT")
		Name string `json:"name"`
	} `json:"origin"`

	Arrival struct {
		Code string `json:"code_icao"` // ICAO airport code (e.g., "KATL")
		Name string `json:"name"`
	} `json:"destination"`

	// Route string in ICAO format (e.g., "KCLT..CHSLY.J121.ATL..KATL")
	// May be empty if flight plan not filed or not available
	RouteString string `json:"route"`

	// Altitude and aircraft
	FiledAltitude int    `json:"filed_altitude"` // Feet MSL
	AircraftType  string `json:"aircraft_type"`  // ICAO aircraft type (e.g., "B738")

	// Timing
	FiledTime time.Time `json:"filed_time"`
	ETD       time.Time `json:"estimated_time_departure"`
	ETA       time.Time `json:"estimated_time_arrival"`

	// Status
	Status string `json:"status"` // e.g., "Scheduled", "Active", "Arrived"
}

// FlightInfo represents basic flight information from AeroAPI.
type FlightInfo struct {
	ICAO         string    `json:"ident"`
	FAFlightID   string    `json:"fa_flight_id"`
	Registration string    `json:"registration"`
	AircraftType string    `json:"aircraft_type"`
	FiledTime    time.Time `json:"filed_time"`

	Origin struct {
		Code string `json:"code_icao"`
	} `json:"origin"`

	Destination struct {
		Code string `json:"code_icao"`
	} `json:"destination"`

	RouteDistance int    `json:"route_distance"` // Nautical miles
	FiledAltitude int    `json:"filed_altitude"` // Feet MSL
	Status        string `json:"status"`
}

// GetFlightPlanByCallsign retrieves the flight plan for a given callsign.
//
// The callsign should be the aircraft's identifier (e.g., "UAL123", "N12345").
// If multiple flights exist, this returns the most recent active flight.
//
// Returns nil, nil if no flight plan is found (not an error).
// Returns error for API failures or network issues.
func (c *Client) GetFlightPlanByCallsign(ctx context.Context, callsign string) (*FlightPlan, error) {
	// Wait for rate limiter
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	// AeroAPI endpoint: /flights/{ident}
	// Returns array of flights, we want the most recent active one
	url := fmt.Sprintf("%s/flights/%s", c.baseURL, callsign)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("x-apikey", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// Handle HTTP errors
	if resp.StatusCode == 404 {
		return nil, nil // No flight found, not an error
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	// Parse response - API returns array of flights
	var response struct {
		Flights []FlightPlan `json:"flights"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if len(response.Flights) == 0 {
		return nil, nil // No flights found
	}

	// Return the first (most recent) flight
	// TODO: Filter for active/scheduled flights only?
	return &response.Flights[0], nil
}

// GetFlightInfoByICAO retrieves basic flight information by ICAO hex code.
//
// This is useful when we only have the Mode-S transponder code (ICAO 24-bit address).
// Note: This requires a different API endpoint that maps ICAO hex to flights.
//
// For now, this is a placeholder. The AeroAPI doesn't directly support ICAO hex lookup.
// We need to use the /flights endpoint with callsign instead.
func (c *Client) GetFlightInfoByICAO(ctx context.Context, icaoHex string) (*FlightInfo, error) {
	// TODO: Implement ICAO hex to callsign mapping
	// This may require maintaining a cache of ICAO hex -> callsign mappings
	// from the ADS-B data we already collect.
	return nil, fmt.Errorf("ICAO hex lookup not yet implemented")
}

// GetRoute retrieves the detailed route for a flight by FlightAware flight ID.
//
// This provides waypoint-by-waypoint information including ETAs.
// The fa_flight_id can be obtained from GetFlightPlanByCallsign.
func (c *Client) GetRoute(ctx context.Context, faFlightID string) ([]Waypoint, error) {
	// Wait for rate limiter
	if err := c.rateLimiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limiter: %w", err)
	}

	url := fmt.Sprintf("%s/flights/%s/route", c.baseURL, faFlightID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("x-apikey", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var response struct {
		Waypoints []Waypoint `json:"waypoints"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return response.Waypoints, nil
}

// Waypoint represents a waypoint along a flight route.
type Waypoint struct {
	Name      string     `json:"name"`      // Waypoint identifier (e.g., "CHSLY")
	Latitude  float64    `json:"latitude"`  // Decimal degrees
	Longitude float64    `json:"longitude"` // Decimal degrees
	Type      string     `json:"type"`      // e.g., "fix", "vor", "airport"
	ETA       *time.Time `json:"eta"`       // Estimated time of arrival at waypoint (may be null)
}
