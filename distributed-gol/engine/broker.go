package main

import (
	"bufio"
	"flag"
	"fmt"
	"net"
	"net/rpc"
	"os"
	"strings"
	"sync"
	"uk.ac.bris.cs/gameoflife/gol"
	"uk.ac.bris.cs/gameoflife/stubs"
	"uk.ac.bris.cs/gameoflife/util"
)

// Global kill channel used to signal the broker to quit.
var kill = make(chan bool)

// Broker struct represents the broker in the distributed Game of Life simulation.
// It holds the current state of the world, the list of connected workers, and synchronisation primitives.
type Broker struct {
	LastWorld     [][]byte             // Previous state of the world, used for detecting changes.
	World         [][]byte             // Current state of the world.
	Turn          int                  // Current turn number.
	Mu            sync.Mutex           // Mutex to protect shared resources.
	Quit          bool                 // Flag to indicate if the simulation should quit.
	Workers       []*rpc.Client        // List of connected worker clients.
	Cell          util.Cell            // A cell in the world (not used in this snippet).
	TurnDone      bool                 // Flag to indicate if a turn has been completed.
	CellUpdates   []util.Cell          // List of cells that have been updated.
	FlippedEvents []stubs.FlippedEvent // Events representing cells that have changed state.
	Continue      bool                 // Flag for fault tolerance, indicates if the simulation should continue from a saved state.
}

// ReadFileLines reads the worker addresses from a file, line by line.
func ReadFileLines(filePath string) []string {

	// Open the file containing worker addresses.
	file, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	defer file.Close() // Ensure the file is closed after reading.

	var lines []string
	scanner := bufio.NewScanner(file)

	// Read each line of the file.
	for scanner.Scan() {
		line := scanner.Text()
		// Split the line into individual elements based on spaces.
		elements := strings.Fields(line)
		lines = append(lines, elements...)
	}

	// Check for any scanning errors.
	if err := scanner.Err(); err != nil {
		return nil
	}

	return lines
}

// ScanForWorkers scans a range of ports to discover active workers.
func ScanForWorkers(startPort, endPort int) []*rpc.Client {
	var workers []*rpc.Client
	for port := startPort; port <= endPort; port++ {
		address := fmt.Sprintf("localhost:%d", port)
		client, err := rpc.Dial("tcp", address)
		if err == nil {
			workers = append(workers, client)
			fmt.Printf("Connected to worker on %s\n", address)
		} else {
			fmt.Printf("Failed to connect to worker on %s: %v\n", address, err)
		}
	}
	return workers
}

// worker function sends a portion of the world to a worker client for processing.
func worker(id int, world [][]byte, results chan<- [][]byte, p gol.Params, client *rpc.Client, threads int) {
	// Calculate the number of rows each worker should process.
	var heightDiff = float32(p.ImageHeight) / float32(threads)

	// Determine the start and end rows for this worker.
	startRow := int(float32(id) * heightDiff)
	endRow := int(float32(id+1) * heightDiff)

	// Ensure that EndRow does not exceed the total number of rows.
	if endRow > p.ImageHeight {
		endRow = p.ImageHeight
	}

	// Create a request object with the portion of the world this worker will process.
	worldReq := stubs.WorldReq{
		World:    world,
		StartRow: startRow,
		EndRow:   endRow,
		Width:    p.ImageWidth,
		Height:   p.ImageHeight,
	}

	// Prepare a response object to receive the processed world.
	worldRes := &stubs.WorldRes{
		World: [][]byte{},
	}

	// Call the worker's WorldHandler function to evolve the world.
	err := client.Call(stubs.WorldHandler, worldReq, worldRes)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Send the resulting world slice back through the results channel.
	results <- worldRes.World
}

func worldSize(world [][]byte) {
	nonEmptyCount := 0
	for _, row := range world {
		for _, cell := range row {
			if cell != 0 {
				nonEmptyCount++
			}
		}
	}
	fmt.Printf("Number of non-empty cells: %d\n", nonEmptyCount)
}

// EvolveWorld handles the evolution of the world by distributing work to connected workers.
func (b *Broker) EvolveWorld(req stubs.EvolveWorldRequest, res *stubs.EvolveResponse) (err error) {
	b.Quit = false // Reset the quit flag at the start of a new simulation run.

	// Fault tolerance: If not continuing from a saved state, initialise the world from the request.
	if !b.Continue {
		b.World = make([][]byte, len(req.World))
		for i := range req.World {
			b.World[i] = make([]byte, len(req.World[i]))
			copy(b.World[i], req.World[i])
		}
		b.Turn = 0
	}

	// For SDL live view and fault tolerance, set LastWorld to the current world.
	b.LastWorld = b.World
	//this is because this implementation compares the current SDL displayed world and next displayed world

	// Extract parameters from the request.
	p := gol.Params{
		Turns:       req.Turn,
		Threads:     req.Threads,
		ImageWidth:  req.ImageWidth,
		ImageHeight: req.ImageHeight,
	}

	// Execute the Game of Life simulation for the specified number of turns.
	for b.Turn < p.Turns && !b.Quit {
		b.Mu.Lock() // Lock the mutex to prevent concurrent access to global variables.

		var newWorld [][]byte                     // New world state after this turn.
		threads := len(b.Workers)                 // Number of available workers.
		results := make([]chan [][]byte, threads) // Channels to receive results from workers.

		// Distribute work to each worker.
		for id, workerClient := range b.Workers {
			results[id] = make(chan [][]byte)
			go worker(id, b.World, results[id], p, workerClient, threads) // Concurrent call to each worker.
		}

		// Collect results from workers and assemble the new world state.
		for i := 0; i < threads; i++ {
			slice := <-results[i]
			newWorld = append(newWorld, slice...)
		}

		b.World = newWorld // Update the global world state.
		b.Turn++           // Increment the turn counter.
		b.TurnDone = true  // Indicate that a turn has been completed.
		b.Mu.Unlock()      // Unlock the mutex.
	}

	// Prepare the response with the final world state and turn number.
	res.World = b.World
	res.Turn = b.Turn
	return
}

// CalculateAliveCells calculates the positions of all alive cells in the current world.
func (b *Broker) CalculateAliveCells(req stubs.Empty, res *stubs.CalculateAliveCellsResponse) (err error) {
	b.Mu.Lock()
	defer b.Mu.Unlock()

	aliveCells := []util.Cell{}
	for i := range b.World { // Iterate over each row.
		for j := range b.World[i] { // Iterate over each cell in the row.
			if b.World[i][j] == 255 { // Check if the cell is alive.
				aliveCells = append(aliveCells, util.Cell{X: j, Y: i})
			}
		}
	}
	// Return the list of alive cells.
	res.AliveCells = aliveCells
	return
}

// AliveCellsCount returns the number of alive cells and the current turn number.
func (b *Broker) AliveCellsCount(req stubs.Empty, res *stubs.AliveCellsCountResponse) (err error) {
	b.Mu.Lock()
	defer b.Mu.Unlock()

	count := 0
	for i := range b.World {
		for j := range b.World[i] {
			if b.World[i][j] == 255 {
				count++
			}
		}
	}

	// Populate the response with the alive cells count and completed turns.
	res.AliveCellsCount = count
	res.CompletedTurns = b.Turn
	return
}

// GetGlobal returns the current world state and turn number.
func (b *Broker) GetGlobal(req stubs.Empty, res *stubs.GetGlobalResponse) (err error) {
	b.Mu.Lock()
	defer b.Mu.Unlock()
	res.World = b.World
	res.Turns = b.Turn
	return
}

// QuitServer sets the flags to indicate that the simulation should quit and saves the current world state.
func (b *Broker) QuitServer(req stubs.Empty, res *stubs.Empty) (err error) {
	b.Mu.Lock()
	defer b.Mu.Unlock()
	b.Continue = true     // Enable fault tolerance to continue from this state.
	b.Quit = true         // Set the quit flag to stop the simulation.
	b.LastWorld = b.World // Save the current world state.
	return
}

// Pause locks the mutex to pause the simulation by preventing access to global variables.
func (b *Broker) Pause(req stubs.Empty, res *stubs.Empty) (err error) {
	b.Mu.Lock()
	return
}

// Unpause unlocks the mutex to resume the simulation.
func (b *Broker) Unpause(req stubs.Empty, res *stubs.Empty) (err error) {
	b.Mu.Unlock()
	return
}

// KillServer terminates the simulation and signals connected workers to shut down.
func (b *Broker) KillServer(req stubs.Empty, res *stubs.Empty) (err error) {
	// Prepare an empty response for the RPC calls.
	emptyRes := stubs.Empty{}

	// Notify each worker to shut down and close the client connections.
	for _, client := range b.Workers {
		err = client.Call(stubs.KillHandler, req, &emptyRes)
		client.Close()
	}

	b.Quit = true // Set the quit flag.
	kill <- true  // Signal the kill channel to exit the program.
	return
}

// GetTurnDone returns TurnDone (SDL live view), and the current turn, sets TurnDone back to false
func (b *Broker) GetTurnDone(req stubs.Empty, res *stubs.GetTurnDoneResponse) (err error) {
	b.Mu.Lock()
	defer b.Mu.Unlock()
	res.TurnDone = b.TurnDone
	res.Turn = b.Turn
	b.TurnDone = false
	return
}

// GetContinue returns the current world state, turn number, and fault tolerance flag.
func (b *Broker) GetContinue(req stubs.Empty, res *stubs.GetContinueResponse) (err error) {
	b.Mu.Lock()
	defer b.Mu.Unlock()
	res.World = b.World
	res.Turn = b.Turn
	res.Continue = b.Continue
	return
}

// GetCellFlipped function returns a struct array which contains variables required for CellFlipped events.
func (b *Broker) GetCellFlipped(req stubs.Empty, res *stubs.GetBrokerCellFlippedResponse) (err error) {
	b.Mu.Lock()
	defer b.Mu.Unlock()

	b.FlippedEvents = []stubs.FlippedEvent{} // Reset the list of flipped events.
	// Find all cells that have changed state between LastWorld and the current World.
	for _, cell := range findFlippedCells(b.World, b.LastWorld) {
		flippedEvent := stubs.FlippedEvent{
			CompletedTurns: b.Turn,
			Cell:           cell,
		}
		b.FlippedEvents = append(b.FlippedEvents, flippedEvent)
	}

	b.LastWorld = b.World               // Update LastWorld for the next comparison.
	res.FlippedEvents = b.FlippedEvents // Return the list of flipped events.
	return
}

// findFlippedCells compares two worlds and returns the cells that have changed state.
func findFlippedCells(next [][]byte, current [][]byte) []util.Cell {
	var flipped []util.Cell

	// If either world is empty, return an empty list.
	if len(current) == 0 || len(next) == 0 || len(current[0]) == 0 || len(next[0]) == 0 {
		return flipped
	}

	// Perform element-wise XOR to find differences between the two worlds.
	xorWorld := xor2D(current, next)

	// Identify the cells that have changed state.
	for i := 0; i < len(xorWorld); i++ {
		for j := 0; j < len(xorWorld[0]); j++ {
			if xorWorld[i][j] != 0 {
				flipped = append(flipped, util.Cell{X: j, Y: i})
			}
		}
	}
	return flipped
}

// xor2D performs an element-wise XOR operation on two 2D byte slices.
func xor2D(a, b [][]byte) [][]byte {
	numRows := len(a)
	numCols := len(a[0])

	result := make([][]byte, numRows)
	for i := 0; i < numRows; i++ {
		result[i] = make([]byte, numCols)
		for j := 0; j < numCols; j++ {
			result[i][j] = a[i][j] ^ b[i][j] // XOR each cell.
		}
	}

	return result
}

// main function initialises the broker, sets up RPC connections, and listens for incoming requests.
func main() {
	pAddr := flag.String("port", "8030", "Port to listen on")
	startPort := flag.Int("startPort", 8040, "Starting port for worker scanning")
	endPort := flag.Int("endPort", 8050, "Ending port for worker scanning")
	flag.Parse()

	// Goroutine to handle the kill signal and exit the program.
	go func() {
		for {
			if <-kill {
				os.Exit(1)
			}
		}
	}()

	// Set up client connections to workers.

	//var workers []*rpc.Client
	//workerPorts := ReadFileLines("workers.txt") // Read worker addresses from a file.
	//for _, detail := range workerPorts {
	//	client, err := rpc.Dial("tcp", detail)
	//	if err == nil {
	//		workers = append(workers, client)
	//		fmt.Printf("Worker connected on: %v\n", detail)
	//	}
	//}

	workers := ScanForWorkers(*startPort, *endPort)

	// Register the Broker type with the RPC server.
	rpc.Register(&Broker{Workers: workers, Continue: false})

	// Start listening for incoming RPC connections.
	listener, err := net.Listen("tcp", ":"+*pAddr)
	if err != nil {
		fmt.Printf("Error starting listener: %s\n", err)
		os.Exit(1)
	}
	defer listener.Close()

	// Accept incoming RPC connections.
	rpc.Accept(listener)
}
