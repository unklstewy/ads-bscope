package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/unklstewy/ads-bscope/pkg/config"
)

// main is the entry point for the ads-bscope application.
// It loads configuration and initializes the HTTP server and routes for the PWA.
func main() {
	// Load configuration from file or use defaults
	// Config path can be overridden with CONFIG_PATH environment variable
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "configs/config.json"
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Printf("Configuration loaded from %s", configPath)
	log.Printf("Database: %s@%s:%d/%s", cfg.Database.Username, cfg.Database.Host, cfg.Database.Port, cfg.Database.Database)
	log.Printf("Telescope: %s (type: %s)", cfg.Telescope.BaseURL, cfg.Telescope.MountType)
	log.Printf("ADS-B: %d sources configured, radius=%.1fnm", len(cfg.ADSB.Sources), cfg.ADSB.SearchRadiusNM)
	log.Printf("Observer: lat=%.4f, lon=%.4f, elev=%.1fm", cfg.Observer.Latitude, cfg.Observer.Longitude, cfg.Observer.Elevation)

	// Setup routes
	http.HandleFunc("/", handleRoot)
	http.HandleFunc("/health", handleHealth)

	// Start server
	addr := fmt.Sprintf("%s:%s", cfg.Server.Host, cfg.Server.Port)
	log.Printf("Starting ads-bscope server on %s", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

// handleRoot serves the main application page.
// This will eventually serve the PWA frontend.
func handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head>
    <title>ADS-B Scope</title>
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
</head>
<body>
    <h1>ADS-B Scope</h1>
    <p>Aircraft tracking and telescope control system</p>
</body>
</html>`)
}

// handleHealth provides a health check endpoint for container orchestration.
// Returns 200 OK if the service is running.
func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok"}`)
}
