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

type distributorChannels struct { //passed in as a pointer as mutexes can not be passed by value
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
	keyPresses <-chan rune
	mu         sync.Mutex
}

type race struct { //struct which lets the go routine access different variables to avoid data races
	turn   int
	client *rpc.Client
	mu     sync.Mutex
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c *distributorChannels) {

	c.ioCommand <- ioInput
	c.ioFilename <- fmt.Sprintf("%d%s%d", p.ImageWidth, "x", p.ImageHeight)

	// TODO: Create a 2D slice to store the world.
	world := make([][]uint8, p.ImageHeight)
	for i := range world {
		world[i] = make([]uint8, p.ImageWidth)
		for j := 0; j < p.ImageWidth; j++ {
			world[i][j] = <-c.ioInput
		}
	}

	//  connect to the server via RPC
	client, err := rpc.Dial("tcp", "127.0.0.1:8030") // replace "127.0.0.1:8030" with your server's IP and port
	if err != nil {
		log.Fatal("Error connecting to server:", err)
	}

	empty := stubs.Empty{}
	continueResponse := &stubs.GetContinueResponse{}
	err = client.Call(stubs.GetContinueHandler, empty, continueResponse)

	//FAULT TOLERANCE
	//if the server has been quit before, assign the world to be the world stored in the broker
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
	//struct to let the goroutine access different variables to the rest of the function code
	r := race{turn: turn, client: client}

	//request to make to server for evolving the world
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

	//let goWorld be a different world variable that the goroutine can access to avoid data races
	goWorld := world
	done := false
	//goroutine that handles sdl live view, alive cells count and key presses
	go func() {
		ticker := time.NewTicker(2 * time.Second)       //ticker for alive cell count
		tickSDL := time.NewTicker(1 * time.Millisecond) //ticker for sdl live view
		goDone := done                                  //to avoid sending on a closed channel
		defer ticker.Stop()
		defer tickSDL.Stop()
		for { //indefinitely loops until a return statement in either 'q' or 'k' key press or 'if goDone == true' at the start
			empty := stubs.Empty{}
			if goDone {
				return
			}
			select {
			//if a tick is received from the tickSDL channel then update SDL view
			case <-tickSDL.C: //SDL LIVE VIEW
				//lock the DistributorChannels while CellFlipped events and TurnComplete event is inputted
				c.mu.Lock()
				cellFlippedResponse := &stubs.GetBrokerCellFlippedResponse{}
				//get the array of structs that represent cell flipped events from broker
				err = client.Call(stubs.GetBrokerCellFlippedHandler, empty, cellFlippedResponse)
				cellUpdates := cellFlippedResponse.FlippedEvents
				if len(cellUpdates) != 0 {
					for i := range cellUpdates { //iterate through the array and send each event into events channel
						if !done { //further validation to see if channel is closed
							c.events <- CellFlipped{cellUpdates[i].CompletedTurns, cellUpdates[i].Cell}
						}
					}
					//after sending all CellFlipped events for the turn, send a TurnComplete event
					if !done { //check if channel is closed
						c.events <- TurnComplete{CompletedTurns: cellUpdates[0].CompletedTurns}
					}
				}
				c.mu.Unlock() //unlock DistributorChannels mutex
			//if a tick is received from the ticker channel then output AliveCellsCount
			case <-ticker.C:
				c.mu.Lock() //lock DistributorChannels
				aliveCellsCountResponse := &stubs.AliveCellsCountResponse{}
				//RPC to alive cells function in broker
				err = client.Call(stubs.AliveCellsCountHandler, empty, aliveCellsCountResponse)
				if err != nil {
					log.Fatal("call error : ", err)
					return
				}
				//get responses from RPC
				numberAliveCells := aliveCellsCountResponse.AliveCellsCount
				r.turn = aliveCellsCountResponse.CompletedTurns
				if !done { //check if channel is closed
					//send AliveCellsCount event with responses
					c.events <- AliveCellsCount{r.turn, numberAliveCells}
				}
				c.mu.Unlock() //unlock DistributorChannels
				// check for keypress events
			case command := <-c.keyPresses:
				// react based on the keypress command
				empty := stubs.Empty{}
				emptyResponse := &stubs.Empty{}
				getGlobal := &stubs.GetGlobalResponse{}
				//RPC call to get the current world and turn on the broker
				err = client.Call(stubs.GetGlobalHandler, empty, getGlobal)
				if err != nil {
					log.Fatal("call error : ", err)
					return
				}
				goWorld = getGlobal.World
				r.turn = getGlobal.Turns
				//assign the local goroutine variables to response

				switch command {
				case 's': // 's' key is pressed
					// StateChange event to indicate execution and save a PGM image
					c.mu.Lock()
					c.events <- StateChange{r.turn, Executing}
					c.mu.Unlock()
					savePGMImage(c, goWorld, p) // Function to save the current state as a PGM image

				case 'q': // 'q' key is pressed
					// StateChange event to indicate quitting and save a PGM image
					err = client.Call(stubs.QuitHandler, empty, emptyResponse)
					c.mu.Lock()
					c.events <- StateChange{r.turn, Quitting}
					c.mu.Unlock()
					savePGMImage(c, goWorld, p) // function to save the current state as a PGM image
					close(c.events)             // close the events channel
					done = true                 // update boolean to know that channel is closed
					return                      // exit goroutine

				case 'k': // 'q' key is pressed
					err = client.Call(stubs.KillServerHandler, empty, emptyResponse)
					c.mu.Lock()
					// StateChange event to indicate quitting and save a PGM image
					c.events <- StateChange{r.turn, Quitting}
					c.mu.Unlock()
					savePGMImage(c, goWorld, p) // function to save the current state as a PGM image
					close(c.events)             // close the events channel
					done = true                 // update boolean to know that channel is closed
					return                      // exit goroutine

				case 'p': // 'p' key is pressed
					c.events <- StateChange{r.turn, Paused}
					//locks the broker mutex so nothing can be changed or accessed during pause
					err = client.Call(stubs.PauseHandler, empty, emptyResponse)
					fmt.Printf("Current turn %d being processed\n", r.turn)
					for { //enter an infinite for loop which only breaks after 'p' is presses again
						if <-c.keyPresses == 'p' { //waits for another 'p' key press
							//unlocks mutex
							err = client.Call(stubs.UnpauseHandler, empty, emptyResponse)
							break
						}
					}
					// StateChange event to indicate execution after pausing
					c.events <- StateChange{r.turn, Executing}
				}
			default: // no events
				if r.turn == p.Turns {
					return
				}
			}
		}
	}()

	//make RPC to start iterating each turn and evolving the world
	err = client.Call(stubs.EvolveWorldHandler, evolveRequest, evolveResponse)
	if err != nil {
		log.Fatal("call error : ", err)
	}
	world = evolveResponse.World
	turn = evolveResponse.Turn

	//assign variables to the final world and turn response
	aliveCellsRequest := stubs.CalculateAliveCellsRequest{
		World: world,
	}
	aliveCellsResponse := &stubs.CalculateAliveCellsResponse{}

	//retrieve alive cells for the FinalTurnComplete event
	err = client.Call(stubs.AliveCellsHandler, aliveCellsRequest, aliveCellsResponse)
	if err != nil {
		log.Fatal("call error : ", err)
	}
	aliveCells := aliveCellsResponse.AliveCells

	// TODO: Report the final state using FinalTurnCompleteEvent.
	c.events <- FinalTurnComplete{turn, aliveCells}
	savePGMImage(c, world, p) //save the final world

	// make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	//send quitting StateChange event
	c.events <- StateChange{turn, Quitting}

	// close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
	done = true //update boolean to know channel is closed

}

func savePGMImage(c *distributorChannels, world [][]byte, p Params) {
	c.ioCommand <- ioOutput
	c.ioFilename <- fmt.Sprintf("%dx%dx%d", p.ImageWidth, p.ImageHeight, p.Turns)
	// Iterate over the world and send each cell's value to the ioOutput channel for writing the PGM image
	for i := range world {
		for j := range world[i] {
			c.ioOutput <- world[i][j] // Send the current cell value to the output channel
		}
	}
}
