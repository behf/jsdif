FROM golang:1.20-alpine

# Install git
RUN apk add --no-cache git

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN go build -o jsdif

# Expose default port
EXPOSE 9093

# Create volume for persistent storage
VOLUME ["/app/js_snapshots"]

# Run the application
ENTRYPOINT ["./jsdif"]
CMD ["run"]
