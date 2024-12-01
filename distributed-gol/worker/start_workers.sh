#!/bin/bash

# Bash script to spin up a specified number of workers for the Game of Life application.

# Usage: ./start_workers.sh <number_of_workers>

# Check if the number of workers is provided as an argument
if [ $# -ne 1 ]; then
    echo "Usage: $0 <number_of_workers>"
    exit 1
fi

# Number of workers to spin up
NUM_WORKERS=$1

# Base port number
BASE_PORT=8040

# Loop to start the workers
for ((i = 0; i < NUM_WORKERS; i++)); do
    PORT=$((BASE_PORT + i))
    echo "Starting worker on port $PORT..."
    # Open a new terminal for each worker
    gnome-terminal -- bash -c "go run . -port=$PORT; exec bash" &
    # Note: Replace `gnome-terminal` with your terminal application if not using GNOME.
done

echo "$NUM_WORKERS workers started successfully starting from port $BASE_PORT!"