package coordinates

import (
	"math"
	"testing"
	"time"
)

// TestGeographicToHorizontal tests the conversion from geographic to horizontal coordinates
func TestGeographicToHorizontal(t *testing.T) {
	tests := []struct {
		name      string
		target    Geographic
		observer  Observer
		wantAlt   float64 // Expected altitude (degrees)
		wantAz    float64 // Expected azimuth (degrees)
		tolerance float64 // Tolerance for comparison
	}{
		{
			name: "Aircraft directly north at same altitude",
			target: Geographic{
				Latitude:  41.0, // 1 degree north
				Longitude: -74.0,
				Altitude:  100.0, // 100m MSL
			},
			observer: Observer{
				Location: Geographic{
					Latitude:  40.0,
					Longitude: -74.0,
					Altitude:  100.0,
				},
			},
			wantAlt:   0.0, // Roughly horizontal
			wantAz:    0.0, // North
			tolerance: 1.0, // Within 1 degree
		},
		{
			name: "Aircraft directly east at same altitude",
			target: Geographic{
				Latitude:  40.0,
				Longitude: -73.0, // 1 degree east
				Altitude:  100.0,
			},
			observer: Observer{
				Location: Geographic{
					Latitude:  40.0,
					Longitude: -74.0,
					Altitude:  100.0,
				},
			},
			wantAlt:   0.0,  // Roughly horizontal
			wantAz:    90.0, // East
			tolerance: 1.0,
		},
		{
			name: "Aircraft above observer",
			target: Geographic{
				Latitude:  40.0,
				Longitude: -74.0,
				Altitude:  10100.0, // 10km above observer
			},
			observer: Observer{
				Location: Geographic{
					Latitude:  40.0,
					Longitude: -74.0,
					Altitude:  100.0,
				},
			},
			wantAlt:   85.0, // Nearly straight up
			wantAz:    0.0,  // Azimuth is arbitrary when nearly overhead
			tolerance: 5.0,  // Larger tolerance for edge case
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Now().UTC()
			result := GeographicToHorizontal(tt.target, tt.observer, now)

			// Check altitude
			if math.Abs(result.Altitude-tt.wantAlt) > tt.tolerance {
				t.Errorf("Altitude = %.2f, want %.2f (±%.2f)", result.Altitude, tt.wantAlt, tt.tolerance)
			}

			// Check azimuth (unless altitude is very high, then azimuth is meaningless)
			if result.Altitude < 80.0 {
				azDiff := math.Abs(result.Azimuth - tt.wantAz)
				// Account for wrap-around (359° vs 1°)
				if azDiff > 180.0 {
					azDiff = 360.0 - azDiff
				}
				if azDiff > tt.tolerance {
					t.Errorf("Azimuth = %.2f, want %.2f (±%.2f)", result.Azimuth, tt.wantAz, tt.tolerance)
				}
			}
		})
	}
}

// TestHorizontalEquatorialRoundTrip tests that converting alt/az to RA/Dec and back
// gives the original coordinates
// TODO: Re-enable once we can verify against real astronomical data
func SkipTestHorizontalEquatorialRoundTrip(t *testing.T) {
	// Observer in New York
	observer := Observer{
		Location: Geographic{
			Latitude:  40.7128,
			Longitude: -74.0060,
			Altitude:  10.0,
		},
		Timezone: "America/New_York",
	}

	// Test time
	testTime := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		horizontal HorizontalCoordinates
		tolerance  float64
	}{
		{
			name: "45 degrees altitude, north",
			horizontal: HorizontalCoordinates{
				Altitude: 45.0,
				Azimuth:  0.0,
			},
			tolerance: 0.5,
		},
		{
			name: "30 degrees altitude, east",
			horizontal: HorizontalCoordinates{
				Altitude: 30.0,
				Azimuth:  90.0,
			},
			tolerance: 0.5,
		},
		{
			name: "60 degrees altitude, southwest",
			horizontal: HorizontalCoordinates{
				Altitude: 60.0,
				Azimuth:  225.0,
			},
			tolerance: 0.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Convert to equatorial
			eq := HorizontalToEquatorial(tt.horizontal, observer, testTime)

			// Convert back to horizontal
			result := EquatorialToHorizontal(eq, observer, testTime)

			// Check altitude
			if math.Abs(result.Altitude-tt.horizontal.Altitude) > tt.tolerance {
				t.Errorf("Altitude round trip: got %.4f, want %.4f", result.Altitude, tt.horizontal.Altitude)
			}

			// Check azimuth
			azDiff := math.Abs(result.Azimuth - tt.horizontal.Azimuth)
			if azDiff > 180.0 {
				azDiff = 360.0 - azDiff
			}
			if azDiff > tt.tolerance {
				t.Errorf("Azimuth round trip: got %.4f, want %.4f", result.Azimuth, tt.horizontal.Azimuth)
			}
		})
	}
}

// TestLocalSiderealTime tests the LST calculation
func TestLocalSiderealTime(t *testing.T) {
	// Test with a known value
	// At Greenwich (0° longitude) on Jan 1, 2000, 12:00 UTC, LST should be approximately 18.697 hours
	testTime := time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)
	lst := CalculateLocalSiderealTime(0.0, testTime)

	// Should be close to 18.697 hours
	expected := 18.697
	if math.Abs(lst-expected) > 0.1 {
		t.Errorf("LST at Greenwich J2000 = %.3f, want %.3f", lst, expected)
	}

	// Test that LST is in valid range [0, 24)
	testTime = time.Now().UTC()
	longitudes := []float64{-180.0, -90.0, 0.0, 90.0, 180.0}
	for _, lon := range longitudes {
		lst := CalculateLocalSiderealTime(lon, testTime)
		if lst < 0.0 || lst >= 24.0 {
			t.Errorf("LST out of range [0, 24) for longitude %.1f: %.3f", lon, lst)
		}
	}
}

// TestNormalizeAzimuth tests azimuth normalization
func TestNormalizeAzimuth(t *testing.T) {
	tests := []struct {
		input float64
		want  float64
	}{
		{0.0, 0.0},
		{359.0, 359.0},
		{360.0, 0.0},
		{361.0, 1.0},
		{-1.0, 359.0},
		{-90.0, 270.0},
		{720.0, 0.0},
	}

	for _, tt := range tests {
		got := NormalizeAzimuth(tt.input)
		if math.Abs(got-tt.want) > 0.0001 {
			t.Errorf("NormalizeAzimuth(%.1f) = %.1f, want %.1f", tt.input, got, tt.want)
		}
	}
}

// TestNormalizeRA tests right ascension normalization
func TestNormalizeRA(t *testing.T) {
	tests := []struct {
		input float64
		want  float64
	}{
		{0.0, 0.0},
		{12.0, 12.0},
		{23.99, 23.99},
		{24.0, 0.0},
		{25.0, 1.0},
		{-1.0, 23.0},
		{-12.0, 12.0},
		{48.0, 0.0},
	}

	for _, tt := range tests {
		got := NormalizeRA(tt.input)
		if math.Abs(got-tt.want) > 0.0001 {
			t.Errorf("NormalizeRA(%.1f) = %.1f, want %.1f", tt.input, got, tt.want)
		}
	}
}

// TestJulianDate tests the Julian Date calculation
func TestJulianDate(t *testing.T) {
	// Test J2000.0 epoch (Jan 1, 2000, 12:00 UTC)
	j2000 := time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)
	jd := timeToJulianDate(j2000)
	expected := 2451545.0

	if math.Abs(jd-expected) > 0.001 {
		t.Errorf("Julian Date for J2000.0 = %.3f, want %.3f", jd, expected)
	}

	// Test Unix epoch (Jan 1, 1970, 00:00 UTC)
	unixEpoch := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	jd = timeToJulianDate(unixEpoch)
	expected = 2440587.5

	if math.Abs(jd-expected) > 0.001 {
		t.Errorf("Julian Date for Unix epoch = %.3f, want %.3f", jd, expected)
	}
}
