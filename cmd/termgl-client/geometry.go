package main

import (
	"math"

	"github.com/unklstewy/ads-bscope/pkg/termgl"
)

// Point represents a 2D point on the screen
type Point struct {
	X, Y int
}

// StereographicProjection converts Alt/Az coordinates to screen X/Y coordinates
// using stereographic projection centered on zenith.
// Altitude: 0° = horizon (edge), 90° = zenith (center)
// Azimuth: 0° = North = top, 90° = East = right, etc.
func StereographicProjection(altitude, azimuth float64, centerX, centerY, radius float64) Point {
	// Convert to radians
	azRad := azimuth * math.Pi / 180.0

	// Stereographic projection formula
	// r = 2R * tan((90° - alt) / 2) = 2R * tan(zen / 2)
	// where zen = zenith angle = 90° - altitude
	zenithAngle := (90.0 - altitude) * math.Pi / 180.0
	r := 2.0 * radius * math.Tan(zenithAngle/2.0)

	// Convert polar (r, azimuth) to Cartesian (x, y)
	// Azimuth 0° = North = up = negative Y
	// Azimuth 90° = East = right = positive X
	x := r * math.Sin(azRad)
	y := -r * math.Cos(azRad) // Negative because Y increases downward

	// Adjust for terminal character aspect ratio (~2:1 height:width)
	// Scale X by 0.5 to make circles appear round
	const aspectRatio = 0.5
	x = x * aspectRatio

	return Point{
		X: int(centerX + x),
		Y: int(centerY + y),
	}
}

// DrawCircle draws a smooth circle using TermGL
func DrawCircle(ctx *termgl.Context, centerX, centerY, radius int, color termgl.Color) {
	ctx.Circle(centerX, centerY, radius, color, 64) // 64 segments for smooth circle
}

// DrawLine draws a smooth line using TermGL
func DrawLine(ctx *termgl.Context, x0, y0, x1, y1 int, color termgl.Color) {
	ctx.Line(x0, y0, x1, y1, color)
}

// DrawRadialLine draws a line from center at a specific azimuth angle
func DrawRadialLine(ctx *termgl.Context, centerX, centerY int, azimuth float64, length int, color termgl.Color) {
	azRad := azimuth * math.Pi / 180.0

	// Calculate end point
	// Azimuth 0° = North = up = negative Y
	// Apply aspect ratio correction for X coordinate
	const aspectRatio = 0.5
	dx := float64(length) * math.Sin(azRad) * aspectRatio
	dy := -float64(length) * math.Cos(azRad)

	x1 := centerX + int(dx)
	y1 := centerY + int(dy)

	DrawLine(ctx, centerX, centerY, x1, y1, color)
}

// DrawText draws text at a specific position using TermGL
func DrawText(ctx *termgl.Context, x, y int, text string, color termgl.Color) {
	ctx.PutString(x, y, text, color)
}

// DrawAltitudeRing draws an altitude ring at a specific angle with label
func DrawAltitudeRing(ctx *termgl.Context, centerX, centerY, radius int, altitudeDeg float64, color termgl.Color) {
	// Draw the circle
	DrawCircle(ctx, centerX, centerY, radius, color)

	// Add label at top (North)
	label := formatAltitude(altitudeDeg)
	labelX := centerX - len(label)/2
	labelY := centerY - radius - 1

	DrawText(ctx, labelX, labelY, label, color)
}

// DrawAzimuthLine draws an azimuth line with label
func DrawAzimuthLine(ctx *termgl.Context, centerX, centerY int, azimuthDeg float64, length int, color termgl.Color, label string) {
	// Draw the radial line
	DrawRadialLine(ctx, centerX, centerY, azimuthDeg, length, color)

	// Calculate label position (slightly beyond the line end)
	azRad := azimuthDeg * math.Pi / 180.0
	const aspectRatio = 0.5
	dx := float64(length+3) * math.Sin(azRad) * aspectRatio
	dy := -float64(length+3) * math.Cos(azRad)

	labelX := centerX + int(dx) - len(label)/2
	labelY := centerY + int(dy)

	DrawText(ctx, labelX, labelY, label, color)
}

// formatAltitude formats an altitude angle for display
func formatAltitude(alt float64) string {
	if alt == 0 {
		return "Horizon"
	} else if alt == 90 {
		return "Zenith"
	}
	return string(rune('0' + int(alt/10))) + string(rune('0' + int(alt)%10)) + "°"
}

// Removed setPixel - TermGL handles pixel-level drawing internally

// abs returns the absolute value of an integer
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// DrawAircraftSymbol draws an aircraft symbol at the given position
func DrawAircraftSymbol(ctx *termgl.Context, x, y int, symbol rune, color termgl.Color) {
	ctx.PutChar(x, y, symbol, color)
}

// DrawAircraftLabel draws a label next to an aircraft
func DrawAircraftLabel(ctx *termgl.Context, x, y int, label string, color termgl.Color) {
	// Draw label to the right of the aircraft position
	DrawText(ctx, x+2, y, label, color)
}

// DrawHorizonLine draws a bold horizon line at the edge
func DrawHorizonLine(ctx *termgl.Context, centerX, centerY, radius int, color termgl.Color) {
	// Draw a thicker circle for horizon (draw multiple circles with small offsets)
	DrawCircle(ctx, centerX, centerY, radius, color)
	DrawCircle(ctx, centerX, centerY, radius-1, color)
}

// DrawZenithMarker draws a marker at the zenith (center)
func DrawZenithMarker(ctx *termgl.Context, centerX, centerY int, color termgl.Color) {
	// Draw a small cross at zenith using lines
	ctx.Line(centerX-2, centerY, centerX+2, centerY, color)
	ctx.Line(centerX, centerY-2, centerX, centerY+2, color)
}

// CalculateSkyViewBounds calculates the optimal bounds for the sky view
// Returns centerX, centerY, and radius
func CalculateSkyViewBounds(width, height int) (int, int, int) {
	// Leave margins for borders
	marginX := 2
	marginY := 1

	effectiveWidth := width - 2*marginX
	effectiveHeight := height - 2*marginY

	// Account for aspect ratio (characters are ~2:1 height:width)
	// So effective width for circles is actually width * 0.5
	const aspectRatio = 0.5
	adjustedWidth := int(float64(effectiveWidth) * aspectRatio)

	// Choose radius based on smaller dimension
	radius := effectiveHeight / 2
	if adjustedWidth < effectiveHeight {
		radius = adjustedWidth
	}

	// Make sure radius is reasonable
	if radius < 10 {
		radius = 10
	}
	if radius > 40 {
		radius = 40
	}

	centerX := width / 2
	centerY := height / 2

	return centerX, centerY, radius
}

// DrawVelocityVector draws a velocity vector (arrow) for an aircraft
func DrawVelocityVector(ctx *termgl.Context, x, y int, heading float64, speed float64, color termgl.Color) {
	// Vector length based on speed (normalized)
	length := int(speed/150.0) + 1
	if length > 6 {
		length = 6
	}
	if length < 2 {
		length = 2
	}

	// Convert heading to screen coordinates
	// Heading 0° = North = up
	headingRad := heading * math.Pi / 180.0
	const aspectRatio = 0.5

	dx := int(float64(length) * math.Sin(headingRad) * aspectRatio)
	dy := -int(float64(length) * math.Cos(headingRad))

	// Draw line for velocity vector
	ctx.Line(x, y, x+dx, y+dy, color)

	// Draw arrowhead at end
	arrowX, arrowY := x+dx, y+dy
	ctx.PutChar(arrowX, arrowY, '→', color)
}
