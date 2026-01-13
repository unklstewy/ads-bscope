package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/unklstewy/ads-bscope/internal/db"
	"github.com/unklstewy/ads-bscope/pkg/config"
	"github.com/unklstewy/ads-bscope/pkg/coordinates"
)

var (
	// Version information (set by build flags)
	version = "dev"
	commit  = "unknown"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "configs/config.json", "Path to configuration file")
	showVersion := flag.Bool("version", false, "Show version information")
	showHelp := flag.Bool("help", false, "Show help information")
	flag.Parse()

	// Show version
	if *showVersion {
		fmt.Printf("termgl-client version %s (commit: %s)\n", version, commit)
		os.Exit(0)
	}

	// Show help
	if *showHelp {
		printHelp()
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Setup observer
	observer := coordinates.Observer{
		Location: coordinates.Geographic{
			Latitude:  cfg.Observer.Latitude,
			Longitude: cfg.Observer.Longitude,
			Altitude:  cfg.Observer.Elevation,
		},
		Timezone: cfg.Observer.TimeZone,
	}

	// Connect to database
	database, err := db.Connect(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()

	// Initialize repositories
	aircraftRepo := db.NewAircraftRepository(database, observer)
	flightPlanRepo := db.NewFlightPlanRepository(database)

	// Create and run the application
	app := NewApp(&AppConfig{
		Config:             cfg,
		ConfigPath:         *configPath,
		Database:           database,
		AircraftRepository: aircraftRepo,
		FlightPlanRepo:     flightPlanRepo,
		Observer:           observer,
	})

	if err := app.Run(); err != nil {
		log.Fatalf("Application error: %v", err)
	}
}

// printHelp prints usage information
func printHelp() {
	fmt.Println("termgl-client - Advanced Terminal UI for ADS-B Scope")
	fmt.Println()
	fmt.Println("USAGE:")
	fmt.Println("  termgl-client [options]")
	fmt.Println()
	fmt.Println("OPTIONS:")
	fmt.Println("  -config string")
	fmt.Println("        Path to configuration file (default: configs/config.json)")
	fmt.Println("  -version")
	fmt.Println("        Show version information")
	fmt.Println("  -help")
	fmt.Println("        Show this help message")
	fmt.Println()
	fmt.Println("KEYBOARD SHORTCUTS:")
	fmt.Println("  Navigation:")
	fmt.Println("    ↑/↓ or j/k     Select aircraft")
	fmt.Println("    PgUp/PgDn      Fast scroll")
	fmt.Println()
	fmt.Println("  Actions:")
	fmt.Println("    ENTER          Track selected aircraft")
	fmt.Println("    SPACE          Stop tracking")
	fmt.Println("    t              Toggle trails")
	fmt.Println("    c              Toggle constellations")
	fmt.Println()
	fmt.Println("  Views:")
	fmt.Println("    s              Switch to sky view")
	fmt.Println("    r              Switch to radar view")
	fmt.Println("    m              Open config menu")
	fmt.Println("    ?              Show help screen")
	fmt.Println()
	fmt.Println("  Zoom:")
	fmt.Println("    +/-            Zoom in/out")
	fmt.Println("    0              Reset zoom")
	fmt.Println()
	fmt.Println("  Control:")
	fmt.Println("    q or Ctrl+C    Quit application")
	fmt.Println()
	fmt.Println("FEATURES:")
	fmt.Println("  - Multi-panel layout with sky/radar view")
	fmt.Println("  - Real-time aircraft tracking")
	fmt.Println("  - Telescope control integration")
	fmt.Println("  - Advanced geometric rendering")
	fmt.Println("  - Track trails and trajectory predictions")
	fmt.Println()
	fmt.Println("For more information, visit:")
	fmt.Println("  https://github.com/unklstewy/ads-bscope")
}
