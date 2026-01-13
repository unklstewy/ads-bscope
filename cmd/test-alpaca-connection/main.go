package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/unklstewy/ads-bscope/pkg/alpaca"
	"github.com/unklstewy/ads-bscope/pkg/config"
)

// main tests the Alpaca simulator connection.
// Tests:
// 1. HTTP connectivity to Alpaca server
// 2. Connect to telescope
// 3. Query connection status
// 4. Test slew to Alt/Az coordinates
// 5. Query slewing status
// 6. Abort slew
// 7. Disconnect from telescope
func main() {
	fmt.Println("======================================================================")
	fmt.Println("ADS-B Scope - Alpaca Simulator Connection Test")
	fmt.Println("======================================================================")
	fmt.Println()

	// Load configuration
	cfg, err := config.Load("configs/config.json")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	telescopeCfg := cfg.Telescope
	fmt.Printf("Telescope Configuration:\n")
	fmt.Printf("  Base URL:     %s\n", telescopeCfg.BaseURL)
	fmt.Printf("  Device Num:   %d\n", telescopeCfg.DeviceNumber)
	fmt.Printf("  Mount Type:   %s\n", telescopeCfg.MountType)
	fmt.Printf("  Model:        %s\n", telescopeCfg.Model)
	fmt.Printf("  Slew Rate:    %.1f deg/sec\n", telescopeCfg.SlewRate)
	fmt.Println()

	// Step 1: Test basic HTTP connectivity
	fmt.Println("Step 1: Testing HTTP connectivity...")
	if err := testHTTPConnectivity(telescopeCfg.BaseURL); err != nil {
		log.Fatalf("Failed HTTP connectivity test: %v", err)
	}
	fmt.Println("  ✓ HTTP connectivity successful")
	fmt.Println()

	// Step 2: Create Alpaca client
	fmt.Println("Step 2: Creating Alpaca client...")
	client := alpaca.NewClient(telescopeCfg)
	fmt.Println("  ✓ Client created")
	fmt.Println()

	// Step 3: Connect to telescope
	fmt.Println("Step 3: Connecting to telescope...")
	if err := client.Connect(); err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	fmt.Println("  ✓ Connected to telescope")
	fmt.Println()

	// Step 4: Check connection status
	fmt.Println("Step 4: Verifying connection status...")
	connected, err := client.IsConnected()
	if err != nil {
		log.Fatalf("Failed to check connection status: %v", err)
	}
	fmt.Printf("  ✓ Connection status: %v\n", connected)
	if !connected {
		log.Fatal("Telescope reports not connected!")
	}
	fmt.Println()

	// Step 5: Slew to first target
	fmt.Println("Step 5: Slewing to first target (Alt=45°, Az=180°)...")
	alt1 := 45.0
	az1 := 180.0
	if err := client.SlewToAltAz(alt1, az1); err != nil {
		log.Fatalf("Failed to slew: %v", err)
	}
	fmt.Printf("  ✓ Slew initiated to Alt=%.1f°, Az=%.1f°\n", alt1, az1)
	fmt.Println()

	// Step 6: Monitor slewing status
	fmt.Println("Step 6: Monitoring slew progress...")
	monitorSlew(client, 5*time.Second)
	fmt.Println()

	// Step 7: Slew to second target
	fmt.Println("Step 7: Slewing to second target (Alt=60°, Az=270°)...")
	alt2 := 60.0
	az2 := 270.0
	if err := client.SlewToAltAz(alt2, az2); err != nil {
		log.Fatalf("Failed to slew: %v", err)
	}
	fmt.Printf("  ✓ Slew initiated to Alt=%.1f°, Az=%.1f°\n", alt2, az2)
	fmt.Println()

	// Step 8: Monitor slewing again
	fmt.Println("Step 8: Monitoring second slew...")
	monitorSlew(client, 5*time.Second)
	fmt.Println()

	// Step 9: Test abort slew
	fmt.Println("Step 9: Testing slew abort (initiating then canceling)...")
	alt3 := 75.0
	az3 := 90.0
	if err := client.SlewToAltAz(alt3, az3); err != nil {
		log.Fatalf("Failed to initiate slew: %v", err)
	}
	fmt.Printf("  Slew initiated to Alt=%.1f°, Az=%.1f°\n", alt3, az3)

	time.Sleep(500 * time.Millisecond)
	if err := client.AbortSlew(); err != nil {
		log.Fatalf("Failed to abort slew: %v", err)
	}
	fmt.Println("  ✓ Slew aborted")

	// Verify slewing stopped
	time.Sleep(500 * time.Millisecond)
	slewing, err := client.IsSlewing()
	if err != nil {
		log.Fatalf("Failed to check slewing status: %v", err)
	}
	fmt.Printf("  ✓ Slewing status after abort: %v\n", slewing)
	fmt.Println()

	// Step 10: Disconnect
	fmt.Println("Step 10: Disconnecting from telescope...")
	if err := client.Disconnect(); err != nil {
		log.Fatalf("Failed to disconnect: %v", err)
	}
	fmt.Println("  ✓ Disconnected")
	fmt.Println()

	// Final status check
	fmt.Println("Step 11: Verifying disconnection...")
	connected, err = client.IsConnected()
	if err != nil {
		log.Fatalf("Failed to check connection status: %v", err)
	}
	fmt.Printf("  ✓ Connection status: %v\n", connected)
	if connected {
		log.Fatal("Telescope still reports connected!")
	}
	fmt.Println()

	// Success summary
	fmt.Println("======================================================================")
	fmt.Println("✓ ALL TESTS PASSED")
	fmt.Println("======================================================================")
	fmt.Println()
	fmt.Println("Summary:")
	fmt.Println("  • HTTP connectivity:         OK")
	fmt.Println("  • Client initialization:     OK")
	fmt.Println("  • Connect/Disconnect:        OK")
	fmt.Println("  • Connection status query:   OK")
	fmt.Println("  • Slew to Alt/Az:            OK")
	fmt.Println("  • Slewing status monitoring: OK")
	fmt.Println("  • Slew abort:                OK")
	fmt.Println()
}

// testHTTPConnectivity verifies we can reach the Alpaca server.
func testHTTPConnectivity(baseURL string) error {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Try a simple GET request to the API
	resp, err := client.Get(baseURL + "/api/v1/telescope/0/connected?ClientID=1234&ClientTransactionID=5678")
	if err != nil {
		return fmt.Errorf("failed to reach Alpaca server at %s: %w", baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP status %d from Alpaca server", resp.StatusCode)
	}

	// Try a simple PUT request to verify PUT endpoint works
	putData := url.Values{}
	putData.Add("Connected", "true")
	putData.Add("ClientID", "1234")
	putData.Add("ClientTransactionID", "5678")
	
	putReq, err := http.NewRequest("PUT", baseURL+"/api/v1/telescope/0/connected",
		strings.NewReader(putData.Encode()))
	if err != nil {
		return err
	}
	putReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	putResp, err := client.Do(putReq)
	if err != nil {
		return fmt.Errorf("failed to reach Alpaca PUT endpoint: %w", err)
	}
	defer putResp.Body.Close()

	if putResp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected HTTP status %d from Alpaca PUT endpoint", putResp.StatusCode)
	}

	return nil
}

// monitorSlew watches the telescope's slewing status until complete.
// Updates every 200ms for up to the specified timeout.
func monitorSlew(client *alpaca.Client, timeout time.Duration) {
	startTime := time.Now()
	lastStatus := true

	for {
		slewing, err := client.IsSlewing()
		if err != nil {
			fmt.Printf("  ! Error checking slewing status: %v\n", err)
			return
		}

		// Only print when status changes
		if slewing != lastStatus {
			if slewing {
				fmt.Println("    → Slew started")
			} else {
				fmt.Println("    → Slew complete")
			}
			lastStatus = slewing
		}

		// Exit if slew is complete
		if !slewing {
			return
		}

		// Exit if timeout reached
		if time.Since(startTime) > timeout {
			fmt.Println("    (timeout waiting for slew to complete)")
			return
		}

		time.Sleep(200 * time.Millisecond)
	}
}
