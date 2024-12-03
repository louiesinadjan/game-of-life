package main

import (
	"flag"
	"fmt"
	"runtime"
	"uk.ac.bris.cs/gameoflife/gol"
	"uk.ac.bris.cs/gameoflife/sdl"
)

// main is the function called when starting Game of Life with 'go run .'
func main() {
	runtime.LockOSThread()

	// Set the maximum number of CPU cores to be used by the Go runtime.
	runtime.GOMAXPROCS(16)

	var params gol.Params

	flag.IntVar(
		&params.Threads,
		"t",
		8,
		"Specify the number of worker threads to use. Defaults to 8.")

	flag.IntVar(
		&params.ImageWidth,
		"w",
		512,
		"Specify the width of the image. Defaults to 512.")

	flag.IntVar(
		&params.ImageHeight,
		"h",
		512,
		"Specify the height of the image. Defaults to 512.")

	flag.IntVar(
		&params.Turns,
		"turns",
		10000000000,
		"Specify the number of turns to process. Defaults to 10000000000.")

	noVis := flag.Bool(
		"noVis",
		false,
		"Disables the SDL window, so there is no visualisation during the tests.")

	random := flag.Bool(
		"random",
		false,
		"Randomly populates alive cells an initial matrix .")

	flag.Parse()

	fmt.Println("Threads:", params.Threads)
	fmt.Println("Width:", params.ImageWidth)
	fmt.Println("Height:", params.ImageHeight)

	// Create a buffered channel to handle key press inputs.
	// Buffer size of 10 allows storing up to 10 rune inputs before blocking.
	keyPresses := make(chan rune, 10)

	// Create a buffered channel to handle events generated during the simulation.
	// Buffer size of 1000 allows storing up to 1000 events before blocking.
	events := make(chan gol.Event, 1000)

	// Start the Game of Life simulation in a separate goroutine.
	// - `params`: The simulation parameters (threads, grid size, turns).
	// - `events`: The channel to send simulation events.
	// - `keyPresses`: The channel to receive key press inputs.
	go gol.Run(params, events, keyPresses, *random)

	// Check if visualisation is enabled.
	if !(*noVis) {
		// Run the SDL visualisation in the main thread.
		sdl.Run(params, events, keyPresses)
	} else {
		// No visualisation mode: handle events without SDL.
		complete := false

		// Process events in a loop until the simulation is complete.
		for !complete {
			event := <-events // Receive an event from the events channel.

			// Check the type of the received event.
			switch event.(type) {
			case gol.FinalTurnComplete:
				// If the event signals the final turn has completed, exit the loop.
				complete = true
			}
		}
	}
}
