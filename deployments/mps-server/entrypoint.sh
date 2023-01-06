#!/bin/bash
set -e

# Start MPS control daemon
nvidia-cuda-mps-control -d && echo "MPS control daemon started"
sleep 2

# Start MPS server
echo "start_server -uid $(id -u)" | nvidia-cuda-mps-control
echo "MPS server started [uid $(id -u)]"

# Show server and daemon logs
tail -f "${CUDA_MPS_LOG_DIRECTORY}"/*.log