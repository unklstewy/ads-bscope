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

	// Get effective collection regions
	collectionRegions := cfg.ADSB.GetCollectionRegions(cfg.Observer)
	enabledRegions := 0
	for _, region := range collectionRegions {
		if region.Enabled {
			enabledRegions++
		}
	}

	log.Printf("Configuration loaded from: %s", *configPath)
	log.Printf("Observer: %s at %.4f°N, %.4f°W, %.0fm MSL",
		cfg.Observer.Name, cfg.Observer.Latitude, cfg.Observer.Longitude, cfg.Observer.Elevation)
	log.Printf("Collection regions: %d total, %d enabled", len(collectionRegions), enabledRegions)
	for _, region := range collectionRegions {
		if region.Enabled {
			log.Printf("  ✓ %s: %.4f°N, %.4f°W (%.0f nm)",
				region.Name, region.Latitude, region.Longitude, region.RadiusNM)
			if region.RadiusNM > 250 {
				log.Printf("    ⚠️  WARNING: Large radius (>250 nm) may cause API rate limit issues")
			}
		}
	}
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
		repo:             repo,
		db:               database,
		adsbClient:       adsbClient,
		observer:         observer,
		collectionRegions: collectionRegions,
		minAlt:           minAlt,
		maxAlt:           maxAlt,
		updateInterval:   time.Duration(cfg.ADSB.UpdateIntervalSeconds) * time.Second,
		rateLimit:        time.Duration(source.RateLimitSeconds * float64(time.Second)),
		regionStats:      make(map[string]*RegionStats),
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

// RegionStats tracks per-region collection statistics.
type RegionStats struct {
	Fetched      int
	Stored       int
	LastUpdate   time.Time
	TotalUpdates int
}

// Collector manages the aircraft data collection process.
type Collector struct {
	repo              *db.AircraftRepository
	db                *db.DB
	adsbClient        *adsb.AirplanesLiveClient
	observer          coordinates.Observer
	collectionRegions []config.CollectionRegion
	minAlt            float64
	maxAlt            float64
	updateInterval    time.Duration
	rateLimit         time.Duration
	
	// Statistics
	regionStats       map[string]*RegionStats
	totalUpdates      int
	totalAircraft     int
	lastUpdateTime    time.Time
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

// update fetches aircraft data from all enabled regions and stores in database.
func (c *Collector) update(ctx context.Context) {
	now := time.Now().UTC()
	c.totalUpdates++
	
	// Collect aircraft from all enabled regions
	type aircraftWithRegion struct {
		aircraft   adsb.Aircraft
		regionName string
	}
	allAircraft := make(map[string]aircraftWithRegion) // ICAO -> Aircraft+Region (deduplication)
	regionCount := 0
	
	for _, region := range c.collectionRegions {
		if !region.Enabled {
			continue
		}
		
		// Fetch aircraft for this region
		aircraft, err := c.fetchRegion(ctx, region)
		if err != nil {
			log.Printf("Error fetching region %s: %v", region.Name, err)
			continue
		}
		
		// Update region stats
		if c.regionStats[region.Name] == nil {
			c.regionStats[region.Name] = &RegionStats{}
		}
		stats := c.regionStats[region.Name]
		stats.Fetched = len(aircraft)
		stats.LastUpdate = now
		stats.TotalUpdates++
		
		// Merge into global collection (deduplicate by ICAO)
		// If aircraft seen in multiple regions, use first region for now
		// (could be enhanced to track multiple regions per aircraft)
		for _, ac := range aircraft {
			if ac.Latitude == 0 && ac.Longitude == 0 {
				continue // Skip invalid positions
			}
			// Only store if not already seen (first region wins for deduplication)
			if _, exists := allAircraft[ac.ICAO]; !exists {
				allAircraft[ac.ICAO] = aircraftWithRegion{
					aircraft:   ac,
					regionName: region.Name,
				}
			}
		}
		
		regionCount++
		
		// Rate limit between regions
		if regionCount < len(c.collectionRegions) {
			time.Sleep(c.rateLimit)
		}
	}
	
	// Store deduplicated aircraft with region tracking
	stored := 0
	for _, acWithRegion := range allAircraft {
		if err := c.repo.UpsertAircraft(ctx, acWithRegion.aircraft, now, acWithRegion.regionName); err != nil {
			log.Printf("Error storing aircraft %s: %v", acWithRegion.aircraft.ICAO, err)
			continue
		}
		stored++
	}
	
	// Update region stats with stored count
	for _, stats := range c.regionStats {
		stats.Stored = stored // Simplified: all regions contribute to total
	}
	
	// Update trackable status for all aircraft
	if err := c.repo.UpdateTrackableStatus(ctx, c.minAlt, c.maxAlt); err != nil {
		log.Printf("Error updating trackable status: %v", err)
	}
	
	c.lastUpdateTime = now
	c.totalAircraft = len(allAircraft)
	
	log.Printf("[%s] Update #%d: %d regions, %d unique aircraft, %d stored",
		now.Format("15:04:05"), c.totalUpdates, regionCount, len(allAircraft), stored)
}

// fetchRegion fetches aircraft from a single collection region.
func (c *Collector) fetchRegion(ctx context.Context, region config.CollectionRegion) ([]adsb.Aircraft, error) {
	aircraft, err := c.adsbClient.GetAircraft(
		region.Latitude,
		region.Longitude,
		region.RadiusNM,
	)
	if err != nil {
		return nil, err
	}
	
	return aircraft, nil
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
