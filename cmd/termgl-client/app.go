package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/unklstewy/ads-bscope/internal/db"
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
	logs         *tview.TextView
	rootLayout   *tview.Flex
	currentView  ViewMode

	// State
	aircraft      []AircraftView
	selectedIndex int
	tracking      bool
	trackICAO     string
	showTrails    bool
	showConstell  bool
	zoom          float64

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
		currentView:    ViewModeSky,
		stopChan:       make(chan struct{}),
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
	// Create a text view for displaying aircraft list (temporary for Phase 2)
	box := tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true)
	box.SetBorder(true).SetTitle(" Sky View - Aircraft List ")

	a.mainView = box
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
	a.logs = tview.NewTextView().
		SetDynamicColors(true).
		SetScrollable(true).
		SetMaxLines(100)
	a.logs.SetBorder(true).SetTitle(" Logs ")

	// Add initial log message
	a.addLog("INFO", "Application started")
}

// createLayout creates the main layout with 4 panels
func (a *App) createLayout() {
	// Right sidebar with 3 panels
	sidebar := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(a.telemetry, 0, 4, false).  // 40% of sidebar
		AddItem(a.controls, 0, 3, false).   // 30% of sidebar
		AddItem(a.logs, 0, 3, false)        // 30% of sidebar

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
	if a.selectedIndex >= 0 && a.selectedIndex < len(a.aircraft) {
		ac := a.aircraft[a.selectedIndex]
		text += fmt.Sprintf("[yellow]AIRCRAFT:[-] [white]%s[-] [gray](%s)[-]\n", ac.Callsign, ac.ICAO)
		text += fmt.Sprintf("[gray]Alt:[-]  [white]%.0f ft[-]  [gray]Spd:[-] [white]%.0f kts[-]\n", ac.Altitude, ac.Speed)
		text += fmt.Sprintf("[gray]Hdg:[-]  [white]%.0f°[-]     [gray]Age:[-] [white]%.1fs[-]\n", ac.Heading, ac.Age.Seconds())
		text += fmt.Sprintf("[gray]Az:[-]   [white]%.1f°[-]  [gray]Alt:[-] [white]%.1f°[-]\n", ac.HorizCoord.Azimuth, ac.HorizCoord.Altitude)
		text += fmt.Sprintf("[gray]Pos:[-]  [white]%.4f°, %.4f°[-]\n", ac.Latitude, ac.Longitude)
	} else {
		text += "[gray]No aircraft selected[-]\n"
	}

	text += "\n"

	// Telescope section
	text += "[yellow]TELESCOPE:[-] [red]Not Connected[-]\n"
	text += "[gray]Pos:[-]  [white]---[-]\n"
	text += "[gray]Mode:[-] [white]IDLE[-]\n"

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

// addLog adds a log message to the log panel
func (a *App) addLog(level, message string) {
	timestamp := time.Now().Format("15:04:05")
	var color string
	switch level {
	case "ERROR":
		color = "red"
	case "WARN":
		color = "yellow"
	case "INFO":
		color = "white"
	case "DEBUG":
		color = "gray"
	default:
		color = "white"
	}

	logLine := fmt.Sprintf("[gray]%s[-] [%s]%-5s[-] %s\n", timestamp, color, level, message)
	fmt.Fprint(a.logs, logLine)
}

// handleKeyboard handles keyboard input
func (a *App) handleKeyboard(event *tcell.EventKey) *tcell.EventKey {
	key := event.Key()
	rune := event.Rune()

	switch {
	// Quit
	case key == tcell.KeyEscape || rune == 'q':
		a.Stop()
		return nil

	// Navigation
	case key == tcell.KeyUp || rune == 'k':
		a.selectPrevious()
		return nil
	case key == tcell.KeyDown || rune == 'j':
		a.selectNext()
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

	ac := a.aircraft[a.selectedIndex]
	a.tracking = true
	a.trackICAO = ac.ICAO

	a.addLog("INFO", fmt.Sprintf("Tracking %s (%s)", ac.Callsign, ac.ICAO))
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

	a.addLog("INFO", "Tracking stopped")
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
		case <-a.stopChan:
			return
		}
	}
}

// fetchAircraftData fetches aircraft data from the database
func (a *App) fetchAircraftData() {
	ctx := context.Background()

	// Get visible aircraft from repository
	aircraft, err := a.aircraftRepo.GetTrackableAircraft(ctx)
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
		a.updateMainView()
	})
}

// updateMainView updates the main view content
func (a *App) updateMainView() {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Get the text view from mainView
	textView, ok := a.mainView.(*tview.TextView)
	if !ok {
		return
	}

	var content string

	if len(a.aircraft) == 0 {
		content = "\n[gray]No aircraft visible[-]\n\n"
		content += "[gray]Waiting for aircraft data...[-]"
		textView.SetText(content)
		return
	}

	// Header
	content = fmt.Sprintf("[yellow]%d Aircraft Visible[-]\n\n", len(a.aircraft))

	// Column headers
	content += "[cyan]" +
		"  CALLSIGN    ICAO    ALT(ft)  SPD(kt)  HDG   AZIMUTH  ALTITUDE  AGE" +
		"[-]\n"
	content += "[gray]" + strings.Repeat("─", 78) + "[-]\n"

	// Aircraft list
	for i, ac := range a.aircraft {
		var marker string
		var color string

		// Determine marker and color
		if ac.Tracking {
			marker = "◉"
			color = "green"
		} else if ac.Selected {
			marker = "●"
			color = "yellow"
		} else {
			marker = "○"
			color = "white"
		}

		// Format callsign with padding
		callsign := ac.Callsign
		if callsign == "" {
			callsign = "---"
		}

		// Format line
		line := fmt.Sprintf(
			"[%s]%s %-10s  %-6s  %6.0f   %5.0f    %3.0f   %6.1f°   %5.1f°    %.1fs[-]\n",
			color,
			marker,
			callsign,
			ac.ICAO,
			ac.Altitude,
			ac.Speed,
			ac.Heading,
			ac.HorizCoord.Azimuth,
			ac.HorizCoord.Altitude,
			ac.Age.Seconds(),
		)

		content += line

		// Highlight selected aircraft
		if i == a.selectedIndex {
			// Add extra spacing after selected
			if i < len(a.aircraft)-1 {
				content += "\n"
			}
		}
	}

	// Footer
	content += "\n[gray]" + strings.Repeat("─", 78) + "[-]\n"
	content += fmt.Sprintf("[gray]Use ↑/↓ to select, ENTER to track, SPACE to stop[-]")

	textView.SetText(content)
}

// Stop stops the application
func (a *App) Stop() {
	a.addLog("INFO", "Shutting down...")
	
	// Stop update loop
	if a.updateTimer != nil {
		a.updateTimer.Stop()
	}
	close(a.stopChan)

	// Stop tview application
	a.tviewApp.Stop()
}
