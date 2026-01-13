package main

import (
	"context"
	"fmt"
	"log"

	"github.com/unklstewy/ads-bscope/internal/db"
	"github.com/unklstewy/ads-bscope/pkg/config"
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
	fmt.Println("  NASR Data Verification")
	fmt.Println("===========================================")

	// Count waypoints by type
	rows, err := database.QueryContext(ctx,
		"SELECT type, COUNT(*) FROM waypoints GROUP BY type ORDER BY COUNT(*) DESC")
	if err != nil {
		log.Fatalf("Failed to query waypoints: %v", err)
	}
	defer rows.Close()

	fmt.Println("Waypoint Type | Count")
	fmt.Println("--------------|-------")
	totalWaypoints := 0
	for rows.Next() {
		var wType string
		var count int
		rows.Scan(&wType, &count)
		fmt.Printf("%-13s | %d\n", wType, count)
		totalWaypoints += count
	}
	fmt.Printf("%-13s | %d\n", "TOTAL", totalWaypoints)

	// Count airways
	var airwayCount int
	err = database.QueryRowContext(ctx,
		"SELECT COUNT(DISTINCT identifier) FROM airways").Scan(&airwayCount)
	if err != nil {
		log.Fatalf("Failed to query airways: %v", err)
	}
	fmt.Printf("\nUnique Airways: %d\n", airwayCount)

	// Sample some waypoints
	fmt.Println("\nSample Waypoints:")
	rows2, err := database.QueryContext(ctx,
		"SELECT identifier, type, latitude, longitude FROM waypoints LIMIT 5")
	if err != nil {
		log.Fatalf("Failed to query sample: %v", err)
	}
	defer rows2.Close()

	for rows2.Next() {
		var id, wType string
		var lat, lon float64
		rows2.Scan(&id, &wType, &lat, &lon)
		fmt.Printf("  %s (%s) - %.4f°, %.4f°\n", id, wType, lat, lon)
	}

	// Check for specific waypoints
	fmt.Println("\nChecking Known Waypoints:")
	checkWaypoint(database, ctx, "ATL")
	checkWaypoint(database, ctx, "CHSLY")
	checkWaypoint(database, ctx, "CLT")

	fmt.Println("\n===========================================")
	fmt.Println("✓ Verification Complete")
	fmt.Println("===========================================")
}

func checkWaypoint(database *db.DB, ctx context.Context, id string) {
	var name, wType string
	var lat, lon float64
	err := database.QueryRowContext(ctx,
		"SELECT COALESCE(name, ''), type, latitude, longitude FROM waypoints WHERE identifier = $1 LIMIT 1",
		id).Scan(&name, &wType, &lat, &lon)

	if err != nil {
		fmt.Printf("  ✗ %s not found\n", id)
	} else {
		fmt.Printf("  ✓ %s (%s) at %.4f°, %.4f°", id, wType, lat, lon)
		if name != "" {
			fmt.Printf(" - %s", name)
		}
		fmt.Println()
	}
}
