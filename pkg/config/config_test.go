package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestDefaultConfig verifies that DefaultConfig returns valid defaults.
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Server defaults
	if cfg.Server.Port != "8080" {
		t.Errorf("Expected default port 8080, got %s", cfg.Server.Port)
	}
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Expected default host 0.0.0.0, got %s", cfg.Server.Host)
	}
	if cfg.Server.TLSEnabled {
		t.Error("Expected TLS disabled by default")
	}

	// Database defaults
	if cfg.Database.Driver != "postgres" {
		t.Errorf("Expected postgres driver, got %s", cfg.Database.Driver)
	}
	if cfg.Database.Port != 5432 {
		t.Errorf("Expected default postgres port 5432, got %d", cfg.Database.Port)
	}
	if cfg.Database.MaxOpenConns != 25 {
		t.Errorf("Expected max open conns 25, got %d", cfg.Database.MaxOpenConns)
	}
	if cfg.Database.MaxIdleConns != 5 {
		t.Errorf("Expected max idle conns 5, got %d", cfg.Database.MaxIdleConns)
	}

	// Telescope defaults
	if cfg.Telescope.MountType != "altaz" {
		t.Errorf("Expected altaz mount type, got %s", cfg.Telescope.MountType)
	}
	if cfg.Telescope.Model != "seestar-s50" {
		t.Errorf("Expected seestar-s50 model, got %s", cfg.Telescope.Model)
	}
	if cfg.Telescope.ImagingMode != "terrestrial" {
		t.Errorf("Expected terrestrial imaging mode, got %s", cfg.Telescope.ImagingMode)
	}

	// ADSB defaults
	if len(cfg.ADSB.Sources) != 1 {
		t.Errorf("Expected 1 default ADS-B source, got %d", len(cfg.ADSB.Sources))
	}
	if cfg.ADSB.Sources[0].Name != "airplanes.live" {
		t.Errorf("Expected airplanes.live source, got %s", cfg.ADSB.Sources[0].Name)
	}
	if cfg.ADSB.UpdateIntervalSeconds != 2 {
		t.Errorf("Expected update interval 2s, got %d", cfg.ADSB.UpdateIntervalSeconds)
	}

	// Observer defaults
	if cfg.Observer.TimeZone != "UTC" {
		t.Errorf("Expected UTC timezone, got %s", cfg.Observer.TimeZone)
	}

	// FlightAware defaults
	if cfg.FlightAware.Enabled {
		t.Error("Expected FlightAware disabled by default")
	}
	if cfg.FlightAware.RequestsPerHour != 1 {
		t.Errorf("Expected 1 request/hour, got %d", cfg.FlightAware.RequestsPerHour)
	}
}

// TestLoadNonExistentFile tests that Load returns default config when file doesn't exist.
func TestLoadNonExistentFile(t *testing.T) {
	cfg, err := Load("/nonexistent/path/config.json")
	if err != nil {
		t.Fatalf("Expected no error for non-existent file, got: %v", err)
	}
	if cfg == nil {
		t.Fatal("Expected default config, got nil")
	}
	// Verify it's actually the default config
	if cfg.Server.Port != "8080" {
		t.Error("Did not get default config for non-existent file")
	}
}

// TestLoadValidConfig tests loading a valid configuration file.
func TestLoadValidConfig(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.json")

	testConfig := &Config{
		Server: ServerConfig{
			Port:       "9090",
			Host:       "127.0.0.1",
			TLSEnabled: true,
		},
		Database: DatabaseConfig{
			Driver:   "postgres",
			Host:     "db.example.com",
			Port:     5433,
			Database: "testdb",
			Username: "testuser",
		},
		Telescope: TelescopeConfig{
			BaseURL:   "http://telescope.local:11111",
			MountType: "equatorial",
			Model:     "seestar-s30",
		},
		ADSB: ADSBConfig{
			Sources: []ADSBSource{
				{
					Name:             "test-source",
					Type:             "airplanes.live",
					Enabled:          true,
					BaseURL:          "https://test.api",
					RateLimitSeconds: 5.0,
				},
			},
			SearchRadiusNM:        100.0,
			UpdateIntervalSeconds: 10,
		},
		Observer: ObserverConfig{
			Name:      "Test Observer",
			Latitude:  35.5,
			Longitude: -80.8,
			Elevation: 200,
			TimeZone:  "America/New_York",
		},
		FlightAware: FlightAwareConfig{
			Enabled:         true,
			APIKey:          "test-key",
			RequestsPerHour: 100,
		},
	}

	// Write config to file
	data, err := json.MarshalIndent(testConfig, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal test config: %v", err)
	}
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Load config
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify loaded values
	if cfg.Server.Port != "9090" {
		t.Errorf("Expected port 9090, got %s", cfg.Server.Port)
	}
	if cfg.Database.Host != "db.example.com" {
		t.Errorf("Expected db.example.com, got %s", cfg.Database.Host)
	}
	if cfg.Telescope.MountType != "equatorial" {
		t.Errorf("Expected equatorial mount, got %s", cfg.Telescope.MountType)
	}
	if cfg.Observer.Latitude != 35.5 {
		t.Errorf("Expected latitude 35.5, got %f", cfg.Observer.Latitude)
	}
}

// TestLoadInvalidJSON tests error handling for malformed JSON.
func TestLoadInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.json")

	// Write invalid JSON
	if err := os.WriteFile(configPath, []byte("{ invalid json }"), 0644); err != nil {
		t.Fatalf("Failed to write invalid config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
	if err != nil && !contains(err.Error(), "failed to parse") {
		t.Errorf("Expected parse error, got: %v", err)
	}
}

// TestSaveConfig tests saving configuration to file.
func TestSaveConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "saved-config.json")

	cfg := DefaultConfig()
	cfg.Server.Port = "9999"
	cfg.Observer.Name = "Test Save"

	// Save config
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatal("Config file was not created")
	}

	// Load it back and verify
	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load saved config: %v", err)
	}

	if loaded.Server.Port != "9999" {
		t.Errorf("Expected port 9999, got %s", loaded.Server.Port)
	}
	if loaded.Observer.Name != "Test Save" {
		t.Errorf("Expected observer name 'Test Save', got %s", loaded.Observer.Name)
	}
}

// TestSaveConfigCreatesDirectory tests that Save creates missing directories.
func TestSaveConfigCreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "nested", "dir", "config.json")

	cfg := DefaultConfig()
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Failed to save config with nested directory: %v", err)
	}

	// Verify directory was created
	dir := filepath.Dir(configPath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("Directory was not created")
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Config file was not created")
	}
}

// TestEnvironmentOverrides tests environment variable overrides.
func TestEnvironmentOverrides(t *testing.T) {
	// Set environment variables
	os.Setenv("ADS_BSCOPE_PORT", "7777")
	os.Setenv("ADS_BSCOPE_DB_HOST", "env-db-host")
	os.Setenv("ADS_BSCOPE_DB_PASSWORD", "env-password")
	os.Setenv("ADS_BSCOPE_TELESCOPE_URL", "http://env-telescope:11111")
	os.Setenv("ADS_BSCOPE_ADSB_API_KEY", "env-adsb-key")
	os.Setenv("ADS_BSCOPE_FLIGHTAWARE_API_KEY", "env-fa-key")
	defer func() {
		os.Unsetenv("ADS_BSCOPE_PORT")
		os.Unsetenv("ADS_BSCOPE_DB_HOST")
		os.Unsetenv("ADS_BSCOPE_DB_PASSWORD")
		os.Unsetenv("ADS_BSCOPE_TELESCOPE_URL")
		os.Unsetenv("ADS_BSCOPE_ADSB_API_KEY")
		os.Unsetenv("ADS_BSCOPE_FLIGHTAWARE_API_KEY")
	}()

	// Create config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	testCfg := DefaultConfig()
	testCfg.Server.Port = "8080"
	testCfg.Database.Host = "localhost"
	testCfg.Database.Password = "original-password"

	data, _ := json.Marshal(testCfg)
	os.WriteFile(configPath, data, 0644)

	// Load config (should apply env overrides)
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify overrides
	if cfg.Server.Port != "7777" {
		t.Errorf("Expected port 7777 from env, got %s", cfg.Server.Port)
	}
	if cfg.Database.Host != "env-db-host" {
		t.Errorf("Expected env-db-host from env, got %s", cfg.Database.Host)
	}
	if cfg.Database.Password != "env-password" {
		t.Errorf("Expected env-password from env, got %s", cfg.Database.Password)
	}
	if cfg.Telescope.BaseURL != "http://env-telescope:11111" {
		t.Errorf("Expected telescope URL from env, got %s", cfg.Telescope.BaseURL)
	}
	if len(cfg.ADSB.Sources) > 0 && cfg.ADSB.Sources[0].APIKey != "env-adsb-key" {
		t.Errorf("Expected ADSB API key from env, got %s", cfg.ADSB.Sources[0].APIKey)
	}
	if cfg.FlightAware.APIKey != "env-fa-key" {
		t.Errorf("Expected FlightAware API key from env, got %s", cfg.FlightAware.APIKey)
	}
}

// TestGetAltitudeLimits tests the GetAltitudeLimits method.
func TestGetAltitudeLimits(t *testing.T) {
	tests := []struct {
		name        string
		config      TelescopeConfig
		expectedMin float64
		expectedMax float64
	}{
		{
			name: "Seestar S50 Alt-Az Terrestrial",
			config: TelescopeConfig{
				Model:       "seestar-s50",
				MountType:   "altaz",
				ImagingMode: "terrestrial",
			},
			expectedMin: 0.0,
			expectedMax: 80.0,
		},
		{
			name: "Seestar S50 Alt-Az Astronomical",
			config: TelescopeConfig{
				Model:       "seestar-s50",
				MountType:   "altaz",
				ImagingMode: "astronomical",
			},
			expectedMin: 20.0,
			expectedMax: 80.0,
		},
		{
			name: "Seestar S50 Equatorial Terrestrial",
			config: TelescopeConfig{
				Model:       "seestar-s50",
				MountType:   "equatorial",
				ImagingMode: "terrestrial",
			},
			expectedMin: 0.0,
			expectedMax: 85.0,
		},
		{
			name: "Seestar S50 Equatorial Astronomical",
			config: TelescopeConfig{
				Model:       "seestar-s50",
				MountType:   "equatorial",
				ImagingMode: "astronomical",
			},
			expectedMin: 15.0,
			expectedMax: 85.0,
		},
		{
			name: "Seestar S30 Alt-Az",
			config: TelescopeConfig{
				Model:       "seestar-s30",
				MountType:   "altaz",
				ImagingMode: "terrestrial",
			},
			expectedMin: 0.0,
			expectedMax: 80.0,
		},
		{
			name: "Generic Telescope",
			config: TelescopeConfig{
				Model:       "generic",
				MountType:   "altaz",
				ImagingMode: "astronomical",
			},
			expectedMin: 15.0,
			expectedMax: 85.0,
		},
		{
			name: "Explicit Limits Override",
			config: TelescopeConfig{
				Model:       "seestar-s50",
				MountType:   "altaz",
				ImagingMode: "terrestrial",
				MinAltitude: 10.0,
				MaxAltitude: 70.0,
			},
			expectedMin: 10.0,
			expectedMax: 70.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			minAlt, maxAlt := tt.config.GetAltitudeLimits()
			if minAlt != tt.expectedMin {
				t.Errorf("Expected min altitude %f, got %f", tt.expectedMin, minAlt)
			}
			if maxAlt != tt.expectedMax {
				t.Errorf("Expected max altitude %f, got %f", tt.expectedMax, maxAlt)
			}
		})
	}
}

// TestGetCollectionRegions tests the GetCollectionRegions method.
func TestGetCollectionRegions(t *testing.T) {
	observer := ObserverConfig{
		Name:      "Test Observer",
		Latitude:  35.0,
		Longitude: -80.0,
	}

	t.Run("Returns configured regions", func(t *testing.T) {
		cfg := ADSBConfig{
			CollectionRegions: []CollectionRegion{
				{Name: "Region 1", Latitude: 35.5, Longitude: -80.5, RadiusNM: 100, Enabled: true},
				{Name: "Region 2", Latitude: 36.0, Longitude: -81.0, RadiusNM: 150, Enabled: false},
			},
			SearchRadiusNM:        50.0,
			MaxCollectionRadiusNM: 200.0,
		}

		regions := cfg.GetCollectionRegions(observer)
		if len(regions) != 2 {
			t.Errorf("Expected 2 regions, got %d", len(regions))
		}
		if regions[0].Name != "Region 1" {
			t.Errorf("Expected Region 1, got %s", regions[0].Name)
		}
	})

	t.Run("Creates default region when none configured", func(t *testing.T) {
		cfg := ADSBConfig{
			CollectionRegions:     []CollectionRegion{},
			SearchRadiusNM:        50.0,
			MaxCollectionRadiusNM: 200.0,
		}

		regions := cfg.GetCollectionRegions(observer)
		if len(regions) != 1 {
			t.Errorf("Expected 1 default region, got %d", len(regions))
		}
		if regions[0].Latitude != observer.Latitude {
			t.Errorf("Expected latitude %f, got %f", observer.Latitude, regions[0].Latitude)
		}
		if regions[0].RadiusNM != 200.0 {
			t.Errorf("Expected radius 200.0, got %f", regions[0].RadiusNM)
		}
		if !regions[0].Enabled {
			t.Error("Expected default region to be enabled")
		}
	})

	t.Run("Falls back to SearchRadiusNM when MaxCollectionRadiusNM is 0", func(t *testing.T) {
		cfg := ADSBConfig{
			CollectionRegions:     []CollectionRegion{},
			SearchRadiusNM:        75.0,
			MaxCollectionRadiusNM: 0,
		}

		regions := cfg.GetCollectionRegions(observer)
		if regions[0].RadiusNM != 75.0 {
			t.Errorf("Expected radius 75.0 from SearchRadiusNM, got %f", regions[0].RadiusNM)
		}
	})
}

// TestConfigRoundTrip tests saving and loading config preserves data.
func TestConfigRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "roundtrip.json")

	// Create a config with various values
	original := DefaultConfig()
	original.Server.Port = "3000"
	original.Server.TLSEnabled = true
	original.Observer.Latitude = 35.1234
	original.Observer.Longitude = -80.5678
	original.ADSB.CollectionRegions = []CollectionRegion{
		{Name: "Test Region", Latitude: 35.0, Longitude: -80.0, RadiusNM: 100, Enabled: true},
	}

	// Save
	if err := original.Save(configPath); err != nil {
		t.Fatalf("Failed to save: %v", err)
	}

	// Load
	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("Failed to load: %v", err)
	}

	// Compare
	if loaded.Server.Port != original.Server.Port {
		t.Error("Port not preserved in round trip")
	}
	if loaded.Server.TLSEnabled != original.Server.TLSEnabled {
		t.Error("TLS setting not preserved in round trip")
	}
	if loaded.Observer.Latitude != original.Observer.Latitude {
		t.Error("Latitude not preserved in round trip")
	}
	if len(loaded.ADSB.CollectionRegions) != len(original.ADSB.CollectionRegions) {
		t.Error("Collection regions not preserved in round trip")
	}
}

// contains is a helper function to check if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && hasSubstring(s, substr)))
}

func hasSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
