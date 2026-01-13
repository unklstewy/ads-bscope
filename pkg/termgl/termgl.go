package termgl

/*
#cgo CFLAGS: -I${SRCDIR}/../../vendor/termgl
#cgo LDFLAGS: -lm
#include <stdlib.h>
#include "../../vendor/termgl/termgl.c"

// Helper function to create a simple pixel shader data structure
static inline void setup_simple_shader(TGLPixelShaderSimple *shader, uint8_t r, uint8_t g, uint8_t b) {
	shader->color.fg.flags = TGL_RGB24;
	shader->color.fg.color.rgb.r = r;
	shader->color.fg.color.rgb.g = g;
	shader->color.fg.color.rgb.b = b;
	shader->color.bkg.flags = TGL_NONE;
	shader->grad = &tgl_gradient_full;
}

// Helper to set RGB color in TGLPixFmt
static inline void set_fg_rgb(TGLPixFmt *fmt, uint8_t r, uint8_t g, uint8_t b) {
	fmt->fg.flags = TGL_RGB24;
	fmt->fg.color.rgb.r = r;
	fmt->fg.color.rgb.g = g;
	fmt->fg.color.rgb.b = b;
	fmt->bkg.flags = TGL_NONE;
}

// Helper wrappers for TGL drawing functions with simple pixel shader
static inline void go_tgl_point(TGL *tgl, TGLVert v0, TGLPixelShaderSimple *shader) {
	tgl_point(tgl, v0, tgl_pixel_shader_simple, shader);
}

static inline void go_tgl_line(TGL *tgl, TGLVert v0, TGLVert v1, TGLPixelShaderSimple *shader) {
	tgl_line(tgl, v0, v1, tgl_pixel_shader_simple, shader);
}

static inline void go_tgl_triangle_fill(TGL *tgl, TGLVert v0, TGLVert v1, TGLVert v2, TGLPixelShaderSimple *shader) {
	tgl_triangle_fill(tgl, v0, v1, v2, tgl_pixel_shader_simple, shader);
}
*/
import "C"
import (
	"fmt"
	"unsafe"
)

// Context wraps the TGL C context
type Context struct {
	tgl *C.TGL
	width, height uint
}

// Color represents an RGB color
type Color struct {
	R, G, B uint8
}

// Common colors
var (
	ColorBlack      = Color{0, 0, 0}
	ColorWhite      = Color{255, 255, 255}
	ColorRed        = Color{255, 0, 0}
	ColorGreen      = Color{0, 255, 0}
	ColorBlue       = Color{0, 0, 255}
	ColorYellow     = Color{255, 255, 0}
	ColorCyan       = Color{0, 255, 255}
	ColorMagenta    = Color{255, 0, 255}
	ColorGray       = Color{128, 128, 128}
	ColorDarkGray   = Color{64, 64, 64}
	ColorLightGray  = Color{192, 192, 192}
	ColorOrange     = Color{255, 165, 0}
	ColorDarkCyan   = Color{0, 139, 139}
	ColorLightBlue  = Color{173, 216, 230}
	ColorDarkRed    = Color{139, 0, 0}
)

// Boot initializes the terminal emulator.
// Must be called at the start of the program before any other TermGL functions.
func Boot() error {
	if C.tgl_boot() != 0 {
		return fmt.Errorf("failed to boot TermGL")
	}
	return nil
}

// Init initializes a TermGL context with the specified dimensions.
// width and height are in character cells.
func Init(width, height uint) (*Context, error) {
	tgl := C.tgl_init(C.uint(width), C.uint(height))
	if tgl == nil {
		return nil, fmt.Errorf("failed to initialize TermGL context")
	}

	ctx := &Context{
		tgl:    tgl,
		width:  width,
		height: height,
	}

	// Enable double-width characters for better aspect ratio
	// Enable output buffer for better performance
	C.tgl_enable(tgl, C.TGL_OUTPUT_BUFFER|C.TGL_DOUBLE_CHARS)

	return ctx, nil
}

// Delete frees the TermGL context
func (ctx *Context) Delete() {
	if ctx.tgl != nil {
		C.tgl_delete(ctx.tgl)
		ctx.tgl = nil
	}
}

// Clear clears the frame buffer and z-buffer
func (ctx *Context) Clear() {
	C.tgl_clear(ctx.tgl, C.TGL_FRAME_BUFFER|C.TGL_Z_BUFFER)
}

// ClearScreen clears the entire screen
func ClearScreen() error {
	if C.tgl_clear_screen() != 0 {
		return fmt.Errorf("failed to clear screen")
	}
	return nil
}

// Flush prints the frame buffer to the terminal
func (ctx *Context) Flush() error {
	if C.tgl_flush(ctx.tgl) != 0 {
		return fmt.Errorf("failed to flush TermGL buffer")
	}
	return nil
}

// Point draws a single point at the specified coordinates
func (ctx *Context) Point(x, y int, z float32, color Color) {
	var shader C.TGLPixelShaderSimple
	C.setup_simple_shader(&shader, C.uint8_t(color.R), C.uint8_t(color.G), C.uint8_t(color.B))

	vert := C.TGLVert{
		x: C.int(x),
		y: C.int(y),
		z: C.float(z),
		u: 255,
		v: 255,
	}

	C.go_tgl_point(ctx.tgl, vert, &shader)
}

// Line draws a line between two points
func (ctx *Context) Line(x0, y0, x1, y1 int, color Color) {
	var shader C.TGLPixelShaderSimple
	C.setup_simple_shader(&shader, C.uint8_t(color.R), C.uint8_t(color.G), C.uint8_t(color.B))

	v0 := C.TGLVert{
		x: C.int(x0),
		y: C.int(y0),
		z: 0.0,
		u: 255,
		v: 255,
	}

	v1 := C.TGLVert{
		x: C.int(x1),
		y: C.int(y1),
		z: 0.0,
		u: 255,
		v: 255,
	}

	C.go_tgl_line(ctx.tgl, v0, v1, &shader)
}

// Circle draws a circle outline using line segments
// centerX, centerY: center of circle in character coordinates
// radius: radius in character cells
// segments: number of line segments to approximate the circle (higher = smoother)
func (ctx *Context) Circle(centerX, centerY, radius int, color Color, segments int) {
	if segments < 8 {
		segments = 32 // Default to 32 segments for smooth circles
	}

	angleStep := 6.283185307179586 / float64(segments) // 2*PI / segments

	// Calculate first point
	var prevX, prevY int
	for i := 0; i <= segments; i++ {
		angle := float64(i) * angleStep
		x := centerX + int(float64(radius)*cosApprox(angle)+0.5)
		y := centerY + int(float64(radius)*sinApprox(angle)+0.5)

		if i > 0 {
			ctx.Line(prevX, prevY, x, y, color)
		}
		prevX, prevY = x, y
	}
}

// FilledCircle draws a filled circle using horizontal line segments
func (ctx *Context) FilledCircle(centerX, centerY, radius int, color Color) {
	// Draw filled circle using horizontal scanlines
	radiusSq := radius * radius
	for y := -radius; y <= radius; y++ {
		dy := y * y
		dx := int(sqrtApprox(float64(radiusSq - dy)))
		ctx.Line(centerX-dx, centerY+y, centerX+dx, centerY+y, color)
	}
}

// Triangle draws a triangle outline
func (ctx *Context) Triangle(x0, y0, x1, y1, x2, y2 int, color Color) {
	ctx.Line(x0, y0, x1, y1, color)
	ctx.Line(x1, y1, x2, y2, color)
	ctx.Line(x2, y2, x0, y0, color)
}

// FilledTriangle draws a filled triangle
func (ctx *Context) FilledTriangle(x0, y0, x1, y1, x2, y2 int, color Color) {
	var shader C.TGLPixelShaderSimple
	C.setup_simple_shader(&shader, C.uint8_t(color.R), C.uint8_t(color.G), C.uint8_t(color.B))

	v0 := C.TGLVert{x: C.int(x0), y: C.int(y0), z: 0.0, u: 255, v: 255}
	v1 := C.TGLVert{x: C.int(x1), y: C.int(y1), z: 0.0, u: 255, v: 255}
	v2 := C.TGLVert{x: C.int(x2), y: C.int(y2), z: 0.0, u: 255, v: 255}

	C.go_tgl_triangle_fill(ctx.tgl, v0, v1, v2, &shader)
}

// PutChar draws a single character at the specified position
func (ctx *Context) PutChar(x, y int, ch rune, color Color) {
	var pixFmt C.TGLPixFmt
	C.set_fg_rgb(&pixFmt, C.uint8_t(color.R), C.uint8_t(color.G), C.uint8_t(color.B))
	C.tgl_putchar(ctx.tgl, C.int(x), C.int(y), C.char(ch), pixFmt)
}

// PutString draws a string at the specified position
func (ctx *Context) PutString(x, y int, text string, color Color) {
	cstr := C.CString(text)
	defer C.free(unsafe.Pointer(cstr))

	var pixFmt C.TGLPixFmt
	C.set_fg_rgb(&pixFmt, C.uint8_t(color.R), C.uint8_t(color.G), C.uint8_t(color.B))
	C.tgl_puts(ctx.tgl, C.int(x), C.int(y), cstr, pixFmt)
}

// GetSize returns the dimensions of the context
func (ctx *Context) GetSize() (width, height uint) {
	return ctx.width, ctx.height
}

// Fast approximations for math functions (to avoid cgo overhead)
func sinApprox(x float64) float64 {
	// Normalize to [-PI, PI]
	for x > 3.141592653589793 {
		x -= 6.283185307179586
	}
	for x < -3.141592653589793 {
		x += 6.283185307179586
	}

	// Taylor series approximation (accurate enough for graphics)
	x2 := x * x
	return x * (1.0 - x2/6.0*(1.0-x2/20.0*(1.0-x2/42.0)))
}

func cosApprox(x float64) float64 {
	return sinApprox(x + 1.5707963267948966) // cos(x) = sin(x + PI/2)
}

func sqrtApprox(x float64) float64 {
	if x <= 0 {
		return 0
	}
	// Newton-Raphson method for fast sqrt
	guess := x
	for i := 0; i < 5; i++ {
		guess = (guess + x/guess) / 2.0
	}
	return guess
}
