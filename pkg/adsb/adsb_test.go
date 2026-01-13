package adsb

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestNewAirplanesLiveClient tests client construction.
func TestNewAirplanesLiveClient(t *testing.T) {
	client := NewAirplanesLiveClient("https://api.test.com")

	if client == nil {
		t.Fatal("Expected client, got nil")
	}
	if client.baseURL != "https://api.test.com" {
		t.Errorf("Expected baseURL https://api.test.com, got %s", client.baseURL)
	}
	if client.httpClient == nil {
		t.Error("Expected HTTP client to be initialized")
	}
	if client.httpClient.Timeout != 10*time.Second {
		t.Errorf("Expected timeout 10s, got %v", client.httpClient.Timeout)
	}
}

// TestGetAircraft tests fetching aircraft within a radius.
func TestGetAircraft(t *testing.T) {
	t.Run("Successful request", func(t *testing.T) {
		// Create test server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify request path
			expectedPath := "/point/35.0000/-80.0000/100"
			if r.URL.Path != expectedPath {
				t.Errorf("Expected path %s, got %s", expectedPath, r.URL.Path)
			}

			// Send mock response
			response := airplanesLiveResponse{
				Aircraft: []airplanesLiveAircraft{
					{
						Hex:      "a12345",
						Flight:   strPtr("UAL123"),
						Lat:      floatPtr(35.5),
						Lon:      floatPtr(-80.5),
						AltBaro:  30000.0,
						Gs:       floatPtr(450.0),
						Track:    floatPtr(90.0),
						BaroRate: floatPtr(0.0),
						Seen:     floatPtr(2.5),
					},
				},
				Total: 1,
			}
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client := NewAirplanesLiveClient(server.URL)
		aircraft, err := client.GetAircraft(35.0, -80.0, 100)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if len(aircraft) != 1 {
			t.Fatalf("Expected 1 aircraft, got %d", len(aircraft))
		}

		// Verify parsed data
		ac := aircraft[0]
		if ac.ICAO != "a12345" {
			t.Errorf("Expected ICAO a12345, got %s", ac.ICAO)
		}
		if ac.Callsign != "UAL123" {
			t.Errorf("Expected callsign UAL123, got %s", ac.Callsign)
		}
		if ac.Latitude != 35.5 {
			t.Errorf("Expected latitude 35.5, got %f", ac.Latitude)
		}
		if ac.Altitude != 30000.0 {
			t.Errorf("Expected altitude 30000, got %f", ac.Altitude)
		}
	})

	t.Run("Caps radius at 250 NM", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Should cap radius
			if r.URL.Path != "/point/35.0000/-80.0000/250" {
				t.Errorf("Expected radius capped at 250, got path %s", r.URL.Path)
			}
			json.NewEncoder(w).Encode(airplanesLiveResponse{Aircraft: []airplanesLiveAircraft{}})
		}))
		defer server.Close()

		client := NewAirplanesLiveClient(server.URL)
		_, err := client.GetAircraft(35.0, -80.0, 500) // Request 500 NM

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
	})

	t.Run("Handles rate limit error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "30")
			w.Header().Set("X-Rate-Limit-Limit", "100")
			w.Header().Set("X-Rate-Limit-Remaining", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("Rate limit exceeded"))
		}))
		defer server.Close()

		client := NewAirplanesLiveClient(server.URL)
		_, err := client.GetAircraft(35.0, -80.0, 100)

		if err == nil {
			t.Fatal("Expected rate limit error, got nil")
		}

		rle, ok := IsRateLimitError(err)
		if !ok {
			t.Fatal("Expected RateLimitError type")
		}
		if rle.StatusCode != 429 {
			t.Errorf("Expected status 429, got %d", rle.StatusCode)
		}
		if rle.RetryAfter != 30*time.Second {
			t.Errorf("Expected retry after 30s, got %v", rle.RetryAfter)
		}
		if rle.Headers.Limit != 100 {
			t.Errorf("Expected limit 100, got %d", rle.Headers.Limit)
		}
	})

	t.Run("Handles HTTP error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal error"))
		}))
		defer server.Close()

		client := NewAirplanesLiveClient(server.URL)
		_, err := client.GetAircraft(35.0, -80.0, 100)

		if err == nil {
			t.Fatal("Expected error, got nil")
		}
	})

	t.Run("Skips aircraft with missing position", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := airplanesLiveResponse{
				Aircraft: []airplanesLiveAircraft{
					{Hex: "a11111", Lat: floatPtr(35.0), Lon: floatPtr(-80.0)}, // Valid
					{Hex: "a22222", Lat: nil, Lon: floatPtr(-80.0)},            // Missing lat
					{Hex: "a33333", Lat: floatPtr(35.0), Lon: nil},             // Missing lon
					{Hex: "a44444", Lat: floatPtr(36.0), Lon: floatPtr(-81.0)}, // Valid
				},
				Total: 4,
			}
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client := NewAirplanesLiveClient(server.URL)
		aircraft, err := client.GetAircraft(35.0, -80.0, 100)

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		// Should only get 2 valid aircraft
		if len(aircraft) != 2 {
			t.Errorf("Expected 2 valid aircraft, got %d", len(aircraft))
		}
	})
}

// TestGetAircraftByICAO tests fetching a specific aircraft.
func TestGetAircraftByICAO(t *testing.T) {
	t.Run("Found aircraft", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			expectedPath := "/hex/a12345"
			if r.URL.Path != expectedPath {
				t.Errorf("Expected path %s, got %s", expectedPath, r.URL.Path)
			}

			response := airplanesLiveResponse{
				Aircraft: []airplanesLiveAircraft{
					{
						Hex:    "a12345",
						Flight: strPtr("DAL456"),
						Lat:    floatPtr(40.0),
						Lon:    floatPtr(-75.0),
					},
				},
			}
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client := NewAirplanesLiveClient(server.URL)
		aircraft, err := client.GetAircraftByICAO("a12345")

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if aircraft == nil {
			t.Fatal("Expected aircraft, got nil")
		}
		if aircraft.ICAO != "a12345" {
			t.Errorf("Expected ICAO a12345, got %s", aircraft.ICAO)
		}
		if aircraft.Callsign != "DAL456" {
			t.Errorf("Expected callsign DAL456, got %s", aircraft.Callsign)
		}
	})

	t.Run("Aircraft not found", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := airplanesLiveResponse{
				Aircraft: []airplanesLiveAircraft{},
				Total:    0,
			}
			json.NewEncoder(w).Encode(response)
		}))
		defer server.Close()

		client := NewAirplanesLiveClient(server.URL)
		aircraft, err := client.GetAircraftByICAO("unknown")

		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if aircraft != nil {
			t.Error("Expected nil for not found aircraft")
		}
	})
}

// TestClose tests the Close method.
func TestClose(t *testing.T) {
	client := NewAirplanesLiveClient("https://api.test.com")
	err := client.Close()
	if err != nil {
		t.Errorf("Expected no error from Close(), got: %v", err)
	}
}

// TestParseAltitude tests altitude parsing from interface{}.
func TestParseAltitude(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected *float64
	}{
		{"nil input", nil, nil},
		{"float64 altitude", 35000.0, floatPtr(35000.0)},
		{"ground string", "ground", floatPtr(0.0)},
		{"invalid string", "invalid", nil},
		{"invalid type", 123, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAltitude(tt.input)
			if tt.expected == nil {
				if result != nil {
					t.Errorf("Expected nil, got %v", *result)
				}
			} else {
				if result == nil {
					t.Error("Expected value, got nil")
				} else if *result != *tt.expected {
					t.Errorf("Expected %f, got %f", *tt.expected, *result)
				}
			}
		})
	}
}

// TestConvertAirplanesLiveAircraft tests data conversion.
func TestConvertAirplanesLiveAircraft(t *testing.T) {
	now := time.Now().UTC()

	input := airplanesLiveAircraft{
		Hex:      "abc123",
		Flight:   strPtr("TEST123"),
		Lat:      floatPtr(35.1234),
		Lon:      floatPtr(-80.5678),
		AltGeom:  35000.0,
		Gs:       floatPtr(450.5),
		Track:    floatPtr(270.0),
		BaroRate: floatPtr(1500.0),
		Seen:     floatPtr(3.0),
	}

	result := convertAirplanesLiveAircraft(input)

	if result.ICAO != "abc123" {
		t.Errorf("Expected ICAO abc123, got %s", result.ICAO)
	}
	if result.Callsign != "TEST123" {
		t.Errorf("Expected callsign TEST123, got %s", result.Callsign)
	}
	if result.Latitude != 35.1234 {
		t.Errorf("Expected latitude 35.1234, got %f", result.Latitude)
	}
	if result.Altitude != 35000.0 {
		t.Errorf("Expected altitude 35000, got %f", result.Altitude)
	}
	if result.GroundSpeed != 450.5 {
		t.Errorf("Expected ground speed 450.5, got %f", result.GroundSpeed)
	}
	if result.VerticalRate != 1500.0 {
		t.Errorf("Expected vertical rate 1500, got %f", result.VerticalRate)
	}

	// Verify LastSeen is approximately 3 seconds ago
	expectedTime := now.Add(-3 * time.Second)
	if result.LastSeen.Sub(expectedTime).Abs() > time.Second {
		t.Errorf("LastSeen not within expected range: %v", result.LastSeen)
	}
}

// TestParseRetryAfter tests Retry-After header parsing.
func TestParseRetryAfter(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		expected time.Duration
	}{
		{"Empty header", "", 0},
		{"Delay seconds", "30", 30 * time.Second},
		{"Zero seconds", "0", 0},
		{"Negative (invalid)", "-10", 0},
		{"HTTP date", "Wed, 21 Oct 2015 07:28:00 GMT", 0}, // Will be 0 since it's in the past
		{"Invalid string", "invalid", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := http.Header{}
			if tt.header != "" {
				headers.Set("Retry-After", tt.header)
			}

			result := parseRetryAfter(headers)
			if tt.name == "HTTP date" {
				// Skip exact comparison for dates (will be past)
				return
			}
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestExtractRateLimitHeaders tests rate limit header extraction.
func TestExtractRateLimitHeaders(t *testing.T) {
	t.Run("Standard headers", func(t *testing.T) {
		headers := http.Header{}
		headers.Set("X-Rate-Limit-Limit", "100")
		headers.Set("X-Rate-Limit-Remaining", "25")
		headers.Set("X-Rate-Limit-Reset", "1609459200")

		result := extractRateLimitHeaders(headers)

		if result.Limit != 100 {
			t.Errorf("Expected limit 100, got %d", result.Limit)
		}
		if result.Remaining != 25 {
			t.Errorf("Expected remaining 25, got %d", result.Remaining)
		}
		expectedReset := time.Unix(1609459200, 0)
		if !result.Reset.Equal(expectedReset) {
			t.Errorf("Expected reset %v, got %v", expectedReset, result.Reset)
		}
	})

	t.Run("Alternative header names", func(t *testing.T) {
		headers := http.Header{}
		headers.Set("X-RateLimit-Limit", "200")
		headers.Set("X-RateLimit-Remaining", "50")

		result := extractRateLimitHeaders(headers)

		if result.Limit != 200 {
			t.Errorf("Expected limit 200, got %d", result.Limit)
		}
		if result.Remaining != 50 {
			t.Errorf("Expected remaining 50, got %d", result.Remaining)
		}
	})

	t.Run("Missing headers", func(t *testing.T) {
		headers := http.Header{}

		result := extractRateLimitHeaders(headers)

		if result.Limit != -1 {
			t.Errorf("Expected limit -1, got %d", result.Limit)
		}
		if result.Remaining != -1 {
			t.Errorf("Expected remaining -1, got %d", result.Remaining)
		}
	})
}

// TestRateLimitError tests rate limit error handling.
func TestRateLimitError(t *testing.T) {
	t.Run("Error message with retry after", func(t *testing.T) {
		err := &RateLimitError{
			StatusCode: 429,
			RetryAfter: 30 * time.Second,
			Message:    "Rate limit exceeded",
		}

		expected := "Rate limit exceeded (retry after 30s)"
		if err.Error() != expected {
			t.Errorf("Expected %q, got %q", expected, err.Error())
		}
	})

	t.Run("Error message without retry after", func(t *testing.T) {
		err := &RateLimitError{
			StatusCode: 429,
			Message:    "Rate limit exceeded",
		}

		expected := "Rate limit exceeded"
		if err.Error() != expected {
			t.Errorf("Expected %q, got %q", expected, err.Error())
		}
	})

	t.Run("IsRateLimitError check", func(t *testing.T) {
		err := &RateLimitError{StatusCode: 429}
		rle, ok := IsRateLimitError(err)
		if !ok {
			t.Error("Expected true for RateLimitError")
		}
		if rle.StatusCode != 429 {
			t.Errorf("Expected status 429, got %d", rle.StatusCode)
		}

		normalErr := fmt.Errorf("normal error")
		_, ok = IsRateLimitError(normalErr)
		if ok {
			t.Error("Expected false for normal error")
		}
	})
}

// TestRateLimitWait tests rate limiting enforcement.
func TestRateLimitWait(t *testing.T) {
	client := NewAirplanesLiveClient("https://api.test.com")

	// First call should be immediate
	start := time.Now()
	client.rateLimitWait()
	elapsed := time.Since(start)
	if elapsed > 100*time.Millisecond {
		t.Error("First call should be immediate")
	}

	// Second call should wait ~1 second
	start = time.Now()
	client.rateLimitWait()
	elapsed = time.Since(start)
	if elapsed < 900*time.Millisecond || elapsed > 1100*time.Millisecond {
		t.Errorf("Expected ~1s wait, got %v", elapsed)
	}
}

// Helper functions
func strPtr(s string) *string {
	return &s
}

func floatPtr(f float64) *float64 {
	return &f
}
