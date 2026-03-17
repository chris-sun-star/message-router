# Stage 1: Build the frontend
FROM node:20 AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm install --force
COPY frontend/ ./
RUN npm run build

# Stage 2: Build the backend
FROM golang:1.25 AS backend-builder
WORKDIR /app
# Copy go mod files
COPY backend/go.mod backend/go.sum ./backend/
RUN cd backend && go mod download

# Copy backend source
COPY backend/ ./backend/
# Copy built frontend to backend/dist for embedding
COPY --from=frontend-builder /app/frontend/dist ./backend/dist

# Build the binary
RUN cd backend && CGO_ENABLED=0 GOOS=linux go build -o /message-router main.go

# Stage 3: Final image
FROM ubuntu:24.04
RUN apt-get update && apt-get install -y ca-certificates tzdata && rm -rf /var/lib/apt/lists/*
WORKDIR /root/
COPY --from=backend-builder /message-router .

# Create a default .env file placeholder if needed
RUN touch .env

EXPOSE 8080
CMD ["./message-router"]
