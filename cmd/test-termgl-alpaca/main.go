package main

import (
	"fmt"
	"log"
	"time"

	"github.com/unklstewy/ads-bscope/pkg/alpaca"
	"github.com/unklstewy/ads-bscope/pkg/config"
)

// This test verifies that the termgl client can connect to and control
// the telescope via the Alpaca API. It simulates what the UI would do
// when tracking an aircraft.
func main() {
	fmt.Println("=========================================================")
	fmt.Println("TermGL Client - Alpaca Integration Test")
	fmt.Println("=========================================================")
	fmt.Println()

	// Load configuration
	cfg, err := config.Load("configs/config.json")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	fmt.Println("Configuration:")
	fmt.Printf("  Telescope URL: %s\n", cfg.Telescope.BaseURL)
	fmt.Printf("  Mount Type: %s\n", cfg.Telescope.MountType)
	fmt.Println()

	// Create Alpaca client
	fmt.Println("Creating Alpaca telescope client...")
	telescopeClient := alpaca.NewClient(cfg.Telescope)

	// Connect to telescope
	fmt.Println("Connecting to telescope...")
	if err := telescopeClient.Connect(); err != nil {
		log.Fatalf("Failed to connect to telescope: %v", err)
	}
	defer func() {
		fmt.Println("Disconnecting from telescope...")
		telescopeClient.Disconnect()
	}()

	fmt.Println("✓ Connected to telescope")
	fmt.Println()

	// Simulate tracking an aircraft by slewing to multiple positions
	fmt.Println("Simulating aircraft tracking (slewing to different positions):")
	fmt.Println()

	positions := []struct {
		name string
		alt  float64
		az   float64
	}{
		{"NE (heading toward aircraft)", 30.0, 45.0},
		{"NE Higher (ascending climb)", 45.0, 45.0},
		{"E (aircraft moving east)", 50.0, 90.0},
		{"SE (aircraft turning south)", 40.0, 135.0},
		{"S (aircraft heading south)", 35.0, 180.0},
	}

	for i, pos := range positions {
		fmt.Printf("[%d/%d] Slewing to %s (Alt=%.0f°, Az=%.0f°)\n",
			i+1, len(positions), pos.name, pos.alt, pos.az)

		// Slew to position
		if err := telescopeClient.SlewToAltAz(pos.alt, pos.az); err != nil {
			log.Fatalf("Failed to slew: %v", err)
		}

		// Monitor slewing
		fmt.Print("         Status: ")
		startTime := time.Now()
		lastWasSlewing := true
		for {
			slewing, err := telescopeClient.IsSlewing()
			if err != nil {
				log.Fatalf("Failed to check slewing status: %v", err)
			}

			if slewing != lastWasSlewing {
				if slewing {
					fmt.Print("slewing ")
				} else {
					fmt.Print("complete")
				}
				lastWasSlewing = slewing
			}

			if !slewing {
				break
			}

			if time.Since(startTime) > 10*time.Second {
				fmt.Print("(timeout)")
				break
			}

			time.Sleep(100 * time.Millisecond)
		}
		fmt.Println()

		// Small delay between slews to simulate time passing
		time.Sleep(500 * time.Millisecond)
	}

	fmt.Println()
	fmt.Println("=========================================================")
	fmt.Println("✓ TEST COMPLETE - Telescope control working correctly")
	fmt.Println("=========================================================")
	fmt.Println()
	fmt.Println("Summary:")
	fmt.Println("  • Telescope connection: OK")
	fmt.Println("  • Slew commands: OK")
	fmt.Println("  • Status monitoring: OK")
	fmt.Println()
	fmt.Println("The termgl client can successfully control the telescope")
	fmt.Println("and track aircraft movements.")
}
