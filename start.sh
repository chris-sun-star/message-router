#!/bin/bash

# Load environment variables from .env file
if [ -f .env ]; then
  export $(grep -v '^#' .env | xargs)
else
  echo ".env file not found. Please create one from the template."
  exit 1
fi

# Check if Gemini API key is set
if [ "$GEMINI_API_KEY" == "your-google-gemini-api-key" ]; then
  echo "Error: Please set your GEMINI_API_KEY in the .env file."
  exit 1
fi

# Start the Docker container
echo "Starting Message Aggregator container..."
docker run -d \
  --name message-router \
  --network host \
  -e DB_DSN="${DB_DSN}" \
  -e JWT_SECRET="${JWT_SECRET}" \
  -e ENCRYPTION_KEY="${ENCRYPTION_KEY}" \
  -e GEMINI_API_KEY="${GEMINI_API_KEY}" \
  -e GIN_MODE="${GIN_MODE}" \
  message-router:latest

echo "Container started! Access the app at http://localhost:${PORT}"
