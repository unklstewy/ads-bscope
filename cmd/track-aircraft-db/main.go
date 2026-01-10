package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/unklstewy/ads-bscope/internal/db"
	"github.com/unklstewy/ads-bscope/pkg/alpaca"
	"github.com/unklstewy/ads-bscope/pkg/config"
	"github.com/unklstewy/ads-bscope/pkg/coordinates"
	"github.com/unklstewy/ads-bscope/pkg/tracking"
)

// main implements aircraft tracking using the database instead of direct API calls.
// This allows multiple trackers to share the same data without hitting API rate limits.
func main() {
	configPath := flag.String("config", "configs/config.json", "Path to configuration file")
	icao := flag.String("icao", "", "ICAO hex code of aircraft to track")
	duration := flag.Int("duration", 60, "Tracking duration in seconds")
	dryRun := flag.Bool("dry-run", false, "Simulate tracking without moving telescope")
	random := flag.Bool("random", false, "Select a random trackable aircraft")
	flag.Parse()

	log.Println("===========================================")
	log.Println("  ADS-B Aircraft Tracking (DB Mode)")
	log.Println("===========================================")

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Configuration loaded from: %s", *configPath)
	log.Printf("Observer location: %.4f°N, %.4f°W, %.0fm MSL",
		cfg.Observer.Latitude, cfg.Observer.Longitude, cfg.Observer.Elevation)
	log.Printf("Telescope: %s (%s mount, %s imaging)", 
		cfg.Telescope.Model, cfg.Telescope.MountType, cfg.Telescope.ImagingMode)

	// Get altitude limits
	minAlt, maxAlt := cfg.Telescope.GetAltitudeLimits()
	log.Printf("Tracking limits: %.0f° - %.0f° altitude", minAlt, maxAlt)

	// Connect to database
	log.Println("\nConnecting to database...")
	database, err := db.Connect(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()
	log.Println("✓ Database connected")

	// Create observer
	observer := coordinates.Observer{
		Location: coordinates.Geographic{
			Latitude:  cfg.Observer.Latitude,
			Longitude: cfg.Observer.Longitude,
			Altitude:  cfg.Observer.Elevation,
		},
		Timezone: cfg.Observer.TimeZone,
	}

	// Create repositories
	repo := db.NewAircraftRepository(database, observer)
	fpRepo := db.NewFlightPlanRepository(database)
	ctx := context.Background()

	// Select target aircraft
	var targetICAO string
	if *icao == "" {
		// Get trackable aircraft from database
		log.Println("\nQuerying trackable aircraft from database...")
		trackable, err := repo.GetTrackableAircraft(ctx)
		if err != nil {
			log.Fatalf("Failed to query trackable aircraft: %v", err)
		}

		if len(trackable) == 0 {
			log.Fatal("❌ No trackable aircraft in database. Is the collector running?")
		}

		log.Printf("✓ Found %d trackable aircraft", len(trackable))

		// Display options
		log.Println("\nTrackable aircraft:")
		for i, ac := range trackable {
			if i >= 10 {
				log.Printf("  ... and %d more", len(trackable)-10)
				break
			}
			log.Printf("  [%d] %s (%s) @ %.0fft", i+1, ac.Callsign, ac.ICAO, ac.Altitude)
		}

		if *random {
			// Pick random aircraft
			target := trackable[time.Now().UnixNano()%int64(len(trackable))]
			targetICAO = target.ICAO
			log.Printf("\n🎯 Randomly selected: %s (%s)", target.Callsign, target.ICAO)
		} else {
			// Use first aircraft
			targetICAO = trackable[0].ICAO
			log.Printf("\n🎯 Auto-selecting first aircraft: %s (%s)", 
				trackable[0].Callsign, trackable[0].ICAO)
			log.Println("   (Use --random flag to select randomly, or --icao to specify)")
		}
	} else {
		targetICAO = *icao
	}

	// Create telescope client if not dry run
	var telescopeClient *alpaca.Client
	if !*dryRun {
		telescopeClient = alpaca.NewClient(cfg.Telescope)
		log.Printf("\nConnecting to telescope at %s...", cfg.Telescope.BaseURL)
		
		if err := telescopeClient.Connect(); err != nil {
			log.Fatalf("Failed to connect to telescope: %v", err)
		}
		defer func() {
			log.Println("Disconnecting from telescope...")
			telescopeClient.Disconnect()
		}()
		
		log.Println("✓ Telescope connected")
	} else {
		log.Println("\nDRY RUN MODE: Telescope commands will be simulated")
	}

	// Tracking loop
	log.Println("\n===========================================")
	log.Printf("Tracking aircraft: %s", targetICAO)
	log.Printf("Duration: %d seconds", *duration)
	log.Println("Press Ctrl+C to stop")
	log.Println("===========================================")

	startTime := time.Now()
	updateInterval := 2 * time.Second // Query database every 2 seconds
	ticker := time.NewTicker(updateInterval)
	defer ticker.Stop()

	trackingLimits := tracking.TrackingLimitsFromConfig(minAlt, maxAlt)
	lastPosition := coordinates.HorizontalCoordinates{}

	for {
		// Check if duration exceeded
		if time.Since(startTime).Seconds() > float64(*duration) {
			log.Println("\n===========================================")
			log.Println("Tracking duration completed")
			log.Println("===========================================")
			break
		}

		// Query aircraft from database
		aircraft, err := repo.GetAircraftByICAO(ctx, targetICAO)
		if err != nil {
			log.Printf("Warning: Database query failed: %v", err)
			<-ticker.C
			continue
		}

		if aircraft == nil {
			log.Printf("Warning: Aircraft %s not in database", targetICAO)
			<-ticker.C
			continue
		}

		now := time.Now().UTC()
		dataAge := now.Sub(aircraft.LastSeen).Seconds()

		// Check for flight plan
		flightPlan, _ := fpRepo.GetFlightPlanByICAO(ctx, targetICAO)
		var waypointList []tracking.Waypoint
		if flightPlan != nil {
			// Get waypoints for flight plan
			routes, err := fpRepo.GetFlightPlanRoute(ctx, flightPlan.ID)
			if err == nil && len(routes) > 0 {
				// Convert to tracking.Waypoint format
				for _, r := range routes {
					waypointList = append(waypointList, tracking.Waypoint{
						Name:      r.WaypointName,
						Latitude:  r.Latitude,
						Longitude: r.Longitude,
						Sequence:  r.Sequence,
						Passed:    r.Passed,
					})
				}
				
				// Update passed waypoints based on current position
				waypointList = tracking.DeterminePassedWaypoints(*aircraft, waypointList)
			}
		}

		// Apply prediction if data is stale (>30 seconds old)
		var acPos coordinates.Geographic
		var predicted bool
		var confidence float64
		var predictionType string // "waypoint", "airway", or "deadreckoning"
		var matchedAirway string

		if dataAge > 30 {
			// Data is stale - use prediction
			predicted = true
			
			// Try waypoint-based prediction first (if flight plan available)
			if len(waypointList) > 0 {
				predictedPos := tracking.PredictPositionWithWaypoints(
					*aircraft,
					waypointList,
					time.Now().UTC().Add(time.Duration(dataAge*float64(time.Second))),
				)
				acPos = predictedPos.Position
				confidence = predictedPos.Confidence
				predictionType = "waypoint"
			} else {
				// No flight plan - try airway matching
				// Query nearby airways within 25 NM radius
				airwaySegs, err := fpRepo.FindNearbyAirways(
					ctx,
					aircraft.Latitude,
					aircraft.Longitude,
					25.0, // 25 NM radius
					int(aircraft.Altitude*0.9), // Min altitude (10% tolerance)
					int(aircraft.Altitude*1.1), // Max altitude (10% tolerance)
				)
				
				if err == nil && len(airwaySegs) > 0 {
					// Convert to tracking.AirwaySegment format
					trackingAirways := make([]tracking.AirwaySegment, len(airwaySegs))
					for i, seg := range airwaySegs {
						trackingAirways[i] = tracking.AirwaySegment{
							AirwayID:    seg.AirwayID,
							AirwayType:  seg.AirwayType,
							FromLat:     seg.FromWaypoint.Latitude,
							FromLon:     seg.FromWaypoint.Longitude,
							ToLat:       seg.ToWaypoint.Latitude,
							ToLon:       seg.ToWaypoint.Longitude,
							MinAltitude: seg.MinAltitude,
							MaxAltitude: seg.MaxAltitude,
						}
					}
					
					// Filter by altitude (Victor vs Jet)
					trackingAirways = tracking.FilterAirwaysByAltitude(trackingAirways, aircraft.Altitude)
					
					// Find best matching airway
					matchedAirwaySeg := tracking.MatchAirway(*aircraft, trackingAirways)
					
					if matchedAirwaySeg != nil {
						// Use airway-based prediction
						predictedPos := tracking.PredictPositionWithAirway(
							*aircraft,
							*matchedAirwaySeg,
							time.Now().UTC().Add(time.Duration(dataAge*float64(time.Second))),
						)
						acPos = predictedPos.Position
						confidence = predictedPos.Confidence
						predictionType = "airway"
						matchedAirway = matchedAirwaySeg.AirwayID
					} else {
						// No airway match - use dead reckoning
						predictedPos := tracking.PredictPositionWithLatency(*aircraft, dataAge)
						acPos = predictedPos.Position
						confidence = predictedPos.Confidence
						predictionType = "deadreckoning"
					}
				} else {
					// Fall back to dead reckoning
					predictedPos := tracking.PredictPositionWithLatency(*aircraft, dataAge)
					acPos = predictedPos.Position
					confidence = predictedPos.Confidence
					predictionType = "deadreckoning"
				}
			}
			
			// Warn if confidence is low
			if confidence < 0.5 {
				log.Printf("⚠️  Warning: Low prediction confidence (%.0f%%) - data is %.0fs old",
					confidence*100, dataAge)
			}
		} else {
			// Data is fresh - use as-is
			predicted = false
			acPos = coordinates.Geographic{
				Latitude:  aircraft.Latitude,
				Longitude: aircraft.Longitude,
				Altitude:  aircraft.Altitude * coordinates.FeetToMeters,
			}
			confidence = 1.0
			predictionType = ""
		}

		horiz := coordinates.GeographicToHorizontal(acPos, observer, now)

		// Calculate range and ETAs
		currentRange := coordinates.DistanceNauticalMiles(observer.Location, acPos)
		closestRange, timeToClosest, approaching := coordinates.EstimateTimeToClosestApproach(
			observer.Location, acPos, aircraft.GroundSpeed, aircraft.Track,
		)
		etaTo5nm := coordinates.EstimateTimeToRange(
			observer.Location, acPos, aircraft.GroundSpeed, aircraft.Track, 5.0,
		)

		// Check tracking limits
		event, message := tracking.CheckMeridianEvent(
			lastPosition, horiz, observer, trackingLimits,
			cfg.Telescope.SupportsMeridianFlip,
		)

		// Display status
		predictionMode := ""
		if predicted {
			switch predictionType {
			case "waypoint":
				predictionMode = " [WAYPOINT PREDICTION]"
			case "airway":
				predictionMode = fmt.Sprintf(" [AIRWAY PREDICTION: %s]", matchedAirway)
			case "deadreckoning":
				predictionMode = " [DEAD RECKONING]"
			}
		}
		
		fmt.Printf("\n[%s] Target: %s (%s)%s\n",
			now.Format("15:04:05"), aircraft.Callsign, aircraft.ICAO, predictionMode)
		
		// Show flight plan info if available
		if flightPlan != nil && len(waypointList) > 0 {
			nextWaypoint := ""
			for _, wp := range waypointList {
				if !wp.Passed {
					nextWaypoint = wp.Name
					break
				}
			}
			if nextWaypoint != "" {
				fmt.Printf("  Flight Plan: %s → %s (next: %s)\n",
					flightPlan.DepartureICAO, flightPlan.ArrivalICAO, nextWaypoint)
			} else {
				fmt.Printf("  Flight Plan: %s → %s (all waypoints passed)\n",
					flightPlan.DepartureICAO, flightPlan.ArrivalICAO)
			}
		}
		
		if predicted {
			fmt.Printf("  Last Known: %.4f°N, %.4f°W, %.0f ft MSL\n",
				aircraft.Latitude, aircraft.Longitude, aircraft.Altitude)
			fmt.Printf("  Predicted:  %.4f°N, %.4f°W, %.0f ft MSL (%.0f%% confidence)\n",
				acPos.Latitude, acPos.Longitude, acPos.Altitude/coordinates.FeetToMeters, confidence*100)
		} else {
			fmt.Printf("  Position: %.4f°N, %.4f°W, %.0f ft MSL\n",
				aircraft.Latitude, aircraft.Longitude, aircraft.Altitude)
		}
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
		if predicted {
			fmt.Printf("  Data age: %.1fs (USING PREDICTION)\n", dataAge)
		} else {
			fmt.Printf("  Data age: %.1fs\n", dataAge)
		}
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

		// Stop tracking if data is too old and prediction confidence is very low
		if dataAge > 300 && confidence < 0.3 {
			fmt.Printf("  Status: ❌ DATA TOO STALE - Lost ADS-B coverage (%.0fs old, %.0f%% confidence)\n",
				dataAge, confidence*100)
			log.Printf("\n⚠️  Aircraft %s has left ADS-B coverage. Stopping tracking.", aircraft.ICAO)
			log.Println("   Select a different aircraft or wait for it to re-enter coverage.")
			break
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

// ternarySting returns one of two strings based on a condition.
func ternarySting(condition bool, trueVal, falseVal string) string {
	if condition {
		return trueVal
	}
	return falseVal
}
