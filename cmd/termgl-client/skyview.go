package main

import (
	"math"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// SkyView is a custom tview primitive that renders the sky chart
type SkyView struct {
	*tview.Box
	app *App
}

// NewSkyView creates a new sky view
func NewSkyView(app *App) *SkyView {
	sv := &SkyView{
		Box: tview.NewBox(),
		app: app,
	}
	sv.SetBorder(true).SetTitle(" Sky View - Alt/Az ")
	return sv
}

// Draw renders the sky view
func (sv *SkyView) Draw(screen tcell.Screen) {
	sv.Box.DrawForSubclass(screen, sv)

	// Get the inner bounds (excluding border)
	x, y, width, height := sv.GetInnerRect()

	// Calculate sky view parameters
	centerX, centerY, radius := CalculateSkyViewBounds(width, height)
	centerX += x // Adjust for view position
	centerY += y

	// Apply zoom
	sv.app.mu.RLock()
	zoom := sv.app.zoom
	sv.app.mu.RUnlock()
	
	radius = int(float64(radius) * zoom)

	// Define styles
	gridStyle := tcell.StyleDefault.Foreground(tcell.ColorGray)
	horizonStyle := tcell.StyleDefault.Foreground(tcell.ColorWhite).Bold(true)
	zenithStyle := tcell.StyleDefault.Foreground(tcell.ColorYellow).Bold(true)

	// Draw altitude rings (concentric circles)
	altitudes := []float64{30, 60, 90} // 0° (horizon) will be drawn separately
	for _, alt := range altitudes {
		// Calculate radius for this altitude using stereographic projection
		zenithAngle := (90.0 - alt) * 3.14159 / 180.0
		ringRadius := int(2.0 * float64(radius) * tan(zenithAngle/2.0))
		
		if alt == 90 {
			// Zenith marker
			DrawZenithMarker(screen, centerX, centerY, zenithStyle)
		} else {
			DrawAltitudeRing(screen, centerX, centerY, ringRadius, alt, gridStyle)
		}
	}

	// Draw horizon (edge circle)
	DrawHorizonLine(screen, centerX, centerY, radius, horizonStyle)

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
		DrawAzimuthLine(screen, centerX, centerY, az.angle, radius, gridStyle, az.label)
	}

	// Draw aircraft
	sv.app.mu.RLock()
	aircraft := sv.app.aircraft
	selectedIndex := sv.app.selectedIndex
	tracking := sv.app.tracking
	trackICAO := sv.app.trackICAO
	sv.app.mu.RUnlock()

	for i, ac := range aircraft {
		// Project aircraft position to screen coordinates
		pt := StereographicProjection(
			ac.HorizCoord.Altitude,
			ac.HorizCoord.Azimuth,
			float64(centerX),
			float64(centerY),
			float64(radius),
		)

		// Skip if outside view bounds
		if pt.X < x || pt.X >= x+width || pt.Y < y || pt.Y >= y+height {
			continue
		}

		// Determine symbol and color
		var symbol rune
		var style tcell.Style

		if tracking && ac.ICAO == trackICAO {
			// Tracking this aircraft
			symbol = '◉'
			style = tcell.StyleDefault.Foreground(tcell.ColorGreen).Bold(true)
		} else if i == selectedIndex {
			// Selected aircraft
			symbol = '●'
			style = tcell.StyleDefault.Foreground(tcell.ColorYellow).Bold(true)
		} else {
			// Normal aircraft
			symbol = '○'
			style = tcell.StyleDefault.Foreground(tcell.ColorLightBlue)
		}

		// Draw aircraft symbol
		DrawAircraftSymbol(screen, pt.X, pt.Y, symbol, style)

		// Draw callsign label for selected or tracked aircraft
		if (i == selectedIndex) || (tracking && ac.ICAO == trackICAO) {
			label := ac.Callsign
			if label == "" {
				label = ac.ICAO
			}
			DrawAircraftLabel(screen, pt.X, pt.Y, label, style)
		}

		// Draw velocity vector if aircraft is moving
		if ac.Speed > 50 {
			vectorStyle := tcell.StyleDefault.Foreground(tcell.ColorDarkCyan)
			DrawVelocityVector(screen, pt.X, pt.Y, ac.Heading, ac.Speed, vectorStyle)
		}
	}

	// Draw altitude limit zones if configured
	sv.drawAltitudeLimitZones(screen, centerX, centerY, radius, x, y, width, height)
}

// drawAltitudeLimitZones draws colored zones for altitude limits
func (sv *SkyView) drawAltitudeLimitZones(screen tcell.Screen, centerX, centerY, radius, viewX, viewY, viewWidth, viewHeight int) {
	sv.app.mu.RLock()
	minAlt := sv.app.minAlt
	maxAlt := sv.app.maxAlt
	sv.app.mu.RUnlock()

	// Only draw if we have altitude limits configured
	if minAlt == 0 && maxAlt == 0 {
		return
	}

	// Calculate radii for limit zones
	// Red zone: 0-20° (too low)
	if minAlt > 0 {
		zenithAngle := (90.0 - minAlt) * 3.14159 / 180.0
		minRadius := int(2.0 * float64(radius) * tan(zenithAngle/2.0))

		// Draw subtle background for low altitude zone
		// (This is simplified - in practice you'd fill the area between horizon and minRadius)
		style := tcell.StyleDefault.Foreground(tcell.ColorDarkRed)
		if minRadius > 0 && minRadius < radius {
			DrawCircle(screen, centerX, centerY, minRadius, style)
		}
	}

	// Red zone: 80-90° (too high, field rotation)
	if maxAlt < 90 && maxAlt > 0 {
		zenithAngle := (90.0 - maxAlt) * 3.14159 / 180.0
		maxRadius := int(2.0 * float64(radius) * tan(zenithAngle/2.0))

		// Draw subtle background for high altitude zone
		style := tcell.StyleDefault.Foreground(tcell.ColorDarkRed)
		if maxRadius > 0 && maxRadius < radius/2 {
			DrawCircle(screen, centerX, centerY, maxRadius, style)
		}
	}
}

// tan is a helper for tangent calculation
func tan(x float64) float64 {
	return math.Tan(x)
}
