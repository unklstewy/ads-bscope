package main

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/unklstewy/ads-bscope/pkg/coordinates"
)

// radarToScreen converts geographic coordinates to radar screen X/Y position.
// Returns coordinates relative to radar center, scaled by radius.
// Returns -1,-1 if aircraft is outside radar radius.
// Applies aspect ratio correction to account for character height:width ratio (~2:1).
func (m model) radarToScreen(lat, lon float64) (int, int) {
	// Calculate distance and bearing from radar center
	acPos := coordinates.Geographic{
		Latitude:  lat,
		Longitude: lon,
		Altitude:  0,
	}
	
	distanceNM := coordinates.DistanceNauticalMiles(m.radarCenter, acPos)
	
	// Check if outside radar radius
	if distanceNM > m.radarRadius {
		return -1, -1
	}
	
	// Calculate bearing from center to aircraft
	bearing := coordinates.Bearing(m.radarCenter, acPos)
	
	// Get radar display dimensions
	radarWidth := m.width - 60 // Reserve space for info panel
	if radarWidth < 80 {
		radarWidth = 80
	}
	radarHeight := m.height - 10 // Reserve space for header and aircraft list
	if radarHeight < 30 {
		radarHeight = 30
	}
	
	// Convert to screen coordinates
	// Center of screen
	centerX := (radarWidth - 2) / 2
	centerY := radarHeight / 2
	
	// Character aspect ratio correction: terminal characters are ~2:1 (height:width)
	// This means we need to scale X distances by 0.5 to make circles look round
	const aspectRatio = 0.5
	
	// Scale factor: fit radius within smaller dimension
	// Use vertical dimension since it's typically smaller
	maxScreenRadiusY := float64(radarHeight/2 - 3)
	maxScreenRadiusX := float64(radarWidth/2 - 3) * aspectRatio
	maxScreenRadius := maxScreenRadiusY
	if maxScreenRadiusX < maxScreenRadiusY {
		maxScreenRadius = maxScreenRadiusX
	}
	scale := maxScreenRadius / m.radarRadius
	
	// Convert polar (distance, bearing) to cartesian (x, y)
	// Bearing 0° = North = up = negative Y
	// Bearing 90° = East = right = positive X
	bearingRad := bearing * math.Pi / 180.0
	screenDist := distanceNM * scale
	
	// Apply aspect ratio correction to X coordinate
	dx := int(screenDist * math.Sin(bearingRad) / aspectRatio)
	dy := -int(screenDist * math.Cos(bearingRad)) // Negative because Y increases downward
	
	x := centerX + dx
	y := centerY + dy
	
	// Check bounds
	if x < 0 || x >= radarWidth-2 || y < 0 || y >= radarHeight {
		return -1, -1
	}
	
	return x, y
}

// renderRadar renders the radar screen view centered on an airport.
func (m model) renderRadar() string {
	var radar strings.Builder
	
	// Get radar display dimensions (dynamic based on terminal size)
	radarWidth := m.width - 60 // Reserve space for info panel
	if radarWidth < 80 {
		radarWidth = 80
	}
	radarHeight := m.height - 10 // Reserve space for header and aircraft list
	if radarHeight < 30 {
		radarHeight = 30
	}
	
	// Draw border
	borderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	radar.WriteString(borderStyle.Render("┌" + strings.Repeat("─", radarWidth-2) + "┐"))
	radar.WriteString("\n")
	
	// Create radar grid
	grid := make([][]rune, radarHeight)
	for i := range grid {
		grid[i] = make([]rune, radarWidth)
		for j := range grid[i] {
			grid[i][j] = ' '
		}
	}
	
	centerX := (radarWidth - 2) / 2
	centerY := radarHeight / 2
	
	// Character aspect ratio correction (terminal chars are ~2:1 height:width)
	const aspectRatio = 0.5
	
	// Draw range rings (concentric circles)
	maxScreenRadiusY := float64(radarHeight/2 - 3)
	maxScreenRadiusX := float64(radarWidth/2 - 3) * aspectRatio
	maxScreenRadius := maxScreenRadiusY
	if maxScreenRadiusX < maxScreenRadiusY {
		maxScreenRadius = maxScreenRadiusX
	}
	scale := maxScreenRadius / m.radarRadius
	
	// Calculate nice ring intervals
	ringIntervals := []float64{50, 100, 250, 500, 1000}
	var ringDistances []float64
	var ringLabels []string
	
	for _, interval := range ringIntervals {
		for dist := interval; dist < m.radarRadius; dist += interval {
			ringDistances = append(ringDistances, dist)
			if dist >= 1000 {
				ringLabels = append(ringLabels, fmt.Sprintf("%.0fk", dist/1000))
			} else {
				ringLabels = append(ringLabels, fmt.Sprintf("%.0f", dist))
			}
		}
		if len(ringDistances) >= 5 {
			break // Don't overcrowd the display
		}
	}
	
	// Draw range rings with better contrast
	for i, ringDist := range ringDistances {
		screenRadius := int(ringDist * scale)
		drawCircle(grid, centerX, centerY, screenRadius, aspectRatio, '─')
		
		// Add range label at top of ring
		label := ringLabels[i]
		labelY := centerY - screenRadius
		labelX := centerX - len(label)/2
		if labelY >= 0 && labelY < radarHeight && labelX >= 0 && labelX+len(label) < radarWidth-2 {
			for j, ch := range label {
				if labelX+j < radarWidth-2 {
					grid[labelY][labelX+j] = ch
				}
			}
		}
	}
	
	// Draw cardinal directions
	// North (top)
	if centerY-int(maxScreenRadius) >= 0 {
		grid[centerY-int(maxScreenRadius)][centerX] = 'N'
	}
	// East (right)
	eastX := centerX + int(maxScreenRadius/aspectRatio)
	if eastX < radarWidth-2 {
		grid[centerY][eastX] = 'E'
	}
	// South (bottom)
	if centerY+int(maxScreenRadius) < radarHeight {
		grid[centerY+int(maxScreenRadius)][centerX] = 'S'
	}
	// West (left)
	westX := centerX - int(maxScreenRadius/aspectRatio)
	if westX >= 0 {
		grid[centerY][westX] = 'W'
	}
	
	// Draw center point (airport)
	grid[centerY][centerX] = '✈'
	
	// Draw aircraft and collect labels
	type aircraftLabel struct {
		x, y  int
		label string
	}
	var labels []aircraftLabel
	
	for i, ac := range m.aircraft {
		x, y := m.radarToScreen(ac.aircraft.Latitude, ac.aircraft.Longitude)
		if x < 0 || y < 0 {
			continue // Outside radar range
		}
		
		symbol := '○'
		isSpecial := false
		if i == m.selected {
			symbol = '●' // Selected aircraft
			isSpecial = true
		}
		if m.tracking && ac.aircraft.ICAO == m.trackICAO {
			symbol = '◉' // Tracked aircraft
			isSpecial = true
		}
		
		grid[y][x] = symbol
		
		// Add label for selected or tracked aircraft
		if isSpecial {
			labelText := ac.aircraft.Callsign
			if labelText == "" {
				labelText = ac.aircraft.ICAO
			}
			labels = append(labels, aircraftLabel{x: x + 2, y: y, label: labelText})
		}
		
		// Draw velocity vector
		if ac.aircraft.GroundSpeed > 50 {
			drawVelocityVectorRadar(grid, x, y, ac.aircraft.Track, ac.aircraft.GroundSpeed, aspectRatio)
		}
	}
	
	// Add aircraft labels to grid (after velocity vectors)
	for _, label := range labels {
		lx, ly := label.x, label.y
		for i, ch := range label.label {
			if ly >= 0 && ly < radarHeight && lx+i >= 0 && lx+i < radarWidth-2 {
				// Only overwrite empty space or range rings
				if grid[ly][lx+i] == ' ' || grid[ly][lx+i] == '─' {
					grid[ly][lx+i] = ch
				}
			}
		}
	}
	
	// Render grid with colors
	for y := 0; y < radarHeight; y++ {
		radar.WriteString(borderStyle.Render("│"))
		for x := 0; x < radarWidth-2; x++ {
			char := grid[y][x]
			switch char {
			case '✈':
				radar.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("208")).Bold(true).Render(string(char)))
			case '◉':
				radar.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Bold(true).Render(string(char)))
			case '●':
				radar.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Render(string(char)))
			case '○':
				radar.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("75")).Render(string(char)))
			case 'N', 'E', 'S', 'W':
				radar.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Bold(true).Render(string(char)))
			case '─': // Range rings - brighter for better contrast
				radar.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(string(char)))
			case '→', '-':
				radar.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Render(string(char)))
			case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', 'k': // Range labels
				radar.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("248")).Render(string(char)))
			default:
				// Check if it's part of an aircraft label (letters)
				if (char >= 'A' && char <= 'Z') || (char >= 'a' && char <= 'z') {
					radar.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Render(string(char)))
				} else {
					radar.WriteRune(char)
				}
			}
		}
		radar.WriteString(borderStyle.Render("│"))
		radar.WriteString("\n")
	}
	
	radar.WriteString(borderStyle.Render("└" + strings.Repeat("─", radarWidth-2) + "┘"))
	
	return radar.String()
}

// drawCircle draws a circle on the grid using Bresenham's circle algorithm.
// Applies aspect ratio correction to draw proper circles on terminal (chars are ~2:1 height:width).
func drawCircle(grid [][]rune, cx, cy, radius int, aspectRatio float64, char rune) {
	x := radius
	y := 0
	err := 0
	
	for x >= y {
		// Draw 8 octants with aspect ratio correction on X coordinates
		xScaled := int(float64(x) / aspectRatio)
		yScaled := int(float64(y) / aspectRatio)
		
		setPixel(grid, cx+xScaled, cy+y, char)
		setPixel(grid, cx+yScaled, cy+x, char)
		setPixel(grid, cx-yScaled, cy+x, char)
		setPixel(grid, cx-xScaled, cy+y, char)
		setPixel(grid, cx-xScaled, cy-y, char)
		setPixel(grid, cx-yScaled, cy-x, char)
		setPixel(grid, cx+yScaled, cy-x, char)
		setPixel(grid, cx+xScaled, cy-y, char)
		
		y++
		err += 1 + 2*y
		if 2*(err-x)+1 > 0 {
			x--
			err += 1 - 2*x
		}
	}
}

// setPixel sets a pixel in the grid if it's within bounds.
// Only overwrites empty space or previous range ring pixels.
func setPixel(grid [][]rune, x, y int, char rune) {
	if y >= 0 && y < len(grid) && x >= 0 && x < len(grid[0]) {
		if grid[y][x] == ' ' || grid[y][x] == '─' {
			grid[y][x] = char
		}
	}
}

// drawVelocityVectorRadar draws velocity vector in radar mode.
// Applies aspect ratio correction for proper vector direction display.
func drawVelocityVectorRadar(grid [][]rune, x, y int, trackDeg, speedKts, aspectRatio float64) {
	// Vector length based on speed (normalized)
	length := int(speedKts / 150.0) + 1
	if length > 4 {
		length = 4
	}
	
	// Convert track to screen coordinates
	trackRad := trackDeg * math.Pi / 180.0
	
	for i := 1; i <= length; i++ {
		// Apply aspect ratio correction to X coordinate
		dx := int(float64(i) * math.Sin(trackRad) / aspectRatio)
		dy := -int(float64(i) * math.Cos(trackRad))
		
		nx, ny := x+dx, y+dy
		if ny >= 0 && ny < len(grid) && nx >= 0 && nx < len(grid[0]) {
			if grid[ny][nx] == ' ' || grid[ny][nx] == '─' {
				if i == length {
					grid[ny][nx] = '→'
				} else {
					grid[ny][nx] = '-'
				}
			}
		}
	}
}

// renderRadarInfo renders information panel for radar mode.
func (m model) renderRadarInfo() string {
	var info strings.Builder
	
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	info.WriteString(headerStyle.Render("RADAR MODE"))
	info.WriteString("\n\n")
	
	// Airport and radius info
	info.WriteString(fmt.Sprintf("Center: %s\n", m.radarAirport))
	info.WriteString(fmt.Sprintf("Radius: %.0f NM\n", m.radarRadius))
	info.WriteString(fmt.Sprintf("Position: %.4f°, %.4f°\n", m.radarCenter.Latitude, m.radarCenter.Longitude))
	info.WriteString(fmt.Sprintf("Aircraft: %d in range\n", len(m.aircraft)))
	info.WriteString("\n")
	
	// Controls
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	info.WriteString(helpStyle.Render("R: Exit radar  +/-: Adjust radius\n"))
	info.WriteString(helpStyle.Render("↑/↓: Select  ENTER: Track  Q: Quit"))
	
	return info.String()
}
