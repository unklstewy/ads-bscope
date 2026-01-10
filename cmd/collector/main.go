package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/unklstewy/ads-bscope/internal/db"
	"github.com/unklstewy/ads-bscope/pkg/adsb"
	"github.com/unklstewy/ads-bscope/pkg/config"
	"github.com/unklstewy/ads-bscope/pkg/coordinates"
)

// Collector continuously fetches aircraft data and stores it in the database.
// This runs as a background service, allowing multiple tracking clients to
// share the same data without hitting the API rate limits.
func main() {
	configPath := flag.String("config", "configs/config.json", "Path to configuration file")
	flag.Parse()

	log.Println("===========================================")
	log.Println("  ADS-B Aircraft Collector Service")
	log.Println("===========================================")

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Configuration loaded from: %s", *configPath)
	log.Printf("Observer location: %.4f°N, %.4f°W, %.0fm MSL",
		cfg.Observer.Latitude, cfg.Observer.Longitude, cfg.Observer.Elevation)
	log.Printf("Search radius: %.0f nm", cfg.ADSB.SearchRadiusNM)
	log.Printf("Update interval: %d seconds", cfg.ADSB.UpdateIntervalSeconds)

	// Get telescope limits
	minAlt, maxAlt := cfg.Telescope.GetAltitudeLimits()
	log.Printf("Telescope limits: %.0f° - %.0f° (%s mode)",
		minAlt, maxAlt, cfg.Telescope.ImagingMode)

	// Connect to database
	log.Println("\nConnecting to database...")
	database, err := db.Connect(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()
	log.Println("✓ Database connected")

	// Initialize schema
	ctx := context.Background()
	if err := database.InitSchema(ctx); err != nil {
		log.Fatalf("Failed to initialize schema: %v", err)
	}
	log.Println("✓ Database schema initialized")

	// Create observer
	observer := coordinates.Observer{
		Location: coordinates.Geographic{
			Latitude:  cfg.Observer.Latitude,
			Longitude: cfg.Observer.Longitude,
			Altitude:  cfg.Observer.Elevation,
		},
		Timezone: cfg.Observer.TimeZone,
	}

	// Create repository
	repo := db.NewAircraftRepository(database, observer)

	// Create ADS-B client
	if len(cfg.ADSB.Sources) == 0 {
		log.Fatal("Error: No ADS-B sources configured")
	}
	source := cfg.ADSB.Sources[0]
	adsbClient := adsb.NewAirplanesLiveClient(source.BaseURL)
	defer adsbClient.Close()

	log.Printf("\n✓ Using ADS-B source: %s", source.Name)
	log.Printf("  Rate limit: %.1f seconds between calls", source.RateLimitSeconds)

	// Start collector
	collector := &Collector{
		repo:         repo,
		db:           database,
		adsbClient:   adsbClient,
		observer:     observer,
		searchRadius: cfg.ADSB.SearchRadiusNM,
		minAlt:       minAlt,
		maxAlt:       maxAlt,
		updateInterval: time.Duration(cfg.ADSB.UpdateIntervalSeconds) * time.Second,
		rateLimit:      time.Duration(source.RateLimitSeconds * float64(time.Second)),
	}

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start collection loop in goroutine
	doneChan := make(chan struct{})
	go func() {
		collector.Run(ctx)
		close(doneChan)
	}()

	log.Println("\n===========================================")
	log.Println("  Collector service started")
	log.Println("  Initializing dataset...")
	log.Println("  Press Ctrl+C to stop")
	log.Println("===========================================\n")

	// Wait for shutdown signal
	select {
	case sig := <-sigChan:
		log.Printf("\nReceived signal: %v", sig)
	case <-doneChan:
		log.Println("\nCollector stopped")
	}

	log.Println("Shutting down gracefully...")
	log.Println("✓ Collector service stopped")
}

// Collector manages the aircraft data collection process.
type Collector struct {
	repo           *db.AircraftRepository
	db             *db.DB
	adsbClient     *adsb.AirplanesLiveClient
	observer       coordinates.Observer
	searchRadius   float64
	minAlt         float64
	maxAlt         float64
	updateInterval time.Duration
	rateLimit      time.Duration
	
	// Statistics
	totalUpdates    int
	totalAircraft   int
	lastUpdateTime  time.Time
	lastAircraftCount int
}

// Run starts the collection loop.
func (c *Collector) Run(ctx context.Context) {
	ticker := time.NewTicker(c.updateInterval)
	defer ticker.Stop()

	// Do first update immediately
	log.Println("Performing initial data fetch...")
	c.update(ctx)
	log.Println("✓ Initial dataset populated")

	// Periodic cleanup (every 5 minutes)
	cleanupTicker := time.NewTicker(5 * time.Minute)
	defer cleanupTicker.Stop()

	// Stats ticker (every 30 seconds)
	statsTicker := time.NewTicker(30 * time.Second)
	defer statsTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.update(ctx)
		case <-cleanupTicker.C:
			c.cleanup(ctx)
		case <-statsTicker.C:
			c.printStats(ctx)
		}
	}
}

// update fetches aircraft data and stores it in the database.
func (c *Collector) update(ctx context.Context) {
	now := time.Now().UTC()
	
	// Fetch aircraft from API
	aircraft, err := c.adsbClient.GetAircraft(
		c.observer.Location.Latitude,
		c.observer.Location.Longitude,
		c.searchRadius,
	)
	if err != nil {
		log.Printf("Error fetching aircraft: %v", err)
		return
	}

	c.lastUpdateTime = now
	c.lastAircraftCount = len(aircraft)
	c.totalUpdates++

	// Store each aircraft in database
	stored := 0
	for _, ac := range aircraft {
		// Skip aircraft without valid position
		if ac.Latitude == 0 && ac.Longitude == 0 {
			continue
		}

		if err := c.repo.UpsertAircraft(ctx, ac, now); err != nil {
			log.Printf("Error storing aircraft %s: %v", ac.ICAO, err)
			continue
		}
		stored++
	}

	// Update trackable status for all aircraft
	if err := c.repo.UpdateTrackableStatus(ctx, c.minAlt, c.maxAlt); err != nil {
		log.Printf("Error updating trackable status: %v", err)
	}

	log.Printf("[%s] Update #%d: Fetched %d aircraft, stored %d",
		now.Format("15:04:05"), c.totalUpdates, len(aircraft), stored)
}

// cleanup removes stale aircraft and old position history.
func (c *Collector) cleanup(ctx context.Context) {
	// Mark aircraft not seen in 2 minutes as not visible
	if err := c.db.CleanupOldData(ctx, 2*time.Minute); err != nil {
		log.Printf("Error during cleanup: %v", err)
		return
	}

	log.Println("✓ Cleanup completed")
}

// printStats displays current statistics.
func (c *Collector) printStats(ctx context.Context) {
	stats, err := c.db.GetStats(ctx)
	if err != nil {
		log.Printf("Error getting stats: %v", err)
		return
	}

	log.Printf("📊 Stats: %d visible, %d trackable, %d approaching | %d positions stored | %d total updates",
		stats["visible_aircraft"],
		stats["trackable_aircraft"],
		stats["approaching_aircraft"],
		stats["position_records"],
		c.totalUpdates,
	)
}
