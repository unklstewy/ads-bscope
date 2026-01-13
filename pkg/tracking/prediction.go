package tracking

import (
	"math"
	"time"

	"github.com/unklstewy/ads-bscope/pkg/adsb"
	"github.com/unklstewy/ads-bscope/pkg/coordinates"
)

// Waypoint represents a navigation waypoint from a flight plan.
type Waypoint struct {
	Name      string
	Latitude  float64
	Longitude float64
	Sequence  int
	Passed    bool
}

// PredictedPosition represents an aircraft's predicted future position.
type PredictedPosition struct {
	// Position is the predicted geographic location
	Position coordinates.Geographic

	// PredictionTime is when this prediction is valid
	PredictionTime time.Time

	// Confidence is a measure of prediction reliability (0-1)
	// Lower confidence for longer predictions or erratic flight paths
	Confidence float64

	// OriginalPosition is the source position used for prediction
	OriginalPosition adsb.Aircraft
}

// PredictPosition predicts where an aircraft will be at a future time.
// This compensates for data latency in ADS-B systems (typically 1-3 seconds).
//
// The prediction uses:
// - Current position (lat/lon/altitude)
// - Ground speed and track (horizontal motion)
// - Vertical rate (climb/descent)
//
// Assumptions:
// - Aircraft maintains current speed and heading (reasonable for short predictions)
// - Vertical rate remains constant
// - No wind correction (would require weather data)
//
// Parameters:
//   - aircraft: Current aircraft state with position and velocity
//   - predictionTime: When to predict the position (typically time.Now() + latency)
//
// Returns: Predicted position with confidence score
func PredictPosition(aircraft adsb.Aircraft, predictionTime time.Time) PredictedPosition {
	// Calculate time delta
	deltaT := predictionTime.Sub(aircraft.LastSeen).Seconds()

	// For very short or negative deltas, return current position
	if deltaT <= 0 {
		return PredictedPosition{
			Position: coordinates.Geographic{
				Latitude:  aircraft.Latitude,
				Longitude: aircraft.Longitude,
				Altitude:  aircraft.Altitude * coordinates.FeetToMeters,
			},
			PredictionTime:   predictionTime,
			Confidence:       1.0,
			OriginalPosition: aircraft,
		}
	}

	// Calculate confidence - decreases with prediction time
	// 1.0 at 0s, 0.9 at 5s, 0.5 at 30s, 0.0 at 60s+
	confidence := math.Max(0.0, 1.0-deltaT/60.0)

	// Also reduce confidence if data is stale
	dataAge := time.Since(aircraft.LastSeen).Seconds()
	if dataAge > 10.0 {
		confidence *= 0.5
	}

	// Predict horizontal position using great circle navigation
	newLat, newLon := predictHorizontalPosition(
		aircraft.Latitude,
		aircraft.Longitude,
		aircraft.GroundSpeed,
		aircraft.Track,
		deltaT,
	)

	// Predict altitude change
	// VerticalRate is in feet per minute, convert to feet per second
	altitudeChangeFt := aircraft.VerticalRate * (deltaT / 60.0)
	newAltitudeFt := aircraft.Altitude + altitudeChangeFt

	// Ensure altitude doesn't go below ground (0 feet MSL minimum)
	if newAltitudeFt < 0 {
		newAltitudeFt = 0
		confidence *= 0.5 // Reduce confidence if we hit ground
	}

	return PredictedPosition{
		Position: coordinates.Geographic{
			Latitude:  newLat,
			Longitude: newLon,
			Altitude:  newAltitudeFt * coordinates.FeetToMeters,
		},
		PredictionTime:   predictionTime,
		Confidence:       confidence,
		OriginalPosition: aircraft,
	}
}

// PredictPositionWithLatency predicts position accounting for typical system latency.
// This is a convenience function that adds an estimated latency to the current time.
//
// Typical latencies:
// - Online ADS-B services: 2-3 seconds
// - Local SDR receivers: 0.5-1 second
//
// Parameters:
//   - aircraft: Current aircraft state
//   - estimatedLatencySeconds: Expected system latency (recommend 2.5 for online, 0.75 for local)
func PredictPositionWithLatency(aircraft adsb.Aircraft, estimatedLatencySeconds float64) PredictedPosition {
	predictionTime := time.Now().UTC().Add(time.Duration(estimatedLatencySeconds * float64(time.Second)))
	return PredictPosition(aircraft, predictionTime)
}

// predictHorizontalPosition calculates new lat/lon after moving along a great circle path.
// This uses the forward azimuth formula from spherical trigonometry.
//
// Parameters:
//   - lat: Starting latitude in decimal degrees
//   - lon: Starting longitude in decimal degrees
//   - speedKnots: Ground speed in knots
//   - trackDeg: Track (heading) in degrees (0-360, 0=North)
//   - deltaT: Time delta in seconds
//
// Returns: New latitude and longitude in decimal degrees
func predictHorizontalPosition(lat, lon, speedKnots, trackDeg, deltaT float64) (float64, float64) {
	// Convert to radians
	latRad := lat * coordinates.DegreesToRadians
	lonRad := lon * coordinates.DegreesToRadians
	trackRad := trackDeg * coordinates.DegreesToRadians

	// Calculate distance traveled
	// 1 knot = 1 nautical mile per hour
	// 1 nautical mile = 1852 meters
	distanceMeters := speedKnots * 1852.0 * (deltaT / 3600.0)

	// Calculate angular distance (distance / Earth radius)
	// Using mean Earth radius
	angularDistance := distanceMeters / (coordinates.EarthRadiusKm * 1000.0)

	// Calculate new latitude using great circle formulas
	// lat2 = asin(sin(lat1)*cos(d) + cos(lat1)*sin(d)*cos(track))
	newLatRad := math.Asin(
		math.Sin(latRad)*math.Cos(angularDistance) +
			math.Cos(latRad)*math.Sin(angularDistance)*math.Cos(trackRad),
	)

	// Calculate new longitude
	// lon2 = lon1 + atan2(sin(track)*sin(d)*cos(lat1), cos(d)-sin(lat1)*sin(lat2))
	newLonRad := lonRad + math.Atan2(
		math.Sin(trackRad)*math.Sin(angularDistance)*math.Cos(latRad),
		math.Cos(angularDistance)-math.Sin(latRad)*math.Sin(newLatRad),
	)

	// Convert back to degrees
	newLat := newLatRad * coordinates.RadiansToDegrees
	newLon := newLonRad * coordinates.RadiansToDegrees

	// Normalize longitude to [-180, 180]
	if newLon > 180.0 {
		newLon -= 360.0
	} else if newLon < -180.0 {
		newLon += 360.0
	}

	return newLat, newLon
}

// CalculateLeadTime calculates how far ahead to predict based on telescope slew speed.
// This accounts for the time it takes the telescope to physically move.
//
// Parameters:
//   - currentAlt, currentAz: Telescope's current position in degrees
//   - targetAlt, targetAz: Desired target position in degrees
//   - slewRateDegPerSec: Telescope slew rate in degrees per second
//
// Returns: Estimated time in seconds for telescope to reach target
func CalculateLeadTime(currentAlt, currentAz, targetAlt, targetAz, slewRateDegPerSec float64) float64 {
	// Calculate angular separation
	deltaAlt := math.Abs(targetAlt - currentAlt)
	deltaAz := math.Abs(targetAz - currentAz)

	// Handle azimuth wrap-around (359° to 1° is 2°, not 358°)
	if deltaAz > 180.0 {
		deltaAz = 360.0 - deltaAz
	}

	// Use the larger of the two deltas (telescope moves both axes simultaneously)
	maxDelta := math.Max(deltaAlt, deltaAz)

	// Calculate time to slew
	if slewRateDegPerSec <= 0 {
		return 0
	}

	return maxDelta / slewRateDegPerSec
}

// PredictTrackingPosition provides a complete tracking prediction that accounts for:
// 1. Data latency (time between aircraft position and now)
// 2. System processing time (time to calculate and send commands)
// 3. Telescope slew time (time for telescope to move)
//
// This is the recommended function for continuous tracking applications.
//
// Parameters:
//   - aircraft: Current aircraft state
//   - currentAlt, currentAz: Telescope's current position
//   - slewRateDegPerSec: Telescope slew rate
//   - systemLatencySeconds: Estimated latency (recommend 2.5s for online, 0.75s for local)
//
// Returns: Predicted position at the time telescope will actually be pointing
func PredictTrackingPosition(
	aircraft adsb.Aircraft,
	currentAlt, currentAz float64,
	slewRateDegPerSec float64,
	systemLatencySeconds float64,
) PredictedPosition {
	// Step 1: Predict position at current time + system latency
	now := time.Now().UTC()
	immediateTarget := PredictPosition(aircraft, now.Add(time.Duration(systemLatencySeconds*float64(time.Second))))

	// Step 2: Calculate telescope slew time to that position
	// For this we need to convert to alt/az - simplified estimate using direct angles
	// (In real system, would use actual coordinate transform)
	// For now, assume we're given alt/az or accept some approximation

	// Step 3: Predict further ahead by the slew time
	// This is iterative - as we predict further, slew time changes
	// For simplicity, use one iteration
	slewTime := CalculateLeadTime(
		currentAlt, currentAz,
		immediateTarget.Position.Latitude,  // Using lat as proxy for alt
		immediateTarget.Position.Longitude, // Using lon as proxy for az
		slewRateDegPerSec,
	)

	// Final prediction time = now + system latency + slew time
	finalPredictionTime := now.Add(time.Duration((systemLatencySeconds + slewTime) * float64(time.Second)))

	return PredictPosition(aircraft, finalPredictionTime)
}

// PredictPositionWithWaypoints predicts position using flight plan waypoints.
// This provides more accurate predictions than dead reckoning when the aircraft
// is following a filed route.
//
// The algorithm:
// 1. Determines which waypoint the aircraft is heading towards
// 2. Calculates bearing to next waypoint vs aircraft's current track
// 3. If aligned (within 45°), predicts along great circle to waypoint
// 4. Otherwise falls back to dead reckoning
//
// Parameters:
//   - aircraft: Current aircraft state
//   - waypoints: Flight plan waypoints in sequence order
//   - predictionTime: When to predict the position
//
// Returns: Predicted position with confidence adjusted for route adherence
func PredictPositionWithWaypoints(
	aircraft adsb.Aircraft,
	waypoints []Waypoint,
	predictionTime time.Time,
) PredictedPosition {
	if len(waypoints) == 0 {
		// No waypoints, use standard prediction
		return PredictPosition(aircraft, predictionTime)
	}

	// Find next waypoint (first one not marked as passed)
	nextWaypoint := findNextWaypoint(waypoints)
	if nextWaypoint == nil {
		// All waypoints passed, use standard prediction
		return PredictPosition(aircraft, predictionTime)
	}

	// Calculate bearing to next waypoint
	currentPos := coordinates.Geographic{
		Latitude:  aircraft.Latitude,
		Longitude: aircraft.Longitude,
	}
	waypointPos := coordinates.Geographic{
		Latitude:  nextWaypoint.Latitude,
		Longitude: nextWaypoint.Longitude,
	}

	bearingToWaypoint := coordinates.Bearing(currentPos, waypointPos)
	distanceToWaypoint := coordinates.DistanceNauticalMiles(currentPos, waypointPos)

	// Calculate track error (how far off course)
	trackError := math.Abs(normalizeAngle(aircraft.Track - bearingToWaypoint))

	// If aircraft is aligned with waypoint (within 45°), predict along great circle
	if trackError <= 45.0 && distanceToWaypoint > 0.1 {
		return predictAlongGreatCircle(
			aircraft,
			waypointPos,
			distanceToWaypoint,
			trackError,
			predictionTime,
		)
	}

	// Not aligned with waypoint, use standard dead reckoning
	// But reduce confidence since we're not following the flight plan
	pred := PredictPosition(aircraft, predictionTime)
	pred.Confidence *= 0.7 // Reduce confidence when off-route
	return pred
}

// findNextWaypoint returns the first waypoint that hasn't been passed.
func findNextWaypoint(waypoints []Waypoint) *Waypoint {
	for i := range waypoints {
		if !waypoints[i].Passed {
			return &waypoints[i]
		}
	}
	return nil
}

// normalizeAngle normalizes an angle to [-180, 180] range.
func normalizeAngle(angle float64) float64 {
	for angle > 180.0 {
		angle -= 360.0
	}
	for angle < -180.0 {
		angle += 360.0
	}
	return angle
}

// predictAlongGreatCircle predicts position along a great circle path to a waypoint.
// This is more accurate than dead reckoning when following a known route.
//
// The algorithm interpolates along the great circle between current position
// and the next waypoint based on aircraft speed and time delta.
func predictAlongGreatCircle(
	aircraft adsb.Aircraft,
	waypointPos coordinates.Geographic,
	distanceToWaypointNM float64,
	trackError float64,
	predictionTime time.Time,
) PredictedPosition {
	deltaT := predictionTime.Sub(aircraft.LastSeen).Seconds()

	// Calculate how far along the path the aircraft will be
	distanceTraveledNM := aircraft.GroundSpeed * (deltaT / 3600.0)

	// Calculate fraction of distance to waypoint
	fraction := distanceTraveledNM / distanceToWaypointNM

	// If we'll reach or pass the waypoint, predict to waypoint location
	if fraction >= 1.0 {
		// Calculate confidence - reduces as we extrapolate beyond waypoint
		confidence := math.Max(0.3, 1.0-(fraction-1.0)*0.5)

		// Reduce confidence based on track error
		confidence *= (1.0 - trackError/90.0)

		return PredictedPosition{
			Position: coordinates.Geographic{
				Latitude:  waypointPos.Latitude,
				Longitude: waypointPos.Longitude,
				Altitude:  aircraft.Altitude * coordinates.FeetToMeters,
			},
			PredictionTime:   predictionTime,
			Confidence:       confidence,
			OriginalPosition: aircraft,
		}
	}

	// Interpolate along great circle
	currentPos := coordinates.Geographic{
		Latitude:  aircraft.Latitude,
		Longitude: aircraft.Longitude,
	}

	newLat, newLon := interpolateGreatCircle(
		currentPos.Latitude, currentPos.Longitude,
		waypointPos.Latitude, waypointPos.Longitude,
		fraction,
	)

	// Predict altitude change
	altitudeChangeFt := aircraft.VerticalRate * (deltaT / 60.0)
	newAltitudeFt := math.Max(0, aircraft.Altitude+altitudeChangeFt)

	// Calculate confidence - higher for waypoint-based prediction
	// Starts at 0.95 (better than dead reckoning's 1.0 due to realism)
	// Decreases with time and track error
	confidence := 0.95 - (deltaT / 120.0) // 0.95 at 0s, 0.45 at 60s
	confidence *= (1.0 - trackError/90.0) // Reduce if not well aligned
	confidence = math.Max(0.3, math.Min(0.95, confidence))

	return PredictedPosition{
		Position: coordinates.Geographic{
			Latitude:  newLat,
			Longitude: newLon,
			Altitude:  newAltitudeFt * coordinates.FeetToMeters,
		},
		PredictionTime:   predictionTime,
		Confidence:       confidence,
		OriginalPosition: aircraft,
	}
}

// interpolateGreatCircle finds a point along a great circle path.
// fraction=0 returns start point, fraction=1 returns end point.
//
// Uses spherical linear interpolation (slerp) formula.
func interpolateGreatCircle(lat1, lon1, lat2, lon2, fraction float64) (float64, float64) {
	// Convert to radians
	lat1Rad := lat1 * coordinates.DegreesToRadians
	lon1Rad := lon1 * coordinates.DegreesToRadians
	lat2Rad := lat2 * coordinates.DegreesToRadians
	lon2Rad := lon2 * coordinates.DegreesToRadians

	// Calculate angular distance
	d := math.Acos(
		math.Sin(lat1Rad)*math.Sin(lat2Rad) +
			math.Cos(lat1Rad)*math.Cos(lat2Rad)*math.Cos(lon2Rad-lon1Rad),
	)

	// Handle case where points are very close
	if d < 1e-10 {
		return lat1, lon1
	}

	// Calculate interpolation coefficients
	a := math.Sin((1-fraction)*d) / math.Sin(d)
	b := math.Sin(fraction*d) / math.Sin(d)

	// Convert to Cartesian coordinates
	x := a*math.Cos(lat1Rad)*math.Cos(lon1Rad) + b*math.Cos(lat2Rad)*math.Cos(lon2Rad)
	y := a*math.Cos(lat1Rad)*math.Sin(lon1Rad) + b*math.Cos(lat2Rad)*math.Sin(lon2Rad)
	z := a*math.Sin(lat1Rad) + b*math.Sin(lat2Rad)

	// Convert back to geographic
	latRad := math.Atan2(z, math.Sqrt(x*x+y*y))
	lonRad := math.Atan2(y, x)

	return latRad * coordinates.RadiansToDegrees, lonRad * coordinates.RadiansToDegrees
}

// DeterminePassedWaypoints analyzes aircraft position and marks waypoints as passed.
// A waypoint is considered passed if:
// 1. Aircraft is within 2 NM of the waypoint, OR
// 2. Aircraft has moved past the waypoint (bearing changed by >90°)
//
// This should be called periodically to update waypoint progress.
func DeterminePassedWaypoints(aircraft adsb.Aircraft, waypoints []Waypoint) []Waypoint {
	currentPos := coordinates.Geographic{
		Latitude:  aircraft.Latitude,
		Longitude: aircraft.Longitude,
	}

	updated := make([]Waypoint, len(waypoints))
	copy(updated, waypoints)

	for i := range updated {
		if updated[i].Passed {
			continue // Already marked as passed
		}

		waypointPos := coordinates.Geographic{
			Latitude:  updated[i].Latitude,
			Longitude: updated[i].Longitude,
		}

		distance := coordinates.DistanceNauticalMiles(currentPos, waypointPos)

		// Mark as passed if within 2 NM
		if distance <= 2.0 {
			updated[i].Passed = true
			continue
		}

		// Check if we've passed it by comparing bearing to track
		// If the waypoint is behind us (bearing difference > 90°), mark as passed
		bearing := coordinates.Bearing(currentPos, waypointPos)
		bearingDiff := math.Abs(normalizeAngle(aircraft.Track - bearing))

		if bearingDiff > 90.0 {
			updated[i].Passed = true
		}
	}

	return updated
}

// AirwaySegment represents a segment of an airway for prediction.
type AirwaySegment struct {
	AirwayID    string
	AirwayType  string
	FromLat     float64
	FromLon     float64
	ToLat       float64
	ToLon       float64
	MinAltitude int
	MaxAltitude int
}

// MatchAirway finds the best matching airway for an aircraft.
// This is used when no flight plan is available.
//
// Matching criteria:
// 1. Aircraft track aligns with airway bearing (within 45°)
// 2. Aircraft altitude within airway altitude limits
// 3. Aircraft is close to airway centerline (<10 NM)
//
// Returns the best matching segment, or nil if no good match found.
func MatchAirway(aircraft adsb.Aircraft, airways []AirwaySegment) *AirwaySegment {
	if len(airways) == 0 {
		return nil
	}

	acPos := coordinates.Geographic{
		Latitude:  aircraft.Latitude,
		Longitude: aircraft.Longitude,
	}

	var bestMatch *AirwaySegment
	bestScore := 0.0

	for i := range airways {
		seg := &airways[i]

		// Check altitude limits
		if seg.MinAltitude > 0 && aircraft.Altitude < float64(seg.MinAltitude) {
			continue
		}
		if seg.MaxAltitude > 0 && aircraft.Altitude > float64(seg.MaxAltitude) {
			continue
		}

		fromPos := coordinates.Geographic{
			Latitude:  seg.FromLat,
			Longitude: seg.FromLon,
		}
		toPos := coordinates.Geographic{
			Latitude:  seg.ToLat,
			Longitude: seg.ToLon,
		}

		// Calculate airway bearing
		airwayBearing := coordinates.Bearing(fromPos, toPos)

		// Calculate track error (how aligned is aircraft with airway)
		trackError := math.Abs(normalizeAngle(aircraft.Track - airwayBearing))

		// Skip if not aligned (>45° off)
		if trackError > 45.0 {
			continue
		}

		// Calculate distance to airway centerline
		distToCenterline := distanceToLineSegment(acPos, fromPos, toPos)

		// Skip if too far from centerline (>10 NM)
		if distToCenterline > 10.0 {
			continue
		}

		// Calculate match score (higher is better)
		// Score based on: track alignment (0-1) and distance to centerline (0-1)
		alignmentScore := 1.0 - (trackError / 45.0)
		distanceScore := 1.0 - (distToCenterline / 10.0)
		score := (alignmentScore * 0.7) + (distanceScore * 0.3)

		if score > bestScore {
			bestScore = score
			bestMatch = seg
		}
	}

	// Only return match if score is good enough (>0.6)
	if bestScore > 0.6 {
		return bestMatch
	}

	return nil
}

// distanceToLineSegment calculates perpendicular distance from a point to a line segment.
// Returns distance in nautical miles.
func distanceToLineSegment(point, lineStart, lineEnd coordinates.Geographic) float64 {
	// Convert to radians
	pLat := point.Latitude * coordinates.DegreesToRadians
	pLon := point.Longitude * coordinates.DegreesToRadians
	sLat := lineStart.Latitude * coordinates.DegreesToRadians
	sLon := lineStart.Longitude * coordinates.DegreesToRadians
	eLat := lineEnd.Latitude * coordinates.DegreesToRadians
	eLon := lineEnd.Longitude * coordinates.DegreesToRadians

	// Calculate distance along track from start to point
	// Using cross-track distance formula
	d13 := math.Acos(
		math.Sin(sLat)*math.Sin(pLat) +
			math.Cos(sLat)*math.Cos(pLat)*math.Cos(pLon-sLon),
	)

	// Bearing from start to point
	bearing13 := math.Atan2(
		math.Sin(pLon-sLon)*math.Cos(pLat),
		math.Cos(sLat)*math.Sin(pLat)-math.Sin(sLat)*math.Cos(pLat)*math.Cos(pLon-sLon),
	)

	// Bearing from start to end
	bearing12 := math.Atan2(
		math.Sin(eLon-sLon)*math.Cos(eLat),
		math.Cos(sLat)*math.Sin(eLat)-math.Sin(sLat)*math.Cos(eLat)*math.Cos(eLon-sLon),
	)

	// Cross-track distance (perpendicular distance to line)
	dxt := math.Asin(math.Sin(d13) * math.Sin(bearing13-bearing12))

	// Convert to nautical miles
	return math.Abs(dxt) * coordinates.EarthRadiusKm / 1.852
}

// PredictPositionWithAirway predicts position along an airway.
// This is used when no flight plan is available but aircraft appears to be on an airway.
//
// The algorithm predicts along the airway centerline (great circle between waypoints)
// similar to waypoint-based prediction.
func PredictPositionWithAirway(
	aircraft adsb.Aircraft,
	airway AirwaySegment,
	predictionTime time.Time,
) PredictedPosition {
	// Create waypoint for the end of the airway segment
	nextWaypoint := coordinates.Geographic{
		Latitude:  airway.ToLat,
		Longitude: airway.ToLon,
	}

	currentPos := coordinates.Geographic{
		Latitude:  aircraft.Latitude,
		Longitude: aircraft.Longitude,
	}

	distanceToWaypoint := coordinates.DistanceNauticalMiles(currentPos, nextWaypoint)
	bearingToWaypoint := coordinates.Bearing(currentPos, nextWaypoint)
	trackError := math.Abs(normalizeAngle(aircraft.Track - bearingToWaypoint))

	// Use great circle prediction
	return predictAlongGreatCircle(
		aircraft,
		nextWaypoint,
		distanceToWaypoint,
		trackError,
		predictionTime,
	)
}

// FilterAirwaysByAltitude filters airways based on aircraft altitude.
// Victor airways: <18,000 ft MSL
// Jet routes: >=18,000 ft MSL
func FilterAirwaysByAltitude(airways []AirwaySegment, altitudeFt float64) []AirwaySegment {
	filtered := make([]AirwaySegment, 0)

	for _, airway := range airways {
		// Victor airways (V-prefix) - low altitude
		if len(airway.AirwayID) > 0 && airway.AirwayID[0] == 'V' {
			if altitudeFt < 18000 {
				filtered = append(filtered, airway)
			}
			continue
		}

		// Jet routes (J-prefix) - high altitude
		if len(airway.AirwayID) > 0 && airway.AirwayID[0] == 'J' {
			if altitudeFt >= 18000 {
				filtered = append(filtered, airway)
			}
			continue
		}

		// RNAV routes (Q/T prefix) - any altitude
		if len(airway.AirwayID) > 0 && (airway.AirwayID[0] == 'Q' || airway.AirwayID[0] == 'T') {
			filtered = append(filtered, airway)
			continue
		}

		// Other airways - check altitude limits if specified
		if airway.MinAltitude > 0 && altitudeFt < float64(airway.MinAltitude) {
			continue
		}
		if airway.MaxAltitude > 0 && altitudeFt > float64(airway.MaxAltitude) {
			continue
		}

		filtered = append(filtered, airway)
	}

	return filtered
}
