package main

import (
	"math"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/unklstewy/ads-bscope/pkg/termgl"
)

// SkyView is a custom tview primitive that renders the sky chart using TermGL
type SkyView struct {
	*tview.Box
	app    *App
	tglCtx *termgl.Context
}

// NewSkyView creates a new sky view with TermGL rendering
func NewSkyView(app *App) *SkyView {
	sv := &SkyView{
		Box: tview.NewBox(),
		app: app,
	}
	sv.SetBorder(true).SetTitle(" Sky View - Alt/Az (TermGL) ")
	return sv
}

// Draw renders the sky view using TermGL
func (sv *SkyView) Draw(screen tcell.Screen) {
	sv.Box.DrawForSubclass(screen, sv)

	// Get the inner bounds (excluding border)
	x, y, width, height := sv.GetInnerRect()

	// Initialize or resize TermGL context if needed
	if sv.tglCtx == nil {
		// First time initialization - boot TermGL
		if err := termgl.Boot(); err != nil {
			// Fall back to error message
			return
		}
		
		ctx, err := termgl.Init(uint(width), uint(height))
		if err != nil {
			return
		}
		sv.tglCtx = ctx
	}

	// Clear TermGL buffer
	sv.tglCtx.Clear()

	// Calculate sky view parameters
	centerX, centerY, radius := CalculateSkyViewBounds(width, height)

	// Apply zoom
	sv.app.mu.RLock()
	zoom := sv.app.zoom
	sv.app.mu.RUnlock()
	
	radius = int(float64(radius) * zoom)

	// Define colors for TermGL
	gridColor := termgl.ColorGray
	horizonColor := termgl.ColorWhite
	zenithColor := termgl.ColorYellow

	// Draw altitude rings (concentric circles)
	altitudes := []float64{30, 60, 90} // 0° (horizon) will be drawn separately
	for _, alt := range altitudes {
		// Calculate radius for this altitude using stereographic projection
		zenithAngle := (90.0 - alt) * 3.14159 / 180.0
		ringRadius := int(2.0 * float64(radius) * tan(zenithAngle/2.0))
		
		if alt == 90 {
			// Zenith marker
			DrawZenithMarker(sv.tglCtx, centerX, centerY, zenithColor)
		} else {
			DrawAltitudeRing(sv.tglCtx, centerX, centerY, ringRadius, alt, gridColor)
		}
	}

	// Draw horizon (edge circle)
	DrawHorizonLine(sv.tglCtx, centerX, centerY, radius, horizonColor)

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
		DrawAzimuthLine(sv.tglCtx, centerX, centerY, az.angle, radius, gridColor, az.label)
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
		var color termgl.Color

		if tracking && ac.ICAO == trackICAO {
			// Tracking this aircraft
			symbol = '◉'
			color = termgl.ColorGreen
		} else if i == selectedIndex {
			// Selected aircraft
			symbol = '●'
			color = termgl.ColorYellow
		} else {
			// Normal aircraft
			symbol = '○'
			color = termgl.ColorLightBlue
		}

		// Draw aircraft symbol
		DrawAircraftSymbol(sv.tglCtx, pt.X, pt.Y, symbol, color)

		// Draw callsign label for selected or tracked aircraft
		if (i == selectedIndex) || (tracking && ac.ICAO == trackICAO) {
			label := ac.Callsign
			if label == "" {
				label = ac.ICAO
			}
			DrawAircraftLabel(sv.tglCtx, pt.X, pt.Y, label, color)
		}

		// Draw velocity vector if aircraft is moving
		if ac.Speed > 50 {
			vectorColor := termgl.ColorDarkCyan
			DrawVelocityVector(sv.tglCtx, pt.X, pt.Y, ac.Heading, ac.Speed, vectorColor)
		}
	}

	// Render TermGL buffer to tcell screen
	// We need to manually copy TermGL's output to tcell's screen
	// This is a workaround since TermGL outputs directly to stdout
	// For now, we'll flush TermGL which will overwrite the terminal
	// In a production system, we'd need a proper integration layer
	sv.tglCtx.Flush()

	// Draw altitude limit zones if configured
	sv.drawAltitudeLimitZones(centerX, centerY, radius)
}

// drawAltitudeLimitZones draws colored zones for altitude limits
func (sv *SkyView) drawAltitudeLimitZones(centerX, centerY, radius int) {
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
		color := termgl.ColorDarkRed
		if minRadius > 0 && minRadius < radius {
			DrawCircle(sv.tglCtx, centerX, centerY, minRadius, color)
		}
	}

	// Red zone: 80-90° (too high, field rotation)
	if maxAlt < 90 && maxAlt > 0 {
		zenithAngle := (90.0 - maxAlt) * 3.14159 / 180.0
		maxRadius := int(2.0 * float64(radius) * tan(zenithAngle/2.0))

		// Draw subtle background for high altitude zone
		color := termgl.ColorDarkRed
		if maxRadius > 0 && maxRadius < radius/2 {
			DrawCircle(sv.tglCtx, centerX, centerY, maxRadius, color)
		}
	}
}

// tan is a helper for tangent calculation
func tan(x float64) float64 {
	return math.Tan(x)
}
