package gol

import (
	"fmt"
	"time"
	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
	keyPresses <-chan rune
}

func worker(id int, p Params, world [][]byte, result chan<- [][]byte, c distributorChannels, turn int) {
	var heightDiff = float32(p.ImageHeight) / float32(p.Threads) //height of each slice that the worker works on

	startRow := int(float32(id) * heightDiff) //distributes the slices of the world to each worker
	endRow := int(float32(id+1) * heightDiff)

	newWorld := calculateNextState(world, startRow, endRow, c, turn, p)

	result <- newWorld //send the result to result channel
}

func savePGMImage(c distributorChannels, world [][]byte, p Params) {

	c.ioCommand <- ioOutput
	c.ioFilename <- fmt.Sprintf("%dx%dx%d", p.ImageWidth, p.ImageHeight, p.Turns)

	for i := range world {
		for j := range world[i] {
			c.ioOutput <- world[i][j]
		}
	}
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	// Create a 2D slice to store the world.

	c.ioCommand <- ioInput
	c.ioFilename <- fmt.Sprintf("%d%s%d", p.ImageWidth, "x", p.ImageHeight)

	world := make([][]uint8, p.ImageHeight)
	newWorld := [][]byte{}

	for i := range world {
		world[i] = make([]uint8, p.ImageWidth)
	}

	for i := 0; i < p.ImageHeight; i++ {
		for j := 0; j < p.ImageWidth; j++ {
			world[i][j] = <-c.ioInput
		}
	}

	for i := range world {
		for j := range world[i] {
			if world[i][j] == 255 {
				c.events <- CellFlipped{0, util.Cell{j, i}} //initially show all alive cells
			}
		}
	}

	turn := 0
	quit := false
	resultCh := make([]chan [][]byte, p.Threads)
	for i := range resultCh {
		resultCh[i] = make(chan [][]byte)
	}

	ticker := time.NewTicker(2 * time.Second) //initialise a ticker that ticks every 2 seconds

	for turn := 0; turn < p.Turns; turn++ {
		if quit {
			break
		}

		for i := 0; i < p.Threads; i++ {
			go worker(i, p, world, resultCh[i], c, turn) //concurrently call worker
		}

		for i := 0; i < p.Threads; i++ {
			resultPart := <-resultCh[i]                //receive from the result channel slice using indices to get it in order
			newWorld = append(newWorld, resultPart...) //append each result to the new world
		}

		world = append([][]byte{}, newWorld...) //assign new world to the world
		newWorld = [][]byte{}

		select {
		case <-ticker.C: //when a tick is received from ticker channel, send an AliveCellsCount event to the channel
			c.events <- AliveCellsCount{turn + 1, len(calculateAliveCells(world))}
		case command := <-c.keyPresses:
			switch command { //key presses
			case 's':
				c.events <- StateChange{turn, Executing}
				savePGMImage(c, world, p)
			case 'q':
				c.events <- StateChange{turn, Quitting}
				savePGMImage(c, world, p)
				quit = true
				break
			case 'p':
				c.events <- StateChange{turn, Paused}
				fmt.Printf("Current turn %d being processed\n", turn)
				for { //infinite for loop that only breaks when another 'p' is pressed
					if <-c.keyPresses == 'p' {
						break
					}
				}
				c.events <- StateChange{turn, Executing}
			}
		default:
		}
		c.events <- TurnComplete{CompletedTurns: turn}

	}

	calculateAliveCells(world)

	c.events <- FinalTurnComplete{turn, calculateAliveCells(world)}
	savePGMImage(c, world, p)

	// Make sure that the IO has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	// Send a StateChange event to indicate quitting
	c.events <- StateChange{p.Turns, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}

func calculateNextState(world [][]byte, startRow, endRow int, c distributorChannels, turn int, p Params) [][]byte {
	height := p.ImageHeight
	width := p.ImageWidth

	nextState := make([][]byte, endRow-startRow)

	for i := 0; i < endRow-startRow; i++ {
		nextState[i] = make([]byte, width)
	}

	for i := startRow; i < endRow; i++ {
		for j := 0; j < width; j++ {
			//sum of neighboring cells around the current one
			sum := (int(world[(i+height-1)%height][(j+width-1)%width]) +
				int(world[(i+height-1)%height][(j+width)%width]) +
				int(world[(i+height-1)%height][(j+width+1)%width]) +
				int(world[(i+height)%height][(j+width-1)%width]) +
				int(world[(i+height)%height][(j+width+1)%width]) +
				int(world[(i+height+1)%height][(j+width-1)%width]) +
				int(world[(i+height+1)%height][(j+width)%width]) +
				int(world[(i+height+1)%height][(j+width+1)%width])) / 255

			//if live cell
			if world[i][j] == 255 {
				//if less than 2 neighbors then die
				if sum < 2 {
					nextState[i-startRow][j] = 0
					c.events <- CellFlipped{turn, util.Cell{j, i}}
				} else if sum == 2 || sum == 3 { //if 2 or 3 neighbors then unaffected
					nextState[i-startRow][j] = 255
				} else { //if more than 3 neighbors then die
					nextState[i-startRow][j] = 0
					c.events <- CellFlipped{turn, util.Cell{j, i}}
				}
			} else { //if dead cell
				//if 3 neighbors then alive
				if sum == 3 {
					nextState[i-startRow][j] = 255

					c.events <- CellFlipped{turn, util.Cell{j, i}}

				} else { //else unaffected
					nextState[i-startRow][j] = 0
				}
			}
		}
	}

	return nextState
}

func calculateAliveCells(world [][]byte) []util.Cell {

	aliveCells := []util.Cell{}

	for i := range world { //height
		for j := range world[i] { //width
			if world[i][j] == 255 {
				aliveCells = append(aliveCells, util.Cell{j, i})
			}
		}
	}

	return aliveCells
}
