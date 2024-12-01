#!/bin/bash

# Bash script to spin up a specified number of workers for the Game of Life application.

# Usage: ./start_workers.sh <number of workers>

# Check if the number of workers is provided as an argument
if [ $# -ne 1 ]; then
    echo "Usage: $0 <number_of_workers>"
    exit 1
fi

# Number of workers to spin up
NUM_WORKERS=$1

# Base port number
BASE_PORT=8040

# Path to the worker directory
WORKER_PATH="/Users/louiesinadjan/Documents/game-of-life/game-of-life/distributed-gol/worker"

# Loop to start the workers
for ((i = 0; i < NUM_WORKERS; i++)); do
    PORT=$((BASE_PORT + i))
    echo "Starting worker on port $PORT..."
    # Open a new terminal for each worker and navigate to the worker directory
    osascript -e "tell app \"Terminal\" to do script \"cd $WORKER_PATH && go run . -port=$PORT\""
done

echo "$NUM_WORKERS workers started successfully starting from port $BASE_PORT"