package main

import (
	"fmt"
	"math"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// SkyView is a custom tview primitive that renders the sky chart using tcell
type SkyView struct {
	*tview.Box
	app *App
}

// NewSkyView creates a new sky view with tcell rendering
func NewSkyView(app *App) *SkyView {
	sv := &SkyView{
		Box: tview.NewBox(),
		app: app,
	}
	sv.SetBorder(true).SetTitle(" Sky View - Alt/Az ")
	return sv
}

// Draw renders the sky view using tcell
func (sv *SkyView) Draw(screen tcell.Screen) {
	sv.Box.DrawForSubclass(screen, sv)

	// Get the inner bounds (excluding border)
	x, y, width, height := sv.GetInnerRect()

	// Calculate sky view parameters
	centerX := x + width/2
	centerY := y + height/2
	radius := width / 2
	if height < width {
		radius = height / 2
	}

	// Apply zoom
	sv.app.mu.RLock()
	zoom := sv.app.zoom
	sv.app.mu.RUnlock()
	
	radius = int(float64(radius) * zoom)

	// Define colors for tcell
	gridStyle := tcell.StyleDefault.Foreground(tcell.ColorGray)
	horizonStyle := tcell.StyleDefault.Foreground(tcell.ColorWhite)
	zenithStyle := tcell.StyleDefault.Foreground(tcell.ColorYellow)

	// Draw altitude rings (concentric circles)
	altitudes := []float64{30, 60, 90} // 0° (horizon) will be drawn separately
	for _, alt := range altitudes {
		// Calculate radius for this altitude using stereographic projection
		zenithAngle := (90.0 - alt) * math.Pi / 180.0
		ringRadius := int(2.0 * float64(radius) * math.Tan(zenithAngle/2.0))
		
		if alt == 90 {
			// Zenith marker - draw a + symbol
			screen.SetContent(centerX, centerY, '+', nil, zenithStyle)
		} else if ringRadius > 0 && ringRadius < radius {
			// Draw circle using Unicode box drawing characters
			drawCircle(screen, centerX, centerY, ringRadius, '·', gridStyle)
			// Add altitude label
			label := fmt.Sprintf("%.0f°", alt)
			for i, ch := range label {
				screen.SetContent(centerX+i-len(label)/2, centerY-ringRadius-1, ch, nil, gridStyle)
			}
		}
	}

	// Draw horizon (edge circle)
	drawCircle(screen, centerX, centerY, radius-1, '○', horizonStyle)

	// Draw azimuth lines (radial lines)
	azimuths := []struct {
		angle float64
		label string
	}{
		{0, "N"},
		{45, "NE"},
		{90, "E"},
		{135, "SE"},
		{180, "S"},
		{225, "SW"},
		{270, "W"},
		{315, "NW"},
	}

	for _, az := range azimuths {
		// Draw radial line from center to horizon
		angle := az.angle * math.Pi / 180.0
		endX := centerX + int(float64(radius)*math.Sin(angle))
		endY := centerY - int(float64(radius)*math.Cos(angle))
		drawLine(screen, centerX, centerY, endX, endY, '·', gridStyle)
		
		// Draw label at the edge
		labelX := centerX + int(float64(radius+1)*math.Sin(angle))
		labelY := centerY - int(float64(radius+1)*math.Cos(angle))
		for i, ch := range az.label {
			screen.SetContent(labelX+i-len(az.label)/2, labelY, ch, nil, horizonStyle)
		}
	}

	// Draw aircraft
	sv.app.mu.RLock()
	aircraft := sv.app.aircraft
	selectedIndex := sv.app.selectedIndex
	tracking := sv.app.tracking
	trackICAO := sv.app.trackICAO
	sv.app.mu.RUnlock()

	for i, ac := range aircraft {
		// Project aircraft position to screen coordinates using stereographic projection
		zenithAngle := (90.0 - ac.HorizCoord.Altitude) * math.Pi / 180.0
		r := 2.0 * float64(radius) * math.Tan(zenithAngle/2.0)
		azimuthRad := ac.HorizCoord.Azimuth * math.Pi / 180.0
		px := centerX + int(r*math.Sin(azimuthRad))
		py := centerY - int(r*math.Cos(azimuthRad))

		// Skip if outside view bounds
		if px < x || px >= x+width || py < y || py >= y+height {
			continue
		}

		// Determine symbol and style
		var symbol rune
		var style tcell.Style

		if tracking && ac.ICAO == trackICAO {
			// Tracking this aircraft
			symbol = '◉' // ◉
			style = tcell.StyleDefault.Foreground(tcell.ColorGreen)
		} else if i == selectedIndex {
			// Selected aircraft
			symbol = '●' // ●
			style = tcell.StyleDefault.Foreground(tcell.ColorYellow)
		} else {
			// Normal aircraft
			symbol = '○' // ○
			style = tcell.StyleDefault.Foreground(tcell.ColorLightBlue)
		}

		// Draw aircraft symbol
		screen.SetContent(px, py, symbol, nil, style)

		// Draw callsign label for selected or tracked aircraft
		if (i == selectedIndex) || (tracking && ac.ICAO == trackICAO) {
			label := ac.Callsign
			if label == "" {
				label = ac.ICAO
			}
			// Draw label to the right of the aircraft
			for j, ch := range label {
				screen.SetContent(px+j+2, py, ch, nil, style)
			}
		}

		// Draw velocity vector if aircraft is moving (simple arrow)
		if ac.Speed > 50 {
			vectorStyle := tcell.StyleDefault.Foreground(tcell.ColorDarkCyan)
			headingRad := ac.Heading * math.Pi / 180.0
			// Draw a short line in the direction of travel
			vx := px + int(3*math.Sin(headingRad))
			vy := py - int(3*math.Cos(headingRad))
			drawLine(screen, px, py, vx, vy, '→', vectorStyle) // →
		}
	}
}

// drawCircle draws a circle using Bresenham's circle algorithm
func drawCircle(screen tcell.Screen, cx, cy, radius int, char rune, style tcell.Style) {
	x := 0
	y := radius
	d := 3 - 2*radius

	for x <= y {
		// Draw 8 octants
		screen.SetContent(cx+x, cy+y, char, nil, style)
		screen.SetContent(cx-x, cy+y, char, nil, style)
		screen.SetContent(cx+x, cy-y, char, nil, style)
		screen.SetContent(cx-x, cy-y, char, nil, style)
		screen.SetContent(cx+y, cy+x, char, nil, style)
		screen.SetContent(cx-y, cy+x, char, nil, style)
		screen.SetContent(cx+y, cy-x, char, nil, style)
		screen.SetContent(cx-y, cy-x, char, nil, style)

		x++
		if d > 0 {
			y--
			d = d + 4*(x-y) + 10
		} else {
			d = d + 4*x + 6
		}
	}
}

// drawLine draws a line using Bresenham's line algorithm
func drawLine(screen tcell.Screen, x0, y0, x1, y1 int, char rune, style tcell.Style) {
	dx := abs(x1 - x0)
	dy := abs(y1 - y0)
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx - dy

	for {
		screen.SetContent(x0, y0, char, nil, style)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
	}
}

