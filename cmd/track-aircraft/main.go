package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/unklstewy/ads-bscope/pkg/adsb"
	"github.com/unklstewy/ads-bscope/pkg/alpaca"
	"github.com/unklstewy/ads-bscope/pkg/config"
	"github.com/unklstewy/ads-bscope/pkg/coordinates"
	"github.com/unklstewy/ads-bscope/pkg/tracking"
)

// main implements a complete aircraft tracking demonstration.
// This shows the full integration of:
// - ADS-B data acquisition (airplanes.live)
// - Position prediction (compensating for latency)
// - Coordinate transformation (geographic → telescope)
// - Telescope control (ASCOM Alpaca)
// - Safety checks (meridian flip detection)
func main() {
	// Parse command line flags
	configPath := flag.String("config", "configs/config.json", "Path to configuration file")
	icao := flag.String("icao", "", "ICAO hex code of aircraft to track (e.g., a12345)")
	duration := flag.Int("duration", 60, "Tracking duration in seconds")
	dryRun := flag.Bool("dry-run", false, "Simulate tracking without moving telescope")
	radius := flag.Float64("radius", 100.0, "Search radius in nautical miles (default: 100)")
	random := flag.Bool("random", false, "Select a random aircraft from available targets")
	flag.Parse()

	log.Println("===========================================")
	log.Println("  ADS-B Aircraft Tracking - Live Demo")
	log.Println("===========================================")

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Configuration loaded from: %s", *configPath)
	log.Printf("Observer location: %.4f°N, %.4f°W, %.0fm MSL",
		cfg.Observer.Latitude, cfg.Observer.Longitude, cfg.Observer.Elevation)
	log.Printf("Telescope: %s (%s mount, %s imaging)", cfg.Telescope.Model, cfg.Telescope.MountType, cfg.Telescope.ImagingMode)

	// Get altitude limits based on telescope config
	minAlt, maxAlt := cfg.Telescope.GetAltitudeLimits()
	log.Printf("Tracking limits: %.0f° - %.0f° altitude", minAlt, maxAlt)

	// Create observer
	observer := coordinates.Observer{
		Location: coordinates.Geographic{
			Latitude:  cfg.Observer.Latitude,
			Longitude: cfg.Observer.Longitude,
			Altitude:  cfg.Observer.Elevation,
		},
		Timezone: cfg.Observer.TimeZone,
	}

	// Create ADS-B client
	if len(cfg.ADSB.Sources) == 0 {
		log.Fatal("Error: No ADS-B sources configured")
	}
	adsbClient := adsb.NewAirplanesLiveClient(cfg.ADSB.Sources[0].BaseURL)
	defer adsbClient.Close()

	// If no ICAO specified, fetch nearby aircraft and select one
	var targetICAO string
	if *icao == "" {
		log.Printf("No ICAO specified, searching for aircraft within %.0fnm...", *radius)
		
		aircraft, err := adsbClient.GetAircraft(
			cfg.Observer.Latitude,
			cfg.Observer.Longitude,
			*radius,
		)
		if err != nil {
			log.Fatalf("Failed to fetch nearby aircraft: %v", err)
		}

		if len(aircraft) == 0 {
			log.Fatalf("No aircraft found within %.0fnm", *radius)
		}

		log.Printf("Found %d aircraft within %.0fnm", len(aircraft), *radius)

		// Filter for trackable aircraft (within altitude limits)
		// Use prediction to match tracking loop behavior
		log.Println("\nFiltering for trackable aircraft...")
		trackable, filtered := filterTrackableAircraftWithReason(aircraft, observer, minAlt, maxAlt, time.Now().UTC(), 2.5)
		
		// Show why aircraft were filtered out
		if len(filtered) > 0 {
			log.Printf("Excluded %d aircraft:", len(filtered))
			for i, f := range filtered {
				if i >= 5 {
					log.Printf("  ... and %d more excluded", len(filtered)-5)
					break
				}
				log.Printf("  ✗ %s (%s) - %s", f.Aircraft.Callsign, f.Aircraft.ICAO, f.Reason)
			}
		}
		
		if len(trackable) == 0 {
			log.Println("\n❌ No trackable aircraft found.")
			log.Fatalf("Try increasing search radius (--radius) or adjusting telescope altitude limits")
		}

		log.Printf("\n✓ Found %d trackable aircraft (%.0f° - %.0f° altitude)", len(trackable), minAlt, maxAlt)

		// Select target
		var target *adsb.Aircraft
		if *random {
			// Pick random aircraft
			target = &trackable[rand.Intn(len(trackable))]
			acPos := coordinates.Geographic{Latitude: target.Latitude, Longitude: target.Longitude, Altitude: target.Altitude}
			horiz := coordinates.GeographicToHorizontal(acPos, observer, time.Now().UTC())
			log.Printf("\n🎯 Randomly selected: %s (%s)", target.Callsign, target.ICAO)
			log.Printf("   Position: Alt %.1f° Az %.1f° @ %.0fft MSL", horiz.Altitude, horiz.Azimuth, target.Altitude)
		} else {
			// Display options and pick first one
			log.Println("\nTrackable aircraft:")
			for i, ac := range trackable {
				if i >= 10 {
					log.Printf("  ... and %d more", len(trackable)-10)
					break
				}
				acPos := coordinates.Geographic{Latitude: ac.Latitude, Longitude: ac.Longitude, Altitude: ac.Altitude}
				horiz := coordinates.GeographicToHorizontal(acPos, observer, time.Now().UTC())
				log.Printf("  [%d] %s (%s) - Alt: %.1f° Az: %.1f° @ %.0fft",
					i+1, ac.Callsign, ac.ICAO, horiz.Altitude, horiz.Azimuth, ac.Altitude)
			}
			target = &trackable[0]
			log.Printf("\n🎯 Auto-selecting first aircraft: %s (%s)", target.Callsign, target.ICAO)
			log.Println("   (Use --random flag to select randomly, or --icao to specify)")
		}

		targetICAO = target.ICAO
	} else {
		targetICAO = *icao
	}

	// Create telescope client
	var telescopeClient *alpaca.Client
	if !*dryRun {
		telescopeClient = alpaca.NewClient(cfg.Telescope)
		log.Printf("Connecting to telescope at %s...", cfg.Telescope.BaseURL)
		
		if err := telescopeClient.Connect(); err != nil {
			log.Fatalf("Failed to connect to telescope: %v", err)
		}
		defer func() {
			log.Println("Disconnecting from telescope...")
			telescopeClient.Disconnect()
		}()
		
		log.Println("✓ Telescope connected")
	} else {
		log.Println("DRY RUN MODE: Telescope commands will be simulated")
	}

	// Tracking loop
	log.Println("===========================================")
	log.Printf("Tracking aircraft: %s", targetICAO)
	log.Printf("Duration: %d seconds", *duration)
	log.Println("Press Ctrl+C to stop")
	log.Println("===========================================")

	startTime := time.Now()
	// Get rate limit from ADS-B source configuration
	rateLimitDuration := time.Duration(cfg.ADSB.Sources[0].RateLimitSeconds * float64(time.Second))
	if rateLimitDuration == 0 {
		rateLimitDuration = time.Second // Default to 1 second if not configured
	}
	
	// Ensure update interval respects rate limit
	updateInterval := time.Duration(cfg.ADSB.UpdateIntervalSeconds) * time.Second
	if updateInterval < rateLimitDuration {
		updateInterval = rateLimitDuration
		log.Printf("Note: Update interval adjusted to %.1fs (API rate limit)", rateLimitDuration.Seconds())
	}
	ticker := time.NewTicker(updateInterval)
	defer ticker.Stop()

	trackingLimits := tracking.TrackingLimitsFromConfig(minAlt, maxAlt)
	lastPosition := coordinates.HorizontalCoordinates{}
	lastAPICall := time.Time{} // Track last API call time

	for {
		// Check if duration exceeded
		if time.Since(startTime).Seconds() > float64(*duration) {
			log.Println("\n===========================================")
			log.Println("Tracking duration completed")
			log.Println("===========================================")
			break
		}

		// Ensure minimum time between API calls per rate limit config
		if !lastAPICall.IsZero() {
			timeSinceLastCall := time.Since(lastAPICall)
			if timeSinceLastCall < rateLimitDuration {
				time.Sleep(rateLimitDuration - timeSinceLastCall)
			}
		}

		// Fetch aircraft data
		aircraft, err := adsbClient.GetAircraftByICAO(targetICAO)
		lastAPICall = time.Now()
		
		if err != nil {
			log.Printf("Warning: Failed to fetch aircraft data: %v", err)
			<-ticker.C
			continue
		}

		if aircraft == nil {
			log.Printf("Warning: Aircraft %s not found in ADS-B data", targetICAO)
			<-ticker.C
			continue
		}

		// Check for valid position
		if aircraft.Latitude == 0 && aircraft.Longitude == 0 {
			log.Printf("Warning: Aircraft has no position data")
			<-ticker.C
			continue
		}

		now := time.Now().UTC()
		
		// Predict position accounting for latency (2.5s for online sources)
		predicted := tracking.PredictPositionWithLatency(*aircraft, 2.5)

		// Convert to telescope coordinates
		horiz := coordinates.GeographicToHorizontal(predicted.Position, observer, now)

		// Check tracking limits and meridian events
		event, message := tracking.CheckMeridianEvent(
			lastPosition,
			horiz,
			observer,
			trackingLimits,
			cfg.Telescope.SupportsMeridianFlip,
		)

		// Calculate range and ETAs
		acPos := coordinates.Geographic{
			Latitude:  aircraft.Latitude,
			Longitude: aircraft.Longitude,
			Altitude:  aircraft.Altitude * coordinates.FeetToMeters,
		}
		currentRange := coordinates.DistanceNauticalMiles(observer.Location, acPos)
		closestRange, timeToClosest, approaching := coordinates.EstimateTimeToClosestApproach(
			observer.Location, acPos, aircraft.GroundSpeed, aircraft.Track,
		)
		etaTo5nm := coordinates.EstimateTimeToRange(
			observer.Location, acPos, aircraft.GroundSpeed, aircraft.Track, 5.0,
		)

		// Display status
		fmt.Printf("\n[%s] Target: %s (%s)\n",
			now.Format("15:04:05"), aircraft.Callsign, aircraft.ICAO)
		fmt.Printf("  Position: %.4f°N, %.4f°W, %.0f ft MSL\n",
			aircraft.Latitude, aircraft.Longitude, aircraft.Altitude)
		fmt.Printf("  Velocity: %.0f knots, track %.0f°, V/S %.0f fpm\n",
			aircraft.GroundSpeed, aircraft.Track, aircraft.VerticalRate)
		fmt.Printf("  Range: %.1f nm", currentRange)
		if approaching {
			fmt.Printf(" (approaching, closest: %.1f nm in %s)", closestRange, formatDuration(timeToClosest))
		} else {
			fmt.Printf(" (receding)")
		}
		fmt.Println()
		if etaTo5nm > 0 {
			fmt.Printf("  ETA to 5nm: %s\n", formatDuration(etaTo5nm))
		}
		if approaching && closestRange < 0.5 {
			fmt.Printf("  ETA to flyover: %s\n", formatDuration(timeToClosest))
		}
		fmt.Printf("  Predicted position: %.0fs ahead (confidence: %.0f%%)\n",
			predicted.PredictionTime.Sub(aircraft.LastSeen).Seconds(),
			predicted.Confidence*100)
		fmt.Printf("  Telescope coordinates:\n")
		fmt.Printf("    Altitude: %6.2f° (limits: %.0f° - %.0f°)\n", horiz.Altitude, minAlt, maxAlt)
		fmt.Printf("    Azimuth:  %6.2f°\n", horiz.Azimuth)

		// Check if target is trackable
		if tracking.ShouldAbortTracking(horiz, trackingLimits) {
			fmt.Printf("  Status: ⚠️  OUT OF RANGE - %s\n", message)
			lastPosition = horiz
			<-ticker.C
			continue
		}

		if event != tracking.NoMeridianEvent {
			fmt.Printf("  Status: ⚠️  %s - %s\n", eventName(event), message)
		} else {
			fmt.Printf("  Status: ✓ TRACKING\n")

			// Send telescope slew command
			if !*dryRun {
				var slewErr error
				if cfg.Telescope.MountType == "altaz" {
					slewErr = telescopeClient.SlewToAltAz(horiz.Altitude, horiz.Azimuth)
				} else {
					// Convert to equatorial for equatorial mounts
					eq := coordinates.HorizontalToEquatorial(horiz, observer, now)
					slewErr = telescopeClient.SlewToCoordinates(eq.RightAscension, eq.Declination)
				}

				if slewErr != nil {
					log.Printf("  Error: Failed to slew telescope: %v", slewErr)
				} else {
					fmt.Printf("  → Telescope slewed to target\n")
				}
			} else {
				fmt.Printf("  → [DRY RUN] Would slew to: Alt=%.2f°, Az=%.2f°\n",
					horiz.Altitude, horiz.Azimuth)
			}
		}

		lastPosition = horiz

		// Wait for next update
		<-ticker.C
	}

	// Final summary
	log.Println("\nTracking session complete!")
}

// eventName returns a human-readable name for a meridian event.
func eventName(event tracking.MeridianEvent) string {
	switch event {
	case tracking.NoMeridianEvent:
		return "OK"
	case tracking.MeridianFlipRequired:
		return "MERIDIAN FLIP"
	case tracking.ZenithCrossing:
		return "ZENITH CROSSING"
	case tracking.HorizonCrossing:
		return "HORIZON CROSSING"
	default:
		return "UNKNOWN"
	}
}

// formatDuration formats a duration in a human-readable way.
func formatDuration(d time.Duration) string {
	if d == 0 {
		return "now"
	}
	
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// FilteredAircraft represents an aircraft that was filtered out with a reason.
type FilteredAircraft struct {
	Aircraft adsb.Aircraft
	Reason   string
}

// filterTrackableAircraftWithReason returns aircraft that are within the telescope's altitude limits,
// along with a list of filtered aircraft and reasons why they were excluded.
// Only returns airborne aircraft (altitude > 0) within visible altitude range.
// Uses position prediction to match the tracking loop's behavior.
func filterTrackableAircraftWithReason(
	aircraft []adsb.Aircraft,
	observer coordinates.Observer,
	minAlt, maxAlt float64,
	now time.Time,
	predictionLatency float64,
) ([]adsb.Aircraft, []FilteredAircraft) {
	var trackable []adsb.Aircraft
	var filtered []FilteredAircraft
	
	for _, ac := range aircraft {
		// Skip aircraft without valid position
		if ac.Latitude == 0 && ac.Longitude == 0 {
			filtered = append(filtered, FilteredAircraft{
				Aircraft: ac,
				Reason:   "No position data",
			})
			continue
		}
		
		// Skip aircraft on ground (altitude = 0 or negative)
		if ac.Altitude <= 0 {
			filtered = append(filtered, FilteredAircraft{
				Aircraft: ac,
				Reason:   "On ground",
			})
			continue
		}
		
		// Predict position to match tracking loop behavior
		predicted := tracking.PredictPositionWithLatency(ac, predictionLatency)
		
		// Convert predicted position to horizontal coordinates
		horiz := coordinates.GeographicToHorizontal(predicted.Position, observer, now)
		
		// Check if within altitude limits (visible range)
		if horiz.Altitude < minAlt {
			filtered = append(filtered, FilteredAircraft{
				Aircraft: ac,
				Reason:   fmt.Sprintf("Too low: %.1f° < %.0f°", horiz.Altitude, minAlt),
			})
			continue
		}
		if horiz.Altitude > maxAlt {
			filtered = append(filtered, FilteredAircraft{
				Aircraft: ac,
				Reason:   fmt.Sprintf("Too high: %.1f° > %.0f°", horiz.Altitude, maxAlt),
			})
			continue
		}
		
		// Aircraft is trackable
		trackable = append(trackable, ac)
	}
	
	return trackable, filtered
}
