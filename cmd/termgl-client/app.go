package main

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/unklstewy/ads-bscope/internal/db"
	"github.com/unklstewy/ads-bscope/pkg/alpaca"
	"github.com/unklstewy/ads-bscope/pkg/config"
	"github.com/unklstewy/ads-bscope/pkg/coordinates"
)

// ViewMode represents the current active view
type ViewMode int

const (
	ViewModeSky ViewMode = iota
	ViewModeRadar
	ViewModeConfig
	ViewModeHelp
)

// TrackingMode represents the telescope tracking state
type TrackingMode int

const (
	TrackingModeIdle TrackingMode = iota
	TrackingModeIntercept  // Initial slew to aircraft
	TrackingModeContinuous // MoveAxis tracking
)

// Position threshold for considering slew complete (degrees)
const positionThreshold = 0.1

// AppConfig holds the application configuration
type AppConfig struct {
	Config             *config.Config
	ConfigPath         string
	Database           *db.DB
	AircraftRepository *db.AircraftRepository
	FlightPlanRepo     *db.FlightPlanRepository
	Observer           coordinates.Observer
}

// App represents the main application
type App struct {
	// Configuration
	config     *config.Config
	configPath string
	observer   coordinates.Observer

	// Data sources
	database       *db.DB
	aircraftRepo   *db.AircraftRepository
	flightPlanRepo *db.FlightPlanRepository

	// UI components
	tviewApp     *tview.Application
	mainView     tview.Primitive
	telemetry    *tview.TextView
	controls     *tview.TextView
	logManager   *LogManager
	rootLayout   *tview.Flex
	currentView  ViewMode

	// Telescope
	telescope          *alpaca.Client
	telescopeConnected bool
	telescopeAlt       float64
	telescopeAz        float64
	telescopeSlewing   bool
	telescopeParked    bool
	trackingMode       TrackingMode // intercept vs continuous
	targetAlt          float64      // target altitude for threshold checking
	targetAz           float64      // target azimuth for threshold checking

	// Focuser
	focuser          *alpaca.FocuserClient
	focuserConnected bool
	focuserPosition  int

	// Filter Wheel
	filterWheel          *alpaca.FilterWheelClient
	filterWheelConnected bool
	filterPosition       alpaca.FilterPosition
	filterName           string

	// Solar Safety
	sunPosition          coordinates.SunPosition
	solarSeparation      float64
	solarSafetyZone      coordinates.SolarSafetyZone
	solarDarkFilterActive bool

	// Switch (Dew Heater)
	switchClient       *alpaca.SwitchClient
	switchConnected    bool
	dewHeaterEnabled   bool

	// State
	aircraft      []AircraftView
	selectedIndex int
	tracking      bool
	trackICAO     string
	showTrails    bool
	showConstell  bool
	zoom          float64
	minAlt        float64
	maxAlt        float64

	// Synchronization
	mu          sync.RWMutex
	updateTimer *time.Ticker
	stopChan    chan struct{}
}

// AircraftView holds display information for an aircraft
type AircraftView struct {
	ICAO       string
	Callsign   string
	Altitude   float64
	Speed      float64
	Heading    float64
	Latitude   float64
	Longitude  float64
	HorizCoord coordinates.HorizontalCoordinates
	Age        time.Duration
	Selected   bool
	Tracking   bool
}

// NewApp creates a new application instance
func NewApp(cfg *AppConfig) *App {
	// Get altitude limits from config
	minAlt, maxAlt := cfg.Config.Telescope.GetAltitudeLimits()

	app := &App{
		config:         cfg.Config,
		configPath:     cfg.ConfigPath,
		observer:       cfg.Observer,
		database:       cfg.Database,
		aircraftRepo:   cfg.AircraftRepository,
		flightPlanRepo: cfg.FlightPlanRepo,
		aircraft:       make([]AircraftView, 0),
		selectedIndex:  0,
		tracking:       false,
		showTrails:     false,
		showConstell:   false,
		zoom:           1.0,
		minAlt:         minAlt,
		maxAlt:         maxAlt,
		currentView:    ViewModeSky,
		stopChan:       make(chan struct{}),
		telescope:      alpaca.NewClient(cfg.Config.Telescope),
	}

	app.setupUI()
	return app
}

// setupUI initializes the user interface
func (a *App) setupUI() {
	a.tviewApp = tview.NewApplication()

	// Create panels
	a.createMainView()
	a.createTelemetryPanel()
	a.createControlsPanel()
	a.createLogsPanel()

	// Create layout
	a.createLayout()

	// Setup keyboard handlers
	a.tviewApp.SetInputCapture(a.handleKeyboard)
}

// createMainView creates the main view (sky or radar)
func (a *App) createMainView() {
	// Create the sky view with geometric rendering
	skyView := NewSkyView(a)
	a.mainView = skyView
}

// createTelemetryPanel creates the telemetry info panel
func (a *App) createTelemetryPanel() {
	a.telemetry = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(false)
	a.telemetry.SetBorder(true).SetTitle(" Telemetry ")

	// Initial content
	a.updateTelemetry()
}

// createControlsPanel creates the controls/shortcuts panel
func (a *App) createControlsPanel() {
	a.controls = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(false)
	a.controls.SetBorder(true).SetTitle(" Controls ")

	// Set static controls text
	controlsText := `[yellow]NAVIGATION[-]
  [white]↑/↓, j/k[-]  Select
  [white]PgUp/PgDn[-] Scroll

[yellow]ACTIONS[-]
  [white]ENTER[-]     Track
  [white]SPACE[-]     Stop
  [white]t[-]         Trails
  [white]c[-]         Constellations

[yellow]VIEWS[-]
  [white]s[-]         Sky view
  [white]r[-]         Radar view
  [white]m[-]         Config
  [white]?[-]         Help

[yellow]ZOOM[-]
  [white]+/-[-]       Zoom
  [white]0[-]         Reset

[yellow]CONTROL[-]
  [white]q[-]         Quit`

	a.controls.SetText(controlsText)
}

// createLogsPanel creates the log viewer panel
func (a *App) createLogsPanel() {
	a.logManager = NewLogManager(100)
	a.logManager.Info("Application started")

	// Attempt telescope connection
	go a.connectTelescope()
}

// createLayout creates the main layout with 4 panels
func (a *App) createLayout() {
	// Right sidebar with 3 panels
	sidebar := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(a.telemetry, 0, 4, false).        // 40% of sidebar
		AddItem(a.controls, 0, 3, false).         // 30% of sidebar
		AddItem(a.logManager.GetView(), 0, 3, false) // 30% of sidebar

	// Main layout: main view (70%) + sidebar (30%)
	a.rootLayout = tview.NewFlex().
		SetDirection(tview.FlexColumn).
		AddItem(a.mainView, 0, 7, true).    // 70% width, focusable
		AddItem(sidebar, 0, 3, false)       // 30% width

	a.tviewApp.SetRoot(a.rootLayout, true)
}

// updateTelemetry updates the telemetry panel content
func (a *App) updateTelemetry() {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var text string

	// Aircraft section
	text += fmt.Sprintf("[yellow]AIRCRAFT:[-] [white][%d][-]\n", len(a.aircraft))
	if len(a.aircraft) == 0 {
		text += "[gray]No aircraft available[-]\n"
		text += "[gray]Start collector to populate data[-]\n"
	} else if a.selectedIndex >= 0 && a.selectedIndex < len(a.aircraft) {
		ac := a.aircraft[a.selectedIndex]
		text += fmt.Sprintf("[white]%d. %s (-) %s[-]\n", a.selectedIndex+1, ac.Callsign, ac.ICAO)
		text += fmt.Sprintf("[gray]Alt:[-]  [white]%.0f ft[-]  [gray]Spd:[-] [white]%.0f kts[-]\n", ac.Altitude, ac.Speed)
		text += fmt.Sprintf("[gray]Hdg:[-]  [white]%.0f°[-]     [gray]Age:[-] [white]%.1fs[-]\n", ac.Heading, ac.Age.Seconds())
		text += fmt.Sprintf("[gray]Az:[-]   [white]%.1f°[-]  [gray]Alt:[-] [white]%.1f°[-]\n", ac.HorizCoord.Azimuth, ac.HorizCoord.Altitude)
		text += fmt.Sprintf("[gray]Pos:[-]  [white]%.4f°, %.4f°[-]\n", ac.Latitude, ac.Longitude)
	} else {
		text += "[gray]No aircraft selected[-]\n"
	}

	text += "\n"

	// Telescope section
	if a.telescopeConnected {
		text += "[yellow]TELESCOPE:[-] [green]Connected[-]\n"
		text += fmt.Sprintf("[gray]Pos:[-]  [white]Az %.1f° Alt %.1f°[-]\n", a.telescopeAz, a.telescopeAlt)
		if a.telescopeSlewing {
			text += "[gray]Mode:[-] [yellow]SLEWING[-]\n"
		} else if a.tracking {
			text += fmt.Sprintf("[gray]Mode:[-] [green]TRACKING %s[-]\n", a.trackICAO)
		} else {
			text += "[gray]Mode:[-] [white]IDLE[-]\n"
		}
	} else {
		text += "[yellow]TELESCOPE:[-] [red]Not Connected[-]\n"
		text += "[gray]Pos:[-]  [white]---[-]\n"
		text += "[gray]Mode:[-] [white]IDLE[-]\n"
	}

	text += "\n"

	// Observer section
	text += fmt.Sprintf("[yellow]OBSERVER:[-] [white]%.4f°, %.4f°[-]\n", 
		a.observer.Location.Latitude, a.observer.Location.Longitude)
	text += fmt.Sprintf("[gray]Time:[-] [white]%s[-]\n", time.Now().Format("15:04:05"))
	text += fmt.Sprintf("[gray]Aircraft:[-] [white]%d visible[-]\n", len(a.aircraft))
	text += fmt.Sprintf("[gray]View:[-] [white]%s[-] [gray]Zoom:[-] [white]%.1fx[-]\n", 
		a.getViewName(), a.zoom)

	a.telemetry.SetText(text)
}

// getViewName returns the current view mode name
func (a *App) getViewName() string {
	switch a.currentView {
	case ViewModeSky:
		return "Sky"
	case ViewModeRadar:
		return "Radar"
	case ViewModeConfig:
		return "Config"
	case ViewModeHelp:
		return "Help"
	default:
		return "Unknown"
	}
}

// addLog adds a log message to the log panel (legacy wrapper)
func (a *App) addLog(level, message string) {
	a.logManager.AddLog(LogLevel(level), "%s", message)
}

// handleKeyboard handles keyboard input
func (a *App) handleKeyboard(event *tcell.EventKey) *tcell.EventKey {
	key := event.Key()
	rune := event.Rune()

	switch {
	// Quit
	case key == tcell.KeyEscape || rune == 'q' || rune == 'Q' || key == tcell.KeyCtrlC:
		a.Stop()
		return nil

	// Navigation
	case key == tcell.KeyUp || rune == 'k':
		if len(a.aircraft) > 0 {
			a.selectPrevious()
		}
		return nil
	case key == tcell.KeyDown || rune == 'j':
		if len(a.aircraft) > 0 {
			a.selectNext()
		}
		return nil

	// Actions
	case key == tcell.KeyEnter:
		a.startTracking()
		return nil
	case rune == ' ':
		a.stopTracking()
		return nil
	case rune == 't':
		a.toggleTrails()
		return nil
	case rune == 'c':
		a.toggleConstellations()
		return nil

	// Views
	case rune == 's':
		a.switchView(ViewModeSky)
		return nil
	case rune == 'r':
		a.switchView(ViewModeRadar)
		return nil
	case rune == 'm':
		a.switchView(ViewModeConfig)
		return nil
	case rune == '?':
		a.switchView(ViewModeHelp)
		return nil

	// Zoom
	case rune == '+' || rune == '=':
		a.zoomIn()
		return nil
	case rune == '-':
		a.zoomOut()
		return nil
	case rune == '0':
		a.resetZoom()
		return nil
	}

	return event
}

// selectPrevious selects the previous aircraft
func (a *App) selectPrevious() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if len(a.aircraft) == 0 {
		return
	}

	a.selectedIndex--
	if a.selectedIndex < 0 {
		a.selectedIndex = len(a.aircraft) - 1
	}

	a.addLog("DEBUG", fmt.Sprintf("Selected aircraft %d/%d", a.selectedIndex+1, len(a.aircraft)))
	a.tviewApp.QueueUpdateDraw(func() {
		a.updateTelemetry()
	})
}

// selectNext selects the next aircraft
func (a *App) selectNext() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if len(a.aircraft) == 0 {
		return
	}

	a.selectedIndex++
	if a.selectedIndex >= len(a.aircraft) {
		a.selectedIndex = 0
	}

	a.addLog("DEBUG", fmt.Sprintf("Selected aircraft %d/%d", a.selectedIndex+1, len(a.aircraft)))
	a.tviewApp.QueueUpdateDraw(func() {
		a.updateTelemetry()
	})
}

// startTracking starts tracking the selected aircraft
func (a *App) startTracking() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.selectedIndex < 0 || a.selectedIndex >= len(a.aircraft) {
		a.addLog("WARN", "No aircraft selected")
		return
	}

	if !a.telescopeConnected {
		a.addLog("ERROR", "Telescope not connected")
		return
	}

	if a.telescopeParked {
		a.addLog("ERROR", "Telescope is parked. Cannot track.")
		return
	}

	ac := a.aircraft[a.selectedIndex]

	// CRITICAL: Solar safety check
	if a.config.Telescope.SolarSafetyEnabled && !a.checkSolarSafety(ac) {
		return
	}

	// Check altitude limits
	alt := ac.HorizCoord.Altitude
	if alt < a.minAlt || alt > a.maxAlt {
		a.addLog("ERROR", fmt.Sprintf("Aircraft altitude %.1f° out of range (%.0f°-%.0f°)", alt, a.minAlt, a.maxAlt))
		return
	}

	a.tracking = true
	a.trackICAO = ac.ICAO
	a.trackingMode = TrackingModeIntercept
	a.targetAlt = ac.HorizCoord.Altitude
	a.targetAz = ac.HorizCoord.Azimuth

	a.addLog("INFO", fmt.Sprintf("Intercepting %s (%s) at Az %.1f° Alt %.1f°", ac.Callsign, ac.ICAO, ac.HorizCoord.Azimuth, ac.HorizCoord.Altitude))

	// Initial intercept slew
	go a.interceptAircraft(ac)
}

// stopTracking stops tracking
func (a *App) stopTracking() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.tracking {
		return
	}

	a.tracking = false
	a.trackICAO = ""
	a.trackingMode = TrackingModeIdle

	a.addLog("INFO", "Tracking stopped")

	// Stop all axis movement
	if a.telescopeConnected {
		go func() {
			if err := a.telescope.StopAxes(); err != nil {
				a.addLog("ERROR", fmt.Sprintf("Failed to stop axes: %v", err))
			} else if a.telescopeSlewing {
				// Also abort any pending slew
				if err := a.telescope.AbortSlew(); err != nil {
					a.addLog("ERROR", fmt.Sprintf("Failed to abort slew: %v", err))
				}
			}
		}()
	}
}

// toggleTrails toggles trail display
func (a *App) toggleTrails() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.showTrails = !a.showTrails
	a.addLog("INFO", fmt.Sprintf("Trails: %v", a.showTrails))
}

// toggleConstellations toggles constellation display
func (a *App) toggleConstellations() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.showConstell = !a.showConstell
	a.addLog("INFO", fmt.Sprintf("Constellations: %v", a.showConstell))
}

// switchView switches to a different view mode
func (a *App) switchView(mode ViewMode) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.currentView = mode
	a.addLog("INFO", fmt.Sprintf("Switched to %s view", a.getViewName()))

	a.tviewApp.QueueUpdateDraw(func() {
		a.updateTelemetry()
	})
}

// zoomIn increases zoom level
func (a *App) zoomIn() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.zoom = a.zoom * 1.2
	if a.zoom > 5.0 {
		a.zoom = 5.0
	}

	a.addLog("DEBUG", fmt.Sprintf("Zoom: %.1fx", a.zoom))
	a.tviewApp.QueueUpdateDraw(func() {
		a.updateTelemetry()
	})
}

// zoomOut decreases zoom level
func (a *App) zoomOut() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.zoom = a.zoom / 1.2
	if a.zoom < 0.5 {
		a.zoom = 0.5
	}

	a.addLog("DEBUG", fmt.Sprintf("Zoom: %.1fx", a.zoom))
	a.tviewApp.QueueUpdateDraw(func() {
		a.updateTelemetry()
	})
}

// resetZoom resets zoom to default
func (a *App) resetZoom() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.zoom = 1.0

	a.addLog("DEBUG", "Zoom reset")
	a.tviewApp.QueueUpdateDraw(func() {
		a.updateTelemetry()
	})
}

// Run starts the application
func (a *App) Run() error {
	// Start data update goroutine
	a.updateTimer = time.NewTicker(2 * time.Second)
	go a.updateLoop()

	// Start telescope position polling if connected
	if a.telescopeConnected {
		go a.telescopeUpdateLoop()
	}

	// Start solar position monitoring if safety enabled
	if a.config.Telescope.SolarSafetyEnabled {
		go a.solarSafetyLoop()
	}

	// Run the tview application
	return a.tviewApp.Run()
}

// updateLoop periodically updates aircraft data
func (a *App) updateLoop() {
	// Initial update
	a.fetchAircraftData()

	for {
		select {
		case <-a.updateTimer.C:
			a.fetchAircraftData()
			// If tracking, update telescope position
			if a.tracking && a.telescopeConnected {
				go a.updateTrackingSlew()
			}
		case <-a.stopChan:
			return
		}
	}
}

// fetchAircraftData fetches aircraft data from the database
func (a *App) fetchAircraftData() {
	ctx := context.Background()

	// Get visible aircraft from repository (all visible, not just trackable)
	aircraft, err := a.aircraftRepo.GetVisibleAircraft(ctx)
	if err != nil {
		a.addLog("ERROR", fmt.Sprintf("Failed to fetch aircraft: %v", err))
		return
	}

	// Convert to display format
	a.mu.Lock()
	oldCount := len(a.aircraft)
	a.aircraft = make([]AircraftView, 0, len(aircraft))

	for _, ac := range aircraft {
		// Calculate horizontal coordinates
		horiz := coordinates.GeographicToHorizontal(
			coordinates.Geographic{
				Latitude:  ac.Latitude,
				Longitude: ac.Longitude,
				Altitude:  ac.Altitude,
			},
			a.observer,
			ac.LastSeen,
		)

		// Calculate age
		age := time.Since(ac.LastSeen)

		// Create view
		view := AircraftView{
			ICAO:       ac.ICAO,
			Callsign:   ac.Callsign,
			Altitude:   ac.Altitude,
			Speed:      ac.GroundSpeed,
			Heading:    ac.Track,
			Latitude:   ac.Latitude,
			Longitude:  ac.Longitude,
			HorizCoord: horiz,
			Age:        age,
			Selected:   false,
			Tracking:   a.tracking && ac.ICAO == a.trackICAO,
		}

		a.aircraft = append(a.aircraft, view)
	}

	// Adjust selection if aircraft list changed
	if a.selectedIndex >= len(a.aircraft) {
		if len(a.aircraft) > 0 {
			a.selectedIndex = len(a.aircraft) - 1
		} else {
			a.selectedIndex = 0
		}
	}

	// Mark selected aircraft
	if a.selectedIndex >= 0 && a.selectedIndex < len(a.aircraft) {
		a.aircraft[a.selectedIndex].Selected = true
	}

	newCount := len(a.aircraft)
	a.mu.Unlock()

	// Log aircraft count changes
	if oldCount != newCount {
		a.addLog("INFO", fmt.Sprintf("Aircraft count: %d", newCount))
	}

	// Update UI
	a.tviewApp.QueueUpdateDraw(func() {
		a.updateTelemetry()
	})
}

// Stop stops the application
func (a *App) Stop() {
	a.addLog("INFO", "Shutting down...")
	
	// Disconnect switch
	if a.switchConnected {
		if err := a.switchClient.Disconnect(); err != nil {
			a.addLog("ERROR", fmt.Sprintf("Failed to disconnect switch: %v", err))
		}
	}

	// Disconnect filter wheel
	if a.filterWheelConnected {
		if err := a.filterWheel.Disconnect(); err != nil {
			a.addLog("ERROR", fmt.Sprintf("Failed to disconnect filter wheel: %v", err))
		}
	}

	// Disconnect focuser
	if a.focuserConnected {
		if err := a.focuser.Disconnect(); err != nil {
			a.addLog("ERROR", fmt.Sprintf("Failed to disconnect focuser: %v", err))
		}
	}

	// Disconnect telescope
	if a.telescopeConnected {
		if err := a.telescope.Disconnect(); err != nil {
			a.addLog("ERROR", fmt.Sprintf("Failed to disconnect telescope: %v", err))
		}
	}

	// Stop update loop
	if a.updateTimer != nil {
		a.updateTimer.Stop()
	}
	close(a.stopChan)

	// Stop tview application
	a.tviewApp.Stop()
}

// connectTelescope attempts to connect to the telescope
func (a *App) connectTelescope() {
	a.addLog("INFO", "Connecting to telescope...")

	err := a.telescope.Connect()
	if err != nil {
		a.addLog("ERROR", fmt.Sprintf("Failed to connect to telescope: %v", err))
		a.addLog("WARN", "Tracking features disabled. Start ASCOM Alpaca Simulator to enable.")
		return
	}

	a.mu.Lock()
	a.telescopeConnected = true
	a.mu.Unlock()

	a.addLog("INFO", "Telescope connected successfully")

	// Check if parked
	atPark, err := a.telescope.GetAtPark()
	if err != nil {
		a.addLog("WARN", fmt.Sprintf("Failed to check park status: %v", err))
	} else if atPark {
		a.mu.Lock()
		a.telescopeParked = true
		a.mu.Unlock()
		a.addLog("WARN", "Telescope is parked. Unpark before tracking.")
		
		// Auto-unpark
		if err := a.telescope.Unpark(); err != nil {
			a.addLog("ERROR", fmt.Sprintf("Failed to unpark: %v", err))
		} else {
			a.addLog("INFO", "Telescope unparked")
			a.mu.Lock()
			a.telescopeParked = false
			a.mu.Unlock()
		}
	}

	// Enable tracking (for equatorial mounts, no effect on alt-az)
	if err := a.telescope.SetTracking(true); err != nil {
		a.addLog("WARN", fmt.Sprintf("Failed to enable tracking: %v", err))
	} else {
		a.addLog("DEBUG", "Tracking enabled")
	}

	// Get initial position
	a.updateTelescopePosition()

	// Initialize focuser for infinity focus (aircraft tracking)
	go a.initializeFocuser()
}

// initializeFocuser connects and sets focuser to infinity
func (a *App) initializeFocuser() {
	// Create focuser client
	a.focuser = alpaca.NewFocuserClient(a.telescope)

	// Connect to focuser
	a.addLog("INFO", "Connecting to focuser...")
	if err := a.focuser.Connect(); err != nil {
		a.addLog("WARN", fmt.Sprintf("Failed to connect to focuser: %v", err))
		a.addLog("INFO", "Focuser unavailable - manual focus required")
		return
	}

	a.mu.Lock()
	a.focuserConnected = true
	a.mu.Unlock()

	a.addLog("INFO", "Focuser connected")

	// Get current position
	pos, err := a.focuser.GetPosition()
	if err != nil {
		a.addLog("WARN", fmt.Sprintf("Failed to get focuser position: %v", err))
		return
	}

	a.mu.Lock()
	a.focuserPosition = pos
	a.mu.Unlock()

	a.addLog("INFO", fmt.Sprintf("Focuser at position %d steps", pos))

	// Auto-move to infinity if configured
	if a.config.Telescope.AutoFocusOnStartup {
		target := a.config.Telescope.InfinityFocusPosition
		if target > 0 {
			a.addLog("INFO", fmt.Sprintf("Moving focuser to infinity position (%d steps)...", target))
			if err := a.focuser.MoveToInfinity(); err != nil {
				a.addLog("ERROR", fmt.Sprintf("Failed to move to infinity: %v", err))
			} else {
				final, _ := a.focuser.GetPosition()
				a.mu.Lock()
				a.focuserPosition = final
				a.mu.Unlock()
				a.addLog("INFO", fmt.Sprintf("Focuser at infinity (%d steps)", final))
			}
		}
	}

	// Initialize filter wheel
	go a.initializeFilterWheel()

	// Initialize dew heater
	go a.initializeDewHeater()
}

// initializeFilterWheel connects and sets filter to UV/IR Cut for tracking
func (a *App) initializeFilterWheel() {
	// Create filter wheel client
	a.filterWheel = alpaca.NewFilterWheelClient(a.telescope)

	// Connect to filter wheel
	a.addLog("INFO", "Connecting to filter wheel...")
	if err := a.filterWheel.Connect(); err != nil {
		a.addLog("WARN", fmt.Sprintf("Failed to connect to filter wheel: %v", err))
		a.addLog("INFO", "Filter wheel unavailable - manual filter selection required")
		return
	}

	a.mu.Lock()
	a.filterWheelConnected = true
	a.mu.Unlock()

	a.addLog("INFO", "Filter wheel connected")

	// Get current filter
	pos, name, err := a.filterWheel.GetCurrentFilter()
	if err != nil {
		a.addLog("WARN", fmt.Sprintf("Failed to get filter position: %v", err))
		return
	}

	a.mu.Lock()
	a.filterPosition = pos
	a.filterName = name
	a.mu.Unlock()

	a.addLog("INFO", fmt.Sprintf("Filter wheel at: %s", name))

	// Set to tracking filter (UV/IR Cut) if not already
	if pos != alpaca.FilterUVIRCut {
		a.addLog("INFO", "Setting tracking filter (UV/IR Cut)...")
		if err := a.filterWheel.SetTrackingFilter(); err != nil {
			a.addLog("ERROR", fmt.Sprintf("Failed to set tracking filter: %v", err))
		} else {
			a.mu.Lock()
			a.filterPosition = alpaca.FilterUVIRCut
			a.filterName = alpaca.FilterNames[alpaca.FilterUVIRCut]
			a.mu.Unlock()
			a.addLog("INFO", "Tracking filter set (UV/IR Cut)")
		}
	}
}

// solarSafetyLoop continuously monitors sun position and enforces safety
func (a *App) solarSafetyLoop() {
	ticker := time.NewTicker(10 * time.Second) // Update every 10 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Calculate current sun position
			sunPos := coordinates.CalculateSunPosition(a.observer, time.Now())

			a.mu.Lock()
			a.sunPosition = sunPos

			// If tracking, check solar proximity
			if a.tracking && a.selectedIndex >= 0 && a.selectedIndex < len(a.aircraft) {
				ac := a.aircraft[a.selectedIndex]
				separation := sunPos.AngularSeparation(ac.HorizCoord.Altitude, ac.HorizCoord.Azimuth)
				a.solarSeparation = separation
				a.solarSafetyZone = coordinates.GetSafetyZone(separation)

				// Check if we need to engage dark filter
				if a.config.Telescope.AutoDarkFilterOnSolarProximity && 
				   a.filterWheelConnected && 
				   !a.solarDarkFilterActive {
					
					// Engage dark filter at WARNING level (< 10°)
					if a.solarSafetyZone >= coordinates.SafeZoneWarning {
						a.mu.Unlock()
						a.addLog("WARN", fmt.Sprintf("Solar proximity %.1f° - engaging dark filter", separation))
						go a.engageSolarDarkFilter()
						continue
					}
				}

				// CRITICAL: Stop tracking if too close to sun
				if separation < a.config.Telescope.MinSolarSeparation {
					a.mu.Unlock()
					a.addLog("ERROR", fmt.Sprintf("CRITICAL: Aircraft %.1f° from sun - EMERGENCY STOP", separation))
					a.stopTracking()
					continue
				}
			}
			a.mu.Unlock()

		case <-a.stopChan:
			return
		}
	}
}

// checkSolarSafety validates that tracking the given aircraft is safe from solar damage.
// Returns false and logs errors if tracking would be dangerous.
func (a *App) checkSolarSafety(ac AircraftView) bool {
	// Calculate sun position
	sunPos := coordinates.CalculateSunPosition(a.observer, time.Now())

	// Only check if sun is above horizon
	if !sunPos.IsSunAboveHorizon() {
		return true // Sun below horizon - safe
	}

	// Calculate angular separation
	separation := sunPos.AngularSeparation(ac.HorizCoord.Altitude, ac.HorizCoord.Azimuth)
	safetyZone := coordinates.GetSafetyZone(separation)

	a.addLog("INFO", fmt.Sprintf("Solar check: %.1f° separation (Sun: Az %.1f° Alt %.1f°)", 
		separation, sunPos.Azimuth, sunPos.Altitude))

	// CRITICAL: Check against configured minimum
	if separation < a.config.Telescope.MinSolarSeparation {
		if !a.config.Telescope.SolarFilterInstalled {
			a.addLog("ERROR", "═══════════════════════════════════════════════════")
			a.addLog("ERROR", "  ⚠️  SOLAR DANGER - TRACKING BLOCKED ⚠️")
			a.addLog("ERROR", fmt.Sprintf("  Aircraft is %.1f° from sun (min: %.0f°)", separation, a.config.Telescope.MinSolarSeparation))
			a.addLog("ERROR", "  RISK: PERMANENT OPTICS DAMAGE")
			a.addLog("ERROR", "")
			a.addLog("ERROR", "  To track near sun, you MUST:")
			a.addLog("ERROR", "  1. Attach physical solar filter to scope")
			a.addLog("ERROR", "  2. Set filter wheel to Solar (Slot 3)")
			a.addLog("ERROR", "  3. Set solar_filter_installed=true in config")
			a.addLog("ERROR", "═══════════════════════════════════════════════════")
			return false
		} else {
			// Solar filter installed - verify filter wheel position
			if a.filterWheelConnected && a.filterPosition != alpaca.FilterSolar {
				a.addLog("ERROR", "═══════════════════════════════════════════════════")
				a.addLog("ERROR", "  ⚠️  SOLAR FILTER MISMATCH ⚠️")
				a.addLog("ERROR", fmt.Sprintf("  Current filter: %s", a.filterName))
				a.addLog("ERROR", "  Required: Solar (Slot 3)")
				a.addLog("ERROR", "")
				a.addLog("ERROR", "  Manually set filter wheel to Solar filter")
				a.addLog("ERROR", "  or increase min_solar_separation in config")
				a.addLog("ERROR", "═══════════════════════════════════════════════════")
				return false
			}
			a.addLog("WARN", fmt.Sprintf("Solar filter active - tracking %.1f° from sun", separation))
		}
	}

	// Warnings for proximity zones
	if safetyZone == coordinates.SafeZoneCaution {
		a.addLog("WARN", fmt.Sprintf("CAUTION: Aircraft %.1f° from sun - monitor carefully", separation))
	} else if safetyZone == coordinates.SafeZoneWarning {
		a.addLog("WARN", fmt.Sprintf("WARNING: Aircraft %.1f° from sun - consider aborting", separation))
	}

	return true
}

// engageSolarDarkFilter activates the dark filter for solar proximity protection
func (a *App) engageSolarDarkFilter() {
	if !a.filterWheelConnected {
		return
	}

	a.addLog("WARN", "Engaging dark filter for solar protection...")

	if err := a.filterWheel.SetDarkFilter(); err != nil {
		a.addLog("ERROR", fmt.Sprintf("Failed to engage dark filter: %v", err))
		return
	}

	a.mu.Lock()
	a.filterPosition = alpaca.FilterDarkField
	a.filterName = alpaca.FilterNames[alpaca.FilterDarkField]
	a.solarDarkFilterActive = true
	a.mu.Unlock()

	a.addLog("WARN", "Dark filter engaged - optics protected")
	a.addLog("INFO", "Tracking suspended - move away from sun or abort")

	// Stop tracking for safety
	a.stopTracking()
}

// initializeDewHeater connects and optionally enables the dew heater
func (a *App) initializeDewHeater() {
	// Create switch client
	a.switchClient = alpaca.NewSwitchClient(a.telescope)

	// Connect to switch
	a.addLog("INFO", "Connecting to switch (dew heater)...")
	if err := a.switchClient.Connect(); err != nil {
		a.addLog("WARN", fmt.Sprintf("Failed to connect to switch: %v", err))
		a.addLog("INFO", "Dew heater unavailable - manual control required")
		return
	}

	a.mu.Lock()
	a.switchConnected = true
	a.mu.Unlock()

	a.addLog("INFO", "Switch connected (dew heater)")

	// Get current dew heater state
	state, err := a.switchClient.GetDewHeater()
	if err != nil {
		a.addLog("WARN", fmt.Sprintf("Failed to get dew heater state: %v", err))
		return
	}

	a.mu.Lock()
	a.dewHeaterEnabled = state
	a.mu.Unlock()

	if state {
		a.addLog("INFO", "Dew heater: ON")
	} else {
		a.addLog("INFO", "Dew heater: OFF")
	}

	// Auto-enable dew heater if configured
	if a.config.Telescope.EnableDewHeaterOnStartup && !state {
		a.addLog("INFO", "Enabling dew heater...")
		if err := a.switchClient.EnableDewHeater(); err != nil {
			a.addLog("ERROR", fmt.Sprintf("Failed to enable dew heater: %v", err))
		} else {
			a.mu.Lock()
			a.dewHeaterEnabled = true
			a.mu.Unlock()
			a.addLog("INFO", "Dew heater enabled")
		}
	}
}
// telescopeUpdateLoop periodically updates telescope position
func (a *App) telescopeUpdateLoop() {
	ticker := time.NewTicker(500 * time.Millisecond) // 2Hz update rate
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			a.updateTelescopePosition()
		case <-a.stopChan:
			return
		}
	}
}

// updateTelescopePosition fetches current telescope position
func (a *App) updateTelescopePosition() {
	if !a.telescopeConnected {
		return
	}

	// Get altitude and azimuth
	alt, err := a.telescope.GetAltitude()
	if err != nil {
		a.addLog("ERROR", fmt.Sprintf("Failed to get telescope altitude: %v", err))
		return
	}

	az, err := a.telescope.GetAzimuth()
	if err != nil {
		a.addLog("ERROR", fmt.Sprintf("Failed to get telescope azimuth: %v", err))
		return
	}

	// Get slewing status
	slewing, err := a.telescope.IsSlewing()
	if err != nil {
		a.addLog("ERROR", fmt.Sprintf("Failed to get slewing status: %v", err))
		return
	}

	a.mu.Lock()
	a.telescopeAlt = alt
	a.telescopeAz = az
	a.telescopeSlewing = slewing
	a.mu.Unlock()

	// Update UI
	a.tviewApp.QueueUpdateDraw(func() {
		a.updateTelemetry()
	})
}

// interceptAircraft performs initial slew to aircraft position
func (a *App) interceptAircraft(ac AircraftView) {
	if !a.telescopeConnected {
		return
	}

	a.addLog("DEBUG", fmt.Sprintf("Slewing to Az %.1f° Alt %.1f°", ac.HorizCoord.Azimuth, ac.HorizCoord.Altitude))

	err := a.telescope.SlewToAltAz(ac.HorizCoord.Altitude, ac.HorizCoord.Azimuth)
	if err != nil {
		a.addLog("ERROR", fmt.Sprintf("Failed to slew telescope: %v", err))
		a.mu.Lock()
		a.tracking = false
		a.trackingMode = TrackingModeIdle
		a.mu.Unlock()
		return
	}

	// Wait for intercept to complete (position within threshold)
	go a.waitForIntercept()
}

// waitForIntercept waits for initial slew to complete, then switches to continuous tracking
func (a *App) waitForIntercept() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(30 * time.Second)

	for {
		select {
		case <-ticker.C:
			a.mu.RLock()
			if !a.tracking || a.trackingMode != TrackingModeIntercept {
				a.mu.RUnlock()
				return
			}

			// Check position threshold
			altDiff := a.targetAlt - a.telescopeAlt
			azDiff := a.targetAz - a.telescopeAz
			
			// Handle azimuth wrap-around
			if azDiff > 180 {
				azDiff -= 360
			} else if azDiff < -180 {
				azDiff += 360
			}

			// Check if within threshold
			if math.Abs(altDiff) < positionThreshold && math.Abs(azDiff) < positionThreshold {
				a.mu.RUnlock()
				a.addLog("INFO", "Intercept complete, switching to continuous tracking")
				
				a.mu.Lock()
				a.trackingMode = TrackingModeContinuous
				a.mu.Unlock()
				return
			}
			a.mu.RUnlock()

		case <-timeout:
			a.addLog("WARN", "Intercept timeout, switching to continuous tracking anyway")
			a.mu.Lock()
			a.trackingMode = TrackingModeContinuous
			a.mu.Unlock()
			return

		case <-a.stopChan:
			return
		}
	}
}

// updateTrackingSlew updates telescope position while tracking
func (a *App) updateTrackingSlew() {
	a.mu.RLock()
	if !a.tracking || !a.telescopeConnected {
		a.mu.RUnlock()
		return
	}

	mode := a.trackingMode

	// Find tracked aircraft
	var tracked *AircraftView
	for i := range a.aircraft {
		if a.aircraft[i].ICAO == a.trackICAO {
			tracked = &a.aircraft[i]
			break
		}
	}

	if tracked == nil {
		a.mu.RUnlock()
		a.addLog("WARN", fmt.Sprintf("Tracked aircraft %s no longer visible", a.trackICAO))
		a.stopTracking()
		return
	}

	// Check altitude limits
	alt := tracked.HorizCoord.Altitude
	if alt < a.minAlt || alt > a.maxAlt {
		a.mu.RUnlock()
		a.addLog("WARN", fmt.Sprintf("Aircraft altitude %.1f° out of range, stopping tracking", alt))
		a.stopTracking()
		return
	}

	// Only do continuous tracking, not intercept (that's handled separately)
	if mode != TrackingModeContinuous {
		a.mu.RUnlock()
		return
	}

	telescopeAlt := a.telescopeAlt
	telescopeAz := a.telescopeAz
	ac := *tracked
	a.mu.RUnlock()

	// Calculate angular velocities needed
	// Delta position / delta time = angular rate
	// We update every 2 seconds, so rates are in deg/sec
	deltaTime := 2.0 // seconds

	altDiff := ac.HorizCoord.Altitude - telescopeAlt
	azDiff := ac.HorizCoord.Azimuth - telescopeAz

	// Handle azimuth wrap-around (choose shortest path)
	if azDiff > 180 {
		azDiff -= 360
	} else if azDiff < -180 {
		azDiff += 360
	}

	// Calculate required rates (deg/sec)
	altRate := altDiff / deltaTime
	azRate := azDiff / deltaTime

	// Clamp to slew rate limits (6 deg/sec for Seestar S30)
	maxRate := a.config.Telescope.SlewRate
	if altRate > maxRate {
		altRate = maxRate
	} else if altRate < -maxRate {
		altRate = -maxRate
	}
	if azRate > maxRate {
		azRate = maxRate
	} else if azRate < -maxRate {
		azRate = -maxRate
	}

	// Apply MoveAxis commands
	if err := a.telescope.MoveAxis(1, altRate); err != nil {
		a.addLog("ERROR", fmt.Sprintf("Failed to move altitude axis: %v", err))
		return
	}

	if err := a.telescope.MoveAxis(0, azRate); err != nil {
		a.addLog("ERROR", fmt.Sprintf("Failed to move azimuth axis: %v", err))
		return
	}

	a.addLog("DEBUG", fmt.Sprintf("Tracking: Az rate %.2f°/s, Alt rate %.2f°/s", azRate, altRate))

	// Update target for threshold checking
	a.mu.Lock()
	a.targetAlt = ac.HorizCoord.Altitude
	a.targetAz = ac.HorizCoord.Azimuth
	a.mu.Unlock()
}
