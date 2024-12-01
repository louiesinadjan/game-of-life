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

var kill = make(chan bool) //global kill channel for quitting the broker

type GOLWorker struct {
	LastWorld     [][]byte
	World         [][]byte
	Turn          int
	Mu            sync.Mutex
	Quit          bool
	Workers       []*rpc.Client
	Cell          util.Cell
	TurnDone      bool
	CellUpdates   []util.Cell
	FlippedEvents []stubs.FlippedEvent
	Continue      bool
}

// ReadFileLines reads worker addresses line by line
func ReadFileLines(filePath string) []string {

	//opens the file
	file, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	defer file.Close() //closes the file after lines have been returned

	var lines []string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		// Split the line into individual elements based on space
		elements := strings.Fields(line)
		lines = append(lines, elements...)
	}

	if err := scanner.Err(); err != nil {
		return nil
	}

	return lines
}

func worker(id int, world [][]byte, results chan<- [][]byte, p gol.Params, client *rpc.Client, threads int) {
	var heightDiff = float32(p.ImageHeight) / float32(threads)

	// calculate StartRow and EndRow based on the thread ID
	startRow := int(float32(id) * heightDiff)
	endRow := int(float32(id+1) * heightDiff)

	// ensure that EndRow does not exceed the total number of rows
	if endRow > p.ImageHeight {
		endRow = p.ImageHeight
	}

	worldReq := stubs.WorldReq{ //initialise the request variables to local variables
		World:    world,
		StartRow: startRow,
		EndRow:   endRow,
		Width:    p.ImageWidth,
		Height:   p.ImageHeight,
	}

	//create a response
	worldRes := &stubs.WorldRes{
		World: [][]byte{},
	}

	//call the worker to evolve the world
	err := client.Call(stubs.WorldHandler, worldReq, worldRes)
	if err != nil {
		print(err)
		return
	}

	//send the resulting world to the results channel
	results <- worldRes.World
	return
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

func (g *GOLWorker) EvolveWorld(req stubs.EvolveWorldRequest, res *stubs.EvolveResponse) (err error) {
	g.Quit = false //set quit to false as this is the first call to broker for this run of the program

	// create a separate copy of the input world to work on
	// FAULT TOLERANCE
	if !g.Continue { //if continue is false then make the global world the request world
		g.World = make([][]byte, len(req.World))
		for i := range req.World {
			g.World[i] = make([]byte, len(req.World[i]))
			copy(g.World[i], req.World[i])
		}
		g.Turn = 0
	}

	//SDL LIVE VIEW & FAULT TOLERANCE
	g.LastWorld = g.World //set the global LastWorld to the global world
	//this is because this implementation compares the current SDL displayed world and next displayed world

	p := gol.Params{
		Turns:       req.Turn,
		Threads:     req.Threads,
		ImageWidth:  req.ImageWidth,
		ImageHeight: req.ImageHeight,
	}

	// TODO: Execute all turns of the Game of Life.
	// Run Game of Life simulation for the specified number of turns
	for g.Turn < p.Turns && g.Quit == false { //run until max turns is reached or if quit is ever set to false
		g.Mu.Lock() //lock the mutex so other functions can not access the global variables e.g. g.World

		var newWorld [][]byte //create a new [][]byte which the worker slices will append to, to make the resulting world
		threads := len(g.Workers)
		results := make([]chan [][]uint8, threads) //make a channel slice so each worker has its own channel to send results to
		for id, workerClient := range g.Workers {  //call the worker function for each worker in g.Workers
			results[id] = make(chan [][]uint8)
			go worker(id, g.World, results[id], p, workerClient, threads) //concurrently call worker function
		}
		for i := 0; i < threads; i++ {
			slice := <-results[i]
			newWorld = append(newWorld, slice...) //append each worker result slice to each either to reconstruct the world
		}

		g.World = newWorld //set the global world to newWorld
		g.Turn++           //increment turn
		g.TurnDone = true  //set TurnDone to true for TurnComplete events
		g.Mu.Unlock()      //unlock the mutex, other functions can now access the global variables
	}

	//assign the response variables
	res.World = g.World
	res.Turn = g.Turn
	return
}
func (g *GOLWorker) CalculateAliveCells(req stubs.Empty, res *stubs.CalculateAliveCellsResponse) (err error) {
	g.Mu.Lock()
	defer g.Mu.Unlock()

	aliveCells := []util.Cell{}
	for i := range g.World { //height
		for j := range g.World[i] { //width
			if g.World[i][j] == 255 { //check if cell is alive
				aliveCells = append(aliveCells, util.Cell{j, i}) //append to slice of alive cells
			}
		}
	}
	//returns the aliveCells slice
	res.AliveCells = aliveCells
	return
}
func (g *GOLWorker) AliveCellsCount(req stubs.Empty, res *stubs.AliveCellsCountResponse) (err error) {
	g.Mu.Lock()
	defer g.Mu.Unlock()

	aliveCells := []util.Cell{}
	for i := range g.World { //height
		for j := range g.World[i] { //width
			if g.World[i][j] == 255 {
				aliveCells = append(aliveCells, util.Cell{j, i})
			}
		}
	}

	//return length of aliveCells and the current turn for the event
	res.AliveCellsCount = len(aliveCells)
	res.CompletedTurns = g.Turn
	return
}

// GetGlobal function returns the current world and turn
func (g *GOLWorker) GetGlobal(req stubs.Empty, res *stubs.GetGlobalResponse) (err error) {
	g.Mu.Lock()
	defer g.Mu.Unlock()
	res.World = g.World
	res.Turns = g.Turn
	return
}

// QuitServer function sets Continue to true (fault tolerance), Quit to true and LastWorld to be World
func (g *GOLWorker) QuitServer(req stubs.Empty, res *stubs.Empty) (err error) {
	g.Mu.Lock()
	defer g.Mu.Unlock()
	g.Continue = true
	g.Quit = true
	g.LastWorld = g.World
	return
}

// Pause function locks the mutex so none of the global variables can be accessed while the program is paused
func (g *GOLWorker) Pause(req stubs.Empty, res *stubs.Empty) (err error) {
	g.Mu.Lock()
	return
}

// Unpause function unlocks the mutex for when the program is unpaused
func (g *GOLWorker) Unpause(req stubs.Empty, res *stubs.Empty) (err error) {
	g.Mu.Unlock()
	return
}

// KillServer function inputs a true bool to the kill channel and calls a kill function in each worker, also sets Quit to true
func (g *GOLWorker) KillServer(req stubs.Empty, res *stubs.Empty) (err error) {
	// Close the existing client connections
	emptyRes := stubs.Empty{}

	for _, client := range g.Workers {
		err = client.Call(stubs.KillHandler, req, emptyRes)
		client.Close()
	}

	g.Quit = true
	kill <- true
	return
}

// GetTurnDone returns TurnDone (SDL live view), and the current turn, sets TurnDone back to false
func (g *GOLWorker) GetTurnDone(req stubs.Empty, res *stubs.GetTurnDoneResponse) (err error) {
	g.Mu.Lock()
	defer g.Mu.Unlock()
	res.TurnDone = g.TurnDone
	res.Turn = g.Turn
	g.TurnDone = false
	return
}

// GetContinue function returns the World, Turn and the Continue bool (fault tolerance)
func (g *GOLWorker) GetContinue(req stubs.Empty, res *stubs.GetContinueResponse) (err error) {
	g.Mu.Lock()
	defer g.Mu.Unlock()
	res.World = g.World
	res.Continue = g.Continue
	res.Turn = g.Turn
	return
}

// GetCellFlipped function returns a struct array which contains variables required for CellFlipped events
func (g *GOLWorker) GetCellFlipped(req stubs.Empty, res *stubs.GetBrokerCellFlippedResponse) (err error) {
	g.Mu.Lock()
	defer g.Mu.Unlock()

	g.FlippedEvents = []stubs.FlippedEvent{}                      //set the global FlippedEvents to be an empty slice
	for _, cell := range findFlippedCells(g.World, g.LastWorld) { //calls function to find all flipped cells, using current and last world
		flippedEvent := stubs.FlippedEvent{ //initialises a struct for each flipped cell
			CompletedTurns: g.Turn,
			Cell:           cell,
		}
		g.FlippedEvents = append(g.FlippedEvents, flippedEvent) //appends to the FlippedEvents slice
	}

	g.LastWorld = g.World               //set the last world to be the current world
	res.FlippedEvents = g.FlippedEvents //assign the response
	return
}

//findFlippedCells function finds the flipped cells between two different worlds
func findFlippedCells(next [][]byte, current [][]byte) []util.Cell {
	var flipped []util.Cell

	//if either world is empty then return an empty slice
	if len(current) == 0 || len(next) == 0 || len(current[0]) == 0 || len(next[0]) == 0 {
		return flipped
	}

	xorWorld := xor2D(current, next) //initialise xorWorld to be the xor of the different worlds

	for i := 0; i < len(xorWorld); i++ { //iterate through the xorWorld to look for non-empty cells
		for j := 0; j < len(xorWorld[0]); j++ {
			if xorWorld[i][j] != 0 {
				flipped = append(flipped, util.Cell{j, i}) //append each flipped cell to the flipped slice
			}
		}
	}
	return flipped
}

// xor2D performs element-wise XOR on two [][]byte slices
func xor2D(a, b [][]byte) [][]byte {
	numRows := len(a)
	numCols := len(a[0])

	result := make([][]byte, numRows)
	for i := 0; i < numRows; i++ {
		result[i] = make([]byte, numCols)
		for j := 0; j < numCols; j++ {
			result[i][j] = a[i][j] ^ b[i][j] //xor each cell
		}
	}

	return result
}

func main() {
	pAddr := flag.String("port", "8030", "Port to listen on")
	flag.Parse()

	go func() { //runs concurrently with rest of the broker
		for { //infinitely loops
			if <-kill { //if a true bool is received from the kill channel then os.Exit()
				os.Exit(1)
			}
		}
	}()

	//set up client connection
	//global list of clients
	var workers []*rpc.Client
	workerPorts := ReadFileLines("workers.txt") //read the files and append the port numbers to the workers list
	for _, detail := range workerPorts {        //
		client, err := rpc.Dial("tcp", detail)
		if err == nil {
			workers = append(workers, client)
			fmt.Printf("Worker connected on: %v\n", detail) //print of a worker has been connected and also print the port
		}
	}

	rpc.Register(&GOLWorker{Workers: workers, Continue: false}) //initialise workers and initialise Continue to be false
	listener, err := net.Listen("tcp", ":"+*pAddr)
	if err != nil {
		fmt.Printf("Error starting listener: %s\n", err)
		os.Exit(1)
	}
	defer listener.Close()
	rpc.Accept(listener)
}
