package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/unklstewy/ads-bscope/internal/db"
	"github.com/unklstewy/ads-bscope/pkg/config"
	"github.com/unklstewy/ads-bscope/pkg/flightaware"
)

// FlightPlanFetcher periodically fetches flight plans for tracked aircraft.
//
// The fetcher queries the database for active aircraft (those that are visible
// and have been seen recently), then retrieves their flight plans from FlightAware.
// Flight plans are stored in the database for use by the prediction algorithm.
//
// Rate limiting is handled by the FlightAware client to avoid exceeding API quotas.
func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// Load configuration
	cfg, err := config.Load("configs/config.json")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Check if FlightAware is enabled
	if !cfg.FlightAware.Enabled {
		log.Println("FlightAware integration is disabled in config")
		log.Println("Set 'flightaware.enabled' to true or provide API key via ADS_BSCOPE_FLIGHTAWARE_API_KEY")
		return
	}

	if cfg.FlightAware.APIKey == "" {
		log.Fatal("FlightAware API key not configured. Set 'flightaware.api_key' or ADS_BSCOPE_FLIGHTAWARE_API_KEY")
	}

	// Connect to database
	database, err := db.Connect(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()

	// Create FlightAware client
	faClient := flightaware.NewClient(flightaware.Config{
		APIKey:          cfg.FlightAware.APIKey,
		RequestsPerHour: cfg.FlightAware.RequestsPerHour,
		Timeout:         10 * time.Second,
	})

	// Create repositories
	fpRepo := db.NewFlightPlanRepository(database)

	log.Println("===========================================")
	log.Println("  FlightAware Flight Plan Fetcher")
	log.Println("===========================================")
	log.Printf("API Rate Limit: %d requests/hour\n", cfg.FlightAware.RequestsPerHour)
	log.Printf("Fetch Interval: %d minutes\n", cfg.FlightAware.FetchIntervalMinutes)
	log.Println("===========================================\n")

	ctx := context.Background()
	ticker := time.NewTicker(time.Duration(cfg.FlightAware.FetchIntervalMinutes) * time.Minute)
	defer ticker.Stop()

	// Run immediately on startup
	if err := fetchFlightPlans(ctx, database, faClient, fpRepo); err != nil {
		log.Printf("Error fetching flight plans: %v", err)
	}

	// Then run periodically
	for range ticker.C {
		if err := fetchFlightPlans(ctx, database, faClient, fpRepo); err != nil {
			log.Printf("Error fetching flight plans: %v", err)
		}
	}
}

// fetchFlightPlans retrieves flight plans for all active aircraft.
func fetchFlightPlans(
	ctx context.Context,
	database *db.DB,
	faClient *flightaware.Client,
	fpRepo *db.FlightPlanRepository,
) error {
	// Query for active aircraft (seen in last 5 minutes, have callsign)
	rows, err := database.QueryContext(ctx,
		`SELECT DISTINCT icao, callsign, last_seen
		 FROM aircraft
		 WHERE is_visible = TRUE 
		   AND callsign IS NOT NULL 
		   AND callsign != ''
		   AND last_seen > NOW() - INTERVAL '5 minutes'
		 ORDER BY last_seen DESC`,
	)
	if err != nil {
		return fmt.Errorf("failed to query aircraft: %w", err)
	}
	defer rows.Close()

	var aircraft []struct {
		ICAO     string
		Callsign string
		LastSeen time.Time
	}

	for rows.Next() {
		var ac struct {
			ICAO     string
			Callsign string
			LastSeen time.Time
		}
		if err := rows.Scan(&ac.ICAO, &ac.Callsign, &ac.LastSeen); err != nil {
			return fmt.Errorf("failed to scan aircraft: %w", err)
		}
		aircraft = append(aircraft, ac)
	}

	if len(aircraft) == 0 {
		log.Println("No active aircraft found with callsigns")
		return nil
	}

	log.Printf("Found %d active aircraft with callsigns\n", len(aircraft))

	// Fetch flight plans for each aircraft
	successCount := 0
	notFoundCount := 0
	errorCount := 0

	for _, ac := range aircraft {
		// Check if we already have a recent flight plan (within last hour)
		existing, err := fpRepo.GetFlightPlanByICAO(ctx, ac.ICAO)
		if err != nil {
			log.Printf("Error checking existing plan for %s: %v", ac.Callsign, err)
		}
		
		if existing != nil && time.Since(existing.LastUpdated) < time.Hour {
			log.Printf("  ✓ %s (%s) - Using cached flight plan", ac.Callsign, ac.ICAO)
			continue
		}

		// Fetch from FlightAware
		log.Printf("  → Fetching flight plan for %s (%s)...", ac.Callsign, ac.ICAO)
		
		flightPlan, err := faClient.GetFlightPlanByCallsign(ctx, ac.Callsign)
		if err != nil {
			log.Printf("    ✗ Error: %v", err)
			errorCount++
			continue
		}

		if flightPlan == nil {
			log.Printf("    - No flight plan found")
			notFoundCount++
			continue
		}

		// Store in database
		fp := db.FlightPlan{
			ICAO:          ac.ICAO,
			Callsign:      flightPlan.ICAO,
			DepartureICAO: flightPlan.Departure.Code,
			ArrivalICAO:   flightPlan.Arrival.Code,
			Route:         flightPlan.RouteString,
			FiledAltitude: flightPlan.FiledAltitude,
			AircraftType:  flightPlan.AircraftType,
			FiledTime:     flightPlan.FiledTime,
			ETD:           flightPlan.ETD,
			ETA:           flightPlan.ETA,
			LastUpdated:   time.Now(),
		}

		fpID, err := fpRepo.UpsertFlightPlan(ctx, fp)
		if err != nil {
			log.Printf("    ✗ Failed to store: %v", err)
			errorCount++
			continue
		}

		// Parse and store route waypoints
		if flightPlan.RouteString != "" {
			waypointCount, err := fpRepo.ParseAndStoreRoute(ctx, fpID, flightPlan.RouteString)
			if err != nil {
				log.Printf("    ⚠ Route parsing error: %v", err)
			} else {
				log.Printf("    ✓ Stored: %s → %s (%d waypoints)",
					flightPlan.Departure.Code, flightPlan.Arrival.Code, waypointCount)
			}
		} else {
			log.Printf("    ✓ Stored: %s → %s (no route string)",
				flightPlan.Departure.Code, flightPlan.Arrival.Code)
		}

		successCount++
	}

	log.Println("\n===========================================")
	log.Printf("Fetch Summary:\n")
	log.Printf("  Success: %d\n", successCount)
	log.Printf("  Not Found: %d\n", notFoundCount)
	log.Printf("  Errors: %d\n", errorCount)
	log.Println("===========================================\n")

	return nil
}
