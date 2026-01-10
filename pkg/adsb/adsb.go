package adsb

import "time"

// Aircraft represents an aircraft tracked via ADS-B.
// All position data is in WGS84 coordinate system.
type Aircraft struct {
	// ICAO is the unique 24-bit ICAO aircraft address (e.g., "A12345")
	ICAO string

	// Callsign is the flight number or aircraft registration
	Callsign string

	// Latitude in decimal degrees (-90 to +90)
	Latitude float64

	// Longitude in decimal degrees (-180 to +180)
	Longitude float64

	// Altitude in feet above mean sea level (MSL)
	// Note: Some aircraft report geometric altitude, others barometric
	Altitude float64

	// GroundSpeed in knots
	GroundSpeed float64

	// Track is the ground track (heading) in degrees (0-359)
	// 0 = North, 90 = East, 180 = South, 270 = West
	Track float64

	// VerticalRate in feet per minute (positive = climbing, negative = descending)
	VerticalRate float64

	// LastSeen is the timestamp of the last position update
	LastSeen time.Time
}

// DataSource is the interface that all ADS-B data providers must implement.
// This abstraction allows switching between online services (ADS-B Exchange, etc.)
// and local SDR receivers (RTL-SDR, HackRF One, etc.).
type DataSource interface {
	// GetAircraft returns all currently tracked aircraft within a given radius.
	// centerLat/centerLon define the search center in decimal degrees.
	// radiusNM is the search radius in nautical miles.
	GetAircraft(centerLat, centerLon, radiusNM float64) ([]Aircraft, error)

	// GetAircraftByICAO returns a specific aircraft by its ICAO address.
	// Returns nil if the aircraft is not currently tracked.
	GetAircraftByICAO(icao string) (*Aircraft, error)

	// Close cleanly shuts down the data source connection.
	Close() error
}
