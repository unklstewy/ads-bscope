package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/unklstewy/ads-bscope/internal/db"
	"github.com/unklstewy/ads-bscope/pkg/config"
)

// NASR Data Importer
// Imports waypoints and airways from FAA NASR (National Airspace System Resources) data.
//
// Download NASR data from:
// https://www.faa.gov/air_traffic/flight_info/aeronav/aero_data/NASR_Subscription/
//
// Required files from the 28-day subscription:
// - FIX.txt (Navigation fixes)
// - NAV.txt (VORs and NDBs)
// - AWY.txt (Airways)
// - APT.txt (Airports - optional, for reference)

func main() {
	configPath := flag.String("config", "configs/config.json", "Path to configuration file")
	nasrDir := flag.String("nasr-dir", "data/nasr", "Directory containing NASR data files")
	flag.Parse()

	log.Println("===========================================")
	log.Println("  NASR Data Importer")
	log.Println("===========================================")

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Connect to database
	log.Println("Connecting to database...")
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
	log.Println("✓ Schema initialized")

	importer := &NASRImporter{
		db:      database,
		nasrDir: *nasrDir,
	}

	// Import waypoints
	log.Println("\n===========================================")
	log.Println("Importing Waypoints")
	log.Println("===========================================")
	
	fixCount, err := importer.ImportFixes(ctx)
	if err != nil {
		log.Printf("Warning: Failed to import fixes: %v", err)
	} else {
		log.Printf("✓ Imported %d navigation fixes", fixCount)
	}

	navCount, err := importer.ImportNavaids(ctx)
	if err != nil {
		log.Printf("Warning: Failed to import navaids: %v", err)
	} else {
		log.Printf("✓ Imported %d navaids (VORs/NDBs)", navCount)
	}

	// Import airways
	log.Println("\n===========================================")
	log.Println("Importing Airways")
	log.Println("===========================================")
	
	awyCount, err := importer.ImportAirways(ctx)
	if err != nil {
		log.Printf("Warning: Failed to import airways: %v", err)
	} else {
		log.Printf("✓ Imported %d airway segments", awyCount)
	}

	// Summary
	log.Println("\n===========================================")
	log.Println("Import Complete")
	log.Println("===========================================")
	log.Printf("Total waypoints: %d", fixCount+navCount)
	log.Printf("Total airway segments: %d", awyCount)
}

// NASRImporter handles importing NASR data files.
type NASRImporter struct {
	db      *db.DB
	nasrDir string
}

// ImportFixes imports navigation fixes from FIX.txt.
func (i *NASRImporter) ImportFixes(ctx context.Context) (int, error) {
	filePath := fmt.Sprintf("%s/FIX.txt", i.nasrDir)
	file, err := os.Open(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open FIX.txt: %w", err)
	}
	defer file.Close()

	count := 0
	scanner := bufio.NewScanner(file)
	
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 100 {
			continue
		}

		// Parse fixed-width FIX record
		// Format defined in NASR Data Format Specification
		recordType := strings.TrimSpace(line[0:4])
		if recordType != "FIX1" {
			continue
		}

		identifier := strings.TrimSpace(line[4:34])
		region := strings.TrimSpace(line[34:36])
		latStr := strings.TrimSpace(line[66:80])
		lonStr := strings.TrimSpace(line[80:94])

		// Parse lat/lon (format: DD-MM-SS.SSSH where H is N/S/E/W)
		lat, err := parseLatLon(latStr)
		if err != nil {
			continue
		}
		lon, err := parseLatLon(lonStr)
		if err != nil {
			continue
		}

		// Insert waypoint
		_, err = i.db.ExecContext(ctx,
			`INSERT INTO waypoints (identifier, latitude, longitude, type, region)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (identifier, region) DO UPDATE SET
			 latitude = EXCLUDED.latitude,
			 longitude = EXCLUDED.longitude`,
			identifier, lat, lon, "fix", region,
		)
		if err != nil {
			log.Printf("Warning: Failed to insert fix %s: %v", identifier, err)
			continue
		}

		count++
		if count%1000 == 0 {
			log.Printf("  Imported %d fixes...", count)
		}
	}

	return count, scanner.Err()
}

// ImportNavaids imports VORs and NDBs from NAV.txt.
func (i *NASRImporter) ImportNavaids(ctx context.Context) (int, error) {
	filePath := fmt.Sprintf("%s/NAV.txt", i.nasrDir)
	file, err := os.Open(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open NAV.txt: %w", err)
	}
	defer file.Close()

	count := 0
	scanner := bufio.NewScanner(file)
	
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 100 {
			continue
		}

		// Parse fixed-width NAV record
		recordType := strings.TrimSpace(line[0:4])
		if recordType != "NAV1" {
			continue
		}

		identifier := strings.TrimSpace(line[4:8])
		navType := strings.TrimSpace(line[8:28])
		name := strings.TrimSpace(line[42:72])
		latStr := strings.TrimSpace(line[371:385])
		lonStr := strings.TrimSpace(line[396:410])

		// Parse lat/lon
		lat, err := parseLatLon(latStr)
		if err != nil {
			continue
		}
		lon, err := parseLatLon(lonStr)
		if err != nil {
			continue
		}

		// Determine waypoint type
		wpType := "vor"
		if strings.Contains(strings.ToLower(navType), "ndb") {
			wpType = "ndb"
		} else if strings.Contains(strings.ToLower(navType), "tacan") {
			wpType = "tacan"
		}

		// Insert waypoint
		_, err = i.db.ExecContext(ctx,
			`INSERT INTO waypoints (identifier, name, latitude, longitude, type, region)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 ON CONFLICT (identifier, region) DO UPDATE SET
			 name = EXCLUDED.name,
			 latitude = EXCLUDED.latitude,
			 longitude = EXCLUDED.longitude`,
			identifier, name, lat, lon, wpType, "US",
		)
		if err != nil {
			log.Printf("Warning: Failed to insert navaid %s: %v", identifier, err)
			continue
		}

		count++
		if count%100 == 0 {
			log.Printf("  Imported %d navaids...", count)
		}
	}

	return count, scanner.Err()
}

// ImportAirways imports airways from AWY.txt.
func (i *NASRImporter) ImportAirways(ctx context.Context) (int, error) {
	filePath := fmt.Sprintf("%s/AWY.txt", i.nasrDir)
	file, err := os.Open(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open AWY.txt: %w", err)
	}
	defer file.Close()

	count := 0
	scanner := bufio.NewScanner(file)
	
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 100 {
			continue
		}

		// Parse fixed-width AWY record
		recordType := strings.TrimSpace(line[0:4])
		if recordType != "AWY2" {
			continue
		}

		airwayID := strings.TrimSpace(line[4:9])
		sequenceStr := strings.TrimSpace(line[9:14])
		waypointID := strings.TrimSpace(line[15:45])
		
		sequence, err := strconv.Atoi(sequenceStr)
		if err != nil {
			continue
		}

		// Determine airway type
		awyType := "other"
		if strings.HasPrefix(airwayID, "V") {
			awyType = "victor"
		} else if strings.HasPrefix(airwayID, "J") {
			awyType = "jet"
		} else if strings.HasPrefix(airwayID, "Q") || strings.HasPrefix(airwayID, "T") {
			awyType = "rnav"
		}

		// Get waypoint ID from database
		var waypointDBID int
		err = i.db.QueryRowContext(ctx,
			`SELECT id FROM waypoints WHERE identifier = $1 LIMIT 1`,
			waypointID,
		).Scan(&waypointDBID)
		
		if err != nil {
			// Waypoint not found - skip this airway segment
			continue
		}

		// Insert airway segment
		_, err = i.db.ExecContext(ctx,
			`INSERT INTO airways (identifier, type, sequence, waypoint_id, direction)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (identifier, sequence) DO UPDATE SET
			 waypoint_id = EXCLUDED.waypoint_id`,
			airwayID, awyType, sequence, waypointDBID, "bidirectional",
		)
		if err != nil {
			log.Printf("Warning: Failed to insert airway %s seq %d: %v", airwayID, sequence, err)
			continue
		}

		count++
		if count%1000 == 0 {
			log.Printf("  Imported %d airway segments...", count)
		}
	}

	return count, scanner.Err()
}

// parseLatLon parses NASR lat/lon format: DD-MM-SS.SSSH (where H is N/S/E/W).
func parseLatLon(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if len(s) < 11 {
		return 0, fmt.Errorf("invalid lat/lon format: %s", s)
	}

	// Extract hemisphere
	hemisphere := s[len(s)-1:]
	s = s[:len(s)-1]

	// Parse DD-MM-SS.SSS
	parts := strings.Split(s, "-")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid lat/lon parts: %s", s)
	}

	degrees, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, err
	}

	minutes, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return 0, err
	}

	seconds, err := strconv.ParseFloat(parts[2], 64)
	if err != nil {
		return 0, err
	}

	// Convert to decimal degrees
	decimal := degrees + (minutes / 60.0) + (seconds / 3600.0)

	// Apply hemisphere
	if hemisphere == "S" || hemisphere == "W" {
		decimal = -decimal
	}

	return decimal, nil
}
