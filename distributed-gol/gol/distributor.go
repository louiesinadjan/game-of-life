package gol

import (
	"fmt"
	"log"
	"net/rpc"
	"sync"
	"time"
	"uk.ac.bris.cs/gameoflife/stubs"
	"uk.ac.bris.cs/gameoflife/util"
)

// distributorChannels struct holds various channels used for communication between goroutines.
// It is passed as a pointer because mutexes cannot be passed by value.
type distributorChannels struct {
	events     chan<- Event     // Channel to send events to the main event loop.
	ioCommand  chan<- ioCommand // Channel to send commands to the IO goroutine.
	ioIdle     <-chan bool      // Channel to receive idle status from the IO goroutine.
	ioFilename chan<- string    // Channel to send filenames to the IO goroutine.
	ioOutput   chan<- uint8     // Channel to send output data to the IO goroutine.
	ioInput    <-chan uint8     // Channel to receive input data from the IO goroutine.
	keyPresses <-chan rune      // Channel to receive key presses.
	mu         sync.Mutex       // Mutex to protect shared resources.
}

// race struct allows goroutines to access shared variables safely, avoiding data races.
type race struct {
	turn   int         // Current turn number.
	client *rpc.Client // RPC client to communicate with the server.
	mu     sync.Mutex  // Mutex to protect shared resources.
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c *distributorChannels) {

	// Send command to read input.
	c.ioCommand <- ioInput
	// Send the filename to read, formatted as "widthxheight".
	c.ioFilename <- fmt.Sprintf("%d%s%d", p.ImageWidth, "x", p.ImageHeight)

	// Create a 2D slice to store the world.
	world := make([][]uint8, p.ImageHeight)
	for i := range world {
		world[i] = make([]uint8, p.ImageWidth)
		for j := 0; j < p.ImageWidth; j++ {
			// Read initial cell states from ioInput channel.
			world[i][j] = <-c.ioInput
		}
	}

	// Connect to the server via RPC.
	client, err := rpc.Dial("tcp", "127.0.0.1:8030") // Replace with your server's IP and port.
	if err != nil {
		log.Fatal("Error connecting to server:", err)
	}

	empty := stubs.Empty{}
	continueResponse := &stubs.GetContinueResponse{}
	// Call RPC method to check if there is a saved state to continue from.
	err = client.Call(stubs.GetContinueHandler, empty, continueResponse)

	// Fault tolerance: if the server has been quit before, assign the world to be the world stored in the broker.
	if continueResponse.Continue {
		world = continueResponse.World
		fmt.Printf("Continuing From Turn %d\n", continueResponse.Turn)
	}

	// Send CellFlipped events for any initial live cells in the world.
	for i := range world {
		for j := range world[i] {
			if world[i][j] == 255 {
				c.events <- CellFlipped{0, util.Cell{j, i}}
			}
		}
	}

	var turn int
	// Create a race struct to allow the goroutine to access shared variables safely.
	r := race{turn: turn, client: client}

	// Prepare request to send to server for evolving the world.
	evolveRequest := stubs.EvolveWorldRequest{
		World:       world,
		Width:       p.ImageWidth,
		Height:      p.ImageHeight,
		Turn:        p.Turns,
		Threads:     p.Threads,
		ImageWidth:  p.ImageWidth,
		ImageHeight: p.ImageHeight,
	}
	evolveResponse := &stubs.EvolveResponse{}

	// Create a separate world variable for the goroutine to avoid data races.
	goWorld := world
	done := false
	// Goroutine that handles SDL live view, alive cells count, and key presses.
	go func() {
		ticker := time.NewTicker(2 * time.Second)       // Ticker for alive cell count (every 2 seconds).
		tickSDL := time.NewTicker(5 * time.Millisecond) // Ticker for SDL live view updates.
		goDone := done                                  // Local copy to avoid sending on a closed channel.
		defer ticker.Stop()
		defer tickSDL.Stop()
		for {
			empty := stubs.Empty{}
			if goDone {
				return
			}
			select {
			// If a tick is received from the tickSDL channel, update SDL view.
			case <-tickSDL.C: // SDL Live View.
				// Lock the DistributorChannels mutex while sending events.
				c.mu.Lock()
				cellFlippedResponse := &stubs.GetBrokerCellFlippedResponse{}
				// Get the array of cell flipped events from the broker via RPC.
				err = client.Call(stubs.GetBrokerCellFlippedHandler, empty, cellFlippedResponse)
				cellUpdates := cellFlippedResponse.FlippedEvents
				if len(cellUpdates) != 0 {
					for i := range cellUpdates {
						if !done { // Further validation to check if channel is closed.
							// Send CellFlipped events to the events channel.
							c.events <- CellFlipped{cellUpdates[i].CompletedTurns, cellUpdates[i].Cell}
						}
					}
					// After sending all CellFlipped events for the turn, send a TurnComplete event.
					if !done { // Check if channel is closed.
						c.events <- TurnComplete{CompletedTurns: cellUpdates[0].CompletedTurns}
					}
				}
				c.mu.Unlock() // Unlock the DistributorChannels mutex.
			// If a tick is received from the ticker channel, output AliveCellsCount.
			case <-ticker.C:
				c.mu.Lock() // Lock DistributorChannels mutex.
				aliveCellsCountResponse := &stubs.AliveCellsCountResponse{}
				// RPC call to get alive cells count from the broker.
				err = client.Call(stubs.AliveCellsCountHandler, empty, aliveCellsCountResponse)
				if err != nil {
					log.Fatal("call error : ", err)
					return
				}
				// Get responses from RPC.
				numberAliveCells := aliveCellsCountResponse.AliveCellsCount
				r.turn = aliveCellsCountResponse.CompletedTurns
				if !done { // Check if channel is closed.
					// Send AliveCellsCount event with responses.
					c.events <- AliveCellsCount{r.turn, numberAliveCells}
				}
				c.mu.Unlock() // Unlock DistributorChannels mutex.
			// Check for keypress events.
			case command := <-c.keyPresses:
				// React based on the keypress command.
				empty := stubs.Empty{}
				emptyResponse := &stubs.Empty{}
				getGlobal := &stubs.GetGlobalResponse{}
				// RPC call to get the current world and turn from the broker.
				err = client.Call(stubs.GetGlobalHandler, empty, getGlobal)
				if err != nil {
					log.Fatal("call error : ", err)
					return
				}
				// Update local variables with responses.
				goWorld = getGlobal.World
				r.turn = getGlobal.Turns

				switch command {
				case 's': // 's' key is pressed.
					// StateChange event to indicate execution and save a PGM image.
					c.mu.Lock()
					c.events <- StateChange{r.turn, Executing}
					c.mu.Unlock()
					savePGMImage(c, goWorld, p) // Function to save the current state as a PGM image.

				case 'q': // 'q' key is pressed.
					// StateChange event to indicate quitting and save a PGM image.
					err = client.Call(stubs.QuitHandler, empty, emptyResponse)
					c.mu.Lock()
					c.events <- StateChange{r.turn, Quitting}
					c.mu.Unlock()
					savePGMImage(c, goWorld, p) // Function to save the current state as a PGM image.
					close(c.events)             // Close the events channel.
					done = true                 // Update boolean to know that channel is closed.
					return                      // Exit goroutine.

				case 'k': // 'k' key is pressed.
					// RPC call to kill the server.
					err = client.Call(stubs.KillServerHandler, empty, emptyResponse)
					c.mu.Lock()
					// StateChange event to indicate quitting and save a PGM image.
					c.events <- StateChange{r.turn, Quitting}
					c.mu.Unlock()
					savePGMImage(c, goWorld, p) // Function to save the current state as a PGM image.
					close(c.events)             // Close the events channel.
					done = true                 // Update boolean to know that channel is closed.
					return                      // Exit goroutine.

				case 'p': // 'p' key is pressed.
					// Pause the simulation.
					c.events <- StateChange{r.turn, Paused}
					// Lock the broker mutex so nothing can be changed or accessed during pause.
					err = client.Call(stubs.PauseHandler, empty, emptyResponse)
					fmt.Printf("Current turn %d being processed\n", r.turn)
					for { // Enter an infinite loop which only breaks after 'p' is pressed again.
						if <-c.keyPresses == 'p' { // Waits for another 'p' key press.
							// Unlock broker mutex.
							err = client.Call(stubs.UnpauseHandler, empty, emptyResponse)
							break
						}
					}
					// StateChange event to indicate execution after pausing.
					c.events <- StateChange{r.turn, Executing}
				}
			default: // No events.
				if r.turn == p.Turns {
					return
				}
			}
		}
	}()

	// Make RPC to start iterating each turn and evolving the world.
	err = client.Call(stubs.EvolveWorldHandler, evolveRequest, evolveResponse)
	if err != nil {
		log.Fatal("call error : ", err)
	}
	// Update world and turn with the response from the server.
	world = evolveResponse.World
	turn = evolveResponse.Turn

	// Prepare request to calculate alive cells for the final turn.
	aliveCellsRequest := stubs.CalculateAliveCellsRequest{
		World: world,
	}
	aliveCellsResponse := &stubs.CalculateAliveCellsResponse{}

	// Retrieve alive cells for the FinalTurnComplete event.
	err = client.Call(stubs.AliveCellsHandler, aliveCellsRequest, aliveCellsResponse)
	if err != nil {
		log.Fatal("call error : ", err)
	}
	aliveCells := aliveCellsResponse.AliveCells

	// Report the final state using FinalTurnCompleteEvent.
	c.events <- FinalTurnComplete{turn, aliveCells}
	savePGMImage(c, world, p) // Save the final world.

	// Make sure that the IO has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	// Send Quitting StateChange event.
	c.events <- StateChange{turn, Quitting}

	// Close the events channel to stop the SDL goroutine gracefully.
	close(c.events)
	done = true // Update boolean to indicate channel is closed.

}

// savePGMImage saves the current world state as a PGM image.
func savePGMImage(c *distributorChannels, world [][]byte, p Params) {
	c.ioCommand <- ioOutput
	c.ioFilename <- fmt.Sprintf("%dx%dx%d", p.ImageWidth, p.ImageHeight, p.Turns)
	// Iterate over the world and send each cell's value to the ioOutput channel for writing the PGM image.
	for i := range world {
		for j := range world[i] {
			c.ioOutput <- world[i][j] // Send the current cell value to the output channel.
		}
	}
}
