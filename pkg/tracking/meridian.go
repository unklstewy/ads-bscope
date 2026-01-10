package tracking

import (
	"math"

	"github.com/unklstewy/ads-bscope/pkg/coordinates"
)

// MeridianEvent describes what happens when tracking crosses the meridian.
type MeridianEvent int

const (
	// NoMeridianEvent means tracking can continue normally
	NoMeridianEvent MeridianEvent = iota

	// MeridianFlipRequired means the telescope must perform a meridian flip
	// This applies to equatorial mounts when the hour angle crosses limits
	MeridianFlipRequired

	// ZenithCrossing means the target passes directly overhead (altitude ~90°)
	// Both alt-az and equatorial mounts have issues here
	ZenithCrossing

	// HorizonCrossing means the target goes below the horizon
	// Tracking must stop
	HorizonCrossing
)

// TrackingLimits defines the safe tracking limits for a telescope mount.
type TrackingLimits struct {
	// MinAltitude is the minimum altitude in degrees (typically 10-20°)
	// Below this, atmospheric refraction and obstacles become issues
	MinAltitude float64

	// MaxAltitude is the maximum altitude in degrees (typically 85-88°)
	// Near zenith (90°), tracking becomes unstable
	MaxAltitude float64

	// MeridianFlipHourAngle is the hour angle limit for equatorial mounts
	// Typical values: -6 to +6 hours (±90°)
	// When |HA| exceeds this, a meridian flip is needed
	MeridianFlipHourAngle float64

	// AzimuthWrapLimit is the azimuth limit for alt-az mounts (degrees)
	// Some mounts have physical stops (e.g., 270° rotation limit)
	// 0 = no limit (full 360° rotation)
	AzimuthWrapLimit float64
}

// DefaultTrackingLimits returns conservative tracking limits suitable for most telescopes.
func DefaultTrackingLimits() TrackingLimits {
	return TrackingLimits{
		MinAltitude:           15.0, // 15° above horizon
		MaxAltitude:           85.0, // 5° from zenith
		MeridianFlipHourAngle: 6.0,  // ±6 hours (±90°)
		AzimuthWrapLimit:      0.0,  // No limit (full rotation)
	}
}

// TrackingLimitsFromConfig creates TrackingLimits from telescope configuration.
// This uses the telescope-specific altitude limits from config.
func TrackingLimitsFromConfig(minAlt, maxAlt float64) TrackingLimits {
	limits := DefaultTrackingLimits()
	limits.MinAltitude = minAlt
	limits.MaxAltitude = maxAlt
	return limits
}

// CheckMeridianEvent determines if tracking will encounter a meridian event.
// This checks both the current position and predicted future position.
//
// Parameters:
//   - currentHoriz: Current telescope horizontal coordinates
//   - targetHoriz: Target horizontal coordinates
//   - observer: Observer location
//   - limits: Tracking limits for this telescope
//   - supportsMeridianFlip: Whether the telescope requires meridian flips (false for Seestar fork mounts)
//
// Returns: MeridianEvent type and a recommendation string
func CheckMeridianEvent(
	currentHoriz, targetHoriz coordinates.HorizontalCoordinates,
	observer coordinates.Observer,
	limits TrackingLimits,
	supportsMeridianFlip bool,
) (MeridianEvent, string) {
	// Check for horizon crossing
	if targetHoriz.Altitude < limits.MinAltitude {
		return HorizonCrossing, "Target is below minimum altitude - tracking not possible"
	}

	// Check for zenith crossing
	// Above MaxAltitude (typically 80-85°), field rotation becomes severe on Alt-Az mounts
	// Seestar specifically: above 80° field rotation causes poor tracking, above 85° may stop stacking
	if targetHoriz.Altitude > limits.MaxAltitude {
		return ZenithCrossing, "Target near zenith - severe field rotation, recommend waiting"
	}

	// Check for azimuth wrap (Alt-Az mounts with physical stops)
	// Skip for telescopes with 360° rotation (like Seestar)
	if supportsMeridianFlip && limits.AzimuthWrapLimit > 0 {
		if isAzimuthWrap(currentHoriz.Azimuth, targetHoriz.Azimuth, limits.AzimuthWrapLimit) {
			return MeridianFlipRequired, "Azimuth wrap limit reached - reposition telescope"
		}
	}

	// Check for rapid azimuth changes near zenith (affects both mount types)
	if targetHoriz.Altitude > 80.0 && currentHoriz.Altitude > 80.0 {
		azimuthChange := azimuthDifference(currentHoriz.Azimuth, targetHoriz.Azimuth)
		if azimuthChange > 90.0 {
			return ZenithCrossing, "Rapid azimuth change near zenith - tracking may be unstable"
		}
	}

	return NoMeridianEvent, "Tracking OK"
}

// CheckEquatorialMeridianFlip checks if an equatorial mount needs a meridian flip.
// This uses hour angle limits rather than altitude/azimuth.
//
// Parameters:
//   - ra: Target right ascension in hours
//   - dec: Target declination in degrees
//   - observer: Observer location
//   - lst: Local sidereal time in hours
//   - limits: Tracking limits
//
// Returns: True if meridian flip is required
func CheckEquatorialMeridianFlip(
	ra, dec float64,
	observer coordinates.Observer,
	lst float64,
	limits TrackingLimits,
) (bool, string) {
	// Calculate hour angle: HA = LST - RA
	ha := lst - ra

	// Normalize to [-12, +12] hours
	if ha > 12.0 {
		ha -= 24.0
	} else if ha < -12.0 {
		ha += 24.0
	}

	// Check if hour angle exceeds limits
	if math.Abs(ha) > limits.MeridianFlipHourAngle {
		side := "west"
		if ha < 0 {
			side = "east"
		}
		return true, "Hour angle limit exceeded - meridian flip required (pier on " + side + " side)"
	}

	// Check for declination near pole (tracking becomes difficult)
	if math.Abs(dec) > 85.0 {
		return false, "Target near celestial pole - tracking may be difficult"
	}

	return false, "Equatorial tracking OK"
}

// PredictMeridianCrossing predicts when a moving target will cross tracking limits.
// This is useful for alerting before a meridian flip is needed.
//
// Parameters:
//   - currentPos, futurePos: Target positions at different times
//   - limits: Tracking limits
//
// Returns: Estimated seconds until meridian event, or -1 if no event predicted
func PredictMeridianCrossing(
	currentPos, futurePos coordinates.HorizontalCoordinates,
	limits TrackingLimits,
) float64 {
	// If already past limits, return 0
	if currentPos.Altitude > limits.MaxAltitude || currentPos.Altitude < limits.MinAltitude {
		return 0
	}

	// Check if altitude is approaching limits
	altitudeRate := futurePos.Altitude - currentPos.Altitude // degrees per prediction interval

	if altitudeRate > 0 && currentPos.Altitude > 70.0 {
		// Approaching zenith
		degreesToLimit := limits.MaxAltitude - currentPos.Altitude
		if degreesToLimit > 0 && altitudeRate > 0 {
			// Rough estimate (would need time delta for accuracy)
			return degreesToLimit / altitudeRate
		}
	} else if altitudeRate < 0 && currentPos.Altitude < 30.0 {
		// Approaching horizon
		degreesToLimit := currentPos.Altitude - limits.MinAltitude
		if degreesToLimit > 0 && altitudeRate < 0 {
			return degreesToLimit / math.Abs(altitudeRate)
		}
	}

	return -1 // No meridian event predicted
}

// azimuthDifference calculates the smallest angle between two azimuths.
// Handles wrap-around (e.g., 359° to 1° is 2°, not 358°).
func azimuthDifference(az1, az2 float64) float64 {
	diff := math.Abs(az2 - az1)
	if diff > 180.0 {
		diff = 360.0 - diff
	}
	return diff
}

// isAzimuthWrap checks if moving from current to target azimuth would exceed wrap limit.
func isAzimuthWrap(currentAz, targetAz, wrapLimit float64) bool {
	if wrapLimit <= 0 {
		return false // No limit
	}

	// Calculate total rotation from starting position (assume 0°)
	// This is simplified - real implementation would track cumulative rotation
	diff := azimuthDifference(currentAz, targetAz)
	return diff > wrapLimit
}

// RecommendTrackingStrategy provides recommendations for tracking through problematic zones.
//
// Parameters:
//   - event: The meridian event type
//   - currentAlt: Current altitude in degrees
//
// Returns: Human-readable recommendation
func RecommendTrackingStrategy(event MeridianEvent, currentAlt float64) string {
	switch event {
	case NoMeridianEvent:
		return "Continue tracking normally"

	case MeridianFlipRequired:
		return "STOP tracking, perform meridian flip, then resume. This will take 30-60 seconds."

	case ZenithCrossing:
		if currentAlt > 85.0 {
			return "Target passing through zenith. Options: 1) Stop tracking and wait for target to descend, or 2) Accept degraded tracking quality."
		}
		return "Target approaching zenith - prepare to pause tracking"

	case HorizonCrossing:
		return "Target below horizon - tracking not possible. Wait for target to rise above minimum altitude."

	default:
		return "Unknown tracking condition"
	}
}

// CalculateMeridianFlipDuration estimates how long a meridian flip will take.
// This is useful for prediction - we need to predict further ahead during flips.
//
// Returns: Estimated seconds for meridian flip
func CalculateMeridianFlipDuration() float64 {
	// Typical meridian flip sequence:
	// 1. Stop tracking (1-2s)
	// 2. Slew to flip position (20-40s depending on mount)
	// 3. Restart tracking (1-2s)
	// Conservative estimate: 45 seconds
	return 45.0
}

// ShouldAbortTracking determines if tracking should be immediately stopped.
// This is a safety check to prevent damage to equipment or loss of target.
func ShouldAbortTracking(horiz coordinates.HorizontalCoordinates, limits TrackingLimits) bool {
	// Abort if target goes below minimum altitude
	if horiz.Altitude < limits.MinAltitude {
		return true
	}

	// Abort if target goes above maximum altitude
	if horiz.Altitude > limits.MaxAltitude {
		return true
	}

	return false
}
