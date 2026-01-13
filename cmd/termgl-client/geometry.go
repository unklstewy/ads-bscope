package main

import (
	"math"

	"github.com/gdamore/tcell/v2"
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

// DrawCircle draws a circle on the screen using Bresenham's circle algorithm
func DrawCircle(screen tcell.Screen, centerX, centerY, radius int, style tcell.Style) {
	// Bresenham's circle algorithm
	x := radius
	y := 0
	err := 0

	for x >= y {
		// Draw 8 octants
		setPixel(screen, centerX+x, centerY+y, style)
		setPixel(screen, centerX+y, centerY+x, style)
		setPixel(screen, centerX-y, centerY+x, style)
		setPixel(screen, centerX-x, centerY+y, style)
		setPixel(screen, centerX-x, centerY-y, style)
		setPixel(screen, centerX-y, centerY-x, style)
		setPixel(screen, centerX+y, centerY-x, style)
		setPixel(screen, centerX+x, centerY-y, style)

		y++
		err += 1 + 2*y
		if 2*(err-x)+1 > 0 {
			x--
			err += 1 - 2*x
		}
	}
}

// DrawLine draws a line between two points using Bresenham's line algorithm
func DrawLine(screen tcell.Screen, x0, y0, x1, y1 int, style tcell.Style) {
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
		setPixel(screen, x0, y0, style)

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

// DrawRadialLine draws a line from center at a specific azimuth angle
func DrawRadialLine(screen tcell.Screen, centerX, centerY int, azimuth float64, length int, style tcell.Style) {
	azRad := azimuth * math.Pi / 180.0

	// Calculate end point
	// Azimuth 0° = North = up = negative Y
	// Apply aspect ratio correction for X coordinate
	const aspectRatio = 0.5
	dx := float64(length) * math.Sin(azRad) * aspectRatio
	dy := -float64(length) * math.Cos(azRad)

	x1 := centerX + int(dx)
	y1 := centerY + int(dy)

	DrawLine(screen, centerX, centerY, x1, y1, style)
}

// DrawText draws text at a specific position
func DrawText(screen tcell.Screen, x, y int, text string, style tcell.Style) {
	width, height := screen.Size()
	for i, ch := range text {
		if x+i >= width || y >= height || x+i < 0 || y < 0 {
			continue
		}
		screen.SetContent(x+i, y, ch, nil, style)
	}
}

// DrawAltitudeRing draws an altitude ring at a specific angle with label
func DrawAltitudeRing(screen tcell.Screen, centerX, centerY, radius int, altitudeDeg float64, style tcell.Style) {
	// Draw the circle
	DrawCircle(screen, centerX, centerY, radius, style)

	// Add label at top (North)
	label := []rune(formatAltitude(altitudeDeg))
	labelX := centerX - len(label)/2
	labelY := centerY - radius - 1
	width, height := screen.Size()

	if labelY >= 0 && labelY < height && labelX >= 0 && labelX+len(label) < width {
		for i, ch := range label {
			screen.SetContent(labelX+i, labelY, ch, nil, style)
		}
	}
}

// DrawAzimuthLine draws an azimuth line with label
func DrawAzimuthLine(screen tcell.Screen, centerX, centerY int, azimuthDeg float64, length int, style tcell.Style, label string) {
	// Draw the radial line
	DrawRadialLine(screen, centerX, centerY, azimuthDeg, length, style)

	// Calculate label position (slightly beyond the line end)
	azRad := azimuthDeg * math.Pi / 180.0
	const aspectRatio = 0.5
	dx := float64(length+3) * math.Sin(azRad) * aspectRatio
	dy := -float64(length+3) * math.Cos(azRad)

	labelX := centerX + int(dx) - len(label)/2
	labelY := centerY + int(dy)

	DrawText(screen, labelX, labelY, label, style)
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

// setPixel sets a single pixel on the screen
func setPixel(screen tcell.Screen, x, y int, style tcell.Style) {
	width, height := screen.Size()
	if x >= 0 && x < width && y >= 0 && y < height {
		screen.SetContent(x, y, '·', nil, style)
	}
}

// abs returns the absolute value of an integer
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// DrawAircraftSymbol draws an aircraft symbol at the given position
func DrawAircraftSymbol(screen tcell.Screen, x, y int, symbol rune, style tcell.Style) {
	width, height := screen.Size()
	if x >= 0 && x < width && y >= 0 && y < height {
		screen.SetContent(x, y, symbol, nil, style)
	}
}

// DrawAircraftLabel draws a label next to an aircraft
func DrawAircraftLabel(screen tcell.Screen, x, y int, label string, style tcell.Style) {
	// Draw label to the right of the aircraft position
	DrawText(screen, x+2, y, label, style)
}

// DrawHorizonLine draws a bold horizon line at the edge
func DrawHorizonLine(screen tcell.Screen, centerX, centerY, radius int, style tcell.Style) {
	// Draw a thicker circle for horizon (draw multiple circles with small offsets)
	DrawCircle(screen, centerX, centerY, radius, style)
	DrawCircle(screen, centerX, centerY, radius-1, style)
}

// DrawZenithMarker draws a marker at the zenith (center)
func DrawZenithMarker(screen tcell.Screen, centerX, centerY int, style tcell.Style) {
	// Draw a small cross at zenith
	screen.SetContent(centerX, centerY, '+', nil, style)
	screen.SetContent(centerX-1, centerY, '─', nil, style)
	screen.SetContent(centerX+1, centerY, '─', nil, style)
	screen.SetContent(centerX, centerY-1, '│', nil, style)
	screen.SetContent(centerX, centerY+1, '│', nil, style)
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
func DrawVelocityVector(screen tcell.Screen, x, y int, heading float64, speed float64, style tcell.Style) {
	// Vector length based on speed (normalized)
	length := int(speed/150.0) + 1
	if length > 4 {
		length = 4
	}
	if length < 1 {
		length = 1
	}

	// Convert heading to screen coordinates
	// Heading 0° = North = up
	headingRad := heading * math.Pi / 180.0
	const aspectRatio = 0.5

	for i := 1; i <= length; i++ {
		dx := int(float64(i) * math.Sin(headingRad) * aspectRatio)
		dy := -int(float64(i) * math.Cos(headingRad))

		nx, ny := x+dx, y+dy
		width, height := screen.Size()
		if nx >= 0 && nx < width && ny >= 0 && ny < height {
			if i == length {
				screen.SetContent(nx, ny, '→', nil, style)
			} else {
				screen.SetContent(nx, ny, '-', nil, style)
			}
		}
	}
}
