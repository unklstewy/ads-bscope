package tracking

import (
	"testing"

	"github.com/unklstewy/ads-bscope/pkg/coordinates"
)

// TestDefaultTrackingLimits tests default limit creation.
func TestDefaultTrackingLimits(t *testing.T) {
	limits := DefaultTrackingLimits()

	if limits.MinAltitude != 15.0 {
		t.Errorf("Expected min altitude 15.0, got %f", limits.MinAltitude)
	}
	if limits.MaxAltitude != 85.0 {
		t.Errorf("Expected max altitude 85.0, got %f", limits.MaxAltitude)
	}
	if limits.MeridianFlipHourAngle != 6.0 {
		t.Errorf("Expected meridian flip HA 6.0, got %f", limits.MeridianFlipHourAngle)
	}
	if limits.AzimuthWrapLimit != 0.0 {
		t.Errorf("Expected azimuth wrap 0.0, got %f", limits.AzimuthWrapLimit)
	}
}

// TestTrackingLimitsFromConfig tests custom limit creation.
func TestTrackingLimitsFromConfig(t *testing.T) {
	limits := TrackingLimitsFromConfig(20.0, 80.0)

	if limits.MinAltitude != 20.0 {
		t.Errorf("Expected min altitude 20.0, got %f", limits.MinAltitude)
	}
	if limits.MaxAltitude != 80.0 {
		t.Errorf("Expected max altitude 80.0, got %f", limits.MaxAltitude)
	}
	// Other values should be defaults
	if limits.MeridianFlipHourAngle != 6.0 {
		t.Error("Expected default meridian flip HA")
	}
}

// TestCheckMeridianEvent tests meridian event detection.
func TestCheckMeridianEvent(t *testing.T) {
	limits := DefaultTrackingLimits()
	observer := coordinates.Observer{}

	t.Run("Below minimum altitude", func(t *testing.T) {
		current := coordinates.HorizontalCoordinates{Altitude: 20.0, Azimuth: 180.0}
		target := coordinates.HorizontalCoordinates{Altitude: 10.0, Azimuth: 180.0}

		event, msg := CheckMeridianEvent(current, target, observer, limits, false)

		if event != HorizonCrossing {
			t.Errorf("Expected HorizonCrossing, got %v", event)
		}
		if msg == "" {
			t.Error("Expected non-empty message")
		}
	})

	t.Run("Above maximum altitude", func(t *testing.T) {
		current := coordinates.HorizontalCoordinates{Altitude: 80.0, Azimuth: 180.0}
		target := coordinates.HorizontalCoordinates{Altitude: 87.0, Azimuth: 180.0}

		event, msg := CheckMeridianEvent(current, target, observer, limits, false)

		if event != ZenithCrossing {
			t.Errorf("Expected ZenithCrossing, got %v", event)
		}
		if msg == "" {
			t.Error("Expected non-empty message")
		}
	})

	t.Run("Normal tracking", func(t *testing.T) {
		current := coordinates.HorizontalCoordinates{Altitude: 40.0, Azimuth: 180.0}
		target := coordinates.HorizontalCoordinates{Altitude: 45.0, Azimuth: 200.0}

		event, msg := CheckMeridianEvent(current, target, observer, limits, false)

		if event != NoMeridianEvent {
			t.Errorf("Expected NoMeridianEvent, got %v", event)
		}
		if msg != "Tracking OK" {
			t.Errorf("Expected 'Tracking OK', got %s", msg)
		}
	})

	t.Run("Rapid azimuth change near zenith", func(t *testing.T) {
		current := coordinates.HorizontalCoordinates{Altitude: 82.0, Azimuth: 90.0}
		target := coordinates.HorizontalCoordinates{Altitude: 83.0, Azimuth: 270.0}

		event, _ := CheckMeridianEvent(current, target, observer, limits, false)

		if event != ZenithCrossing {
			t.Errorf("Expected ZenithCrossing for rapid azimuth change, got %v", event)
		}
	})

	t.Run("Azimuth wrap with limit", func(t *testing.T) {
		limitsWithWrap := limits
		limitsWithWrap.AzimuthWrapLimit = 10.0 // Very restrictive limit

		current := coordinates.HorizontalCoordinates{Altitude: 40.0, Azimuth: 10.0}
		target := coordinates.HorizontalCoordinates{Altitude: 40.0, Azimuth: 350.0}

		event, _ := CheckMeridianEvent(current, target, observer, limitsWithWrap, true)

		if event != MeridianFlipRequired {
			t.Errorf("Expected MeridianFlipRequired, got %v", event)
		}
	})
}

// TestCheckEquatorialMeridianFlip tests equatorial mount flip detection.
func TestCheckEquatorialMeridianFlip(t *testing.T) {
	limits := DefaultTrackingLimits()
	observer := coordinates.Observer{}

	t.Run("Within hour angle limits", func(t *testing.T) {
		ra := 12.0  // 12h RA
		dec := 30.0 // 30° Dec
		lst := 15.0 // LST: 15h (HA = 3h)

		needsFlip, msg := CheckEquatorialMeridianFlip(ra, dec, observer, lst, limits)

		if needsFlip {
			t.Error("Should not need flip within HA limits")
		}
		if msg != "Equatorial tracking OK" {
			t.Errorf("Expected 'Equatorial tracking OK', got %s", msg)
		}
	})

	t.Run("Exceeds hour angle east", func(t *testing.T) {
		ra := 12.0 // 12h RA
		dec := 30.0
		lst := 4.0 // LST: 4h (HA = -8h, exceeds -6h limit)

		needsFlip, msg := CheckEquatorialMeridianFlip(ra, dec, observer, lst, limits)

		if !needsFlip {
			t.Error("Should need flip when HA exceeds limit")
		}
		if msg == "" {
			t.Error("Expected non-empty message")
		}
	})

	t.Run("Exceeds hour angle west", func(t *testing.T) {
		ra := 12.0 // 12h RA
		dec := 30.0
		lst := 20.0 // LST: 20h (HA = +8h, exceeds +6h limit)

		needsFlip, msg := CheckEquatorialMeridianFlip(ra, dec, observer, lst, limits)

		if !needsFlip {
			t.Error("Should need flip when HA exceeds limit")
		}
		if msg == "" {
			t.Error("Expected non-empty message")
		}
	})

	t.Run("Near celestial pole", func(t *testing.T) {
		ra := 12.0
		dec := 87.0 // Very high declination
		lst := 12.0

		needsFlip, msg := CheckEquatorialMeridianFlip(ra, dec, observer, lst, limits)

		if needsFlip {
			t.Error("Should not require flip near pole (different warning)")
		}
		if msg == "" {
			t.Error("Expected warning about pole tracking")
		}
	})
}

// TestPredictMeridianCrossing tests meridian crossing prediction.
func TestPredictMeridianCrossing(t *testing.T) {
	limits := DefaultTrackingLimits()

	t.Run("Already past limits", func(t *testing.T) {
		current := coordinates.HorizontalCoordinates{Altitude: 87.0, Azimuth: 180.0}
		future := coordinates.HorizontalCoordinates{Altitude: 88.0, Azimuth: 180.0}

		seconds := PredictMeridianCrossing(current, future, limits)

		if seconds != 0 {
			t.Errorf("Expected 0 for already past limits, got %f", seconds)
		}
	})

	t.Run("Approaching zenith", func(t *testing.T) {
		current := coordinates.HorizontalCoordinates{Altitude: 75.0, Azimuth: 180.0}
		future := coordinates.HorizontalCoordinates{Altitude: 80.0, Azimuth: 180.0}

		seconds := PredictMeridianCrossing(current, future, limits)

		// Should predict time to reach max altitude (85°)
		// Rate: 5° per interval, need 10° more
		if seconds <= 0 {
			t.Error("Expected positive prediction for approaching zenith")
		}
	})

	t.Run("Approaching horizon", func(t *testing.T) {
		current := coordinates.HorizontalCoordinates{Altitude: 25.0, Azimuth: 180.0}
		future := coordinates.HorizontalCoordinates{Altitude: 20.0, Azimuth: 180.0}

		seconds := PredictMeridianCrossing(current, future, limits)

		// Should predict time to reach min altitude (15°)
		if seconds <= 0 {
			t.Error("Expected positive prediction for approaching horizon")
		}
	})

	t.Run("No event predicted", func(t *testing.T) {
		current := coordinates.HorizontalCoordinates{Altitude: 45.0, Azimuth: 180.0}
		future := coordinates.HorizontalCoordinates{Altitude: 45.5, Azimuth: 185.0}

		seconds := PredictMeridianCrossing(current, future, limits)

		if seconds != -1 {
			t.Errorf("Expected -1 for no event, got %f", seconds)
		}
	})
}

// TestAzimuthDifference tests azimuth wrap-around calculation.
func TestAzimuthDifference(t *testing.T) {
	tests := []struct {
		az1      float64
		az2      float64
		expected float64
	}{
		{0.0, 90.0, 90.0},
		{90.0, 0.0, 90.0},
		{0.0, 180.0, 180.0},
		{0.0, 270.0, 90.0}, // Wraps around
		{359.0, 1.0, 2.0},  // Wraps around
		{1.0, 359.0, 2.0},  // Wraps around
		{180.0, 0.0, 180.0},
		{270.0, 90.0, 180.0},
	}

	for _, tt := range tests {
		result := azimuthDifference(tt.az1, tt.az2)
		if result != tt.expected {
			t.Errorf("azimuthDifference(%f, %f) = %f, expected %f",
				tt.az1, tt.az2, result, tt.expected)
		}
	}
}

// TestIsAzimuthWrap tests azimuth wrap detection.
func TestIsAzimuthWrap(t *testing.T) {
	t.Run("No limit means no wrap", func(t *testing.T) {
		result := isAzimuthWrap(10.0, 350.0, 0.0)
		if result {
			t.Error("Expected false when limit is 0")
		}
	})

	t.Run("Within limit", func(t *testing.T) {
		result := isAzimuthWrap(10.0, 100.0, 180.0)
		if result {
			t.Error("Expected false when within limit")
		}
	})

	t.Run("Exceeds limit", func(t *testing.T) {
		// Azimuth difference between 10 and 350 is 20 (wraps around)
		// So a limit of 10 should cause a wrap
		result := isAzimuthWrap(10.0, 350.0, 10.0)
		if !result {
			t.Error("Expected true when exceeding limit")
		}
	})
}

// TestRecommendTrackingStrategy tests recommendation generation.
func TestRecommendTrackingStrategy(t *testing.T) {
	tests := []struct {
		event       MeridianEvent
		altitude    float64
		expectsStop bool
	}{
		{NoMeridianEvent, 40.0, false},
		{MeridianFlipRequired, 40.0, true},
		{ZenithCrossing, 87.0, true},
		{ZenithCrossing, 82.0, false},
		{HorizonCrossing, 10.0, true},
	}

	for _, tt := range tests {
		rec := RecommendTrackingStrategy(tt.event, tt.altitude)

		if rec == "" {
			t.Errorf("Expected non-empty recommendation for event %v", tt.event)
		}
		if tt.expectsStop && rec == "Continue tracking normally" {
			t.Errorf("Expected stop recommendation for event %v", tt.event)
		}
	}
}

// TestCalculateMeridianFlipDuration tests flip duration estimate.
func TestCalculateMeridianFlipDuration(t *testing.T) {
	duration := CalculateMeridianFlipDuration()

	if duration != 45.0 {
		t.Errorf("Expected 45 seconds, got %f", duration)
	}
}

// TestShouldAbortTracking tests abort detection.
func TestShouldAbortTracking(t *testing.T) {
	limits := DefaultTrackingLimits()

	t.Run("Below minimum altitude", func(t *testing.T) {
		horiz := coordinates.HorizontalCoordinates{Altitude: 10.0, Azimuth: 180.0}

		if !ShouldAbortTracking(horiz, limits) {
			t.Error("Should abort below minimum altitude")
		}
	})

	t.Run("Above maximum altitude", func(t *testing.T) {
		horiz := coordinates.HorizontalCoordinates{Altitude: 87.0, Azimuth: 180.0}

		if !ShouldAbortTracking(horiz, limits) {
			t.Error("Should abort above maximum altitude")
		}
	})

	t.Run("Within limits", func(t *testing.T) {
		horiz := coordinates.HorizontalCoordinates{Altitude: 45.0, Azimuth: 180.0}

		if ShouldAbortTracking(horiz, limits) {
			t.Error("Should not abort within limits")
		}
	})
}

// TestMeridianEvent tests the MeridianEvent type.
func TestMeridianEvent(t *testing.T) {
	if NoMeridianEvent == MeridianFlipRequired {
		t.Error("Event types should be distinct")
	}
	if ZenithCrossing == HorizonCrossing {
		t.Error("Event types should be distinct")
	}
}

// TestTrackingLimits tests the TrackingLimits struct.
func TestTrackingLimits(t *testing.T) {
	limits := TrackingLimits{
		MinAltitude:           20.0,
		MaxAltitude:           80.0,
		MeridianFlipHourAngle: 5.0,
		AzimuthWrapLimit:      270.0,
	}

	if limits.MinAltitude != 20.0 {
		t.Error("MinAltitude not set correctly")
	}
	if limits.AzimuthWrapLimit != 270.0 {
		t.Error("AzimuthWrapLimit not set correctly")
	}
}
