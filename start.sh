#!/bin/bash

# Check if config.yaml exists
if [ ! -f config.yaml ]; then
  echo "config.yaml not found. Please create one from the template."
  exit 1
fi

# Start the Docker container
echo "Starting Message Router container..."
docker run -d \
  --name message-router \
  --network host \
  -v $(pwd)/config.yaml:/root/config.yaml \
  message-router:latest

echo "Container started! Access the app at http://localhost:8080"
