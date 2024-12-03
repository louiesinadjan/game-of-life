// File given by university, self-commented

package sdl

import (
	"fmt"
	"github.com/veandco/go-sdl2/sdl" // SDL2 library for graphical rendering and event handling
	"uk.ac.bris.cs/gameoflife/gol"
)

func Run(p gol.Params, events <-chan gol.Event, keyPresses chan<- rune) {
	// Create a new window for rendering the simulation grid.
	w := NewWindow(int32(p.ImageWidth), int32(p.ImageHeight))

sdlLoop:
	for {
		event := w.PollEvent()
		if event != nil {
			// Handle specific keyboard events.
			switch e := event.(type) {
			case *sdl.KeyboardEvent: // Check if the event is a keyboard event.
				switch e.Keysym.Sym {
				case sdl.K_p:
					keyPresses <- 'p'
				case sdl.K_s:
					keyPresses <- 's'
				case sdl.K_q:
					keyPresses <- 'q'
				case sdl.K_k:
					keyPresses <- 'k'
				}
			}
		}

		// Handle events from the simulation.
		select {
		case event, ok := <-events: // Check for simulation events from the events channel.
			if !ok {
				// If the events channel is closed, destroy the window and exit the loop.
				w.Destroy()
				break sdlLoop
			}
			switch e := event.(type) {
			case gol.CellFlipped:
				w.FlipPixel(e.Cell.X, e.Cell.Y)
			case gol.TurnComplete:
				w.RenderFrame()
			case gol.FinalTurnComplete:
				w.Destroy()
				break sdlLoop
			default:
				if len(event.String()) > 0 {
					fmt.Printf("Completed Turns %-8v%v\n", event.GetCompletedTurns(), event)
				}
			}
		default:
			// No event to handle, continue looping.
			break
		}
	}

}
