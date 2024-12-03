// File given by university, self-commented

package sdl

import (
	"fmt"

	"github.com/veandco/go-sdl2/sdl" // SDL2 library for graphical rendering and event handling
	"uk.ac.bris.cs/gameoflife/util"  // SDL2 library for graphical rendering and event handling
)

// Window represents a graphical window with SDL components for rendering the Game of Life grid.
type Window struct {
	Width, Height int32         // Dimensions of the window (in pixels)
	window        *sdl.Window   // Pointer to the SDL window object
	renderer      *sdl.Renderer // Pointer to the SDL renderer object
	texture       *sdl.Texture  // Pointer to the SDL texture for pixel data
	pixels        []byte        // Slice of bytes representing pixel data (ARGB format)
}

// filterEvent determines which SDL events should be processed.
// Returns true for keyboard presses (KEYDOWN) or quit events (QUIT).
func filterEvent(e sdl.Event, userdata interface{}) bool {
	return e.GetType() == sdl.KEYDOWN || e.GetType() == sdl.QUIT
}

// NewWindow creates and initialises a new SDL window with a renderer and texture.
// - width, height: Dimensions of the window.
// Returns a pointer to the newly created Window struct.
func NewWindow(width, height int32) *Window {
	// Initialise SDL.
	err := sdl.Init(sdl.INIT_EVERYTHING)
	util.Check(err)

	// Create the SDL window centered on the screen with specified dimensions.
	window, err := sdl.CreateWindow(
		"GOL GUI",
		sdl.WINDOWPOS_CENTERED,
		sdl.WINDOWPOS_CENTERED,
		width, height,
		sdl.WINDOW_SHOWN)
	util.Check(err)

	// Create the SDL renderer for the window.
	renderer, err := sdl.CreateRenderer(window, -1, sdl.WINDOW_SHOWN)
	util.Check(err)

	// Set rendering quality to linear scaling for better visuals.
	sdl.SetHint(sdl.HINT_RENDER_SCALE_QUALITY, "linear")
	err = renderer.SetLogicalSize(width, height)
	util.Check(err)

	// Create a texture for rendering pixels in ARGB8888 format.
	texture, err := renderer.CreateTexture(sdl.PIXELFORMAT_ARGB8888, sdl.TEXTUREACCESS_STATIC, width, height)
	util.Check(err)

	// Set the SDL event filter to handle only relevant events.
	sdl.SetEventFilterFunc(filterEvent, nil)

	// Return the initialised Window object with pixel data storage.
	return &Window{
		width,
		height,
		window,
		renderer,
		texture,
		make([]byte, width*height*4), // Allocate space for pixel data (4 bytes per pixel for ARGB).
	}
}

// Destroy cleans up resources allocated for the SDL window, renderer, and texture.
// Also shuts down SDL subsystems.
func (w *Window) Destroy() {
	err := w.texture.Destroy() // Destroy the texture.
	util.Check(err)

	err = w.renderer.Destroy() // Destroy the renderer.
	util.Check(err)

	err = w.window.Destroy() // Destroy the window.
	util.Check(err)

	sdl.Quit() // Quit SDL subsystems.
}

// RenderFrame updates the window with the current pixel data.
// This includes uploading pixels to the texture and presenting them on the screen.
func (w *Window) RenderFrame() {
	// Update the texture with the current pixel data.
	err := w.texture.Update(nil, w.pixels, int(w.Width*4)) // Width*4 because each pixel uses 4 bytes (ARGB).
	util.Check(err)

	// Clear the renderer before drawing.
	err = w.renderer.Clear()
	util.Check(err)

	// Copy the texture to the renderer.
	err = w.renderer.Copy(w.texture, nil, nil)
	util.Check(err)

	// Present the rendered frame on the screen.
	w.renderer.Present()
}

// PollEvent returns the next event or nil if there are no events.
func (w *Window) PollEvent() sdl.Event {
	return sdl.PollEvent()
}

// SetPixel sets a specific pixel (x, y) in the grid to white (ARGB = 0xFFFFFFFF).
func (w *Window) SetPixel(x, y int) {
	width := int(w.Width)
	// Set the 4 bytes (ARGB) for the specified pixel to white.
	w.pixels[4*(y*width+x)+0] = 0xFF // Alpha
	w.pixels[4*(y*width+x)+1] = 0xFF // Red
	w.pixels[4*(y*width+x)+2] = 0xFF // Green
	w.pixels[4*(y*width+x)+3] = 0xFF // Blue
}

// FlipPixel toggles the state of a specific pixel (x, y) by inverting its ARGB values.
func (w *Window) FlipPixel(x, y int) {
	// Check that the coordinates are within the bounds of the window.
	if x < 0 || y < 0 || x >= int(w.Width) || y >= int(w.Height) {
		panic(fmt.Sprintf("CellFlipped event at (%d, %d) is outside the bounds of the window.", x, y))
	}

	width := int(w.Width)

	// Invert the 4 bytes (ARGB) for the specified pixel using bitwise NOT.
	w.pixels[4*(y*width+x)+0] = ^w.pixels[4*(y*width+x)+0] // Alpha
	w.pixels[4*(y*width+x)+1] = ^w.pixels[4*(y*width+x)+1] // Red
	w.pixels[4*(y*width+x)+2] = ^w.pixels[4*(y*width+x)+2] // Green
	w.pixels[4*(y*width+x)+3] = ^w.pixels[4*(y*width+x)+3] // Blue
}

// CountPixels counts the number of white pixels (ARGB = 0xFFFFFFFF) in the grid.
// Returns the count of white pixels.
func (w *Window) CountPixels() int {
	count := 0
	// Iterate over all pixels (4 bytes per pixel).
	for i := 0; i < int(w.Width)*int(w.Height)*4; i += 4 {
		if w.pixels[i] == 0xFF { // Check the Alpha byte for a white pixel.
			count++
		}
	}
	return count
}

// ClearPixels resets all pixels in the grid to black (ARGB = 0x00000000).
func (w *Window) ClearPixels() {
	// Set every byte in the pixel array to 0 (black).
	for i := range w.pixels {
		w.pixels[i] = 0
	}
}
