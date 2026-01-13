package coordinates

import (
	"math"
	"time"
)

// SunPosition represents the sun's position in the sky
type SunPosition struct {
	Altitude  float64   // Degrees above horizon
	Azimuth   float64   // Degrees from north
	Elevation float64   // Same as altitude (alias)
	Time      time.Time // Calculation time
}

// CalculateSunPosition calculates the sun's position for a given observer and time.
// Uses simplified algorithms accurate to about 1 arcminute.
// Based on NOAA solar calculator algorithms.
func CalculateSunPosition(observer Observer, t time.Time) SunPosition {
	// Convert to UTC
	utc := t.UTC()

	// Julian date calculation
	jd := julianDate(utc)

	// Julian century from J2000.0
	jc := (jd - 2451545.0) / 36525.0

	// Sun's geometric mean longitude (degrees)
	L0 := math.Mod(280.46646+jc*(36000.76983+jc*0.0003032), 360.0)

	// Sun's mean anomaly (degrees)
	M := 357.52911 + jc*(35999.05029-0.0001537*jc)
	Mrad := deg2rad(M)

	// Sun's equation of center
	C := math.Sin(Mrad)*(1.914602-jc*(0.004817+0.000014*jc)) +
		math.Sin(2*Mrad)*(0.019993-0.000101*jc) +
		math.Sin(3*Mrad)*0.000289

	// Sun's true longitude (degrees)
	sunTrueLong := L0 + C

	// Sun's apparent longitude (degrees) - corrected for aberration and nutation
	omega := 125.04 - 1934.136*jc
	lambda := sunTrueLong - 0.00569 - 0.00478*math.Sin(deg2rad(omega))

	// Obliquity of ecliptic (degrees)
	epsilon0 := 23.0 + (26.0+(21.448-jc*(46.815+jc*(0.00059-jc*0.001813))))/60.0/60.0
	epsilon := epsilon0 + 0.00256*math.Cos(deg2rad(omega))

	// Sun's right ascension (degrees)
	lambdaRad := deg2rad(lambda)
	epsilonRad := deg2rad(epsilon)
	ra := rad2deg(math.Atan2(math.Cos(epsilonRad)*math.Sin(lambdaRad), math.Cos(lambdaRad)))
	if ra < 0 {
		ra += 360
	}

	// Sun's declination (degrees)
	dec := rad2deg(math.Asin(math.Sin(epsilonRad) * math.Sin(lambdaRad)))

	// Greenwich mean sidereal time (degrees)
	gmst := math.Mod(280.46061837+360.98564736629*(jd-2451545.0)+
		0.000387933*jc*jc-jc*jc*jc/38710000.0, 360.0)

	// Local sidereal time (degrees)
	lst := math.Mod(gmst+observer.Location.Longitude, 360.0)

	// Hour angle (degrees)
	ha := lst - ra
	if ha < 0 {
		ha += 360
	}
	if ha > 180 {
		ha -= 360
	}

	// Convert to horizontal coordinates (altitude and azimuth)
	latRad := deg2rad(observer.Location.Latitude)
	decRad := deg2rad(dec)
	haRad := deg2rad(ha)

	// Altitude (elevation)
	sinAlt := math.Sin(latRad)*math.Sin(decRad) + math.Cos(latRad)*math.Cos(decRad)*math.Cos(haRad)
	altitude := rad2deg(math.Asin(sinAlt))

	// Azimuth (from north, eastward)
	cosAz := (math.Sin(decRad) - math.Sin(latRad)*math.Sin(deg2rad(altitude))) / (math.Cos(latRad) * math.Cos(deg2rad(altitude)))
	// Clamp to prevent domain errors
	if cosAz > 1.0 {
		cosAz = 1.0
	}
	if cosAz < -1.0 {
		cosAz = -1.0
	}

	azimuth := rad2deg(math.Acos(cosAz))

	// Adjust azimuth based on hour angle
	if math.Sin(haRad) > 0 {
		azimuth = 360.0 - azimuth
	}

	// Atmospheric refraction correction (only if sun is above horizon)
	if altitude > -0.833 { // -0.833° accounts for sun's radius and typical refraction
		// Simple refraction formula
		if altitude < 85.0 {
			tanAlt := math.Tan(deg2rad(altitude))
			refraction := 0.0
			if altitude > 5.0 {
				refraction = 58.1/tanAlt - 0.07/(tanAlt*tanAlt*tanAlt) + 0.000086/(tanAlt*tanAlt*tanAlt*tanAlt*tanAlt)
			} else if altitude > -0.575 {
				refraction = 1735.0 + altitude*(-518.2+altitude*(103.4+altitude*(-12.79+altitude*0.711)))
			}
			altitude += refraction / 3600.0 // Convert arcseconds to degrees
		}
	}

	return SunPosition{
		Altitude:  altitude,
		Azimuth:   azimuth,
		Elevation: altitude,
		Time:      t,
	}
}

// IsSunAboveHorizon returns true if the sun is above the horizon
func (sp SunPosition) IsSunAboveHorizon() bool {
	return sp.Altitude > -0.833 // Accounts for sun's radius and refraction
}

// AngularSeparation calculates the angular distance between the sun and a point in the sky.
// Returns the separation in degrees.
func (sp SunPosition) AngularSeparation(altitude, azimuth float64) float64 {
	// Convert to radians
	sunAltRad := deg2rad(sp.Altitude)
	sunAzRad := deg2rad(sp.Azimuth)
	targetAltRad := deg2rad(altitude)
	targetAzRad := deg2rad(azimuth)

	// Haversine formula for great circle distance
	dAz := targetAzRad - sunAzRad

	sinDist := math.Sqrt(
		math.Pow(math.Cos(targetAltRad)*math.Sin(dAz), 2) +
			math.Pow(math.Cos(sunAltRad)*math.Sin(targetAltRad)-
				math.Sin(sunAltRad)*math.Cos(targetAltRad)*math.Cos(dAz), 2),
	)

	cosDist := math.Sin(sunAltRad)*math.Sin(targetAltRad) +
		math.Cos(sunAltRad)*math.Cos(targetAltRad)*math.Cos(dAz)

	return rad2deg(math.Atan2(sinDist, cosDist))
}

// SolarSafetyZone represents safety thresholds for solar proximity
type SolarSafetyZone int

const (
	SafeZoneClear    SolarSafetyZone = 0 // > 20° from sun - safe
	SafeZoneCaution  SolarSafetyZone = 1 // 10-20° from sun - caution
	SafeZoneWarning  SolarSafetyZone = 2 // 5-10° from sun - warning
	SafeZoneDanger   SolarSafetyZone = 3 // 2-5° from sun - danger
	SafeZoneCritical SolarSafetyZone = 4 // < 2° from sun - CRITICAL
)

// GetSafetyZone returns the safety zone based on angular separation from the sun.
func GetSafetyZone(separation float64) SolarSafetyZone {
	if separation < 2.0 {
		return SafeZoneCritical // IMMEDIATE DANGER
	} else if separation < 5.0 {
		return SafeZoneDanger // High risk
	} else if separation < 10.0 {
		return SafeZoneWarning // Moderate risk
	} else if separation < 20.0 {
		return SafeZoneCaution // Low risk
	}
	return SafeZoneClear // Safe
}

// GetSafetyZoneName returns a human-readable name for the safety zone
func GetSafetyZoneName(zone SolarSafetyZone) string {
	switch zone {
	case SafeZoneClear:
		return "CLEAR"
	case SafeZoneCaution:
		return "CAUTION"
	case SafeZoneWarning:
		return "WARNING"
	case SafeZoneDanger:
		return "DANGER"
	case SafeZoneCritical:
		return "CRITICAL"
	default:
		return "UNKNOWN"
	}
}

// julianDate calculates the Julian Date from a time.Time
func julianDate(t time.Time) float64 {
	year := t.Year()
	month := int(t.Month())
	day := t.Day()
	hour := t.Hour()
	minute := t.Minute()
	second := t.Second()

	// Adjust for January and February
	if month <= 2 {
		year--
		month += 12
	}

	// Julian day number
	a := year / 100
	b := 2 - a + a/4

	jd := float64(int(365.25*float64(year+4716))) +
		float64(int(30.6001*float64(month+1))) +
		float64(day+b) - 1524.5

	// Add fractional day
	dayFraction := (float64(hour) + float64(minute)/60.0 + float64(second)/3600.0) / 24.0
	jd += dayFraction

	return jd
}

// deg2rad converts degrees to radians
func deg2rad(deg float64) float64 {
	return deg * math.Pi / 180.0
}

// rad2deg converts radians to degrees
func rad2deg(rad float64) float64 {
	return rad * 180.0 / math.Pi
}
