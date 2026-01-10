package adsb

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// AirplanesLiveClient implements the DataSource interface for airplanes.live API.
// API Documentation: https://airplanes.live/api-guide/
// Rate Limit: 1 request per second
type AirplanesLiveClient struct {
	// baseURL is the API base URL (default: https://api.airplanes.live/v2)
	baseURL string

	// httpClient is the HTTP client used for API requests
	httpClient *http.Client

	// lastRequest tracks the last API call time for rate limiting
	lastRequest time.Time
}

// NewAirplanesLiveClient creates a new airplanes.live API client.
// baseURL should be "https://api.airplanes.live/v2" (or custom for testing)
func NewAirplanesLiveClient(baseURL string) *AirplanesLiveClient {
	return &AirplanesLiveClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		lastRequest: time.Time{},
	}
}

// GetAircraft returns all aircraft within a radius of a given point.
// Uses the /point/[lat]/[lon]/[radius] endpoint.
// Maximum radius is 250 nautical miles.
//
// centerLat/centerLon: Center point in decimal degrees
// radiusNM: Search radius in nautical miles (max 250)
func (c *AirplanesLiveClient) GetAircraft(centerLat, centerLon, radiusNM float64) ([]Aircraft, error) {
	// Enforce maximum radius
	if radiusNM > 250.0 {
		radiusNM = 250.0
	}

	// Apply rate limiting: max 1 request per second
	c.rateLimitWait()

	// Build API URL
	url := fmt.Sprintf("%s/point/%.4f/%.4f/%.0f", c.baseURL, centerLat, centerLon, radiusNM)

	// Make API request
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch aircraft data: %w", err)
	}
	defer resp.Body.Close()

	// Check for rate limit (HTTP 429)
	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := parseRetryAfter(resp.Header)
		return nil, &RateLimitError{
			StatusCode: resp.StatusCode,
			RetryAfter: retryAfter,
			Message:    "Rate limit exceeded",
			Headers:    extractRateLimitHeaders(resp.Header),
		}
	}
	
	// Check other error status codes
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var apiResp airplanesLiveResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}

	// Convert to our Aircraft type
	aircraft := make([]Aircraft, 0, len(apiResp.Aircraft))
	for _, ac := range apiResp.Aircraft {
		// Skip aircraft with invalid data
		if ac.Lat == nil || ac.Lon == nil {
			continue
		}

		aircraft = append(aircraft, convertAirplanesLiveAircraft(ac))
	}

	return aircraft, nil
}

// GetAircraftByICAO returns a specific aircraft by its ICAO hex code.
// Uses the /hex/[hex] endpoint.
func (c *AirplanesLiveClient) GetAircraftByICAO(icao string) (*Aircraft, error) {
	// Apply rate limiting
	c.rateLimitWait()

	// Build API URL
	url := fmt.Sprintf("%s/hex/%s", c.baseURL, icao)

	// Make API request
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch aircraft data: %w", err)
	}
	defer resp.Body.Close()

	// Check for rate limit (HTTP 429)
	if resp.StatusCode == http.StatusTooManyRequests {
		retryAfter := parseRetryAfter(resp.Header)
		return nil, &RateLimitError{
			StatusCode: resp.StatusCode,
			RetryAfter: retryAfter,
			Message:    "Rate limit exceeded",
			Headers:    extractRateLimitHeaders(resp.Header),
		}
	}
	
	// Check other error status codes
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	// Parse response
	var apiResp airplanesLiveResponse
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse API response: %w", err)
	}

	// Check if aircraft was found
	if len(apiResp.Aircraft) == 0 {
		return nil, nil
	}

	// Return first match
	ac := convertAirplanesLiveAircraft(apiResp.Aircraft[0])
	return &ac, nil
}

// Close cleanly shuts down the client.
// For airplanes.live, this is a no-op as there are no persistent connections.
func (c *AirplanesLiveClient) Close() error {
	return nil
}

// rateLimitWait enforces the 1 request per second rate limit.
func (c *AirplanesLiveClient) rateLimitWait() {
	if !c.lastRequest.IsZero() {
		elapsed := time.Since(c.lastRequest)
		if elapsed < time.Second {
			time.Sleep(time.Second - elapsed)
		}
	}
	c.lastRequest = time.Now()
}

// airplanesLiveResponse represents the JSON response from airplanes.live API.
type airplanesLiveResponse struct {
	// Aircraft is the array of aircraft data
	Aircraft []airplanesLiveAircraft `json:"ac"`

	// Total number of aircraft
	Total int `json:"total"`

	// Current timestamp
	Now float64 `json:"now"`

	// Message count
	Messages int `json:"messages"`
}

// airplanesLiveAircraft represents a single aircraft in the airplanes.live API response.
// Field documentation: https://airplanes.live/adsb-field-explanations/
type airplanesLiveAircraft struct {
	// Hex is the ICAO Mode S hex code (e.g., "a12345")
	Hex string `json:"hex"`

	// Flight is the callsign/flight number
	Flight *string `json:"flight"`

	// Lat is latitude in decimal degrees
	Lat *float64 `json:"lat"`

	// Lon is longitude in decimal degrees
	Lon *float64 `json:"lon"`

	// AltBaro is barometric altitude in feet
	// Note: Can be string "ground" or float
	AltBaro interface{} `json:"alt_baro"`

	// AltGeom is geometric (GPS) altitude in feet
	// Note: Can be string "ground" or float
	AltGeom interface{} `json:"alt_geom"`

	// Gs is ground speed in knots
	Gs *float64 `json:"gs"`

	// Track is ground track in degrees (0-360)
	Track *float64 `json:"track"`

	// BaroRate is barometric vertical rate in feet/minute
	BaroRate *float64 `json:"baro_rate"`

	// Seen is seconds since last position update
	Seen *float64 `json:"seen"`

	// SeenPos is seconds since last position message
	SeenPos *float64 `json:"seen_pos"`
}

// convertAirplanesLiveAircraft converts an airplanes.live aircraft to our Aircraft type.
func convertAirplanesLiveAircraft(ac airplanesLiveAircraft) Aircraft {
	aircraft := Aircraft{
		ICAO: ac.Hex,
	}

	// Callsign (trim whitespace)
	if ac.Flight != nil {
		aircraft.Callsign = *ac.Flight
	}

	// Position
	if ac.Lat != nil {
		aircraft.Latitude = *ac.Lat
	}
	if ac.Lon != nil {
		aircraft.Longitude = *ac.Lon
	}

	// Altitude - prefer geometric (GPS) over barometric
	// Handle interface{} which can be float64 or string ("ground")
	if alt := parseAltitude(ac.AltGeom); alt != nil {
		aircraft.Altitude = *alt
	} else if alt := parseAltitude(ac.AltBaro); alt != nil {
		aircraft.Altitude = *alt
	}

	// Velocity
	if ac.Gs != nil {
		aircraft.GroundSpeed = *ac.Gs
	}
	if ac.Track != nil {
		aircraft.Track = *ac.Track
	}
	if ac.BaroRate != nil {
		aircraft.VerticalRate = *ac.BaroRate
	}

	// Timestamp - calculate from "seen" seconds ago
	if ac.Seen != nil {
		seenDuration := time.Duration(*ac.Seen * float64(time.Second))
		aircraft.LastSeen = time.Now().UTC().Add(-seenDuration)
	} else {
		aircraft.LastSeen = time.Now().UTC()
	}

	return aircraft
}

// parseAltitude safely extracts altitude from interface{} which can be float64 or string.
// Returns nil if the value is invalid or represents "ground".
func parseAltitude(val interface{}) *float64 {
	if val == nil {
		return nil
	}

	switch v := val.(type) {
	case float64:
		return &v
	case string:
		// "ground" means altitude is 0 or on ground
		if v == "ground" {
			zero := 0.0
			return &zero
		}
		return nil
	default:
		return nil
	}
}

// RateLimitError represents an HTTP 429 rate limit error with retry information.
type RateLimitError struct {
	StatusCode int
	RetryAfter time.Duration
	Message    string
	Headers    RateLimitHeaders
}

// RateLimitHeaders contains rate limit information from response headers.
type RateLimitHeaders struct {
	Limit     int // X-Rate-Limit-Limit: Maximum requests allowed
	Remaining int // X-Rate-Limit-Remaining: Requests remaining in current window
	Reset     time.Time // X-Rate-Limit-Reset: When the rate limit resets
}

func (e *RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("%s (retry after %v)", e.Message, e.RetryAfter)
	}
	return e.Message
}

// IsRateLimitError checks if an error is a rate limit error.
func IsRateLimitError(err error) (*RateLimitError, bool) {
	if rle, ok := err.(*RateLimitError); ok {
		return rle, true
	}
	return nil, false
}

// parseRetryAfter extracts the Retry-After header value.
// Returns the duration to wait, or 0 if header is not present.
// Supports both delay-seconds (integer) and HTTP-date formats.
//
// Examples:
//   Retry-After: 30                           -> 30 seconds
//   Retry-After: Wed, 21 Oct 2015 07:28:00 GMT -> duration until that time
func parseRetryAfter(headers http.Header) time.Duration {
	retryAfter := headers.Get("Retry-After")
	if retryAfter == "" {
		return 0
	}
	
	// Try parsing as delay-seconds (e.g., "30")
	if seconds, err := strconv.Atoi(retryAfter); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	
	// Try parsing as HTTP-date (e.g., "Wed, 21 Oct 2015 07:28:00 GMT")
	if retryTime, err := http.ParseTime(retryAfter); err == nil {
		duration := time.Until(retryTime)
		if duration > 0 {
			return duration
		}
	}
	
	return 0
}

// extractRateLimitHeaders extracts common rate limit headers from the response.
// These headers help understand the current rate limit status.
func extractRateLimitHeaders(headers http.Header) RateLimitHeaders {
	rlh := RateLimitHeaders{
		Limit:     -1,
		Remaining: -1,
	}
	
	// X-Rate-Limit-Limit or X-RateLimit-Limit
	if limit := headers.Get("X-Rate-Limit-Limit"); limit != "" {
		if val, err := strconv.Atoi(limit); err == nil {
			rlh.Limit = val
		}
	} else if limit := headers.Get("X-RateLimit-Limit"); limit != "" {
		if val, err := strconv.Atoi(limit); err == nil {
			rlh.Limit = val
		}
	}
	
	// X-Rate-Limit-Remaining or X-RateLimit-Remaining
	if remaining := headers.Get("X-Rate-Limit-Remaining"); remaining != "" {
		if val, err := strconv.Atoi(remaining); err == nil {
			rlh.Remaining = val
		}
	} else if remaining := headers.Get("X-RateLimit-Remaining"); remaining != "" {
		if val, err := strconv.Atoi(remaining); err == nil {
			rlh.Remaining = val
		}
	}
	
	// X-Rate-Limit-Reset or X-RateLimit-Reset (Unix timestamp)
	if reset := headers.Get("X-Rate-Limit-Reset"); reset != "" {
		if timestamp, err := strconv.ParseInt(reset, 10, 64); err == nil {
			rlh.Reset = time.Unix(timestamp, 0)
		}
	} else if reset := headers.Get("X-RateLimit-Reset"); reset != "" {
		if timestamp, err := strconv.ParseInt(reset, 10, 64); err == nil {
			rlh.Reset = time.Unix(timestamp, 0)
		}
	}
	
	return rlh
}
