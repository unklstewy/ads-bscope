// ADS-B Scope Web Server
// Serves the PWA interface and provides REST API + WebSocket endpoints
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	_ "github.com/lib/pq"

	"github.com/unklstewy/ads-bscope/internal/auth"
	"github.com/unklstewy/ads-bscope/internal/db"
	"github.com/unklstewy/ads-bscope/pkg/alpaca"
	"github.com/unklstewy/ads-bscope/pkg/config"
	"github.com/unklstewy/ads-bscope/pkg/coordinates"
)

var (
	configPath = flag.String("config", "configs/config.json", "Path to configuration file")
	port       = flag.Int("port", 8080, "HTTP server port")
)

// Server holds the HTTP server and its dependencies
type Server struct {
	router       *chi.Mux
	db           *sql.DB
	authSvc      *auth.Service
	userRepo     *db.UserRepository
	aircraftRepo *db.AircraftRepository
	observerRepo *db.ObservationPointRepository
	telescope    *alpaca.TelescopeClient
	cfg          *config.Config
}

func main() {
	flag.Parse()

	log.Println("🚀 Starting ADS-B Scope Web Server...")

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Connect to database
	database, err := connectDatabase(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()

	// Run migrations
	if err := runMigrations(database); err != nil {
		log.Printf("Warning: Migrations failed: %v", err)
	}

	// Initialize auth service
	authSvc := auth.NewService(auth.Config{
		JWTSecret:     getEnvOrDefault("JWT_SECRET", "dev-secret-change-in-production"),
		TokenDuration: 24 * time.Hour,
	})

	// Initialize repositories
	userRepo := db.NewUserRepository(database)
	
	// Create observer for aircraft calculations (default from config)
	observer := coordinates.Observer{
		Location: coordinates.Geographic{
			Latitude:  cfg.Observer.Latitude,
			Longitude: cfg.Observer.Longitude,
			Altitude:  cfg.Observer.Elevation,
		},
	}
	
	// Wrap sql.DB in db.DB for aircraft repository
	dbWrapper := &db.DB{DB: database}
	aircraftRepo := db.NewAircraftRepository(dbWrapper, observer)
	observerRepo := db.NewObservationPointRepository(dbWrapper)
	
	// Initialize telescope client
	// Use environment variable if set, otherwise use config
	telescopeURL := getEnvOrDefault("TELESCOPE_URL", cfg.Telescope.BaseURL)
	telescopeClient := alpaca.NewTelescopeClient(telescopeURL, cfg.Telescope.DeviceNumber)
	log.Printf("🔭 Telescope client initialized: %s (device %d)", telescopeURL, cfg.Telescope.DeviceNumber)

	// Create server
	srv := &Server{
		router:       chi.NewRouter(),
		db:           database,
		authSvc:      authSvc,
		userRepo:     userRepo,
		aircraftRepo: aircraftRepo,
		observerRepo: observerRepo,
		telescope:    telescopeClient,
		cfg:          cfg,
	}

	// Setup routes
	srv.setupRoutes()

	// Start HTTP server
	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", *port),
		Handler:      srv.router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("📡 Server listening on http://localhost:%d", *port)
		log.Printf("💡 Open http://localhost:%d in your browser", *port)
		log.Printf("   Demo login: admin / admin\n")
		
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("\n👋 Shutting down server...")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("✅ Server stopped")
}

// setupRoutes configures all HTTP routes
func (s *Server) setupRoutes() {
	r := s.router

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Compress(5))

	// CORS for development
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// API routes
	r.Route("/api/v1", func(r chi.Router) {
		// Public routes
		r.Post("/auth/login", s.handleLogin)
		
		// Protected routes (require authentication)
		r.Group(func(r chi.Router) {
			r.Use(s.authMiddleware)
			
			r.Post("/auth/logout", s.handleLogout)
			r.Get("/auth/me", s.handleGetCurrentUser)
			
			// Aircraft endpoints
			r.Get("/aircraft", s.handleGetAircraft)
			r.Get("/aircraft/{icao}", s.handleGetAircraftByICAO)
			
			// Observation point endpoints
			r.Get("/observer/points", s.handleGetObservationPoints)
			r.Get("/observer/active", s.handleGetActiveObservationPoint)
			r.Post("/observer/points", s.handleCreateObservationPoint)
			r.Put("/observer/points/{id}", s.handleUpdateObservationPoint)
			r.Delete("/observer/points/{id}", s.handleDeleteObservationPoint)
			r.Post("/observer/points/{id}/activate", s.handleActivateObservationPoint)
			
			// Telescope endpoints
			r.Get("/telescope/config", s.handleGetTelescopeConfig)
			r.Get("/telescope/status", s.handleGetTelescopeStatus)
			r.Post("/telescope/slew", s.handleTelescopeSlew)
			r.Post("/telescope/track/{icao}", s.handleTelescopeTrack)
			r.Post("/telescope/stop", s.handleTelescopeStop)
			r.Post("/telescope/abort", s.handleTelescopeAbort)
			
			// System endpoints
			r.Get("/system/status", s.handleGetSystemStatus)
		})
		
		// WebSocket endpoint (will implement later)
		// r.Get("/ws", s.handleWebSocket)
	})

	// Serve static files (PWA)
	// Get absolute path to static directory
	execPath, _ := os.Executable()
	execDir := filepath.Dir(execPath)
	staticDir := filepath.Join(execDir, "../../web/static")
	
	// Check if static directory exists
	if _, err := os.Stat(staticDir); os.IsNotExist(err) {
		// Try relative to current directory
		staticDir = "web/static"
	}
	
	log.Printf("📁 Serving static files from: %s", staticDir)
	
	// Serve all static files
	fileServer := http.FileServer(http.Dir(staticDir))
	r.Handle("/css/*", fileServer)
	r.Handle("/js/*", fileServer)
	r.Handle("/icons/*", fileServer)
	r.Handle("/manifest.json", fileServer)
	r.Handle("/sw.js", fileServer)
	
	// Serve index.html for all other routes (SPA routing)
	r.Get("/*", func(w http.ResponseWriter, r *http.Request) {
		indexPath := filepath.Join(staticDir, "index.html")
		http.ServeFile(w, r, indexPath)
	})
}

// Auth middleware
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get token from Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Missing authorization header", http.StatusUnauthorized)
			return
		}

		// Extract token (format: "Bearer <token>")
		var token string
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
		} else {
			http.Error(w, "Invalid authorization header format", http.StatusUnauthorized)
			return
		}

		// Validate token
		claims, err := s.authSvc.ValidateToken(token)
		if err != nil {
			http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		// Add claims to context
		ctx := context.WithValue(r.Context(), "user_id", claims.UserID)
		ctx = context.WithValue(ctx, "username", claims.Username)
		ctx = context.WithValue(ctx, "role", claims.Role)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// handleLogin handles user login
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get user from database
	user, err := s.userRepo.GetByUsername(r.Context(), req.Username)
	if err != nil {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	// Verify password
	if err := s.authSvc.ComparePassword(user.PasswordHash, req.Password); err != nil {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	// Check if user is active
	if !user.IsActive {
		http.Error(w, "Account is disabled", http.StatusForbidden)
		return
	}

	// Generate JWT token
	token, err := s.authSvc.GenerateToken(user.ID, user.Username, user.Role)
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	// Update last login
	_ = s.userRepo.UpdateLastLogin(r.Context(), user.ID)

	// Send response
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"token":   token,
		"user": map[string]interface{}{
			"id":       user.ID,
			"username": user.Username,
			"email":    user.Email,
			"role":     user.Role,
		},
	})
}

// handleLogout handles user logout
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	// In a production system, you would invalidate the token here
	// For now, we just return success
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

// handleGetCurrentUser returns the currently authenticated user
func (s *Server) handleGetCurrentUser(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(int)
	username := r.Context().Value("username").(string)
	role := r.Context().Value("role").(string)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"id":       userID,
		"username": username,
		"role":     role,
	})
}

// handleGetAircraft returns all visible aircraft from the database
func (s *Server) handleGetAircraft(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(int)
	
	// Get user's active observation point
	obsPoint, err := s.observerRepo.GetActivePoint(r.Context(), userID)
	if err != nil {
		log.Printf("Error getting active observation point: %v", err)
		http.Error(w, "Failed to get observation point", http.StatusInternalServerError)
		return
	}
	
	if obsPoint == nil {
		// No active point - use default from config
		obsPoint = &db.ObservationPoint{
			Latitude:        s.cfg.Observer.Latitude,
			Longitude:       s.cfg.Observer.Longitude,
			ElevationMeters: s.cfg.Observer.Elevation,
		}
	}
	
	// Create observer for calculations
	observer := coordinates.Observer{
		Location: coordinates.Geographic{
			Latitude:  obsPoint.Latitude,
			Longitude: obsPoint.Longitude,
			Altitude:  obsPoint.ElevationMeters,
		},
	}
	
	aircraft, err := s.aircraftRepo.GetVisibleAircraft(r.Context())
	if err != nil {
		log.Printf("Error getting aircraft: %v", err)
		http.Error(w, "Failed to get aircraft", http.StatusInternalServerError)
		return
	}
	
	// Transform aircraft to include observer-relative data
	type AircraftResponse struct {
		ICAO          string    `json:"icao"`
		Callsign      string    `json:"callsign"`
		Latitude      float64   `json:"lat"`
		Longitude     float64   `json:"lon"`
		Altitude      float64   `json:"altitude"`
		GroundSpeed   float64   `json:"speed"`
		Track         float64   `json:"heading"`
		VerticalRate  float64   `json:"verticalRate"`
		LastSeen      time.Time `json:"lastSeen"`
		Distance      float64   `json:"distance"`      // Distance from observer in km
		Azimuth       float64   `json:"azimuth"`       // Azimuth from observer in degrees
		Elevation     float64   `json:"elevation"`     // Elevation angle from observer in degrees
	}
	
	response := make([]AircraftResponse, len(aircraft))
	for i, ac := range aircraft {
		// Calculate observer-relative coordinates
		acLocation := coordinates.Geographic{
			Latitude:  ac.Latitude,
			Longitude: ac.Longitude,
			Altitude:  ac.Altitude * coordinates.FeetToMeters, // Convert feet to meters
		}
		
		// Calculate azimuth (bearing from observer to aircraft)
		azimuth := coordinates.Bearing(observer.Location, acLocation)
		
		// Calculate distance in nautical miles and convert to km
		distanceNM := coordinates.DistanceNauticalMiles(observer.Location, acLocation)
		distanceKm := distanceNM * 1.852
		
		// Calculate elevation angle
		// elevation = arctan((aircraft_altitude - observer_altitude) / ground_distance)
		altitudeDiff := acLocation.Altitude - observer.Location.Altitude
		groundDistanceMeters := distanceKm * 1000.0
		elevationRad := math.Atan2(altitudeDiff, groundDistanceMeters)
		elevationDeg := elevationRad * coordinates.RadiansToDegrees
		
		response[i] = AircraftResponse{
			ICAO:         ac.ICAO,
			Callsign:     ac.Callsign,
			Latitude:     ac.Latitude,
			Longitude:    ac.Longitude,
			Altitude:     ac.Altitude,
			GroundSpeed:  ac.GroundSpeed,
			Track:        ac.Track,
			VerticalRate: ac.VerticalRate,
			LastSeen:     ac.LastSeen,
			Distance:     distanceKm,
			Azimuth:      azimuth,
			Elevation:    elevationDeg,
		}
	}
	
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"aircraft": response,
		"count":    len(response),
		"observer": map[string]interface{}{
			"latitude":        obsPoint.Latitude,
			"longitude":       obsPoint.Longitude,
			"elevationMeters": obsPoint.ElevationMeters,
		},
	})
}

func (s *Server) handleGetAircraftByICAO(w http.ResponseWriter, r *http.Request) {
	icao := chi.URLParam(r, "icao")
	
	aircraft, err := s.aircraftRepo.GetAircraftByICAO(r.Context(), icao)
	if err != nil {
		log.Printf("Error getting aircraft %s: %v", icao, err)
		http.Error(w, "Failed to get aircraft", http.StatusInternalServerError)
		return
	}
	
	if aircraft == nil {
		http.Error(w, "Aircraft not found", http.StatusNotFound)
		return
	}
	
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"icao":         aircraft.ICAO,
		"callsign":     aircraft.Callsign,
		"lat":          aircraft.Latitude,
		"lon":          aircraft.Longitude,
		"altitude":     aircraft.Altitude,
		"speed":        aircraft.GroundSpeed,
		"heading":      aircraft.Track,
		"verticalRate": aircraft.VerticalRate,
		"lastSeen":     aircraft.LastSeen,
	})
}

func (s *Server) handleGetTelescopeConfig(w http.ResponseWriter, r *http.Request) {
	// Get capabilities from telescope
	capabilities, err := s.telescope.GetCapabilities()
	if err != nil {
		log.Printf("Error getting telescope capabilities: %v", err)
		// Return config-only if Alpaca query fails
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"minAltitude": s.cfg.Telescope.MinAltitude,
			"maxAltitude": s.cfg.Telescope.MaxAltitude,
			"mountType":   s.cfg.Telescope.MountType,
			"model":       s.cfg.Telescope.Model,
			"imagingMode": s.cfg.Telescope.ImagingMode,
		})
		return
	}
	
	// Combine config and capabilities
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"minAltitude":      s.cfg.Telescope.MinAltitude,
		"maxAltitude":      s.cfg.Telescope.MaxAltitude,
		"mountType":        s.cfg.Telescope.MountType,
		"model":            s.cfg.Telescope.Model,
		"imagingMode":      s.cfg.Telescope.ImagingMode,
		"description":      capabilities.Description,
		"driverInfo":       capabilities.DriverInfo,
		"interfaceVersion": capabilities.InterfaceVersion,
		"canSetTracking":   capabilities.CanSetTracking,
		"canSlew":          capabilities.CanSlew,
		"canSlewAltAz":     capabilities.CanSlewAltAz,
		"supportedActions": capabilities.SupportedActions,
	})
}

func (s *Server) handleGetTelescopeStatus(w http.ResponseWriter, r *http.Request) {
	status, err := s.telescope.GetStatus()
	if err != nil {
		log.Printf("Error getting telescope status: %v", err)
		http.Error(w, "Failed to get telescope status", http.StatusInternalServerError)
		return
	}
	
	respondJSON(w, http.StatusOK, status)
}

func (s *Server) handleTelescopeSlew(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Altitude float64 `json:"altitude"`
		Azimuth  float64 `json:"azimuth"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	
	// Validate altitude limits
	if req.Altitude < s.cfg.Telescope.MinAltitude || req.Altitude > s.cfg.Telescope.MaxAltitude {
		http.Error(w, fmt.Sprintf("Altitude out of range (%.1f-%.1f°)", s.cfg.Telescope.MinAltitude, s.cfg.Telescope.MaxAltitude), http.StatusBadRequest)
		return
	}
	
	if err := s.telescope.SlewToAltAz(req.Altitude, req.Azimuth); err != nil {
		log.Printf("Error slewing telescope: %v", err)
		http.Error(w, "Failed to slew telescope", http.StatusInternalServerError)
		return
	}
	
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

func (s *Server) handleTelescopeTrack(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(int)
	icao := chi.URLParam(r, "icao")
	
	// Get user's active observation point
	obsPoint, err := s.observerRepo.GetActivePoint(r.Context(), userID)
	if err != nil {
		log.Printf("Error getting active observation point: %v", err)
		http.Error(w, "Failed to get observation point", http.StatusInternalServerError)
		return
	}
	
	if obsPoint == nil {
		// Use default from config
		obsPoint = &db.ObservationPoint{
			Latitude:        s.cfg.Observer.Latitude,
			Longitude:       s.cfg.Observer.Longitude,
			ElevationMeters: s.cfg.Observer.Elevation,
		}
	}
	
	// Get aircraft data
	aircraft, err := s.aircraftRepo.GetAircraftByICAO(r.Context(), icao)
	if err != nil || aircraft == nil {
		log.Printf("Error getting aircraft %s: %v", icao, err)
		http.Error(w, "Aircraft not found", http.StatusNotFound)
		return
	}
	
	// Calculate target coordinates
	observer := coordinates.Observer{
		Location: coordinates.Geographic{
			Latitude:  obsPoint.Latitude,
			Longitude: obsPoint.Longitude,
			Altitude:  obsPoint.ElevationMeters,
		},
	}
	
	acLocation := coordinates.Geographic{
		Latitude:  aircraft.Latitude,
		Longitude: aircraft.Longitude,
		Altitude:  aircraft.Altitude * coordinates.FeetToMeters,
	}
	
	// Calculate azimuth and elevation
	azimuth := coordinates.Bearing(observer.Location, acLocation)
	altitudeDiff := acLocation.Altitude - observer.Location.Altitude
	distanceNM := coordinates.DistanceNauticalMiles(observer.Location, acLocation)
	groundDistanceMeters := distanceNM * 1.852 * 1000.0
	elevationRad := math.Atan2(altitudeDiff, groundDistanceMeters)
	elevation := elevationRad * coordinates.RadiansToDegrees
	
	// Check if target is within limits
	if elevation < s.cfg.Telescope.MinAltitude || elevation > s.cfg.Telescope.MaxAltitude {
		http.Error(w, fmt.Sprintf("Target elevation %.1f° is out of telescope limits (%.1f-%.1f°)", elevation, s.cfg.Telescope.MinAltitude, s.cfg.Telescope.MaxAltitude), http.StatusBadRequest)
		return
	}
	
	// Slew to target
	if err := s.telescope.SlewToAltAz(elevation, azimuth); err != nil {
		log.Printf("Error slewing to aircraft: %v", err)
		http.Error(w, "Failed to slew telescope", http.StatusInternalServerError)
		return
	}
	
	// Enable tracking
	if err := s.telescope.SetTracking(true); err != nil {
		log.Printf("Error enabling tracking: %v", err)
		// Don't fail the request, just log the error
	}
	
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success":   true,
		"icao":      icao,
		"altitude":  elevation,
		"azimuth":   azimuth,
		"callsign":  aircraft.Callsign,
	})
}

func (s *Server) handleTelescopeStop(w http.ResponseWriter, r *http.Request) {
	if err := s.telescope.SetTracking(false); err != nil {
		log.Printf("Error stopping tracking: %v", err)
		http.Error(w, "Failed to stop tracking", http.StatusInternalServerError)
		return
	}
	
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

func (s *Server) handleTelescopeAbort(w http.ResponseWriter, r *http.Request) {
	if err := s.telescope.AbortSlew(); err != nil {
		log.Printf("Error aborting slew: %v", err)
		http.Error(w, "Failed to abort slew", http.StatusInternalServerError)
		return
	}
	
	// Also stop tracking
	if err := s.telescope.SetTracking(false); err != nil {
		log.Printf("Error stopping tracking: %v", err)
		// Don't fail, just log
	}
	
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

func (s *Server) handleGetSystemStatus(w http.ResponseWriter, r *http.Request) {
	// Check telescope connection
	telescopeConnected := false
	telescopeTracking := false
	
	if status, err := s.telescope.GetStatus(); err == nil {
		telescopeConnected = status.Connected
		telescopeTracking = status.Tracking
	}
	
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"telescope": telescopeConnected,
		"adsb":      true, // Assume ADS-B is working if we have aircraft data
		"tracking":  telescopeTracking,
	})
}

// Observation point handlers

func (s *Server) handleGetObservationPoints(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(int)
	
	points, err := s.observerRepo.GetUserPoints(r.Context(), userID)
	if err != nil {
		log.Printf("Error getting observation points: %v", err)
		http.Error(w, "Failed to get observation points", http.StatusInternalServerError)
		return
	}
	
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"points": points,
		"count":  len(points),
	})
}

func (s *Server) handleGetActiveObservationPoint(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(int)
	
	point, err := s.observerRepo.GetActivePoint(r.Context(), userID)
	if err != nil {
		log.Printf("Error getting active observation point: %v", err)
		http.Error(w, "Failed to get active observation point", http.StatusInternalServerError)
		return
	}
	
	if point == nil {
		http.Error(w, "No active observation point found", http.StatusNotFound)
		return
	}
	
	respondJSON(w, http.StatusOK, point)
}

func (s *Server) handleCreateObservationPoint(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(int)
	
	var req struct {
		Name            string  `json:"name"`
		Latitude        float64 `json:"latitude"`
		Longitude       float64 `json:"longitude"`
		ElevationMeters float64 `json:"elevationMeters"`
		IsActive        bool    `json:"isActive"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	
	point := &db.ObservationPoint{
		UserID:          userID,
		Name:            req.Name,
		Latitude:        req.Latitude,
		Longitude:       req.Longitude,
		ElevationMeters: req.ElevationMeters,
		IsActive:        req.IsActive,
	}
	
	if err := s.observerRepo.Create(r.Context(), point); err != nil {
		log.Printf("Error creating observation point: %v", err)
		http.Error(w, "Failed to create observation point", http.StatusInternalServerError)
		return
	}
	
	respondJSON(w, http.StatusCreated, point)
}

func (s *Server) handleUpdateObservationPoint(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(int)
	pointIDStr := chi.URLParam(r, "id")
	
	var pointID int
	if _, err := fmt.Sscanf(pointIDStr, "%d", &pointID); err != nil {
		http.Error(w, "Invalid point ID", http.StatusBadRequest)
		return
	}
	
	var req struct {
		Name            string  `json:"name"`
		Latitude        float64 `json:"latitude"`
		Longitude       float64 `json:"longitude"`
		ElevationMeters float64 `json:"elevationMeters"`
		IsActive        bool    `json:"isActive"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	
	point := &db.ObservationPoint{
		ID:              pointID,
		UserID:          userID,
		Name:            req.Name,
		Latitude:        req.Latitude,
		Longitude:       req.Longitude,
		ElevationMeters: req.ElevationMeters,
		IsActive:        req.IsActive,
	}
	
	if err := s.observerRepo.Update(r.Context(), point); err != nil {
		log.Printf("Error updating observation point: %v", err)
		http.Error(w, "Failed to update observation point", http.StatusInternalServerError)
		return
	}
	
	respondJSON(w, http.StatusOK, point)
}

func (s *Server) handleDeleteObservationPoint(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(int)
	pointIDStr := chi.URLParam(r, "id")
	
	var pointID int
	if _, err := fmt.Sscanf(pointIDStr, "%d", &pointID); err != nil {
		http.Error(w, "Invalid point ID", http.StatusBadRequest)
		return
	}
	
	if err := s.observerRepo.Delete(r.Context(), pointID, userID); err != nil {
		log.Printf("Error deleting observation point: %v", err)
		http.Error(w, "Failed to delete observation point", http.StatusInternalServerError)
		return
	}
	
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

func (s *Server) handleActivateObservationPoint(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(int)
	pointIDStr := chi.URLParam(r, "id")
	
	var pointID int
	if _, err := fmt.Sscanf(pointIDStr, "%d", &pointID); err != nil {
		http.Error(w, "Invalid point ID", http.StatusBadRequest)
		return
	}
	
	if err := s.observerRepo.SetActive(r.Context(), pointID, userID); err != nil {
		log.Printf("Error activating observation point: %v", err)
		http.Error(w, "Failed to activate observation point", http.StatusInternalServerError)
		return
	}
	
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
	})
}

// Helper functions

func connectDatabase(cfg *config.Config) (*sql.DB, error) {
	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.Username,
		cfg.Database.Password,
		cfg.Database.Database,
		cfg.Database.SSLMode,
	)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	log.Println("✅ Connected to database")
	return db, nil
}

func runMigrations(db *sql.DB) error {
	// For now, just create a default admin user if it doesn't exist
	_, err := db.Exec(`
		INSERT INTO users (username, email, password_hash, role, is_active, email_verified)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (username) DO NOTHING
	`, "admin", "admin@ads-bscope.local",
		"$2a$10$YourHashedPasswordHere", // This will need to be properly hashed
		"admin", true, true)
	
	return err
}

func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
