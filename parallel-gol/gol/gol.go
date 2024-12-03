package gol

// Params provides the details of how to run the Game of Life and which image to load.
type Params struct {
	Turns       int // Number of turns to simulate
	Threads     int // Number of concurrent worker threads
	ImageWidth  int
	ImageHeight int
}

// Run starts the processing of Game of Life. It initialises channels and goroutines.
func Run(p Params, events chan<- Event, keyPresses <-chan rune, random bool) {
	// Initialise I/O channels for communication with the I/O handler.
	ioCommand := make(chan ioCommand) // Channel for sending I/O commands (e.g., load, save).
	ioIdle := make(chan bool)         // Channel to monitor if the I/O handler is idle.
	ioFilename := make(chan string)   // Channel for sending file names for load/save operations.
	ioOutput := make(chan uint8)      // Channel for sending output data to the I/O handler.
	ioInput := make(chan uint8)       // Channel for receiving input data from the I/O handler.

	// Group the I/O channels into a single structure for easier management and passing to functions.
	ioChannels := ioChannels{
		command:  ioCommand,  // Command channel for I/O operations.
		idle:     ioIdle,     // Idle state channel for I/O monitoring.
		filename: ioFilename, // File name channel for load/save.
		output:   ioOutput,   // Output channel for writing data.
		input:    ioInput,    // Input channel for reading data.
	}

	// Start the I/O handler as a goroutine. This handles all file I/O operations
	// such as loading initial grid states or saving simulation results.
	go startIo(p, ioChannels)

	// Initialise the distributor channels for communication between the distributor, I/O handler, and simulation workers.
	distributorChannels := distributorChannels{
		events:     events, // Channel for sending simulation events (e.g., cell updates) to the visualisation or external handlers.
		ioCommand:  ioCommand,
		ioIdle:     ioIdle,
		ioFilename: ioFilename,
		ioOutput:   ioOutput,
		ioInput:    ioInput,
		keyPresses: keyPresses, // Channel for handling user key presses (e.g., pause, quit).
	}

	distributor(p, distributorChannels, random)
}
