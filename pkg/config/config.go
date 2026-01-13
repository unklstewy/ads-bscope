package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config represents the complete application configuration.
// Configuration can be loaded from a file or database.
type Config struct {
	Server       ServerConfig       `json:"server"`
	Database     DatabaseConfig     `json:"database"`
	Telescope    TelescopeConfig    `json:"telescope"`
	ADSB         ADSBConfig         `json:"adsb"`
	Observer     ObserverConfig     `json:"observer"`
	FlightAware  FlightAwareConfig  `json:"flightaware"`
}

// ServerConfig contains HTTP server configuration.
type ServerConfig struct {
	// Port is the HTTP server port (default: 8080)
	Port string `json:"port"`

	// Host is the server bind address (default: "0.0.0.0")
	Host string `json:"host"`

	// TLSEnabled determines if HTTPS should be used
	TLSEnabled bool `json:"tls_enabled"`

	// TLSCertFile is the path to the TLS certificate
	TLSCertFile string `json:"tls_cert_file"`

	// TLSKeyFile is the path to the TLS private key
	TLSKeyFile string `json:"tls_key_file"`
}

// DatabaseConfig contains database connection settings.
type DatabaseConfig struct {
	// Driver is the database driver (postgres, mysql, sqlite)
	Driver string `json:"driver"`

	// Host is the database server hostname
	Host string `json:"host"`

	// Port is the database server port
	Port int `json:"port"`

	// Database is the database name
	Database string `json:"database"`

	// Username for database authentication
	Username string `json:"username"`

	// Password for database authentication (should be loaded from environment)
	Password string `json:"password"`

	// SSLMode for PostgreSQL connections (disable, require, verify-ca, verify-full)
	SSLMode string `json:"ssl_mode"`

	// MaxOpenConns is the maximum number of open connections
	MaxOpenConns int `json:"max_open_conns"`

	// MaxIdleConns is the maximum number of idle connections
	MaxIdleConns int `json:"max_idle_conns"`
}

// TelescopeConfig contains ASCOM Alpaca telescope settings.
type TelescopeConfig struct {
	// BaseURL is the Alpaca server address (e.g., "http://192.168.1.100:11111")
	BaseURL string `json:"base_url"`

	// DeviceNumber is the Alpaca device number (typically 0)
	DeviceNumber int `json:"device_number"`

	// MountType is either "altaz" or "equatorial"
	MountType string `json:"mount_type"`

	// SlewRate is the slew speed in degrees per second
	SlewRate float64 `json:"slew_rate"`

	// TrackingEnabled determines if telescope tracking should be enabled
	TrackingEnabled bool `json:"tracking_enabled"`

	// Model is the telescope model (e.g., "seestar-s30", "seestar-s50", "generic")
	// Used to determine telescope-specific capabilities
	Model string `json:"model"`

	// ImagingMode determines the operational mode: "astronomical" or "terrestrial"
	// astronomical: Traditional sky viewing with atmospheric refraction limits (15-20° minimum)
	// terrestrial: Earth-based targets (aircraft, birds, landscapes) - can point near/below horizon (0° minimum)
	ImagingMode string `json:"imaging_mode"`

	// SupportsMeridianFlip indicates if the telescope requires meridian flips
	// Seestar fork mounts: false (360° rotation, no flip needed)
	// German Equatorial Mounts: true (flip required to avoid pier collision)
	SupportsMeridianFlip bool `json:"supports_meridian_flip"`

	// MaxAltitude is the maximum safe tracking altitude in degrees
	// Alt-Az mode (Seestar): 80° (field rotation limit)
	// Equatorial mode (Seestar with wedge): 85° (physical/stability limit)
	// Generic telescopes: 85-88°
	MaxAltitude float64 `json:"max_altitude"`

	// MinAltitude is the minimum tracking altitude in degrees
	// Astronomical mode: 15-20° (atmospheric refraction)
	// Terrestrial mode: 0° or negative for below-horizon targets
	// Set to 0 for auto-detection based on imaging_mode
	MinAltitude float64 `json:"min_altitude"`
}

// CollectionRegion represents a geographic region for aircraft data collection.
// The collector will fetch aircraft data from all enabled regions.
type CollectionRegion struct {
	// Name is a friendly identifier for this region
	Name string `json:"name"`

	// Latitude in decimal degrees (-90 to +90)
	Latitude float64 `json:"latitude"`

	// Longitude in decimal degrees (-180 to +180)
	Longitude float64 `json:"longitude"`

	// RadiusNM is the collection radius in nautical miles
	RadiusNM float64 `json:"radius_nm"`

	// Enabled determines if this region should be actively collected
	Enabled bool `json:"enabled"`
}

// ADSBConfig contains ADS-B data source configuration.
type ADSBConfig struct {
	// Sources is a list of configured ADS-B data sources
	// Multiple sources can be configured for redundancy
	Sources []ADSBSource `json:"sources"`

	// SearchRadiusNM is the default search radius in nautical miles
	// Used for default TUI zoom and backward compatibility
	SearchRadiusNM float64 `json:"search_radius_nm"`

	// MaxCollectionRadiusNM is the maximum radius for collecting aircraft data
	// DEPRECATED: Use CollectionRegions instead for multi-region support
	// If CollectionRegions is empty, this creates a default region at observer location
	// If 0 or not specified, defaults to SearchRadiusNM for backward compatibility
	MaxCollectionRadiusNM float64 `json:"max_collection_radius_nm"`

	// CollectionRegions defines multiple geographic regions to collect aircraft from
	// The collector will fetch data from all enabled regions
	// If empty, creates a default region using observer location + MaxCollectionRadiusNM
	CollectionRegions []CollectionRegion `json:"collection_regions"`

	// UpdateIntervalSeconds is how often to refresh aircraft data
	UpdateIntervalSeconds int `json:"update_interval_seconds"`
}

// ADSBSource represents a single ADS-B data source configuration.
type ADSBSource struct {
	// Name is a friendly name for this source
	Name string `json:"name"`

	// Type is the source type: "airplanes.live", "adsbexchange", "local", etc.
	Type string `json:"type"`

	// Enabled determines if this source should be used
	Enabled bool `json:"enabled"`

	// BaseURL is the API base URL for online sources
	BaseURL string `json:"base_url"`

	// APIKey is the API key for services that require authentication
	APIKey string `json:"api_key,omitempty"`

	// LocalHost is the hostname for local SDR receivers
	LocalHost string `json:"local_host,omitempty"`

	// LocalPort is the port for local SDR receivers
	LocalPort int `json:"local_port,omitempty"`

	// RateLimitSeconds is the minimum time between API calls in seconds
	// 0 = no rate limit, >0 = enforce minimum delay between calls
	// airplanes.live: recommend 3 seconds to avoid 429 errors
	RateLimitSeconds float64 `json:"rate_limit_seconds"`
}

// ObserverConfig contains the observer's geographic location.
// This is critical for accurate coordinate transformations and telescope control.
// The observer location is separate from collection regions - it represents
// the physical location of the telescope/viewing equipment.
type ObserverConfig struct {
	// Name is a friendly identifier for this observer location
	Name string `json:"name"`

	// Latitude in decimal degrees (-90 to +90)
	Latitude float64 `json:"latitude"`

	// Longitude in decimal degrees (-180 to +180)
	Longitude float64 `json:"longitude"`

	// Elevation in meters above sea level
	Elevation float64 `json:"elevation"`

	// TimeZone is the IANA timezone name (e.g., "America/New_York")
	TimeZone string `json:"timezone"`
}

// FlightAwareConfig contains FlightAware AeroAPI settings.
type FlightAwareConfig struct {
	// APIKey is the FlightAware API key for AeroAPI v4
	// Sign up at: https://www.flightaware.com/aeroapi/
	APIKey string `json:"api_key"`

	// Enabled determines if FlightAware integration should be used
	Enabled bool `json:"enabled"`

	// RequestsPerHour limits the API call rate
	// Free tier: ~0.7 requests/hour (500/month)
	// Basic tier: ~340 requests/hour (250,000/month)
	RequestsPerHour int `json:"requests_per_hour"`

	// AutoFetchEnabled determines if flight plans should be automatically
	// fetched for tracked aircraft
	AutoFetchEnabled bool `json:"auto_fetch_enabled"`

	// FetchIntervalMinutes is how often to refresh flight plans for active aircraft
	FetchIntervalMinutes int `json:"fetch_interval_minutes"`
}

// Load reads configuration from a JSON file.
// If the file doesn't exist, returns a default configuration.
func Load(path string) (*Config, error) {
	// Check if file exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return DefaultConfig(), nil
	}

	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse JSON
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Override with environment variables
	cfg.applyEnvironmentOverrides()

	return &cfg, nil
}

// Save writes the configuration to a JSON file.
func (c *Config) Save(path string) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Marshal to JSON with indentation
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Write to file
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:       "8080",
			Host:       "0.0.0.0",
			TLSEnabled: false,
		},
		Database: DatabaseConfig{
			Driver:       "postgres",
			Host:         "localhost",
			Port:         5432,
			Database:     "adsbscope",
			Username:     "adsbscope",
			SSLMode:      "disable",
			MaxOpenConns: 25,
			MaxIdleConns: 5,
		},
		Telescope: TelescopeConfig{
			BaseURL:              "http://localhost:11111",
			DeviceNumber:         0,
			MountType:            "altaz",       // "altaz" or "equatorial" (when using EQ wedge)
			SlewRate:             1.0,
			TrackingEnabled:      true,
			Model:                "seestar-s50",
			ImagingMode:          "terrestrial", // "astronomical" or "terrestrial"
			SupportsMeridianFlip: false,         // Seestar: false (360° rotation), GEM: true
			MaxAltitude:          0.0,           // 0 = auto-detect based on model+mount_type
			MinAltitude:          0.0,           // 0 = auto-detect based on imaging_mode
		},
		ADSB: ADSBConfig{
			Sources: []ADSBSource{
				{
					Name:             "airplanes.live",
					Type:             "airplanes.live",
					Enabled:          true,
					BaseURL:          "https://api.airplanes.live/v2",
					RateLimitSeconds: 3.0,
				},
			},
			SearchRadiusNM:        50.0,
			MaxCollectionRadiusNM: 200.0, // Deprecated: use CollectionRegions
			CollectionRegions: []CollectionRegion{
				// Example regions - customize based on your location
				// By default, no regions enabled - will use legacy MaxCollectionRadiusNM
			},
			UpdateIntervalSeconds: 2,
		},
		Observer: ObserverConfig{
			Name:      "Primary Observer",
			Latitude:  0.0,
			Longitude: 0.0,
			Elevation: 0.0,
			TimeZone:  "UTC",
		},
		FlightAware: FlightAwareConfig{
			Enabled:              false,
			RequestsPerHour:      1,  // Conservative default for free tier
			AutoFetchEnabled:     false,
			FetchIntervalMinutes: 60, // Refresh every hour
		},
	}
}

// GetAltitudeLimits returns the appropriate altitude limits based on telescope model, mount type, and imaging mode.
// This automatically adjusts limits for Seestar Alt-Az mode field rotation issues and terrestrial vs astronomical use.
func (cfg *TelescopeConfig) GetAltitudeLimits() (minAlt, maxAlt float64) {
	// If explicit limits are set in config, use those
	if cfg.MaxAltitude > 0 {
		maxAlt = cfg.MaxAltitude
	} else {
		// Auto-detect max altitude based on model and mount type
		if cfg.Model == "seestar-s30" || cfg.Model == "seestar-s50" {
			if cfg.MountType == "altaz" {
				// Alt-Az mode: field rotation limits apply
				maxAlt = 80.0
			} else {
				// Equatorial mode (with wedge): field rotation eliminated
				maxAlt = 85.0
			}
		} else {
			// Generic telescope
			maxAlt = 85.0
		}
	}

	// Determine minimum altitude based on imaging mode
	if cfg.MinAltitude != 0 {
		// Use explicit config value (can be negative for below-horizon)
		minAlt = cfg.MinAltitude
	} else {
		// Auto-detect based on imaging mode
		if cfg.ImagingMode == "terrestrial" {
			// Terrestrial mode: can point at or below horizon
			// Use 0° for at-horizon, or -5° to allow slight below-horizon for distant objects
			minAlt = 0.0
		} else {
			// Astronomical mode (default): atmospheric refraction and practical limits
			if cfg.Model == "seestar-s30" || cfg.Model == "seestar-s50" {
				if cfg.MountType == "altaz" {
					minAlt = 20.0 // Alt-Az: practical viewing range
				} else {
					minAlt = 15.0 // Equatorial: atmospheric limit
				}
			} else {
				minAlt = 15.0 // Generic telescope
			}
		}
	}

	return minAlt, maxAlt
}

// GetCollectionRegions returns the effective collection regions.
// Provides backward compatibility: if CollectionRegions is empty,
// creates a default region using observer location + MaxCollectionRadiusNM.
func (cfg *ADSBConfig) GetCollectionRegions(observer ObserverConfig) []CollectionRegion {
	if len(cfg.CollectionRegions) > 0 {
		return cfg.CollectionRegions
	}

	// Backward compatibility: create default region from legacy settings
	regionRadius := cfg.MaxCollectionRadiusNM
	if regionRadius == 0 {
		regionRadius = cfg.SearchRadiusNM
	}

	return []CollectionRegion{
		{
			Name:      observer.Name + " Region",
			Latitude:  observer.Latitude,
			Longitude: observer.Longitude,
			RadiusNM:  regionRadius,
			Enabled:   true,
		},
	}
}

// applyEnvironmentOverrides applies environment variable overrides to the config.
// This allows sensitive data like passwords to be kept out of config files.
func (c *Config) applyEnvironmentOverrides() {
	if port := os.Getenv("ADS_BSCOPE_PORT"); port != "" {
		c.Server.Port = port
	}
	if dbPassword := os.Getenv("ADS_BSCOPE_DB_PASSWORD"); dbPassword != "" {
		c.Database.Password = dbPassword
	}
	if telescopeURL := os.Getenv("ADS_BSCOPE_TELESCOPE_URL"); telescopeURL != "" {
		c.Telescope.BaseURL = telescopeURL
	}
	// Override ADS-B source API keys if provided
	if apiKey := os.Getenv("ADS_BSCOPE_ADSB_API_KEY"); apiKey != "" {
		for i := range c.ADSB.Sources {
			c.ADSB.Sources[i].APIKey = apiKey
		}
	}
	if faKey := os.Getenv("ADS_BSCOPE_FLIGHTAWARE_API_KEY"); faKey != "" {
		c.FlightAware.APIKey = faKey
	}
}
