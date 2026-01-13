package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/unklstewy/ads-bscope/internal/db"
	"github.com/unklstewy/ads-bscope/pkg/config"
	"github.com/unklstewy/ads-bscope/pkg/flightaware"
)

func main() {
	cfg, err := config.Load("configs/config.json")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	database, err := db.Connect(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()

	ctx := context.Background()

	fmt.Println("===========================================")
	fmt.Println("  Flight Plan Verification")
	fmt.Println("===========================================")

	// If a callsign argument is provided, test API fetch
	if len(os.Args) > 1 {
		callsign := os.Args[1]
		fmt.Printf("Testing FlightAware API for callsign: %s\n\n", callsign)

		if !cfg.FlightAware.Enabled || cfg.FlightAware.APIKey == "" {
			fmt.Println("✗ FlightAware not configured")
			fmt.Println("  Set 'flightaware.enabled' and 'flightaware.api_key' in config")
			return
		}

		faClient := flightaware.NewClient(flightaware.Config{
			APIKey:          cfg.FlightAware.APIKey,
			RequestsPerHour: cfg.FlightAware.RequestsPerHour,
		})

		fp, err := faClient.GetFlightPlanByCallsign(ctx, callsign)
		if err != nil {
			fmt.Printf("✗ API Error: %v\n", err)
			return
		}

		if fp == nil {
			fmt.Printf("- No flight plan found for %s\n", callsign)
			return
		}

		fmt.Printf("✓ Flight Plan Retrieved:\n")
		fmt.Printf("  Callsign: %s\n", fp.ICAO)
		fmt.Printf("  Route: %s → %s\n", fp.Departure.Code, fp.Arrival.Code)
		fmt.Printf("  Aircraft: %s\n", fp.AircraftType)
		fmt.Printf("  Altitude: %d ft\n", fp.FiledAltitude)
		fmt.Printf("  Route String: %s\n", fp.RouteString)
		fmt.Printf("  Status: %s\n", fp.Status)
		fmt.Println()
		return
	}

	// Otherwise, show database statistics
	var totalPlans int
	err = database.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM flight_plans").Scan(&totalPlans)
	if err != nil {
		log.Fatalf("Failed to query plans: %v", err)
	}

	fmt.Printf("Total Flight Plans: %d\n\n", totalPlans)

	if totalPlans == 0 {
		fmt.Println("No flight plans in database yet.")
		fmt.Println("Run 'go run cmd/fetch-flightplans/main.go' to fetch plans.")
		return
	}

	// Show recent flight plans
	fmt.Println("Recent Flight Plans:")
	rows, err := database.QueryContext(ctx,
		`SELECT fp.callsign, fp.departure_icao, fp.arrival_icao, 
		        fp.aircraft_type, fp.filed_altitude, fp.last_updated,
		        COUNT(fpr.id) as waypoint_count
		 FROM flight_plans fp
		 LEFT JOIN flight_plan_routes fpr ON fpr.flight_plan_id = fp.id
		 GROUP BY fp.id, fp.callsign, fp.departure_icao, fp.arrival_icao,
		          fp.aircraft_type, fp.filed_altitude, fp.last_updated
		 ORDER BY fp.last_updated DESC
		 LIMIT 10`)
	if err != nil {
		log.Fatalf("Failed to query plans: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var callsign, dep, arr, acType string
		var altitude, waypointCount int
		var lastUpdated string

		rows.Scan(&callsign, &dep, &arr, &acType, &altitude, &lastUpdated, &waypointCount)
		fmt.Printf("  %s: %s → %s (%s, %d ft, %d waypoints)\n",
			callsign, dep, arr, acType, altitude, waypointCount)
	}

	// Show waypoint resolution stats
	fmt.Println("\nWaypoint Resolution:")
	var totalRoutes, routesWithWaypoints int
	database.QueryRowContext(ctx,
		`SELECT 
			COUNT(*) as total,
			COUNT(CASE WHEN EXISTS(
				SELECT 1 FROM flight_plan_routes fpr 
				WHERE fpr.flight_plan_id = fp.id
			) THEN 1 END) as with_waypoints
		 FROM flight_plans fp`).Scan(&totalRoutes, &routesWithWaypoints)

	fmt.Printf("  Plans with resolved waypoints: %d/%d (%.1f%%)\n",
		routesWithWaypoints, totalRoutes,
		float64(routesWithWaypoints)/float64(totalRoutes)*100)

	fmt.Println("\n===========================================")
	fmt.Println("✓ Verification Complete")
	fmt.Println("===========================================")
	fmt.Println("\nUsage:")
	fmt.Println("  go run cmd/verify-flightplans/main.go          - Show database stats")
	fmt.Println("  go run cmd/verify-flightplans/main.go UAL123   - Test API fetch")
}
