package coordinates

import (
	"math"
	"time"
)

// Constants for coordinate calculations
const (
	// DegreesToRadians converts degrees to radians
	DegreesToRadians = math.Pi / 180.0

	// RadiansToDegrees converts radians to degrees
	RadiansToDegrees = 180.0 / math.Pi

	// EarthRadiusKm is the Earth's radius in kilometers (WGS84 mean radius)
	EarthRadiusKm = 6371.0

	// FeetToMeters converts feet to meters
	FeetToMeters = 0.3048

	// MetersToFeet converts meters to feet
	MetersToFeet = 3.28084
)

// Geographic represents a position on Earth's surface.
// Uses the WGS84 coordinate system (same as GPS).
type Geographic struct {
	// Latitude in decimal degrees (-90 to +90)
	// Positive = North, Negative = South
	Latitude float64

	// Longitude in decimal degrees (-180 to +180)
	// Positive = East, Negative = West
	Longitude float64

	// Altitude in meters above mean sea level (MSL)
	Altitude float64
}

// HorizontalCoordinates represents a position in the local horizontal coordinate system.
// Also known as Alt/Az (Altitude-Azimuth) coordinates.
// This is the "natural" coordinate system for alt-azimuth telescope mounts.
type HorizontalCoordinates struct {
	// Altitude (elevation) in degrees above the horizon (0-90)
	// 0 = horizon, 90 = zenith (straight up)
	// Negative values are below the horizon
	Altitude float64

	// Azimuth in degrees from north (0-360)
	// 0/360 = North, 90 = East, 180 = South, 270 = West
	Azimuth float64
}

// EquatorialCoordinates represents a position in the equatorial coordinate system.
// This is used for equatorially mounted telescopes.
type EquatorialCoordinates struct {
	// RightAscension (RA) in decimal hours (0-24)
	// The celestial equivalent of longitude
	// Increases eastward along the celestial equator
	RightAscension float64

	// Declination (Dec) in decimal degrees (-90 to +90)
	// The celestial equivalent of latitude
	// 0 = celestial equator, +90 = north celestial pole, -90 = south celestial pole
	Declination float64
}

// Observer represents the geographic location of the observer/telescope.
// This is required for all coordinate transformations as they depend on
// the observer's position on Earth.
type Observer struct {
	// Location is the observer's position on Earth
	Location Geographic

	// Timezone is the IANA timezone name (e.g., "America/New_York")
	// Used for time conversions, though all internal calculations use UTC
	Timezone string
}

// AircraftPosition represents a complete aircraft position.
// This combines geographic position with velocity information.
type AircraftPosition struct {
	// Position is the aircraft's current geographic location
	Position Geographic

	// Timestamp is when this position was measured (UTC)
	Timestamp time.Time

	// GroundSpeed in knots
	GroundSpeed float64

	// Track is the ground track (heading) in degrees (0-360)
	Track float64

	// VerticalRate in feet per minute (positive = climbing)
	VerticalRate float64
}

// ToRadians converts the Geographic coordinates to radians.
// Returns (latRad, lonRad, altMeters).
func (g Geographic) ToRadians() (float64, float64, float64) {
	return g.Latitude * DegreesToRadians,
		g.Longitude * DegreesToRadians,
		g.Altitude
}

// ToRadians converts HorizontalCoordinates to radians.
// Returns (altRad, azRad).
func (h HorizontalCoordinates) ToRadians() (float64, float64) {
	return h.Altitude * DegreesToRadians,
		h.Azimuth * DegreesToRadians
}

// ToDegrees converts radians to HorizontalCoordinates in degrees.
func ToHorizontalDegrees(altRad, azRad float64) HorizontalCoordinates {
	return HorizontalCoordinates{
		Altitude: altRad * RadiansToDegrees,
		Azimuth:  azRad * RadiansToDegrees,
	}
}

// ToRadians converts EquatorialCoordinates to radians.
// Returns (raRad, decRad).
// Note: RA is converted from hours to radians (1 hour = 15 degrees = π/12 radians)
func (e EquatorialCoordinates) ToRadians() (float64, float64) {
	raRad := e.RightAscension * 15.0 * DegreesToRadians // Convert hours to degrees to radians
	decRad := e.Declination * DegreesToRadians
	return raRad, decRad
}

// ToEquatorialDegrees converts radians to EquatorialCoordinates.
// raRad is in radians, decRad is in radians.
// Returns RA in hours and Dec in degrees.
func ToEquatorialDegrees(raRad, decRad float64) EquatorialCoordinates {
	raHours := (raRad * RadiansToDegrees) / 15.0 // Convert radians to degrees to hours
	decDegrees := decRad * RadiansToDegrees
	return EquatorialCoordinates{
		RightAscension: raHours,
		Declination:    decDegrees,
	}
}

// NormalizeAzimuth ensures azimuth is in the range [0, 360).
func NormalizeAzimuth(azimuth float64) float64 {
	az := math.Mod(azimuth, 360.0)
	if az < 0 {
		az += 360.0
	}
	return az
}

// NormalizeRA ensures right ascension is in the range [0, 24).
func NormalizeRA(ra float64) float64 {
	raHours := math.Mod(ra, 24.0)
	if raHours < 0 {
		raHours += 24.0
	}
	return raHours
}

// Bearing calculates the initial bearing (forward azimuth) from one point to another.
// Uses spherical trigonometry to calculate the bearing along a great circle.
// Returns bearing in degrees (0-360), where 0/360 = North, 90 = East, 180 = South, 270 = West.
func Bearing(from, to Geographic) float64 {
	lat1 := from.Latitude * DegreesToRadians
	lon1 := from.Longitude * DegreesToRadians
	lat2 := to.Latitude * DegreesToRadians
	lon2 := to.Longitude * DegreesToRadians

	dLon := lon2 - lon1
	y := math.Sin(dLon) * math.Cos(lat2)
	x := math.Cos(lat1)*math.Sin(lat2) - math.Sin(lat1)*math.Cos(lat2)*math.Cos(dLon)
	bearing := math.Atan2(y, x) * RadiansToDegrees
	
	// Normalize to 0-360
	if bearing < 0 {
		bearing += 360
	}
	
	return bearing
}

// DistanceNauticalMiles calculates the great-circle distance between two points.
// Uses the Haversine formula for accuracy over short and long distances.
// Returns distance in nautical miles.
func DistanceNauticalMiles(from, to Geographic) float64 {
	lat1Rad := from.Latitude * DegreesToRadians
	lon1Rad := from.Longitude * DegreesToRadians
	lat2Rad := to.Latitude * DegreesToRadians
	lon2Rad := to.Longitude * DegreesToRadians

	dLat := lat2Rad - lat1Rad
	dLon := lon2Rad - lon1Rad

	// Haversine formula
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	distanceKm := EarthRadiusKm * c
	// Convert km to nautical miles (1 nm = 1.852 km)
	return distanceKm / 1.852
}

// EstimateTimeToClosestApproach calculates when an aircraft will be closest to the observer.
// Returns:
//   - closestRangeNM: The minimum distance in nautical miles
//   - timeToClosest: Duration until closest approach (negative if moving away)
//   - isApproaching: True if aircraft is currently approaching
func EstimateTimeToClosestApproach(
	observerPos Geographic,
	aircraftPos Geographic,
	groundSpeedKnots float64,
	trackDegrees float64,
) (closestRangeNM float64, timeToClosest time.Duration, isApproaching bool) {
	// Current range
	currentRange := DistanceNauticalMiles(observerPos, aircraftPos)

	// Calculate bearing from observer to aircraft
	lat1 := observerPos.Latitude * DegreesToRadians
	lon1 := observerPos.Longitude * DegreesToRadians
	lat2 := aircraftPos.Latitude * DegreesToRadians
	lon2 := aircraftPos.Longitude * DegreesToRadians

	dLon := lon2 - lon1
	y := math.Sin(dLon) * math.Cos(lat2)
	x := math.Cos(lat1)*math.Sin(lat2) - math.Sin(lat1)*math.Cos(lat2)*math.Cos(dLon)
	bearingToAircraft := math.Atan2(y, x) * RadiansToDegrees
	if bearingToAircraft < 0 {
		bearingToAircraft += 360
	}

	// Calculate relative angle: difference between aircraft track and bearing to observer
	// If aircraft is flying directly toward observer, this is ~180°
	// If aircraft is flying directly away, this is ~0°
	bearingFromAircraft := NormalizeAzimuth(bearingToAircraft + 180)
	relativeAngle := math.Abs(trackDegrees - bearingFromAircraft)
	if relativeAngle > 180 {
		relativeAngle = 360 - relativeAngle
	}

	// Component of velocity toward/away from observer
	// Positive = approaching, Negative = receding
	relativeAngleRad := relativeAngle * DegreesToRadians
	velocityToward := groundSpeedKnots * math.Cos(relativeAngleRad)

	isApproaching = velocityToward > 0.1 // Consider approaching if > 0.1 knots toward

	if !isApproaching || math.Abs(velocityToward) < 0.1 {
		// Moving away or perpendicular - current range is closest
		return currentRange, 0, false
	}

	// Time to closest approach (when radial velocity = 0)
	// Using simple geometry: t = current_range * cos(angle) / speed
	timeHours := currentRange * math.Cos(relativeAngleRad) / groundSpeedKnots
	if timeHours < 0 {
		timeHours = 0
	}
	timeToClosest = time.Duration(timeHours * float64(time.Hour))

	// Closest range is the perpendicular distance (cross-track distance)
	closestRangeNM = currentRange * math.Sin(relativeAngleRad)
	if closestRangeNM < 0 {
		closestRangeNM = -closestRangeNM
	}

	return closestRangeNM, timeToClosest, isApproaching
}

// EstimateTimeToRange calculates when an aircraft will reach a specific range from observer.
// Returns duration until target range (0 if already past or not on intercept course).
func EstimateTimeToRange(
	observerPos Geographic,
	aircraftPos Geographic,
	groundSpeedKnots float64,
	trackDegrees float64,
	targetRangeNM float64,
) time.Duration {
	currentRange := DistanceNauticalMiles(observerPos, aircraftPos)

	// Already within target range
	if currentRange <= targetRangeNM {
		return 0
	}

	// Check if approaching
	_, _, approaching := EstimateTimeToClosestApproach(observerPos, aircraftPos, groundSpeedKnots, trackDegrees)
	if !approaching {
		return 0 // Not approaching, will never reach target range
	}

	// Simple approximation: assume straight-line approach
	// More accurate would use great circle, but this is reasonable for short distances
	distanceToTravel := currentRange - targetRangeNM
	if distanceToTravel <= 0 {
		return 0
	}

	// Calculate bearing and relative angle (same as EstimateTimeToClosestApproach)
	lat1 := observerPos.Latitude * DegreesToRadians
	lon1 := observerPos.Longitude * DegreesToRadians
	lat2 := aircraftPos.Latitude * DegreesToRadians
	lon2 := aircraftPos.Longitude * DegreesToRadians

	dLon := lon2 - lon1
	y := math.Sin(dLon) * math.Cos(lat2)
	x := math.Cos(lat1)*math.Sin(lat2) - math.Sin(lat1)*math.Cos(lat2)*math.Cos(dLon)
	bearingToAircraft := math.Atan2(y, x) * RadiansToDegrees
	if bearingToAircraft < 0 {
		bearingToAircraft += 360
	}

	bearingFromAircraft := NormalizeAzimuth(bearingToAircraft + 180)
	relativeAngle := math.Abs(trackDegrees - bearingFromAircraft)
	if relativeAngle > 180 {
		relativeAngle = 360 - relativeAngle
	}

	relativeAngleRad := relativeAngle * DegreesToRadians
	velocityToward := groundSpeedKnots * math.Cos(relativeAngleRad)

	if velocityToward <= 0 {
		return 0 // Not moving toward observer
	}

	timeHours := distanceToTravel / velocityToward
	return time.Duration(timeHours * float64(time.Hour))
}
