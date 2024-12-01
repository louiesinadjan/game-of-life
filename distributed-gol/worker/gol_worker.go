package main

import (
	"flag"
	"fmt"
	"net"
	"net/rpc"
	"os"
	"uk.ac.bris.cs/gameoflife/stubs"
)

// Global kill channel used to signal the worker to quit.
var kill = make(chan bool)

// WorldOps struct provides methods for calculating the next state of the world
// and for handling termination of the worker process.
type WorldOps struct{}

// CalculateWorld processes a slice of the world assigned to this worker and computes its next state.
// Only the specified rows (from startRow to endRow) are updated, and the rest remain unchanged.
func (w *WorldOps) CalculateWorld(req *stubs.WorldReq, res *stubs.WorldRes) (err error) {
	// Compute the next state for the assigned rows and return the result.
	res.World = calculateNextState(req.World, req.Width, req.Height, req.StartRow, req.EndRow)
	return
}

// KillWorker function sends a signal to the kill channel to terminate the worker process.
func (w *WorldOps) KillWorker(req *stubs.Empty, res *stubs.Empty) (err error) {
	kill <- true // Send a true signal to the kill channel.
	return
}

// calculateNextState computes the next state of the world based on the rules of Conway's Game of Life.
// The computation is limited to the rows between startRow and endRow for efficiency.
func calculateNextState(world [][]byte, width int, height int, startRow int, endRow int) [][]byte {
	// Initialise the next state for the given slice of rows.
	nextState := make([][]byte, endRow-startRow)
	for i := 0; i < endRow-startRow; i++ {
		nextState[i] = make([]byte, width)
	}

	// Iterate through each cell in the specified rows and compute its next state.
	for i := startRow; i < endRow; i++ {
		for j := 0; j < width; j++ {
			// Calculate the sum of the states of the 8 neighbouring cells, accounting for wrap-around edges.
			sum := (int(world[(i+height-1)%height][(j+width-1)%width]) +
				int(world[(i+height-1)%height][(j+width)%width]) +
				int(world[(i+height-1)%height][(j+width+1)%width]) +
				int(world[(i+height)%height][(j+width-1)%width]) +
				int(world[(i+height)%height][(j+width+1)%width]) +
				int(world[(i+height+1)%height][(j+width-1)%width]) +
				int(world[(i+height+1)%height][(j+width)%width]) +
				int(world[(i+height+1)%height][(j+width+1)%width])) / 255

			// Update the cell state based on the rules of Conway's Game of Life.
			if world[i][j] == 255 { // If the cell is alive.
				if sum < 2 || sum > 3 { // Underpopulation or overpopulation causes death.
					nextState[i-startRow][j] = 0
				} else { // Cell survives if it has 2 or 3 neighbours.
					nextState[i-startRow][j] = 255
				}
			} else { // If the cell is dead.
				if sum == 3 { // Reproduction occurs if exactly 3 neighbours are alive.
					nextState[i-startRow][j] = 255
				} else { // Cell remains dead.
					nextState[i-startRow][j] = 0
				}
			}
		}
	}

	return nextState
}

func main() {
	// Define a command-line flag for specifying the port number.
	pAddr := flag.String("port", "8040", "Port to listen on")
	flag.Parse() // Parse the flag input from the terminal.

	// Initialise the WorldOps struct and register its methods for RPC.
	ops := &WorldOps{}
	rpc.Register(ops)

	// Goroutine that listens for a kill signal and terminates the worker process.
	go func() {
		for { // Infinite loop to continuously check for kill signals.
			if <-kill { // If a true signal is received, terminate the process.
				os.Exit(1)
			}
		}
	}()

	// Set up a TCP listener to accept RPC connections.
	listener, err := net.Listen("tcp", ":"+*pAddr)
	if err != nil { // Handle errors when starting the listener.
		fmt.Println("Error starting listener:", err)
		return
	}
	defer listener.Close() // Ensure the listener is closed when the program exits.

	fmt.Println("Listening on port", *pAddr)

	// Accept incoming RPC connections and process them.
	rpc.Accept(listener)
}
