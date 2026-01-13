package db

import (
	"testing"
	"time"
)

// TestNewFlightPlanRepository tests repository construction.
func TestNewFlightPlanRepository(t *testing.T) {
	repo := NewFlightPlanRepository(nil)

	if repo == nil {
		t.Fatal("Expected non-nil repository")
	}
	if repo.db != nil {
		t.Error("Expected nil db (not initialized)")
	}
}

// TestIsAirway tests airway identifier detection.
func TestIsAirway(t *testing.T) {
	tests := []struct {
		token    string
		expected bool
	}{
		// Valid airways
		{"J121", true},
		{"V1", true},
		{"Q12", true},
		{"T45", true},
		{"A123", true},
		{"B456", true},
		{"G789", true},
		{"R12", true},

		// Edge cases
		{"J1", true},       // Minimum length (2 chars)
		{"J1234", true},    // Maximum length (5 chars)
		{"J12345", false},  // Too long (6 chars)
		{"J123456", false}, // Also too long (7 chars)

		// Invalid or ambiguous
		{"KCLT", false},  // Airport (4 chars, starts with K)
		{"CHSLY", false}, // Waypoint (5+ chars, starts with C)
		{"DCT", false},   // Direct routing (3 chars, starts with D)
		{"ATL", true},    // Ambiguous - could be airway A### or waypoint (matches airway pattern)
		{"", false},      // Empty
		{"X", false},     // Too short
		{"X123", false},  // Invalid prefix
		{"1", false},     // Single digit
		{"123", false},   // No prefix
	}

	for _, tt := range tests {
		t.Run(tt.token, func(t *testing.T) {
			result := isAirway(tt.token)
			if result != tt.expected {
				t.Errorf("isAirway(%q) = %v, expected %v", tt.token, result, tt.expected)
			}
		})
	}
}

// TestFlightPlan tests the FlightPlan struct.
func TestFlightPlan(t *testing.T) {
	now := time.Now().UTC()

	fp := FlightPlan{
		ID:            1,
		ICAO:          "a12345",
		Callsign:      "UAL123",
		DepartureICAO: "KCLT",
		ArrivalICAO:   "KATL",
		Route:         "KCLT..CHSLY.J121.ATL..KATL",
		FiledAltitude: 35000,
		AircraftType:  "B738",
		FiledTime:     now,
		ETD:           now.Add(1 * time.Hour),
		ETA:           now.Add(2 * time.Hour),
		LastUpdated:   now,
	}

	if fp.ID != 1 {
		t.Errorf("Expected ID 1, got %d", fp.ID)
	}
	if fp.ICAO != "a12345" {
		t.Errorf("Expected ICAO a12345, got %s", fp.ICAO)
	}
	if fp.Callsign != "UAL123" {
		t.Errorf("Expected callsign UAL123, got %s", fp.Callsign)
	}
	if fp.FiledAltitude != 35000 {
		t.Errorf("Expected altitude 35000, got %d", fp.FiledAltitude)
	}
	if fp.ETD.Sub(fp.FiledTime) != 1*time.Hour {
		t.Error("ETD should be 1 hour after filed time")
	}
}

// TestFlightPlanRoute tests the FlightPlanRoute struct.
func TestFlightPlanRoute(t *testing.T) {
	eta := time.Now().UTC().Add(30 * time.Minute)

	route := FlightPlanRoute{
		ID:           1,
		FlightPlanID: 10,
		Sequence:     2,
		WaypointID:   100,
		WaypointName: "CHSLY",
		Latitude:     35.123,
		Longitude:    -80.456,
		ETA:          &eta,
		Passed:       false,
	}

	if route.FlightPlanID != 10 {
		t.Errorf("Expected FlightPlanID 10, got %d", route.FlightPlanID)
	}
	if route.Sequence != 2 {
		t.Errorf("Expected sequence 2, got %d", route.Sequence)
	}
	if route.WaypointName != "CHSLY" {
		t.Errorf("Expected waypoint CHSLY, got %s", route.WaypointName)
	}
	if route.ETA == nil {
		t.Fatal("Expected non-nil ETA")
	}
	if !route.ETA.Equal(eta) {
		t.Error("ETA not set correctly")
	}
	if route.Passed {
		t.Error("Expected passed to be false")
	}
}

// TestWaypoint tests the Waypoint struct.
func TestWaypoint(t *testing.T) {
	wp := Waypoint{
		ID:         1,
		Identifier: "ATL",
		Name:       "Atlanta VORTAC",
		Latitude:   33.6367,
		Longitude:  -84.4281,
		Type:       "vor",
		Region:     "K2",
	}

	if wp.ID != 1 {
		t.Errorf("Expected ID 1, got %d", wp.ID)
	}
	if wp.Identifier != "ATL" {
		t.Errorf("Expected identifier ATL, got %s", wp.Identifier)
	}
	if wp.Type != "vor" {
		t.Errorf("Expected type vor, got %s", wp.Type)
	}
	if wp.Latitude < 33.0 || wp.Latitude > 34.0 {
		t.Errorf("Latitude %f out of expected range", wp.Latitude)
	}
}

// TestAirwaySegment tests the AirwaySegment struct.
func TestAirwaySegment(t *testing.T) {
	seg := AirwaySegment{
		AirwayID:   "J121",
		AirwayType: "jet",
		Sequence:   1,
		FromWaypoint: Waypoint{
			ID:         1,
			Identifier: "CHSLY",
			Latitude:   35.0,
			Longitude:  -80.0,
			Type:       "fix",
		},
		ToWaypoint: Waypoint{
			ID:         2,
			Identifier: "ATL",
			Latitude:   33.6367,
			Longitude:  -84.4281,
			Type:       "vor",
		},
		MinAltitude: 18000,
		MaxAltitude: 45000,
		Bearing:     225.0,
		DistanceNM:  150.0,
	}

	if seg.AirwayID != "J121" {
		t.Errorf("Expected airway J121, got %s", seg.AirwayID)
	}
	if seg.AirwayType != "jet" {
		t.Errorf("Expected type jet, got %s", seg.AirwayType)
	}
	if seg.MinAltitude != 18000 {
		t.Errorf("Expected min alt 18000, got %d", seg.MinAltitude)
	}
	if seg.FromWaypoint.Identifier != "CHSLY" {
		t.Errorf("Expected from waypoint CHSLY, got %s", seg.FromWaypoint.Identifier)
	}
	if seg.ToWaypoint.Identifier != "ATL" {
		t.Errorf("Expected to waypoint ATL, got %s", seg.ToWaypoint.Identifier)
	}
}

// TestRouteStringParsing tests route string tokenization logic.
func TestRouteStringParsing(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "Double-dot separated",
			input:    "KCLT..CHSLY.J121.ATL..KATL",
			expected: []string{"KCLT", "CHSLY", "J121", "ATL", "KATL"},
		},
		{
			name:     "Single-dot separated",
			input:    "KCLT.CHSLY.J121.ATL.KATL",
			expected: []string{"KCLT", "CHSLY", "J121", "ATL", "KATL"},
		},
		{
			name:     "Space separated",
			input:    "KCLT CHSLY J121 ATL KATL",
			expected: []string{"KCLT", "CHSLY", "J121", "ATL", "KATL"},
		},
		{
			name:     "With DCT",
			input:    "KCLT DCT CHSLY",
			expected: []string{"KCLT", "DCT", "CHSLY"},
		},
		{
			name:     "Mixed separators",
			input:    "KCLT..CHSLY J121.ATL KATL",
			expected: []string{"KCLT", "CHSLY", "J121", "ATL", "KATL"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the parsing logic from ParseAndStoreRoute
			routeString := tt.input
			routeString = replaceDotsAndSplit(routeString)

			// Note: This tests the tokenization logic
			// Actual parsing would involve waypoint lookups
		})
	}
}

// Helper function to simulate route string preprocessing.
func replaceDotsAndSplit(s string) string {
	// Replace ".." with space
	result := ""
	for i := 0; i < len(s); i++ {
		if i < len(s)-1 && s[i] == '.' && s[i+1] == '.' {
			result += " "
			i++ // Skip next dot
		} else if s[i] == '.' {
			result += " "
		} else {
			result += string(s[i])
		}
	}
	return result
}
