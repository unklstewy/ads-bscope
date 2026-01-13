package db

import (
	"testing"
	"time"

	"github.com/unklstewy/ads-bscope/pkg/adsb"
	"github.com/unklstewy/ads-bscope/pkg/coordinates"
)

// TestNewAircraftRepository tests repository construction.
func TestNewAircraftRepository(t *testing.T) {
	observer := coordinates.Observer{
		Location: coordinates.Geographic{
			Latitude:  35.0,
			Longitude: -80.0,
			Altitude:  200.0,
		},
	}

	repo := NewAircraftRepository(nil, observer)

	if repo == nil {
		t.Fatal("Expected non-nil repository")
	}
	if repo.observer.Location.Latitude != 35.0 {
		t.Errorf("Expected observer lat 35.0, got %f", repo.observer.Location.Latitude)
	}
}

// TestPositionsEqual tests the position equality logic.
func TestPositionsEqual(t *testing.T) {
	tests := []struct {
		name     string
		current  adsb.Aircraft
		prev     aircraftPosition
		expected bool
	}{
		{
			name: "Identical position, not moving",
			current: adsb.Aircraft{
				Latitude:    35.123456,
				Longitude:   -80.654321,
				Altitude:    10000.0,
				GroundSpeed: 0.5,
			},
			prev: aircraftPosition{
				Latitude:       35.123456,
				Longitude:      -80.654321,
				AltitudeFt:     10000.0,
				GroundSpeedKts: 0.5,
			},
			expected: true,
		},
		{
			name: "Identical position, currently moving",
			current: adsb.Aircraft{
				Latitude:    35.123456,
				Longitude:   -80.654321,
				Altitude:    10000.0,
				GroundSpeed: 150.0,
			},
			prev: aircraftPosition{
				Latitude:       35.123456,
				Longitude:      -80.654321,
				AltitudeFt:     10000.0,
				GroundSpeedKts: 0.5,
			},
			expected: false, // Moving now
		},
		{
			name: "Position changed slightly",
			current: adsb.Aircraft{
				Latitude:    35.123457, // +0.000001 (changed)
				Longitude:   -80.654321,
				Altitude:    10000.0,
				GroundSpeed: 0.0,
			},
			prev: aircraftPosition{
				Latitude:       35.123456,
				Longitude:      -80.654321,
				AltitudeFt:     10000.0,
				GroundSpeedKts: 0.0,
			},
			expected: false, // Position changed
		},
		{
			name: "Altitude changed",
			current: adsb.Aircraft{
				Latitude:    35.123456,
				Longitude:   -80.654321,
				Altitude:    10002.0, // Changed by 2 feet
				GroundSpeed: 0.0,
			},
			prev: aircraftPosition{
				Latitude:       35.123456,
				Longitude:      -80.654321,
				AltitudeFt:     10000.0,
				GroundSpeedKts: 0.0,
			},
			expected: false, // Altitude changed
		},
		{
			name: "Previously moving, now stopped",
			current: adsb.Aircraft{
				Latitude:    35.123456,
				Longitude:   -80.654321,
				Altitude:    10000.0,
				GroundSpeed: 0.0,
			},
			prev: aircraftPosition{
				Latitude:       35.123456,
				Longitude:      -80.654321,
				AltitudeFt:     10000.0,
				GroundSpeedKts: 150.0,
			},
			expected: false, // Was moving
		},
		{
			name: "Longitude changed",
			current: adsb.Aircraft{
				Latitude:    35.123456,
				Longitude:   -80.654323, // Changed by >0.000001
				Altitude:    10000.0,
				GroundSpeed: 0.0,
			},
			prev: aircraftPosition{
				Latitude:       35.123456,
				Longitude:      -80.654321,
				AltitudeFt:     10000.0,
				GroundSpeedKts: 0.0,
			},
			expected: false,
		},
		{
			name: "Sub-tolerance position change",
			current: adsb.Aircraft{
				Latitude:    35.1234560005, // Within 0.000001 tolerance
				Longitude:   -80.654321,
				Altitude:    10000.0,
				GroundSpeed: 0.0,
			},
			prev: aircraftPosition{
				Latitude:       35.123456,
				Longitude:      -80.654321,
				AltitudeFt:     10000.0,
				GroundSpeedKts: 0.0,
			},
			expected: true, // Within tolerance
		},
		{
			name: "Sub-tolerance altitude change",
			current: adsb.Aircraft{
				Latitude:    35.123456,
				Longitude:   -80.654321,
				Altitude:    10000.5, // Within 1 foot tolerance
				GroundSpeed: 0.0,
			},
			prev: aircraftPosition{
				Latitude:       35.123456,
				Longitude:      -80.654321,
				AltitudeFt:     10000.0,
				GroundSpeedKts: 0.0,
			},
			expected: true, // Within tolerance
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := positionsEqual(tt.current, tt.prev)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// TestCalculateAverageVelocity tests velocity averaging from position history.
func TestCalculateAverageVelocity(t *testing.T) {
	t.Run("Empty history", func(t *testing.T) {
		positions := []Position{}
		avgSpeed, avgVR := CalculateAverageVelocity(positions)

		if avgSpeed != 0 {
			t.Errorf("Expected 0 speed for empty history, got %f", avgSpeed)
		}
		if avgVR != 0 {
			t.Errorf("Expected 0 VR for empty history, got %f", avgVR)
		}
	})

	t.Run("Single position", func(t *testing.T) {
		positions := []Position{
			{ActualSpeedKts: 200.0, ActualVerticalRateFpm: 500.0},
		}
		avgSpeed, avgVR := CalculateAverageVelocity(positions)

		if avgSpeed != 0 {
			t.Errorf("Expected 0 speed for single position, got %f", avgSpeed)
		}
		if avgVR != 0 {
			t.Errorf("Expected 0 VR for single position, got %f", avgVR)
		}
	})

	t.Run("Multiple positions", func(t *testing.T) {
		positions := []Position{
			{ActualSpeedKts: 200.0, ActualVerticalRateFpm: 500.0},
			{ActualSpeedKts: 220.0, ActualVerticalRateFpm: 600.0},
			{ActualSpeedKts: 210.0, ActualVerticalRateFpm: 550.0},
		}
		avgSpeed, avgVR := CalculateAverageVelocity(positions)

		expectedSpeed := (200.0 + 220.0 + 210.0) / 3.0
		if avgSpeed != expectedSpeed {
			t.Errorf("Expected speed %f, got %f", expectedSpeed, avgSpeed)
		}

		expectedVR := (500.0 + 600.0 + 550.0) / 3.0
		if avgVR != expectedVR {
			t.Errorf("Expected VR %f, got %f", expectedVR, avgVR)
		}
	})

	t.Run("Zero speed positions", func(t *testing.T) {
		positions := []Position{
			{ActualSpeedKts: 0.0, ActualVerticalRateFpm: 0.0},
			{ActualSpeedKts: 0.0, ActualVerticalRateFpm: 0.0},
		}
		avgSpeed, avgVR := CalculateAverageVelocity(positions)

		if avgSpeed != 0 {
			t.Errorf("Expected 0 speed, got %f", avgSpeed)
		}
		if avgVR != 0 {
			t.Errorf("Expected 0 VR, got %f", avgVR)
		}
	})

	t.Run("Mixed valid and invalid speeds", func(t *testing.T) {
		positions := []Position{
			{ActualSpeedKts: 200.0, ActualVerticalRateFpm: 500.0},
			{ActualSpeedKts: 0.0, ActualVerticalRateFpm: 0.0}, // Zero speed (not counted)
			{ActualSpeedKts: 220.0, ActualVerticalRateFpm: 600.0},
		}
		avgSpeed, avgVR := CalculateAverageVelocity(positions)

		// Only 2 valid speeds: 200 and 220
		expectedSpeed := (200.0 + 220.0) / 2.0
		if avgSpeed != expectedSpeed {
			t.Errorf("Expected speed %f, got %f", expectedSpeed, avgSpeed)
		}

		// All VR values counted
		expectedVR := (500.0 + 0.0 + 600.0) / 3.0
		if avgVR != expectedVR {
			t.Errorf("Expected VR %f, got %f", expectedVR, avgVR)
		}
	})
}

// TestPosition tests the Position struct.
func TestPosition(t *testing.T) {
	now := time.Now().UTC()

	p := Position{
		Timestamp:             now,
		Latitude:              35.0,
		Longitude:             -80.0,
		AltitudeFt:            10000.0,
		GroundSpeedKts:        250.0,
		TrackDeg:              90.0,
		VerticalRateFpm:       1000.0,
		DeltaTimeSeconds:      5.0,
		DeltaDistanceNM:       0.3472,
		DeltaAltitudeFt:       83.33,
		ActualSpeedKts:        250.0,
		ActualVerticalRateFpm: 1000.0,
		RangeNM:               25.0,
		AltitudeAngleDeg:      30.0,
		AzimuthDeg:            45.0,
	}

	if p.Timestamp != now {
		t.Error("Timestamp not set correctly")
	}
	if p.Latitude != 35.0 {
		t.Errorf("Expected latitude 35.0, got %f", p.Latitude)
	}
	if p.DeltaTimeSeconds != 5.0 {
		t.Errorf("Expected delta time 5.0, got %f", p.DeltaTimeSeconds)
	}
}

// TestAircraftPosition tests the aircraftPosition struct.
func TestAircraftPosition(t *testing.T) {
	now := time.Now().UTC()

	pos := aircraftPosition{
		Latitude:        35.5,
		Longitude:       -80.5,
		AltitudeFt:      12000.0,
		GroundSpeedKts:  300.0,
		TrackDeg:        180.0,
		VerticalRateFpm: -500.0,
		Timestamp:       now,
	}

	if pos.Latitude != 35.5 {
		t.Errorf("Expected latitude 35.5, got %f", pos.Latitude)
	}
	if pos.GroundSpeedKts != 300.0 {
		t.Errorf("Expected ground speed 300, got %f", pos.GroundSpeedKts)
	}
	if pos.Timestamp != now {
		t.Error("Timestamp not set correctly")
	}
}
