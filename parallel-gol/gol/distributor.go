package gol

import (
	"fmt"
	"math/rand"
	"time"
	"uk.ac.bris.cs/gameoflife/util"
)

// distributorChannels struct holds all the channels used for communication between goroutines.
type distributorChannels struct {
	events     chan<- Event     // Channel to send events to the GUI or tests.
	ioCommand  chan<- ioCommand // Channel to send IO commands.
	ioIdle     <-chan bool      // Channel to receive IO idle signal.
	ioFilename chan<- string    // Channel to send filenames for IO operations.
	ioOutput   chan<- uint8     // Channel to send output data to the IO goroutine.
	ioInput    <-chan uint8     // Channel to receive input data from the IO goroutine.
	keyPresses <-chan rune      // Channel to receive key presses from the GUI.
}

// worker function computes the next state of a slice of the world.
func worker(id int, p Params, world [][]byte, result chan<- [][]byte, c distributorChannels, turn int) {
	// Calculate the base number of rows per worker and the remainder.
	rowsPerWorker := p.ImageHeight / p.Threads
	remainder := p.ImageHeight % p.Threads

	var startRow, endRow int

	if id < remainder {
		// Workers with id less than remainder get an extra row.
		startRow = id * (rowsPerWorker + 1)
		endRow = startRow + (rowsPerWorker + 1)
	} else {
		// Workers with id greater or equal to remainder get the base number of rows.
		startRow = id*rowsPerWorker + remainder
		endRow = startRow + rowsPerWorker
	}

	// Calculate the next state for this worker's slice.
	newWorld := calculateNextState(world, startRow, endRow, c, turn, p)

	// Send the computed slice back to the distributor.
	result <- newWorld
}

// savePGMImage function saves the current state of the world as a PGM image.
func savePGMImage(c distributorChannels, world [][]byte, p Params) {
	c.ioCommand <- ioOutput
	c.ioFilename <- fmt.Sprintf("%dx%dx%d", p.ImageWidth, p.ImageHeight, p.Turns)
	// Iterate over the world and send each cell's value to the ioOutput channel for writing the PGM image.
	for i := range world {
		for j := range world[i] {
			c.ioOutput <- world[i][j] // Send the current cell value to the output channel.
		}
	}
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels, random bool) {
	// Signal the IO goroutine to start input operation.
	c.ioCommand <- ioInput
	c.ioFilename <- fmt.Sprintf("%d%s%d", p.ImageWidth, "x", p.ImageHeight)

	// Initialise the world grid as a 2D slice
	world := make([][]uint8, p.ImageHeight)
	newWorld := [][]byte{}

	for i := range world {
		world[i] = make([]uint8, p.ImageWidth)
	}

	// Read the initial world state from the IO goroutine or randomly populate.
	if random {
		// Populate the grid with random alive (255) or dead (0) cells.
		for i := 0; i < p.ImageHeight; i++ {
			for j := 0; j < p.ImageWidth; j++ {
				if rand.Float64() < 0.1 { // % chance for alive cell
					world[i][j] = 255
				} else {
					world[i][j] = 0
				}
				<-c.ioInput // To stop blocking and allow keyPresses
			}
		}
	} else {
		// Read the grid state from the IO goroutine.
		for i := 0; i < p.ImageHeight; i++ {
			for j := 0; j < p.ImageWidth; j++ {
				world[i][j] = <-c.ioInput // Read from the input channel
			}
		}
	}

	// Send CellFlipped events for all initially alive cells.
	for i := range world {
		for j := range world[i] {
			if world[i][j] == 255 {
				c.events <- CellFlipped{0, util.Cell{j, i}}
			}
		}
	}

	turn := 0                                    // Initialise the turn counter.
	quit := false                                // Flag to indicate if the program should quit.
	resultCh := make([]chan [][]byte, p.Threads) // Channels to receive results from workers.

	// Initialise result channels for each worker.
	for i := range resultCh {
		resultCh[i] = make(chan [][]byte)
	}

	// Create a ticker to send AliveCellsCount events every 2 seconds.
	ticker := time.NewTicker(2 * time.Second)

	// Main loop to process each turn.
	for turn := 0; turn < p.Turns; turn++ {
		if quit {
			break // Exit the loop if quit flag is set.
		}

		// Start worker goroutines to compute the next state in parallel.
		for i := 0; i < p.Threads; i++ {
			go worker(i, p, world, resultCh[i], c, turn)
		}

		// Collect results from all workers and assemble the new world state.
		for i := 0; i < p.Threads; i++ {
			resultPart := <-resultCh[i]                // Receive the computed slice.
			newWorld = append(newWorld, resultPart...) // Append the slice to form the new world.
		}

		// Update the world with the new state.
		world = append([][]byte{}, newWorld...)
		newWorld = [][]byte{} // Reset newWorld for the next turn.

		// Handle events such as key presses and ticker ticks.
		select {
		case <-ticker.C:
			// Send AliveCellsCount event every 2 seconds.
			c.events <- AliveCellsCount{turn + 1, len(calculateAliveCells(world))}
		case command := <-c.keyPresses:
			// Handle key press events.
			switch command {
			case 's':
				// Save the current state as a PGM image.
				c.events <- StateChange{turn, Executing}
				savePGMImage(c, world, p)
			case 'q':
				// Set the quit flag to exit.
				c.events <- StateChange{turn, Quitting}
				quit = true
				break
			case 'p':
				// Pause the execution until 'p' is pressed again.
				c.events <- StateChange{turn, Paused}
				fmt.Printf("Current turn %d being processed\n", turn)
				for {
					if <-c.keyPresses == 'p' {
						break // Resume execution when 'p' is pressed again.
					}
				}
				c.events <- StateChange{turn, Executing}
			}
		default:
			// No event; continue processing.
		}

		// Send TurnComplete event after finishing the turn.
		c.events <- TurnComplete{CompletedTurns: turn}
	}

	// Calculate the final list of alive cells.
	calculateAliveCells(world)

	// Send FinalTurnComplete event with the list of alive cells.
	c.events <- FinalTurnComplete{turn, calculateAliveCells(world)}

	// Save the final state as a PGM image.
	savePGMImage(c, world, p)

	// Ensure the IO goroutine has finished all operations before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	// Send a StateChange event to indicate the program is quitting.
	c.events <- StateChange{p.Turns, Quitting}

	// Close the events channel to allow the GUI to shut down gracefully.
	close(c.events)
}

// calculateNextState computes the next state of a slice of the world grid.
func calculateNextState(world [][]byte, startRow, endRow int, c distributorChannels, turn int, p Params) [][]byte {
	height := p.ImageHeight
	width := p.ImageWidth

	// Initialise the next state slice.
	nextState := make([][]byte, endRow-startRow)
	for i := 0; i < endRow-startRow; i++ {
		nextState[i] = make([]byte, width)
	}

	// Iterate over each cell in the assigned slice.
	for i := startRow; i < endRow; i++ {
		for j := 0; j < width; j++ {
			// Calculate the sum of alive neighbouring cells.
			sum := (int(world[(i+height-1)%height][(j+width-1)%width]) +
				int(world[(i+height-1)%height][(j+width)%width]) +
				int(world[(i+height-1)%height][(j+width+1)%width]) +
				int(world[(i+height)%height][(j+width-1)%width]) +
				int(world[(i+height)%height][(j+width+1)%width]) +
				int(world[(i+height+1)%height][(j+width-1)%width]) +
				int(world[(i+height+1)%height][(j+width)%width]) +
				int(world[(i+height+1)%height][(j+width+1)%width])) / 255

			// Apply the Game of Life rules.
			if world[i][j] == 255 { // If the cell is alive.
				if sum < 2 || sum > 3 {
					// Cell dies due to underpopulation or overpopulation.
					nextState[i-startRow][j] = 0
					c.events <- CellFlipped{turn, util.Cell{j, i}}
				} else {
					// Cell stays alive.
					nextState[i-startRow][j] = 255
				}
			} else { // If the cell is dead.
				if sum == 3 {
					// Cell becomes alive due to reproduction.
					nextState[i-startRow][j] = 255
					c.events <- CellFlipped{turn, util.Cell{j, i}}
				} else {
					// Cell stays dead.
					nextState[i-startRow][j] = 0
				}
			}
		}
	}

	return nextState
}

// calculateAliveCells returns a list of coordinates of all alive cells in the world.
func calculateAliveCells(world [][]byte) []util.Cell {
	aliveCells := []util.Cell{}
	for i := range world { // Iterate over rows.
		for j := range world[i] { // Iterate over columns.
			if world[i][j] == 255 {
				// Append the cell's coordinates if it is alive.
				aliveCells = append(aliveCells, util.Cell{j, i})
			}
		}
	}
	return aliveCells
}
