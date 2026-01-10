package main

import (
	"flag"
	"log"
	"time"

	"github.com/unklstewy/ads-bscope/pkg/adsb"
	"github.com/unklstewy/ads-bscope/pkg/config"
)

// main tests the ADS-B API to find the optimal rate limit using a bracketing approach.
// This helps determine the maximum safe call rate before hitting 429 (Too Many Requests) errors.
func main() {
	configPath := flag.String("config", "configs/config.json", "Path to configuration file")
	minDelay := flag.Float64("min", 1.0, "Minimum delay between calls in seconds")
	maxDelay := flag.Float64("max", 10.0, "Maximum delay between calls in seconds")
	testCalls := flag.Int("calls", 5, "Number of test calls per interval")
	flag.Parse()

	log.Println("=========================================")
	log.Println("  ADS-B API Rate Limit Tester")
	log.Println("=========================================")

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	if len(cfg.ADSB.Sources) == 0 {
		log.Fatal("Error: No ADS-B sources configured")
	}

	source := cfg.ADSB.Sources[0]
	log.Printf("Testing API: %s (%s)", source.Name, source.BaseURL)
	log.Printf("Observer location: %.4f°N, %.4f°W", cfg.Observer.Latitude, cfg.Observer.Longitude)
	log.Printf("Bracketing range: %.1fs - %.1fs", *minDelay, *maxDelay)
	log.Printf("Test calls per interval: %d", *testCalls)
	log.Println()

	// Create client
	client := adsb.NewAirplanesLiveClient(source.BaseURL)
	defer client.Close()

	// Bracketing algorithm
	currentDelay := *maxDelay // Start with conservative (slow) rate
	minSafe := *maxDelay
	maxFailed := *minDelay

	log.Println("Starting bracketing test...")
	log.Println("This will test different call rates to find the optimal setting.")
	log.Println()

	iteration := 1
	for maxFailed < minSafe-0.5 { // Continue until bracket is within 0.5 seconds
		log.Printf("Iteration %d: Testing %.2f second delay...", iteration, currentDelay)
		
		success, err := testCallRate(client, cfg.Observer.Latitude, cfg.Observer.Longitude, currentDelay, *testCalls)
		
		if success {
			log.Printf("  ✓ Success with %.2fs delay", currentDelay)
			minSafe = currentDelay
			// Try faster (smaller delay)
			if currentDelay > maxFailed {
				currentDelay = (currentDelay + maxFailed) / 2.0
			} else {
				break // Found minimum
			}
		} else {
			if err != nil {
				log.Printf("  ✗ Failed with %.2fs delay: %v", currentDelay, err)
			} else {
				log.Printf("  ✗ Rate limited (429) with %.2fs delay", currentDelay)
			}
			maxFailed = currentDelay
			// Try slower (larger delay)
			if currentDelay < minSafe {
				currentDelay = (currentDelay + minSafe) / 2.0
			} else {
				// Start higher
				currentDelay = *maxDelay
				minSafe = *maxDelay
			}
		}
		
		log.Println()
		iteration++
		
		// Safety limit
		if iteration > 10 {
			log.Println("Maximum iterations reached")
			break
		}
		
		// Wait before next test iteration
		time.Sleep(3 * time.Second)
	}

	// Results
	log.Println("=========================================")
	log.Println("  Test Results")
	log.Println("=========================================")
	log.Printf("Recommended rate limit: %.1f seconds", minSafe)
	log.Printf("Update your config.json:")
	log.Printf("  \"rate_limit_seconds\": %.1f", minSafe)
	log.Println()
	
	// Calculate practical rates
	callsPerMinute := 60.0 / minSafe
	log.Printf("This allows approximately %.0f API calls per minute", callsPerMinute)
	log.Println("=========================================")
}

// testCallRate tests making multiple API calls at a specific rate.
// Returns true if all calls succeed, false if any hit rate limits.
func testCallRate(
	client *adsb.AirplanesLiveClient,
	lat, lon float64,
	delaySeconds float64,
	numCalls int,
) (bool, error) {
	delay := time.Duration(delaySeconds * float64(time.Second))
	
	for i := 0; i < numCalls; i++ {
		if i > 0 {
			time.Sleep(delay)
		}
		
		// Make API call
		aircraft, err := client.GetAircraft(lat, lon, 50.0)
		
		if err != nil {
			// Check if it's a rate limit error
			if isRateLimitError(err) {
				return false, nil
			}
			// Other error
			return false, err
		}
		
		log.Printf("    Call %d/%d: Success (%d aircraft found)", i+1, numCalls, len(aircraft))
	}
	
	return true, nil
}

// isRateLimitError checks if the error is a 429 rate limit error.
func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	
	// Check if error message contains "429" or "rate limit"
	errMsg := err.Error()
	return contains(errMsg, "429") || contains(errMsg, "rate limit") || contains(errMsg, "Too Many Requests")
}

// contains checks if a string contains a substring (case-insensitive check could be added).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && 
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || 
		containsInner(s, substr)))
}

func containsInner(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
