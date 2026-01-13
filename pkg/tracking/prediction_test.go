package tracking

import (
	"math"
	"testing"
	"time"

	"github.com/unklstewy/ads-bscope/pkg/adsb"
	"github.com/unklstewy/ads-bscope/pkg/coordinates"
)

// TestPredictPosition tests basic position prediction.
func TestPredictPosition(t *testing.T) {
	now := time.Now().UTC()

	t.Run("Zero delta time returns current position", func(t *testing.T) {
		aircraft := adsb.Aircraft{
			Latitude:     35.0,
			Longitude:    -80.0,
			Altitude:     10000.0,
			GroundSpeed:  250.0,
			Track:        90.0,
			VerticalRate: 0.0,
			LastSeen:     now,
		}

		pred := PredictPosition(aircraft, now)

		if pred.Position.Latitude != 35.0 {
			t.Errorf("Expected lat 35.0, got %f", pred.Position.Latitude)
		}
		if pred.Confidence != 1.0 {
			t.Errorf("Expected confidence 1.0, got %f", pred.Confidence)
		}
	})

	t.Run("Negative delta time returns current position", func(t *testing.T) {
		aircraft := adsb.Aircraft{
			Latitude:  35.0,
			Longitude: -80.0,
			Altitude:  10000.0,
			LastSeen:  now,
		}

		pred := PredictPosition(aircraft, now.Add(-5*time.Second))

		if pred.Confidence != 1.0 {
			t.Errorf("Expected confidence 1.0 for past time, got %f", pred.Confidence)
		}
	})

	t.Run("Confidence decreases with time", func(t *testing.T) {
		aircraft := adsb.Aircraft{
			Latitude:  35.0,
			Longitude: -80.0,
			Altitude:  10000.0,
			LastSeen:  now,
		}

		// Predict 30 seconds ahead
		pred := PredictPosition(aircraft, now.Add(30*time.Second))

		// Confidence should be 0.5 at 30s (1.0 - 30/60)
		expectedConf := 0.5
		if math.Abs(pred.Confidence-expectedConf) > 0.01 {
			t.Errorf("Expected confidence ~%f at 30s, got %f", expectedConf, pred.Confidence)
		}
	})

	t.Run("Stale data reduces confidence", func(t *testing.T) {
		// Data is 15 seconds old
		aircraft := adsb.Aircraft{
			Latitude:  35.0,
			Longitude: -80.0,
			Altitude:  10000.0,
			LastSeen:  now.Add(-15 * time.Second),
		}

		pred := PredictPosition(aircraft, now.Add(5*time.Second))

		// Base confidence: 1.0 - (5+15)/60 = 0.667
		// With stale data penalty: 0.667 * 0.5 = 0.333
		if pred.Confidence > 0.4 {
			t.Errorf("Expected reduced confidence for stale data, got %f", pred.Confidence)
		}
	})

	t.Run("Altitude prediction with climb", func(t *testing.T) {
		aircraft := adsb.Aircraft{
			Latitude:     35.0,
			Longitude:    -80.0,
			Altitude:     10000.0,
			VerticalRate: 1000.0, // 1000 fpm climb
			LastSeen:     now,
		}

		// Predict 1 minute ahead
		pred := PredictPosition(aircraft, now.Add(60*time.Second))

		expectedAlt := 11000.0 * coordinates.FeetToMeters
		if math.Abs(pred.Position.Altitude-expectedAlt) > 10.0 {
			t.Errorf("Expected altitude ~%f, got %f", expectedAlt, pred.Position.Altitude)
		}
	})

	t.Run("Altitude doesn't go below ground", func(t *testing.T) {
		aircraft := adsb.Aircraft{
			Latitude:     35.0,
			Longitude:    -80.0,
			Altitude:     500.0,
			VerticalRate: -1000.0, // Descending
			LastSeen:     now,
		}

		// Predict 1 minute ahead (would go to -500 ft)
		pred := PredictPosition(aircraft, now.Add(60*time.Second))

		if pred.Position.Altitude < 0 {
			t.Error("Altitude should not go below ground")
		}
		// Confidence should be reduced
		if pred.Confidence >= 0.5 {
			t.Errorf("Expected reduced confidence for ground collision, got %f", pred.Confidence)
		}
	})
}

// TestPredictPositionWithLatency tests latency compensation.
func TestPredictPositionWithLatency(t *testing.T) {
	now := time.Now().UTC()

	aircraft := adsb.Aircraft{
		Latitude:  35.0,
		Longitude: -80.0,
		Altitude:  10000.0,
		LastSeen:  now.Add(-2 * time.Second), // 2 seconds ago
	}

	pred := PredictPositionWithLatency(aircraft, 2.5)

	// Should predict ~4.5 seconds ahead (2s data age + 2.5s latency)
	if pred.OriginalPosition.ICAO != aircraft.ICAO {
		t.Error("Original position not preserved")
	}
}

// TestPredictHorizontalPosition tests great circle navigation.
func TestPredictHorizontalPosition(t *testing.T) {
	t.Run("Eastward movement", func(t *testing.T) {
		lat, lon := predictHorizontalPosition(
			35.0, -80.0, // Starting position
			300.0,  // 300 knots
			90.0,   // East
			3600.0, // 1 hour
		)

		// Should move east (longitude increases)
		if lon <= -80.0 {
			t.Errorf("Expected longitude to increase, got %f", lon)
		}
		// Latitude should stay relatively close
		if math.Abs(lat-35.0) > 1.0 {
			t.Errorf("Expected latitude ~35.0, got %f", lat)
		}
	})

	t.Run("Northward movement", func(t *testing.T) {
		lat, lon := predictHorizontalPosition(
			35.0, -80.0,
			300.0,
			0.0,    // North
			3600.0, // 1 hour
		)

		// Should move north (latitude increases)
		if lat <= 35.0 {
			t.Errorf("Expected latitude to increase, got %f", lat)
		}
		// Longitude should stay close
		if math.Abs(lon-(-80.0)) > 0.5 {
			t.Errorf("Expected longitude ~-80.0, got %f", lon)
		}
	})

	t.Run("Longitude normalization", func(t *testing.T) {
		// Start near 180° and move east
		_, lon := predictHorizontalPosition(
			0.0, 179.0,
			300.0,
			90.0,   // East
			3600.0, // 1 hour
		)

		// Should wrap to negative longitude
		if lon > 180.0 {
			t.Errorf("Longitude not normalized, got %f", lon)
		}
	})
}

// TestCalculateLeadTime tests telescope slew time calculation.
func TestCalculateLeadTime(t *testing.T) {
	t.Run("Simple angular separation", func(t *testing.T) {
		leadTime := CalculateLeadTime(
			30.0, 180.0, // Current: 30° alt, 180° az
			40.0, 200.0, // Target: 40° alt, 200° az
			5.0, // 5 deg/sec slew rate
		)

		// Delta alt: 10°, Delta az: 20°
		// Max delta: 20°, Time: 20/5 = 4 seconds
		expectedTime := 4.0
		if math.Abs(leadTime-expectedTime) > 0.01 {
			t.Errorf("Expected lead time %f, got %f", expectedTime, leadTime)
		}
	})

	t.Run("Azimuth wrap-around", func(t *testing.T) {
		leadTime := CalculateLeadTime(
			30.0, 359.0, // Current: near 360°
			30.0, 1.0, // Target: near 0°
			5.0,
		)

		// Azimuth delta should be 2°, not 358°
		expectedTime := 0.4 // 2/5 = 0.4 seconds
		if math.Abs(leadTime-expectedTime) > 0.01 {
			t.Errorf("Expected lead time %f, got %f", expectedTime, leadTime)
		}
	})

	t.Run("Zero slew rate returns zero", func(t *testing.T) {
		leadTime := CalculateLeadTime(30.0, 180.0, 40.0, 200.0, 0.0)

		if leadTime != 0 {
			t.Errorf("Expected 0 for zero slew rate, got %f", leadTime)
		}
	})
}

// TestNormalizeAngle tests angle normalization.
func TestNormalizeAngle(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
	}{
		{0.0, 0.0},
		{90.0, 90.0},
		{180.0, 180.0},
		{-180.0, -180.0},
		{270.0, -90.0},
		{-270.0, 90.0},
		{360.0, 0.0},
		{-360.0, 0.0},
		{450.0, 90.0},
		{-450.0, -90.0},
	}

	for _, tt := range tests {
		result := normalizeAngle(tt.input)
		if math.Abs(result-tt.expected) > 0.01 {
			t.Errorf("normalizeAngle(%f) = %f, expected %f", tt.input, result, tt.expected)
		}
	}
}

// TestFindNextWaypoint tests waypoint selection.
func TestFindNextWaypoint(t *testing.T) {
	t.Run("Returns first non-passed waypoint", func(t *testing.T) {
		waypoints := []Waypoint{
			{Name: "WP1", Passed: true},
			{Name: "WP2", Passed: false},
			{Name: "WP3", Passed: false},
		}

		next := findNextWaypoint(waypoints)

		if next == nil {
			t.Fatal("Expected waypoint, got nil")
		}
		if next.Name != "WP2" {
			t.Errorf("Expected WP2, got %s", next.Name)
		}
	})

	t.Run("Returns nil when all passed", func(t *testing.T) {
		waypoints := []Waypoint{
			{Name: "WP1", Passed: true},
			{Name: "WP2", Passed: true},
		}

		next := findNextWaypoint(waypoints)

		if next != nil {
			t.Error("Expected nil for all passed waypoints")
		}
	})

	t.Run("Returns nil for empty list", func(t *testing.T) {
		waypoints := []Waypoint{}

		next := findNextWaypoint(waypoints)

		if next != nil {
			t.Error("Expected nil for empty waypoints")
		}
	})
}

// TestDeterminePassedWaypoints tests waypoint passing detection.
func TestDeterminePassedWaypoints(t *testing.T) {
	now := time.Now().UTC()

	t.Run("Mark waypoint as passed when within 2 NM", func(t *testing.T) {
		aircraft := adsb.Aircraft{
			Latitude:  35.0,
			Longitude: -80.0,
			Track:     90.0,
			LastSeen:  now,
		}

		waypoints := []Waypoint{
			{
				Name:      "CLOSE",
				Latitude:  35.01, // ~1 NM away
				Longitude: -80.0,
				Passed:    false,
			},
		}

		updated := DeterminePassedWaypoints(aircraft, waypoints)

		if !updated[0].Passed {
			t.Error("Expected waypoint to be marked as passed")
		}
	})

	t.Run("Mark waypoint as passed when behind aircraft", func(t *testing.T) {
		aircraft := adsb.Aircraft{
			Latitude:  35.0,
			Longitude: -80.0,
			Track:     90.0, // Flying east
			LastSeen:  now,
		}

		waypoints := []Waypoint{
			{
				Name:      "BEHIND",
				Latitude:  35.0,
				Longitude: -80.5, // To the west (behind)
				Passed:    false,
			},
		}

		updated := DeterminePassedWaypoints(aircraft, waypoints)

		if !updated[0].Passed {
			t.Error("Expected waypoint behind aircraft to be marked as passed")
		}
	})

	t.Run("Don't mark waypoint ahead", func(t *testing.T) {
		aircraft := adsb.Aircraft{
			Latitude:  35.0,
			Longitude: -80.0,
			Track:     90.0, // Flying east
			LastSeen:  now,
		}

		waypoints := []Waypoint{
			{
				Name:      "AHEAD",
				Latitude:  35.0,
				Longitude: -79.5, // To the east (ahead)
				Passed:    false,
			},
		}

		updated := DeterminePassedWaypoints(aircraft, waypoints)

		if updated[0].Passed {
			t.Error("Waypoint ahead should not be marked as passed")
		}
	})

	t.Run("Preserve already passed waypoints", func(t *testing.T) {
		aircraft := adsb.Aircraft{
			Latitude:  35.0,
			Longitude: -80.0,
			LastSeen:  now,
		}

		waypoints := []Waypoint{
			{Name: "WP1", Latitude: 35.0, Longitude: -81.0, Passed: true},
			{Name: "WP2", Latitude: 35.0, Longitude: -79.0, Passed: false},
		}

		updated := DeterminePassedWaypoints(aircraft, waypoints)

		if !updated[0].Passed {
			t.Error("Already passed waypoint should stay passed")
		}
	})
}

// TestInterpolateGreatCircle tests great circle interpolation.
func TestInterpolateGreatCircle(t *testing.T) {
	t.Run("Fraction 0 returns start point", func(t *testing.T) {
		lat, lon := interpolateGreatCircle(35.0, -80.0, 40.0, -75.0, 0.0)

		if math.Abs(lat-35.0) > 0.01 || math.Abs(lon-(-80.0)) > 0.01 {
			t.Errorf("Expected start point (35.0, -80.0), got (%f, %f)", lat, lon)
		}
	})

	t.Run("Fraction 1 returns end point", func(t *testing.T) {
		lat, lon := interpolateGreatCircle(35.0, -80.0, 40.0, -75.0, 1.0)

		if math.Abs(lat-40.0) > 0.01 || math.Abs(lon-(-75.0)) > 0.01 {
			t.Errorf("Expected end point (40.0, -75.0), got (%f, %f)", lat, lon)
		}
	})

	t.Run("Fraction 0.5 returns midpoint", func(t *testing.T) {
		lat, lon := interpolateGreatCircle(35.0, -80.0, 40.0, -75.0, 0.5)

		// Midpoint should be approximately halfway
		if lat < 36.0 || lat > 39.0 {
			t.Errorf("Expected midpoint latitude between 36-39, got %f", lat)
		}
		if lon > -77.0 || lon < -78.0 {
			t.Errorf("Expected midpoint longitude between -78 and -77, got %f", lon)
		}
	})

	t.Run("Identical points return same point", func(t *testing.T) {
		lat, lon := interpolateGreatCircle(35.0, -80.0, 35.0, -80.0, 0.5)

		if math.Abs(lat-35.0) > 0.01 || math.Abs(lon-(-80.0)) > 0.01 {
			t.Errorf("Expected same point for identical start/end, got (%f, %f)", lat, lon)
		}
	})
}

// TestFilterAirwaysByAltitude tests airway altitude filtering.
func TestFilterAirwaysByAltitude(t *testing.T) {
	airways := []AirwaySegment{
		{AirwayID: "V123", AirwayType: "victor"},
		{AirwayID: "J456", AirwayType: "jet"},
		{AirwayID: "Q789", AirwayType: "rnav"},
		{AirwayID: "T012", AirwayType: "rnav"},
	}

	t.Run("Low altitude selects Victor airways", func(t *testing.T) {
		filtered := FilterAirwaysByAltitude(airways, 15000.0)

		found := false
		for _, airway := range filtered {
			if airway.AirwayID == "V123" {
				found = true
			}
			if airway.AirwayID == "J456" {
				t.Error("Jet route should not be included at low altitude")
			}
		}
		if !found {
			t.Error("Victor airway should be included at low altitude")
		}
	})

	t.Run("High altitude selects Jet routes", func(t *testing.T) {
		filtered := FilterAirwaysByAltitude(airways, 25000.0)

		found := false
		for _, airway := range filtered {
			if airway.AirwayID == "J456" {
				found = true
			}
			if airway.AirwayID == "V123" {
				t.Error("Victor airway should not be included at high altitude")
			}
		}
		if !found {
			t.Error("Jet route should be included at high altitude")
		}
	})

	t.Run("RNAV routes included at any altitude", func(t *testing.T) {
		lowAlt := FilterAirwaysByAltitude(airways, 10000.0)
		highAlt := FilterAirwaysByAltitude(airways, 30000.0)

		// Both should include RNAV routes
		for _, filtered := range [][]AirwaySegment{lowAlt, highAlt} {
			foundQ := false
			foundT := false
			for _, airway := range filtered {
				if airway.AirwayID == "Q789" {
					foundQ = true
				}
				if airway.AirwayID == "T012" {
					foundT = true
				}
			}
			if !foundQ || !foundT {
				t.Error("RNAV routes should be included at all altitudes")
			}
		}
	})
}

// TestMatchAirway tests airway matching algorithm.
func TestMatchAirway(t *testing.T) {
	t.Run("Returns nil for empty airways", func(t *testing.T) {
		aircraft := adsb.Aircraft{
			Latitude:  35.0,
			Longitude: -80.0,
			Altitude:  25000.0,
			Track:     90.0,
		}

		match := MatchAirway(aircraft, []AirwaySegment{})

		if match != nil {
			t.Error("Expected nil for empty airways")
		}
	})

	t.Run("Rejects aircraft outside altitude limits", func(t *testing.T) {
		aircraft := adsb.Aircraft{
			Latitude:  35.0,
			Longitude: -80.0,
			Altitude:  10000.0,
			Track:     90.0,
		}

		airways := []AirwaySegment{
			{
				AirwayID:    "J456",
				FromLat:     35.0,
				FromLon:     -81.0,
				ToLat:       35.0,
				ToLon:       -79.0,
				MinAltitude: 18000,
				MaxAltitude: 45000,
			},
		}

		match := MatchAirway(aircraft, airways)

		if match != nil {
			t.Error("Should reject aircraft below minimum altitude")
		}
	})
}

// TestDistanceToLineSegment tests perpendicular distance calculation.
func TestDistanceToLineSegment(t *testing.T) {
	// Line segment from (35°N, 80°W) to (35°N, 79°W)
	lineStart := coordinates.Geographic{Latitude: 35.0, Longitude: -80.0}
	lineEnd := coordinates.Geographic{Latitude: 35.0, Longitude: -79.0}

	t.Run("Point on line has zero distance", func(t *testing.T) {
		point := coordinates.Geographic{Latitude: 35.0, Longitude: -79.5}

		dist := distanceToLineSegment(point, lineStart, lineEnd)

		if dist > 1.0 { // Within 1 NM tolerance
			t.Errorf("Expected distance ~0, got %f NM", dist)
		}
	})

	t.Run("Point perpendicular to line", func(t *testing.T) {
		// Point north of line midpoint
		point := coordinates.Geographic{Latitude: 35.5, Longitude: -79.5}

		dist := distanceToLineSegment(point, lineStart, lineEnd)

		// Should be ~30 NM (0.5 degrees latitude)
		if dist < 25.0 || dist > 35.0 {
			t.Errorf("Expected distance ~30 NM, got %f NM", dist)
		}
	})
}

// TestWaypoint tests the Waypoint struct.
func TestWaypoint(t *testing.T) {
	wp := Waypoint{
		Name:      "TEST",
		Latitude:  35.0,
		Longitude: -80.0,
		Sequence:  1,
		Passed:    false,
	}

	if wp.Name != "TEST" {
		t.Errorf("Expected name TEST, got %s", wp.Name)
	}
	if wp.Passed {
		t.Error("Expected Passed to be false")
	}
}

// TestPredictedPosition tests the PredictedPosition struct.
func TestPredictedPosition(t *testing.T) {
	now := time.Now().UTC()

	pred := PredictedPosition{
		Position: coordinates.Geographic{
			Latitude:  35.0,
			Longitude: -80.0,
			Altitude:  3000.0,
		},
		PredictionTime: now,
		Confidence:     0.95,
	}

	if pred.Confidence != 0.95 {
		t.Errorf("Expected confidence 0.95, got %f", pred.Confidence)
	}
	if !pred.PredictionTime.Equal(now) {
		t.Error("Prediction time not set correctly")
	}
}

// TestAirwaySegment tests the AirwaySegment struct.
func TestAirwaySegment(t *testing.T) {
	seg := AirwaySegment{
		AirwayID:    "J123",
		AirwayType:  "jet",
		FromLat:     35.0,
		FromLon:     -80.0,
		ToLat:       40.0,
		ToLon:       -75.0,
		MinAltitude: 18000,
		MaxAltitude: 45000,
	}

	if seg.AirwayID != "J123" {
		t.Errorf("Expected airway J123, got %s", seg.AirwayID)
	}
	if seg.MinAltitude != 18000 {
		t.Errorf("Expected min altitude 18000, got %d", seg.MinAltitude)
	}
}
