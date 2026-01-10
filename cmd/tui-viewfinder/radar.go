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
	
	// Convert to screen coordinates
	// Center of screen is (skyWidth/2, skyHeight/2)
	// Scale distance to fit within screen
	centerX := (skyWidth - 2) / 2
	centerY := skyHeight / 2
	
	// Scale factor: fit radius within smaller dimension
	maxScreenRadius := float64(skyHeight/2 - 2)
	scale := maxScreenRadius / m.radarRadius
	
	// Convert polar (distance, bearing) to cartesian (x, y)
	// Bearing 0° = North = up = negative Y
	// Bearing 90° = East = right = positive X
	bearingRad := bearing * math.Pi / 180.0
	screenDist := distanceNM * scale
	
	dx := int(screenDist * math.Sin(bearingRad))
	dy := -int(screenDist * math.Cos(bearingRad)) // Negative because Y increases downward
	
	x := centerX + dx
	y := centerY + dy
	
	// Check bounds
	if x < 0 || x >= skyWidth-2 || y < 0 || y >= skyHeight {
		return -1, -1
	}
	
	return x, y
}

// renderRadar renders the radar screen view centered on an airport.
func (m model) renderRadar() string {
	var radar strings.Builder
	
	// Draw border
	borderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	radar.WriteString(borderStyle.Render("┌" + strings.Repeat("─", skyWidth-2) + "┐"))
	radar.WriteString("\n")
	
	// Create radar grid
	grid := make([][]rune, skyHeight)
	for i := range grid {
		grid[i] = make([]rune, skyWidth)
		for j := range grid[i] {
			grid[i][j] = ' '
		}
	}
	
	centerX := (skyWidth - 2) / 2
	centerY := skyHeight / 2
	
	// Draw range rings (concentric circles)
	maxScreenRadius := float64(skyHeight/2 - 2)
	scale := maxScreenRadius / m.radarRadius
	
	// Calculate nice ring intervals
	ringIntervals := []float64{50, 100, 250, 500, 1000}
	var ringDistances []float64
	
	for _, interval := range ringIntervals {
		for dist := interval; dist < m.radarRadius; dist += interval {
			ringDistances = append(ringDistances, dist)
		}
		if len(ringDistances) >= 3 {
			break // Don't overcrowd the display
		}
	}
	
	// Draw range rings
	for _, ringDist := range ringDistances {
		screenRadius := int(ringDist * scale)
		drawCircle(grid, centerX, centerY, screenRadius, '·')
	}
	
	// Draw cardinal directions
	// North (top)
	if centerY-int(maxScreenRadius) >= 0 {
		grid[centerY-int(maxScreenRadius)][centerX] = 'N'
	}
	// East (right)
	if centerX+int(maxScreenRadius) < skyWidth-2 {
		grid[centerY][centerX+int(maxScreenRadius)] = 'E'
	}
	// South (bottom)
	if centerY+int(maxScreenRadius) < skyHeight {
		grid[centerY+int(maxScreenRadius)][centerX] = 'S'
	}
	// West (left)
	if centerX-int(maxScreenRadius) >= 0 {
		grid[centerY][centerX-int(maxScreenRadius)] = 'W'
	}
	
	// Draw center point (airport)
	grid[centerY][centerX] = '✈'
	
	// Draw aircraft
	for i, ac := range m.aircraft {
		x, y := m.radarToScreen(ac.aircraft.Latitude, ac.aircraft.Longitude)
		if x < 0 || y < 0 {
			continue // Outside radar range
		}
		
		symbol := '○'
		if i == m.selected {
			symbol = '●' // Selected aircraft
		}
		if m.tracking && ac.aircraft.ICAO == m.trackICAO {
			symbol = '◉' // Tracked aircraft
		}
		
		grid[y][x] = symbol
		
		// Draw velocity vector
		if ac.aircraft.GroundSpeed > 50 {
			drawVelocityVectorRadar(grid, x, y, ac.aircraft.Track, ac.aircraft.GroundSpeed)
		}
	}
	
	// Render grid with colors
	for y := 0; y < skyHeight; y++ {
		radar.WriteString(borderStyle.Render("│"))
		for x := 0; x < skyWidth-2; x++ {
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
				radar.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Render(string(char)))
			case '·':
				radar.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("237")).Render(string(char)))
			case '→', '-':
				radar.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Render(string(char)))
			default:
				radar.WriteRune(char)
			}
		}
		radar.WriteString(borderStyle.Render("│"))
		radar.WriteString("\n")
	}
	
	radar.WriteString(borderStyle.Render("└" + strings.Repeat("─", skyWidth-2) + "┘"))
	
	return radar.String()
}

// drawCircle draws a circle on the grid using Bresenham's circle algorithm.
func drawCircle(grid [][]rune, cx, cy, radius int, char rune) {
	x := radius
	y := 0
	err := 0
	
	for x >= y {
		// Draw 8 octants
		setPixel(grid, cx+x, cy+y, char)
		setPixel(grid, cx+y, cy+x, char)
		setPixel(grid, cx-y, cy+x, char)
		setPixel(grid, cx-x, cy+y, char)
		setPixel(grid, cx-x, cy-y, char)
		setPixel(grid, cx-y, cy-x, char)
		setPixel(grid, cx+y, cy-x, char)
		setPixel(grid, cx+x, cy-y, char)
		
		y++
		err += 1 + 2*y
		if 2*(err-x)+1 > 0 {
			x--
			err += 1 - 2*x
		}
	}
}

// setPixel sets a pixel in the grid if it's within bounds.
func setPixel(grid [][]rune, x, y int, char rune) {
	if y >= 0 && y < len(grid) && x >= 0 && x < len(grid[0]) {
		if grid[y][x] == ' ' || grid[y][x] == '·' {
			grid[y][x] = char
		}
	}
}

// drawVelocityVectorRadar draws velocity vector in radar mode.
func drawVelocityVectorRadar(grid [][]rune, x, y int, trackDeg, speedKts float64) {
	// Vector length based on speed (normalized)
	length := int(speedKts / 150.0) + 1
	if length > 3 {
		length = 3
	}
	
	// Convert track to screen coordinates
	trackRad := trackDeg * math.Pi / 180.0
	
	for i := 1; i <= length; i++ {
		dx := int(float64(i) * math.Sin(trackRad))
		dy := -int(float64(i) * math.Cos(trackRad))
		
		nx, ny := x+dx, y+dy
		if ny >= 0 && ny < len(grid) && nx >= 0 && nx < len(grid[0]) {
			if grid[ny][nx] == ' ' || grid[ny][nx] == '·' {
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
