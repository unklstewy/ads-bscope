package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/unklstewy/ads-bscope/internal/db"
	"github.com/unklstewy/ads-bscope/pkg/adsb"
	"github.com/unklstewy/ads-bscope/pkg/config"
	"github.com/unklstewy/ads-bscope/pkg/coordinates"
	"github.com/unklstewy/ads-bscope/pkg/tracking"
)

// Sky viewport dimensions
const (
	skyWidth  = 80
	skyHeight = 30
)

// Track trail stores recent positions for breadcrumb display
type trackTrail struct {
	positions []coordinates.HorizontalCoordinates
	times     []time.Time
	maxLength int
}

type model struct {
	cfg        *config.Config
	database   *db.DB
	repo       *db.AircraftRepository
	fpRepo     *db.FlightPlanRepository
	observer   coordinates.Observer
	aircraft   []aircraftView
	selected   int
	tracking   bool
	trackICAO  string
	telesAlt   float64
	telesAz    float64
	err        error
	minAlt     float64
	maxAlt     float64
	zoom       float64  // Zoom level: 1.0 = normal, 2.0 = 2x closer
	trails     map[string]*trackTrail  // ICAO -> trail
	
	// Radar mode
	radarMode    bool
	radarCenter  coordinates.Geographic
	radarRadius  float64  // Nautical miles
	radarAirport string
	inputMode    string  // "airport" or "radius" or ""
	inputBuffer  string
}

type aircraftView struct {
	aircraft       adsb.Aircraft
	horiz          coordinates.HorizontalCoordinates
	range_nm       float64
	age            float64
	predictionMode string  // "", "waypoint", "airway", "deadreckoning"
	matchedAirway  string  // For airway predictions
	flightPlan     *db.FlightPlan
	nextWaypoint   string
}

type tickMsg time.Time

func tick() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m model) Init() tea.Cmd {
	return tick()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle input mode (airport code or radius entry)
		if m.inputMode != "" {
			switch msg.String() {
			case "enter":
				// Process input
				if m.inputMode == "airport" {
					m.radarAirport = strings.ToUpper(m.inputBuffer)
					// Lookup airport coordinates
					ctx := context.Background()
					wp, err := m.fpRepo.GetWaypointByIdentifier(ctx, m.radarAirport)
					if err == nil && wp != nil {
						m.radarCenter = coordinates.Geographic{
							Latitude:  wp.Latitude,
							Longitude: wp.Longitude,
							Altitude:  0,
						}
						// Move to radius input
						m.inputMode = "radius"
						m.inputBuffer = fmt.Sprintf("%.0f", m.radarRadius)
					} else {
						m.err = fmt.Errorf("airport %s not found", m.radarAirport)
						m.inputMode = ""
						m.inputBuffer = ""
					}
				} else if m.inputMode == "radius" {
					// Parse radius
					var radius float64
					if _, err := fmt.Sscanf(m.inputBuffer, "%f", &radius); err == nil {
						if radius >= 50 && radius <= 2500 {
							m.radarRadius = radius
							m.radarMode = true
						} else {
							m.err = fmt.Errorf("radius must be between 50 and 2500 NM")
						}
					} else {
						m.err = fmt.Errorf("invalid radius: %s", m.inputBuffer)
					}
					m.inputMode = ""
					m.inputBuffer = ""
				}
			case "esc":
				// Cancel input
				m.inputMode = ""
				m.inputBuffer = ""
			case "backspace":
				if len(m.inputBuffer) > 0 {
					m.inputBuffer = m.inputBuffer[:len(m.inputBuffer)-1]
				}
			default:
				// Add character to buffer
				if len(msg.String()) == 1 {
					m.inputBuffer += msg.String()
				}
			}
			return m, nil
		}
		
		// Clear error on any keypress (but don't quit)
		if m.err != nil {
			m.err = nil
			return m, nil
		}
		
		// Normal mode controls
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "r":
			// Toggle radar mode or start radar setup
			if m.radarMode {
				m.radarMode = false
			} else {
				// Start airport input
				m.inputMode = "airport"
				m.inputBuffer = ""
				m.err = nil
			}
		case "up", "k":
			if !m.radarMode && m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if !m.radarMode && m.selected < len(m.aircraft)-1 {
				m.selected++
			}
		case "enter", " ":
			if !m.radarMode && len(m.aircraft) > 0 && m.selected < len(m.aircraft) {
				m.tracking = true
				m.trackICAO = m.aircraft[m.selected].aircraft.ICAO
				m.telesAlt = m.aircraft[m.selected].horiz.Altitude
				m.telesAz = m.aircraft[m.selected].horiz.Azimuth
			}
		case "s":
			m.tracking = false
		case "+", "=":
			// Zoom in (max 4x in sky mode, increase radius in radar mode)
			if m.radarMode {
				if m.radarRadius < 2500 {
					m.radarRadius *= 1.5
					if m.radarRadius > 2500 {
						m.radarRadius = 2500
					}
				}
			} else if m.zoom < 4.0 {
				m.zoom *= 1.5
			}
		case "-", "_":
			// Zoom out (min 0.5x in sky mode, decrease radius in radar mode)
			if m.radarMode {
				if m.radarRadius > 50 {
					m.radarRadius /= 1.5
					if m.radarRadius < 50 {
						m.radarRadius = 50
					}
				}
			} else if m.zoom > 0.5 {
				m.zoom /= 1.5
			}
		case "0":
			// Reset zoom
			m.zoom = 1.0
		}

	case tickMsg:
		m.updateAircraft()
		if m.tracking && m.trackICAO != "" {
			// Update telescope position to track selected aircraft
			for _, ac := range m.aircraft {
				if ac.aircraft.ICAO == m.trackICAO {
					m.telesAlt = ac.horiz.Altitude
					m.telesAz = ac.horiz.Azimuth
					break
				}
			}
		}
		return m, tick()
	}

	return m, nil
}

func (m *model) updateAircraft() {
	ctx := context.Background()
	
	// Get all trackable aircraft from database
	aircraftList, err := m.repo.GetTrackableAircraft(ctx)
	if err != nil {
		m.err = err
		return
	}

	m.aircraft = make([]aircraftView, 0)
	now := time.Now().UTC()

	for _, ac := range aircraftList {
		dataAge := now.Sub(ac.LastSeen).Seconds()
		
		// Get flight plan if available
		flightPlan, _ := m.fpRepo.GetFlightPlanByICAO(ctx, ac.ICAO)
		var waypointList []tracking.Waypoint
		var nextWaypoint string
		
		if flightPlan != nil {
			routes, err := m.fpRepo.GetFlightPlanRoute(ctx, flightPlan.ID)
			if err == nil && len(routes) > 0 {
				for _, r := range routes {
					waypointList = append(waypointList, tracking.Waypoint{
						Name:      r.WaypointName,
						Latitude:  r.Latitude,
						Longitude: r.Longitude,
						Sequence:  r.Sequence,
						Passed:    r.Passed,
					})
				}
				waypointList = tracking.DeterminePassedWaypoints(ac, waypointList)
				
				// Find next waypoint
				for _, wp := range waypointList {
					if !wp.Passed {
						nextWaypoint = wp.Name
						break
					}
				}
			}
		}
		
		// Calculate position (with prediction if needed)
		var acPos coordinates.Geographic
		var predictionMode string
		var matchedAirway string
		
		if dataAge > 30 {
			// Data is stale - use prediction
			if len(waypointList) > 0 {
				// Waypoint-based prediction
				predictedPos := tracking.PredictPositionWithWaypoints(
					ac,
					waypointList,
					now.Add(time.Duration(dataAge*float64(time.Second))),
				)
				acPos = predictedPos.Position
				predictionMode = "waypoint"
			} else {
				// Try airway matching
				airwaySegs, err := m.fpRepo.FindNearbyAirways(
					ctx,
					ac.Latitude,
					ac.Longitude,
					25.0,
					int(ac.Altitude*0.9),
					int(ac.Altitude*1.1),
				)
				
				if err == nil && len(airwaySegs) > 0 {
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
					
					trackingAirways = tracking.FilterAirwaysByAltitude(trackingAirways, ac.Altitude)
					matchedAirwaySeg := tracking.MatchAirway(ac, trackingAirways)
					
					if matchedAirwaySeg != nil {
						predictedPos := tracking.PredictPositionWithAirway(
							ac,
							*matchedAirwaySeg,
							now.Add(time.Duration(dataAge*float64(time.Second))),
						)
						acPos = predictedPos.Position
						predictionMode = "airway"
						matchedAirway = matchedAirwaySeg.AirwayID
					} else {
						// Fall back to dead reckoning
						predictedPos := tracking.PredictPositionWithLatency(ac, dataAge)
						acPos = predictedPos.Position
						predictionMode = "deadreckoning"
					}
				} else {
					// Fall back to dead reckoning
					predictedPos := tracking.PredictPositionWithLatency(ac, dataAge)
					acPos = predictedPos.Position
					predictionMode = "deadreckoning"
				}
			}
		} else {
			// Data is fresh
			acPos = coordinates.Geographic{
				Latitude:  ac.Latitude,
				Longitude: ac.Longitude,
				Altitude:  ac.Altitude * coordinates.FeetToMeters,
			}
		}

		horiz := coordinates.GeographicToHorizontal(acPos, m.observer, now)
		rangeNM := coordinates.DistanceNauticalMiles(m.observer.Location, acPos)
		
		// Update track trail
		if m.trails[ac.ICAO] == nil {
			m.trails[ac.ICAO] = &trackTrail{
				positions: make([]coordinates.HorizontalCoordinates, 0),
				times:     make([]time.Time, 0),
				maxLength: 10,
			}
		}
		trail := m.trails[ac.ICAO]
		trail.positions = append(trail.positions, horiz)
		trail.times = append(trail.times, now)
		if len(trail.positions) > trail.maxLength {
			trail.positions = trail.positions[1:]
			trail.times = trail.times[1:]
		}

		m.aircraft = append(m.aircraft, aircraftView{
			aircraft:       ac,
			horiz:          horiz,
			range_nm:       rangeNM,
			age:            dataAge,
			predictionMode: predictionMode,
			matchedAirway:  matchedAirway,
			flightPlan:     flightPlan,
			nextWaypoint:   nextWaypoint,
		})
	}
}

func (m model) View() string {
	var s strings.Builder

	// Header
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("86")).
		Background(lipgloss.Color("235")).
		Padding(0, 1)
	
	title := "ADS-B SCOPE TUI VIEWFINDER"
	if m.radarMode {
		title = "ADS-B SCOPE RADAR MODE"
	}
	s.WriteString(titleStyle.Render(title))
	s.WriteString("\n\n")

	// Handle input mode prompts
	if m.inputMode != "" {
		promptStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
		inputStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
		helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
		
		if m.inputMode == "airport" {
			s.WriteString(promptStyle.Render("Enter airport code (e.g., KATL, JFK):"))
			s.WriteString("\n")
			s.WriteString(inputStyle.Render("> " + m.inputBuffer + "_"))
			s.WriteString("\n\n")
			s.WriteString(helpStyle.Render("ENTER: Submit  ESC: Cancel"))
		} else if m.inputMode == "radius" {
			s.WriteString(promptStyle.Render("Enter radar radius (50-2500 NM):"))
			s.WriteString("\n")
			s.WriteString(inputStyle.Render("> " + m.inputBuffer + "_"))
			s.WriteString("\n\n")
			s.WriteString(helpStyle.Render("ENTER: Submit  ESC: Cancel"))
		}
		return s.String()
	}

	if m.err != nil {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
		s.WriteString(errStyle.Render(fmt.Sprintf("Error: %v", m.err)))
		s.WriteString("\n\n")
		helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
		s.WriteString(helpStyle.Render("Press SPACE to continue (use 3-letter codes: RDU, ATL, JFK, LAX, ORD)..."))
		return s.String()
	}

	// Render based on mode
	if m.radarMode {
		// Radar mode view
		radar := m.renderRadar()
		radInfo := m.renderRadarInfo()
		
		// Split and combine
		radarLines := strings.Split(radar, "\n")
		infoLines := strings.Split(radInfo, "\n")
		
		maxLines := len(radarLines)
		if len(infoLines) > maxLines {
			maxLines = len(infoLines)
		}
		
		for i := 0; i < maxLines; i++ {
			if i < len(radarLines) {
				s.WriteString(radarLines[i])
			} else {
				s.WriteString(strings.Repeat(" ", skyWidth))
			}
			s.WriteString("  ") // Spacing
			if i < len(infoLines) {
				s.WriteString(infoLines[i])
			}
			s.WriteString("\n")
		}
		
		// Aircraft list
		s.WriteString(m.renderAircraftList())
		s.WriteString("\n")
	} else {
		// Sky view mode
		sky := m.renderSky()
		legend := m.renderLegend()
		
		// Split sky and legend
		skyLines := strings.Split(sky, "\n")
		legendLines := strings.Split(legend, "\n")
		
		// Combine side by side
		maxLines := len(skyLines)
		if len(legendLines) > maxLines {
			maxLines = len(legendLines)
		}
		
		for i := 0; i < maxLines; i++ {
			if i < len(skyLines) {
				s.WriteString(skyLines[i])
			} else {
				s.WriteString(strings.Repeat(" ", skyWidth))
			}
			s.WriteString("  ") // Spacing
			if i < len(legendLines) {
				s.WriteString(legendLines[i])
			}
			s.WriteString("\n")
		}

		// Aircraft list
		s.WriteString(m.renderAircraftList())
		s.WriteString("\n")

		// Controls
		helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
		s.WriteString(helpStyle.Render("↑/↓: Select  ENTER/SPACE: Track  S: Stop  R: Radar  +/-: Zoom  0: Reset  Q: Quit"))
		s.WriteString("\n")
	}

	return s.String()
}

func (m model) renderSky() string {
	var sky strings.Builder

	// Draw border
	borderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	sky.WriteString(borderStyle.Render("┌" + strings.Repeat("─", skyWidth-2) + "┐"))
	sky.WriteString("\n")

	// Create sky grid
	grid := make([][]rune, skyHeight)
	for i := range grid {
		grid[i] = make([]rune, skyWidth)
		for j := range grid[i] {
			grid[i][j] = ' '
		}
	}

	// Draw horizon line
	horizonY := int(float64(skyHeight) * 0.8) // 80% down is horizon
	for x := 0; x < skyWidth; x++ {
		grid[horizonY][x] = '·'
	}

	// Draw cardinal directions
	grid[skyHeight-1][skyWidth/4] = 'E'
	grid[skyHeight-1][skyWidth/2] = 'S'
	grid[skyHeight-1][skyWidth*3/4] = 'W'
	grid[skyHeight-1][0] = 'N'
	
	// Draw range rings (concentric circles at 5, 10, 25, 50 NM)
	for _, ac := range m.aircraft {
		if ac.horiz.Altitude < m.minAlt || ac.horiz.Altitude > m.maxAlt {
			continue
		}
		
		// Draw rings at different ranges
		for _, ringRange := range []float64{5.0, 10.0, 25.0, 50.0} {
			if math.Abs(ac.range_nm-ringRange) < 1.0 {
				// Aircraft is near this ring distance, draw a partial ring
				m.drawRingSegment(grid, ac.horiz.Azimuth, ringRange)
			}
		}
		
		// Draw track trails
		if trail, ok := m.trails[ac.aircraft.ICAO]; ok {
			for i := 0; i < len(trail.positions)-1; i++ {
				pos := trail.positions[i]
				if pos.Altitude >= m.minAlt && pos.Altitude <= m.maxAlt {
					tx, ty := m.altAzToScreen(pos.Altitude, pos.Azimuth)
					if tx >= 0 && tx < skyWidth && ty >= 0 && ty < skyHeight {
						if grid[ty][tx] == ' ' || grid[ty][tx] == '·' {
							grid[ty][tx] = '·' // Breadcrumb
						}
					}
				}
			}
		}
	}

	// Draw telescope crosshair
	if m.telesAlt >= m.minAlt && m.telesAlt <= m.maxAlt {
		tx, ty := m.altAzToScreen(m.telesAlt, m.telesAz)
		if tx >= 0 && tx < skyWidth && ty >= 0 && ty < skyHeight {
			grid[ty][tx] = '+'
		}
	}

	// Draw aircraft and velocity vectors
	for i, ac := range m.aircraft {
		if ac.horiz.Altitude < m.minAlt || ac.horiz.Altitude > m.maxAlt {
			continue
		}

		x, y := m.altAzToScreen(ac.horiz.Altitude, ac.horiz.Azimuth)
		if x >= 0 && x < skyWidth && y >= 0 && y < skyHeight {
			symbol := '○'
			if i == m.selected {
				symbol = '●' // Selected aircraft
			}
			if m.tracking && ac.aircraft.ICAO == m.trackICAO {
				symbol = '◉' // Tracked aircraft
			}
			grid[y][x] = symbol
			
			// Draw velocity vector (arrow showing direction of motion)
			if ac.aircraft.GroundSpeed > 50 { // Only for moving aircraft
				m.drawVelocityVector(grid, x, y, ac.aircraft.Track, ac.aircraft.GroundSpeed)
			}
		}
	}

	// Render grid
	for y := 0; y < skyHeight; y++ {
		sky.WriteString(borderStyle.Render("│"))
		for x := 0; x < skyWidth-2; x++ {
			char := grid[y][x]
			switch char {
			case '+':
				sky.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Render(string(char)))
			case '◉':
				sky.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true).Render(string(char)))
			case '●':
				sky.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Render(string(char)))
			case '○':
				sky.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Render(string(char)))
			case 'N', 'E', 'S', 'W':
				sky.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(string(char)))
			case '·':
				sky.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("237")).Render(string(char)))
			default:
				sky.WriteRune(char)
			}
		}
		sky.WriteString(borderStyle.Render("│"))
		sky.WriteString("\n")
	}

	sky.WriteString(borderStyle.Render("└" + strings.Repeat("─", skyWidth-2) + "┘"))

	return sky.String()
}

func (m model) altAzToScreen(altitude, azimuth float64) (int, int) {
	// Map altitude (0-90°) to screen Y (bottom to top)
	// Map azimuth (0-360°) to screen X (left to right, N=0, E=90, S=180, W=270)
	
	// Normalize azimuth to 0-360
	for azimuth < 0 {
		azimuth += 360
	}
	for azimuth >= 360 {
		azimuth -= 360
	}

	// X coordinate: map azimuth to screen width
	// Azimuth 0° (North) = left, 90° (East) = 1/4, 180° (South) = middle, 270° (West) = 3/4
	x := int((azimuth / 360.0) * float64(skyWidth-2))

	// Y coordinate: map altitude to screen height (inverted, 0° at bottom, 90° at top)
	// Apply zoom: higher zoom = smaller altitude range visible
	altRange := (m.maxAlt - m.minAlt) / m.zoom
	if altRange <= 0 {
		altRange = 80
	}
	normalizedAlt := (altitude - m.minAlt) / altRange
	y := skyHeight - 1 - int(normalizedAlt*float64(skyHeight-1))

	return x, y
}

// drawRingSegment draws a partial range ring indicator
func (m model) drawRingSegment(grid [][]rune, azimuth, rangeNM float64) {
	// Draw a small arc segment near the aircraft
	for az := azimuth - 5; az <= azimuth + 5; az++ {
		// Calculate altitude for ring display (fixed at horizon level)
		alt := m.minAlt + 5.0
		x, y := m.altAzToScreen(alt, az)
		if x >= 0 && x < skyWidth && y >= 0 && y < skyHeight {
			if grid[y][x] == ' ' {
				grid[y][x] = '◦' // Ring marker
			}
		}
	}
}

// drawVelocityVector draws an arrow showing aircraft motion
func (m model) drawVelocityVector(grid [][]rune, x, y int, trackDeg, speedKts float64) {
	// Vector length based on speed (normalized)
	length := int(speedKts / 100.0) + 1
	if length > 5 {
		length = 5
	}
	
	// Convert track to screen coordinates
	// Track: 0=N, 90=E, 180=S, 270=W
	trackRad := trackDeg * math.Pi / 180.0
	
	for i := 1; i <= length; i++ {
		dx := int(float64(i) * math.Sin(trackRad) * 0.5)
		dy := -int(float64(i) * math.Cos(trackRad) * 0.5) // Negative because screen Y is inverted
		
		nx, ny := x+dx, y+dy
		if nx >= 0 && nx < skyWidth && ny >= 0 && ny < skyHeight {
			if grid[ny][nx] == ' ' || grid[ny][nx] == '·' {
				if i == length {
					grid[ny][nx] = '→' // Arrow head
				} else {
					grid[ny][nx] = '-' // Arrow shaft
				}
			}
		}
	}
}

func (m model) renderAircraftList() string {
	var list strings.Builder

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	list.WriteString(headerStyle.Render("Trackable Aircraft:"))
	list.WriteString(fmt.Sprintf(" (%d)", len(m.aircraft)))
	list.WriteString("\n\n")

	if len(m.aircraft) == 0 {
		list.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("  No trackable aircraft in range"))
		return list.String()
	}

	// Show up to 5 aircraft
	start := 0
	if m.selected > 2 && len(m.aircraft) > 5 {
		start = m.selected - 2
	}
	end := start + 5
	if end > len(m.aircraft) {
		end = len(m.aircraft)
	}

	for i := start; i < end; i++ {
		ac := m.aircraft[i]
		
		// Selection indicator
		prefix := "  "
		if i == m.selected {
			prefix = "→ "
		}

		// Tracking indicator
		trackIndicator := ""
		if m.tracking && ac.aircraft.ICAO == m.trackICAO {
			trackIndicator = " [TRACKING]"
		}

		// Prediction mode indicator
		predMode := ""
		switch ac.predictionMode {
		case "waypoint":
			predMode = " [WPT]"
		case "airway":
			predMode = fmt.Sprintf(" [AWY:%s]", ac.matchedAirway)
		case "deadreckoning":
			predMode = " [DR]"
		}
		
		// Age indicator
		ageStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
		if ac.age > 30 {
			ageStyle = ageStyle.Foreground(lipgloss.Color("226"))
		}
		if ac.age > 60 {
			ageStyle = ageStyle.Foreground(lipgloss.Color("196"))
		}

		// Format line
		callsign := ac.aircraft.Callsign
		if callsign == "" {
			callsign = "--------"
		}

		line := fmt.Sprintf("%s%-8s  %6.0f ft  %5.1f nm  Az:%3.0f° Alt:%2.0f°  %4.0fs%s%s",
			prefix,
			callsign,
			ac.aircraft.Altitude,
			ac.range_nm,
			ac.horiz.Azimuth,
			ac.horiz.Altitude,
			ac.age,
			predMode,
			trackIndicator,
		)

		if i == m.selected {
			line = lipgloss.NewStyle().
				Background(lipgloss.Color("237")).
				Render(line)
		}

		list.WriteString(line)
		list.WriteString("\n")
		
		// Show flight plan info if this is the selected aircraft
		if i == m.selected && ac.flightPlan != nil {
			fpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
			if ac.nextWaypoint != "" {
				list.WriteString(fpStyle.Render(fmt.Sprintf("    Plan: %s → %s (next: %s)\n",
					ac.flightPlan.DepartureICAO, ac.flightPlan.ArrivalICAO, ac.nextWaypoint)))
			} else {
				list.WriteString(fpStyle.Render(fmt.Sprintf("    Plan: %s → %s\n",
					ac.flightPlan.DepartureICAO, ac.flightPlan.ArrivalICAO)))
			}
		}
	}

	// Telescope position
	if m.tracking {
		list.WriteString("\n")
		telescopeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true)
		list.WriteString(telescopeStyle.Render(fmt.Sprintf("Telescope: Az %.1f°  Alt %.1f°  Zoom: %.1fx", m.telesAz, m.telesAlt, m.zoom)))
	}

	return list.String()
}

// renderLegend renders the legend panel showing symbols and ranges
func (m model) renderLegend() string {
	var leg strings.Builder
	
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	leg.WriteString(headerStyle.Render("Legend"))
	leg.WriteString("\n\n")
	
	// Symbols
	leg.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Render("○"))
	leg.WriteString(" Untracked\n")
	leg.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Render("●"))
	leg.WriteString(" Selected\n")
	leg.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true).Render("◉"))
	leg.WriteString(" Tracking\n")
	leg.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Render("+"))
	leg.WriteString(" Telescope\n")
	leg.WriteString("· Trail/Ring\n")
	leg.WriteString("→ Velocity\n")
	leg.WriteString("\n")
	
	// Prediction modes
	headerStyle2 := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	leg.WriteString(headerStyle2.Render("Prediction"))
	leg.WriteString("\n")
	leg.WriteString("[WPT] Waypoint\n")
	leg.WriteString("[AWY] Airway\n")
	leg.WriteString("[DR]  Dead Reckoning\n")
	leg.WriteString("\n")
	
	// Range rings
	headerStyle3 := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	leg.WriteString(headerStyle3.Render("Range Rings"))
	leg.WriteString("\n")
	leg.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Render("◦"))
	leg.WriteString("  5 nm\n")
	leg.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Render("◦"))
	leg.WriteString(" 10 nm\n")
	leg.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render("◦"))
	leg.WriteString(" 25 nm\n")
	leg.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render("◦"))
	leg.WriteString(" 50 nm\n")
	
	return leg.String()
}

func main() {
	// Load config
	cfg, err := config.Load("configs/config.json")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Connect to database
	database, err := db.Connect(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer database.Close()

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

	// Get altitude limits
	minAlt, maxAlt := cfg.Telescope.GetAltitudeLimits()

	// Create model
	m := model{
		cfg:         cfg,
		database:    database,
		repo:        repo,
		fpRepo:      fpRepo,
		observer:    observer,
		minAlt:      minAlt,
		maxAlt:      maxAlt,
		telesAlt:    45, // Start at 45° altitude
		telesAz:     180, // Start pointing south
		zoom:        1.0, // Normal zoom
		trails:      make(map[string]*trackTrail),
		radarRadius: 100.0, // Default radar radius 100 NM
	}

	// Initial data load
	m.updateAircraft()

	// Start TUI
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
