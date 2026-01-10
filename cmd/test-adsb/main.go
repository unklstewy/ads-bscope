package main

import (
	"log"
	"time"

	"github.com/unklstewy/ads-bscope/pkg/adsb"
	"github.com/unklstewy/ads-bscope/pkg/config"
	"github.com/unklstewy/ads-bscope/pkg/coordinates"
	"github.com/unklstewy/ads-bscope/pkg/tracking"
)

// main is a test program to verify airplanes.live integration.
// It fetches aircraft near Charlotte Douglas International Airport (CLT)
// and calculates their positions relative to an observer ~3nm SE of the airport.
func main() {
	log.Println("ADS-B Data Source Test - airplanes.live")
	log.Println("Testing near Charlotte Douglas International Airport (CLT)")
	log.Println("=====================================")

	// Observer position: ~3nm southeast of CLT
	// CLT coordinates: 35.2144° N, 80.9431° W
	// Observer: 35.1871° N, 80.9218° W
	observer := coordinates.Observer{
		Location: coordinates.Geographic{
			Latitude:  35.1871,
			Longitude: -80.9218,
			Altitude:  230.0, // meters MSL (~754 feet)
		},
		Timezone: "America/New_York",
	}

	log.Printf("Observer Location: %.4f°N, %.4f°W, %.0fm MSL\n",
		observer.Location.Latitude, observer.Location.Longitude, observer.Location.Altitude)

	// Create airplanes.live client
	cfg := config.DefaultConfig()
	client := adsb.NewAirplanesLiveClient(cfg.ADSB.Sources[0].BaseURL)
	defer client.Close()

	// Search for aircraft within 50nm
	searchRadius := 50.0
	log.Printf("Fetching aircraft within %.0f nm...\n", searchRadius)

	aircraft, err := client.GetAircraft(
		observer.Location.Latitude,
		observer.Location.Longitude,
		searchRadius,
	)
	if err != nil {
		log.Fatalf("Failed to fetch aircraft: %v", err)
	}

	log.Printf("Found %d aircraft\n", len(aircraft))
	log.Println("=====================================")

	// Display each aircraft with calculated horizontal coordinates
	now := time.Now().UTC()
	for i, ac := range aircraft {
		// Skip aircraft with missing position data
		if ac.Latitude == 0 && ac.Longitude == 0 {
			continue
		}

		// Convert aircraft position to horizontal coordinates
		targetPos := coordinates.Geographic{
			Latitude:  ac.Latitude,
			Longitude: ac.Longitude,
			Altitude:  ac.Altitude * 0.3048, // Convert feet to meters
		}

		horiz := coordinates.GeographicToHorizontal(targetPos, observer, now)

		// Also calculate equatorial coordinates for equatorial mounts
		eq := coordinates.HorizontalToEquatorial(horiz, observer, now)

		// Predict position with typical online ADS-B latency (2.5 seconds)
		predicted := tracking.PredictPositionWithLatency(ac, 2.5)
		predictedHoriz := coordinates.GeographicToHorizontal(predicted.Position, observer, now)

		// Print aircraft info
		log.Printf("\nAircraft #%d:", i+1)
		log.Printf("  ICAO:     %s", ac.ICAO)
		log.Printf("  Callsign: %s", ac.Callsign)
		log.Printf("  Position: %.4f°N, %.4f°W", ac.Latitude, ac.Longitude)
		log.Printf("  Altitude: %.0f ft MSL", ac.Altitude)
		log.Printf("  Speed:    %.0f knots", ac.GroundSpeed)
		log.Printf("  Track:    %.0f°", ac.Track)
		log.Printf("  V/S:      %.0f fpm", ac.VerticalRate)
		log.Printf("  Last Seen: %s (%.1fs ago)",
			ac.LastSeen.Format("15:04:05"),
			time.Since(ac.LastSeen).Seconds())
		log.Printf("  → Telescope Coordinates:")
		log.Printf("     [Alt/Az Mount]")
		log.Printf("       Altitude: %6.2f° (elevation above horizon)", horiz.Altitude)
		log.Printf("       Azimuth:  %6.2f° (bearing from north)", horiz.Azimuth)
		log.Printf("     [Equatorial Mount]")
		log.Printf("       RA:       %6.2fh (right ascension)", eq.RightAscension)
		log.Printf("       Dec:      %+6.2f° (declination)", eq.Declination)
		log.Printf("     [Predicted Position (2.5s ahead)]")
		log.Printf("       Altitude: %6.2f° (confidence: %.0f%%)", predictedHoriz.Altitude, predicted.Confidence*100)
		log.Printf("       Azimuth:  %6.2f°", predictedHoriz.Azimuth)

		// Determine cardinal direction
		log.Printf("     Direction: %s", azimuthToCardinal(horiz.Azimuth))

		// Check if visible (above horizon)
		if horiz.Altitude > 0 {
			log.Printf("     Status: VISIBLE")
		} else {
			log.Printf("     Status: Below horizon")
		}

		// Limit to first 10 aircraft
		if i >= 9 {
			log.Printf("\n... and %d more aircraft", len(aircraft)-10)
			break
		}
	}

	log.Println("\n=====================================")
	log.Println("Test complete!")
}

// azimuthToCardinal converts azimuth in degrees to cardinal direction.
func azimuthToCardinal(azimuth float64) string {
	directions := []string{"N", "NNE", "NE", "ENE", "E", "ESE", "SE", "SSE",
		"S", "SSW", "SW", "WSW", "W", "WNW", "NW", "NNW"}
	index := int((azimuth + 11.25) / 22.5)
	return directions[index%16]
}
