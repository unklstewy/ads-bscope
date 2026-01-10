package coordinates

import (
	"math"
	"time"
)

// GeographicToHorizontal converts geographic coordinates (lat/lon/alt) to
// horizontal coordinates (alt/az) as seen from the observer's location.
//
// This is the core transformation for Alt/Az telescope mounts.
// The calculation accounts for:
// - Observer's position on Earth
// - Target's position on Earth
// - Earth's curvature
//
// Parameters:
//   - target: The geographic position to observe (e.g., aircraft position)
//   - observer: The observer's geographic location (telescope position)
//   - timestamp: The time of observation (UTC)
//
// Returns: HorizontalCoordinates (altitude and azimuth in degrees)
//
// Reference: This uses the "great circle" method for calculating bearing
// and distance, then converts to altitude based on the elevation angle.
func GeographicToHorizontal(target Geographic, observer Observer, timestamp time.Time) HorizontalCoordinates {
	// Convert to radians for trigonometric calculations
	obsLatRad, obsLonRad, obsAltM := observer.Location.ToRadians()
	tgtLatRad, tgtLonRad, tgtAltM := target.ToRadians()

	// Calculate the difference in longitude
	deltaLon := tgtLonRad - obsLonRad

	// Calculate azimuth using the bearing formula
	// azimuth = atan2(sin(Δlon)·cos(lat2), cos(lat1)·sin(lat2) − sin(lat1)·cos(lat2)·cos(Δlon))
	y := math.Sin(deltaLon) * math.Cos(tgtLatRad)
	x := math.Cos(obsLatRad)*math.Sin(tgtLatRad) -
		math.Sin(obsLatRad)*math.Cos(tgtLatRad)*math.Cos(deltaLon)
	azimuthRad := math.Atan2(y, x)

	// Convert azimuth to degrees and normalize to [0, 360)
	azimuth := NormalizeAzimuth(azimuthRad * RadiansToDegrees)

	// Calculate great circle distance on Earth's surface
	// Using the Haversine formula for better accuracy
	deltaLat := tgtLatRad - obsLatRad
	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(obsLatRad)*math.Cos(tgtLatRad)*
			math.Sin(deltaLon/2)*math.Sin(deltaLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	surfaceDistanceKm := EarthRadiusKm * c

	// Calculate altitude (elevation angle)
	// This accounts for:
	// 1. The altitude difference between observer and target
	// 2. The curved surface distance between them
	// altitude = atan2(Δh, d)
	// where Δh is the altitude difference and d is the surface distance
	deltaAltitudeM := tgtAltM - obsAltM
	surfaceDistanceM := surfaceDistanceKm * 1000.0

	// Calculate elevation angle
	altitudeRad := math.Atan2(deltaAltitudeM, surfaceDistanceM)
	altitude := altitudeRad * RadiansToDegrees

	return HorizontalCoordinates{
		Altitude: altitude,
		Azimuth:  azimuth,
	}
}

// HorizontalToEquatorial converts horizontal coordinates (alt/az) to
// equatorial coordinates (RA/Dec) for a given observer and time.
//
// This is required for equatorially mounted telescopes.
// The calculation requires:
// - Local Sidereal Time (LST) which depends on observer's longitude and UTC time
// - Observer's latitude
//
// Parameters:
//   - horizontal: The horizontal coordinates to convert
//   - observer: The observer's geographic location
//   - timestamp: The time of observation (UTC)
//
// Returns: EquatorialCoordinates (RA in hours, Dec in degrees)
//
// Reference: Standard astronomical coordinate transformation
// using the alt/az to RA/Dec formulas.
func HorizontalToEquatorial(horizontal HorizontalCoordinates, observer Observer, timestamp time.Time) EquatorialCoordinates {
	// Convert to radians
	altRad, azRad := horizontal.ToRadians()
	latRad, _, _ := observer.Location.ToRadians()

	// Calculate Local Sidereal Time (LST)
	// LST is the right ascension currently on the observer's meridian
	lst := CalculateLocalSiderealTime(observer.Location.Longitude, timestamp)
	lstRad := lst * 15.0 * DegreesToRadians // Convert hours to radians

	// Calculate Hour Angle (HA)
	// HA = atan2(-sin(az), cos(az)·sin(lat) - tan(alt)·cos(lat))
	haRad := math.Atan2(
		-math.Sin(azRad),
		math.Cos(azRad)*math.Sin(latRad)-math.Tan(altRad)*math.Cos(latRad),
	)

	// Calculate Declination
	// dec = asin(sin(lat)·sin(alt) + cos(lat)·cos(alt)·cos(az))
	decRad := math.Asin(
		math.Sin(latRad)*math.Sin(altRad) +
			math.Cos(latRad)*math.Cos(altRad)*math.Cos(azRad),
	)

	// Calculate Right Ascension
	// RA = LST - HA
	raRad := lstRad - haRad

	// Convert to standard units and normalize
	eq := ToEquatorialDegrees(raRad, decRad)
	eq.RightAscension = NormalizeRA(eq.RightAscension)

	return eq
}

// EquatorialToHorizontal converts equatorial coordinates (RA/Dec) to
// horizontal coordinates (alt/az) for a given observer and time.
//
// This is the inverse of HorizontalToEquatorial.
//
// Parameters:
//   - equatorial: The equatorial coordinates to convert
//   - observer: The observer's geographic location
//   - timestamp: The time of observation (UTC)
//
// Returns: HorizontalCoordinates (altitude and azimuth in degrees)
func EquatorialToHorizontal(equatorial EquatorialCoordinates, observer Observer, timestamp time.Time) HorizontalCoordinates {
	// Convert to radians
	raRad, decRad := equatorial.ToRadians()
	latRad, _, _ := observer.Location.ToRadians()

	// Calculate Local Sidereal Time
	lst := CalculateLocalSiderealTime(observer.Location.Longitude, timestamp)
	lstRad := lst * 15.0 * DegreesToRadians

	// Calculate Hour Angle
	// HA = LST - RA
	haRad := lstRad - raRad

	// Calculate Altitude
	// alt = asin(sin(dec)·sin(lat) + cos(dec)·cos(lat)·cos(HA))
	altRad := math.Asin(
		math.Sin(decRad)*math.Sin(latRad) +
			math.Cos(decRad)*math.Cos(latRad)*math.Cos(haRad),
	)

	// Calculate Azimuth
	// az = atan2(-sin(HA), cos(HA)·sin(lat) - tan(dec)·cos(lat))
	azRad := math.Atan2(
		-math.Sin(haRad),
		math.Cos(haRad)*math.Sin(latRad)-math.Tan(decRad)*math.Cos(latRad),
	)

	// Convert to degrees and normalize
	horiz := ToHorizontalDegrees(altRad, azRad)
	horiz.Azimuth = NormalizeAzimuth(horiz.Azimuth)

	return horiz
}

// CalculateLocalSiderealTime calculates the Local Sidereal Time (LST) for
// a given longitude and UTC time.
//
// LST is the right ascension that is currently on the observer's meridian.
// It's required for converting between horizontal and equatorial coordinates.
//
// Parameters:
//   - longitudeDeg: Observer's longitude in decimal degrees
//   - utcTime: The time in UTC
//
// Returns: LST in decimal hours (0-24)
//
// Reference: Simplified formula accurate to ~1 second
// For more precision, use the IAU SOFA library or similar.
func CalculateLocalSiderealTime(longitudeDeg float64, utcTime time.Time) float64 {
	// Calculate Julian Date
	jd := timeToJulianDate(utcTime)

	// Calculate number of days since J2000.0 (Jan 1, 2000, 12:00 UTC)
	d := jd - 2451545.0

	// Calculate Greenwich Mean Sidereal Time (GMST) in hours
	// This is a simplified formula accurate to about 1 second
	gmst := 18.697374558 + 24.06570982441908*d

	// Convert to range [0, 24)
	gmst = math.Mod(gmst, 24.0)
	if gmst < 0 {
		gmst += 24.0
	}

	// Calculate Local Sidereal Time
	// LST = GMST + longitude (in hours)
	lst := gmst + (longitudeDeg / 15.0)

	// Normalize to [0, 24)
	return NormalizeRA(lst)
}

// timeToJulianDate converts a Go time.Time to Julian Date.
// The Julian Date is the number of days since noon on January 1, 4713 BC.
func timeToJulianDate(t time.Time) float64 {
	// Get UTC time components
	year := t.Year()
	month := int(t.Month())
	day := t.Day()
	hour := t.Hour()
	minute := t.Minute()
	second := t.Second()

	// Convert to decimal day
	decimalDay := float64(day) +
		float64(hour)/24.0 +
		float64(minute)/(24.0*60.0) +
		float64(second)/(24.0*60.0*60.0)

	// Adjust for January/February
	if month <= 2 {
		year--
		month += 12
	}

	// Calculate Julian Date using the standard formula
	a := year / 100
	b := 2 - a + a/4

	jd := float64(int(365.25*float64(year+4716))) +
		float64(int(30.6001*float64(month+1))) +
		decimalDay + float64(b) - 1524.5

	return jd
}
