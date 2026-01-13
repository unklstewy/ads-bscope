package db

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"time"

	"github.com/unklstewy/ads-bscope/pkg/adsb"
	"github.com/unklstewy/ads-bscope/pkg/coordinates"
)

// AircraftRepository handles database operations for aircraft tracking.
type AircraftRepository struct {
	db       *DB
	observer coordinates.Observer
}

// NewAircraftRepository creates a new aircraft repository.
func NewAircraftRepository(db *DB, observer coordinates.Observer) *AircraftRepository {
	return &AircraftRepository{
		db:       db,
		observer: observer,
	}
}

// UpsertAircraft inserts or updates an aircraft record.
// Calculates deltas, observer-relative measurements, and stores position history.
func (r *AircraftRepository) UpsertAircraft(ctx context.Context, aircraft adsb.Aircraft, now time.Time) error {
	// Get previous position if exists
	var prevPos aircraftPosition
	err := r.db.QueryRowContext(ctx,
		`SELECT latitude, longitude, altitude_ft, ground_speed_kts, track_deg, 
		        vertical_rate_fpm, last_seen
		 FROM aircraft 
		 WHERE icao = $1`,
		aircraft.ICAO,
	).Scan(&prevPos.Latitude, &prevPos.Longitude, &prevPos.AltitudeFt,
		&prevPos.GroundSpeedKts, &prevPos.TrackDeg, &prevPos.VerticalRateFpm,
		&prevPos.Timestamp)

	var prevPosPtr *aircraftPosition
	if err == nil {
		prevPosPtr = &prevPos
	} else if err != sql.ErrNoRows {
		return fmt.Errorf("failed to query previous position: %w", err)
	}

	// Calculate observer-relative measurements
	acPos := coordinates.Geographic{
		Latitude:  aircraft.Latitude,
		Longitude: aircraft.Longitude,
		Altitude:  aircraft.Altitude * coordinates.FeetToMeters,
	}

	rangeNM := coordinates.DistanceNauticalMiles(r.observer.Location, acPos)
	horiz := coordinates.GeographicToHorizontal(acPos, r.observer, now)
	
	// Calculate approach information
	closestRange, timeToClosest, approaching := coordinates.EstimateTimeToClosestApproach(
		r.observer.Location, acPos, aircraft.GroundSpeed, aircraft.Track,
	)

	etaSeconds := 0
	if approaching {
		etaSeconds = int(timeToClosest.Seconds())
	}

	// Upsert aircraft record
	_, err = r.db.ExecContext(ctx,
		`INSERT INTO aircraft (
			icao, callsign, latitude, longitude, altitude_ft,
			ground_speed_kts, track_deg, vertical_rate_fpm,
			first_seen, last_seen, last_updated, position_count,
			range_nm, bearing_deg, altitude_deg, azimuth_deg,
			is_approaching, closest_range_nm, eta_closest_seconds,
			is_visible
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 1,
			$12, $13, $14, $15, $16, $17, $18, TRUE
		)
		ON CONFLICT (icao) DO UPDATE SET
			callsign = EXCLUDED.callsign,
			latitude = EXCLUDED.latitude,
			longitude = EXCLUDED.longitude,
			altitude_ft = EXCLUDED.altitude_ft,
			ground_speed_kts = EXCLUDED.ground_speed_kts,
			track_deg = EXCLUDED.track_deg,
			vertical_rate_fpm = EXCLUDED.vertical_rate_fpm,
			last_seen = EXCLUDED.last_seen,
			last_updated = EXCLUDED.last_updated,
			position_count = aircraft.position_count + 1,
			range_nm = EXCLUDED.range_nm,
			bearing_deg = EXCLUDED.bearing_deg,
			altitude_deg = EXCLUDED.altitude_deg,
			azimuth_deg = EXCLUDED.azimuth_deg,
			is_approaching = EXCLUDED.is_approaching,
			closest_range_nm = EXCLUDED.closest_range_nm,
			eta_closest_seconds = EXCLUDED.eta_closest_seconds,
			is_visible = TRUE`,
		aircraft.ICAO, aircraft.Callsign,
		aircraft.Latitude, aircraft.Longitude, aircraft.Altitude,
		aircraft.GroundSpeed, aircraft.Track, aircraft.VerticalRate,
		now, now, now,
		rangeNM, 0.0, horiz.Altitude, horiz.Azimuth,
		approaching, closestRange, etaSeconds,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert aircraft: %w", err)
	}

	// Store position history with deltas
	if err := r.insertPositionHistory(ctx, aircraft, now, prevPosPtr, rangeNM, horiz); err != nil {
		return fmt.Errorf("failed to insert position history: %w", err)
	}

	return nil
}

// aircraftPosition represents a previous aircraft position for delta calculations.
type aircraftPosition struct {
	Latitude        float64
	Longitude       float64
	AltitudeFt      float64
	GroundSpeedKts  float64
	TrackDeg        float64
	VerticalRateFpm float64
	Timestamp       time.Time
}

// insertPositionHistory stores a position record with calculated deltas.
// Skips insertion if aircraft position hasn't changed (prevents redundant data).
func (r *AircraftRepository) insertPositionHistory(
	ctx context.Context,
	aircraft adsb.Aircraft,
	now time.Time,
	prevPos *aircraftPosition,
	rangeNM float64,
	horiz coordinates.HorizontalCoordinates,
) error {
	var (
		deltaTime         sql.NullFloat64
		deltaDistance     sql.NullFloat64
		deltaAltitude     sql.NullFloat64
		deltaTrack        sql.NullFloat64
		actualSpeed       sql.NullFloat64
		actualVerticalRate sql.NullFloat64
	)
	
	// Check if position has changed since last update
	if prevPos != nil {
		// Skip insertion if position is unchanged (common for grounded aircraft)
		// Consider position unchanged if:
		// - Lat/Lon unchanged (to 6 decimal places = ~0.1m precision)
		// - Altitude unchanged (to nearest foot)
		// - Ground speed near zero (<1 knot)
		if positionsEqual(aircraft, *prevPos) {
			return nil // Skip redundant position insert
		}
	}

	// Calculate deltas if we have a previous position
	if prevPos != nil {
		timeDelta := now.Sub(prevPos.Timestamp).Seconds()
		if timeDelta > 0 {
			deltaTime = sql.NullFloat64{Float64: timeDelta, Valid: true}

			// Distance delta
			prevGeo := coordinates.Geographic{
				Latitude:  prevPos.Latitude,
				Longitude: prevPos.Longitude,
			}
			currentGeo := coordinates.Geographic{
				Latitude:  aircraft.Latitude,
				Longitude: aircraft.Longitude,
			}
			distDelta := coordinates.DistanceNauticalMiles(prevGeo, currentGeo)
			deltaDistance = sql.NullFloat64{Float64: distDelta, Valid: true}

			// Altitude delta
			altDelta := aircraft.Altitude - prevPos.AltitudeFt
			deltaAltitude = sql.NullFloat64{Float64: altDelta, Valid: true}

			// Track delta (handle wrap-around)
			trackDelta := aircraft.Track - prevPos.TrackDeg
			if trackDelta > 180 {
				trackDelta -= 360
			} else if trackDelta < -180 {
				trackDelta += 360
			}
			deltaTrack = sql.NullFloat64{Float64: trackDelta, Valid: true}

			// Actual speed from position delta (more accurate than reported)
			timeHours := timeDelta / 3600.0
			if timeHours > 0 {
				actualSpeed = sql.NullFloat64{
					Float64: distDelta / timeHours,
					Valid:   true,
				}
			}

			// Actual vertical rate from altitude delta
			timeMinutes := timeDelta / 60.0
			if timeMinutes > 0 {
				actualVerticalRate = sql.NullFloat64{
					Float64: altDelta / timeMinutes,
					Valid:   true,
				}
			}
		}
	}

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO aircraft_positions (
			icao, timestamp, latitude, longitude, altitude_ft,
			ground_speed_kts, track_deg, vertical_rate_fpm,
			delta_time_seconds, delta_distance_nm, delta_altitude_ft, delta_track_deg,
			actual_speed_kts, actual_vertical_rate_fpm,
			range_nm, altitude_angle_deg, azimuth_deg
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17
		)`,
		aircraft.ICAO, now,
		aircraft.Latitude, aircraft.Longitude, aircraft.Altitude,
		aircraft.GroundSpeed, aircraft.Track, aircraft.VerticalRate,
		deltaTime, deltaDistance, deltaAltitude, deltaTrack,
		actualSpeed, actualVerticalRate,
		rangeNM, horiz.Altitude, horiz.Azimuth,
	)

	return err
}

// positionsEqual checks if two aircraft positions are effectively identical.
// This prevents storing redundant position history for stationary aircraft.
func positionsEqual(current adsb.Aircraft, prev aircraftPosition) bool {
	// Position tolerance: 0.000001 degrees ≈ 0.1 meters
	const positionTolerance = 0.000001
	// Altitude tolerance: 1 foot
	const altitudeTolerance = 1.0
	// Speed threshold: Consider stationary if <1 knot
	const speedThreshold = 1.0
	
	// Check if lat/lon unchanged
	latChanged := math.Abs(current.Latitude-prev.Latitude) > positionTolerance
	lonChanged := math.Abs(current.Longitude-prev.Longitude) > positionTolerance
	
	// Check if altitude changed
	altChanged := math.Abs(current.Altitude-prev.AltitudeFt) > altitudeTolerance
	
	// Check if aircraft is moving (either current or previous speed >1 knot)
	isMoving := current.GroundSpeed >= speedThreshold || prev.GroundSpeedKts >= speedThreshold
	
	// Position is considered equal if:
	// - Lat/lon unchanged AND altitude unchanged AND not moving
	// This allows position updates for:
	// - Any aircraft in motion
	// - Any change in position (even at low speed)
	// - Any change in altitude
	return !latChanged && !lonChanged && !altChanged && !isMoving
}

// UpdateTrackableStatus updates the is_trackable flag based on altitude limits.
func (r *AircraftRepository) UpdateTrackableStatus(
	ctx context.Context,
	minAlt, maxAlt float64,
) error {
	// Mark as trackable if within altitude limits and airborne
	_, err := r.db.ExecContext(ctx,
		`UPDATE aircraft 
		 SET is_trackable = (
			altitude_deg >= $1 AND 
			altitude_deg <= $2 AND 
			altitude_ft > 0 AND
			is_visible = TRUE
		 ),
		 last_trackable = CASE 
			WHEN altitude_deg >= $1 AND altitude_deg <= $2 AND altitude_ft > 0 
			THEN NOW() 
			ELSE last_trackable 
		 END
		 WHERE is_visible = TRUE`,
		minAlt, maxAlt,
	)

	return err
}

// GetTrackableAircraft returns all currently trackable aircraft.
// This uses the observer location configured in the repository.
func (r *AircraftRepository) GetTrackableAircraft(ctx context.Context) ([]adsb.Aircraft, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT icao, callsign, latitude, longitude, altitude_ft,
		        ground_speed_kts, track_deg, vertical_rate_fpm, last_seen
		 FROM aircraft
		 WHERE is_trackable = TRUE AND is_visible = TRUE
		 ORDER BY range_nm ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var aircraft []adsb.Aircraft
	for rows.Next() {
		var ac adsb.Aircraft
		err := rows.Scan(
			&ac.ICAO, &ac.Callsign,
			&ac.Latitude, &ac.Longitude, &ac.Altitude,
			&ac.GroundSpeed, &ac.Track, &ac.VerticalRate,
			&ac.LastSeen,
		)
		if err != nil {
			return nil, err
		}
		aircraft = append(aircraft, ac)
	}

	return aircraft, rows.Err()
}

// GetAircraftNear returns aircraft within a specified radius of an arbitrary center point.
// This enables radar mode centered on any airport or location, not just the observer.
// Only returns visible aircraft with valid positions.
func (r *AircraftRepository) GetAircraftNear(
	ctx context.Context,
	centerLat, centerLon, radiusNM, minAlt, maxAlt float64,
) ([]adsb.Aircraft, error) {
	// Fetch all visible aircraft
	rows, err := r.db.QueryContext(ctx,
		`SELECT icao, callsign, latitude, longitude, altitude_ft,
		        ground_speed_kts, track_deg, vertical_rate_fpm, last_seen
		 FROM aircraft
		 WHERE is_visible = TRUE AND altitude_ft > 0
		   AND latitude IS NOT NULL AND longitude IS NOT NULL`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Filter by distance from center point
	centerPos := coordinates.Geographic{
		Latitude:  centerLat,
		Longitude: centerLon,
		Altitude:  0,
	}

	var aircraft []adsb.Aircraft
	for rows.Next() {
		var ac adsb.Aircraft
		err := rows.Scan(
			&ac.ICAO, &ac.Callsign,
			&ac.Latitude, &ac.Longitude, &ac.Altitude,
			&ac.GroundSpeed, &ac.Track, &ac.VerticalRate,
			&ac.LastSeen,
		)
		if err != nil {
			return nil, err
		}

		// Calculate distance from center point
		acPos := coordinates.Geographic{
			Latitude:  ac.Latitude,
			Longitude: ac.Longitude,
			Altitude:  ac.Altitude * coordinates.FeetToMeters,
		}
		distanceNM := coordinates.DistanceNauticalMiles(centerPos, acPos)

		// Check if within radius
		if distanceNM > radiusNM {
			continue
		}

		// Calculate altitude angle from center (for altitude filtering)
		// Use current time for calculation
		horiz := coordinates.GeographicToHorizontal(acPos, coordinates.Observer{
			Location: centerPos,
		}, time.Now().UTC())

		// Check altitude limits
		if horiz.Altitude < minAlt || horiz.Altitude > maxAlt {
			continue
		}

		aircraft = append(aircraft, ac)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return aircraft, nil
}

// GetAircraftByICAO retrieves an aircraft by ICAO code.
func (r *AircraftRepository) GetAircraftByICAO(ctx context.Context, icao string) (*adsb.Aircraft, error) {
	var ac adsb.Aircraft
	err := r.db.QueryRowContext(ctx,
		`SELECT icao, callsign, latitude, longitude, altitude_ft,
		        ground_speed_kts, track_deg, vertical_rate_fpm, last_seen
		 FROM aircraft
		 WHERE icao = $1 AND is_visible = TRUE`,
		icao,
	).Scan(
		&ac.ICAO, &ac.Callsign,
		&ac.Latitude, &ac.Longitude, &ac.Altitude,
		&ac.GroundSpeed, &ac.Track, &ac.VerticalRate,
		&ac.LastSeen,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &ac, nil
}

// GetPositionHistory returns recent positions for an aircraft.
// Used to calculate accurate velocities and accelerations.
func (r *AircraftRepository) GetPositionHistory(
	ctx context.Context,
	icao string,
	since time.Time,
) ([]Position, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT timestamp, latitude, longitude, altitude_ft,
		        ground_speed_kts, track_deg, vertical_rate_fpm,
		        delta_time_seconds, delta_distance_nm, delta_altitude_ft,
		        actual_speed_kts, actual_vertical_rate_fpm,
		        range_nm, altitude_angle_deg, azimuth_deg
		 FROM aircraft_positions
		 WHERE icao = $1 AND timestamp >= $2
		 ORDER BY timestamp ASC`,
		icao, since,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var positions []Position
	for rows.Next() {
		var p Position
		var deltaTime, deltaDistance, deltaAltitude sql.NullFloat64
		var actualSpeed, actualVR sql.NullFloat64

		err := rows.Scan(
			&p.Timestamp, &p.Latitude, &p.Longitude, &p.AltitudeFt,
			&p.GroundSpeedKts, &p.TrackDeg, &p.VerticalRateFpm,
			&deltaTime, &deltaDistance, &deltaAltitude,
			&actualSpeed, &actualVR,
			&p.RangeNM, &p.AltitudeAngleDeg, &p.AzimuthDeg,
		)
		if err != nil {
			return nil, err
		}

		if deltaTime.Valid {
			p.DeltaTimeSeconds = deltaTime.Float64
		}
		if deltaDistance.Valid {
			p.DeltaDistanceNM = deltaDistance.Float64
		}
		if deltaAltitude.Valid {
			p.DeltaAltitudeFt = deltaAltitude.Float64
		}
		if actualSpeed.Valid {
			p.ActualSpeedKts = actualSpeed.Float64
		}
		if actualVR.Valid {
			p.ActualVerticalRateFpm = actualVR.Float64
		}

		positions = append(positions, p)
	}

	return positions, rows.Err()
}

// Position represents a historical aircraft position with deltas.
type Position struct {
	Timestamp            time.Time
	Latitude             float64
	Longitude            float64
	AltitudeFt           float64
	GroundSpeedKts       float64
	TrackDeg             float64
	VerticalRateFpm      float64
	DeltaTimeSeconds     float64
	DeltaDistanceNM      float64
	DeltaAltitudeFt      float64
	ActualSpeedKts       float64
	ActualVerticalRateFpm float64
	RangeNM              float64
	AltitudeAngleDeg     float64
	AzimuthDeg           float64
}

// CalculateAverageVelocity calculates average velocity from position history.
// More accurate than instantaneous reported values.
func CalculateAverageVelocity(positions []Position) (avgSpeed, avgVerticalRate float64) {
	if len(positions) < 2 {
		return 0, 0
	}

	var totalSpeed, totalVR float64
	count := 0

	for _, p := range positions {
		if p.ActualSpeedKts > 0 {
			totalSpeed += p.ActualSpeedKts
			count++
		}
		if !math.IsNaN(p.ActualVerticalRateFpm) {
			totalVR += p.ActualVerticalRateFpm
		}
	}

	if count > 0 {
		avgSpeed = totalSpeed / float64(count)
		avgVerticalRate = totalVR / float64(len(positions))
	}

	return avgSpeed, avgVerticalRate
}
