# Use the official Golang image as the base image
FROM golang:1.24rc1

# Set the working directory inside the container
WORKDIR /app

# Copy the go.mod and go.sum files to the working directory
COPY go.mod go.sum ./

# Download the Go module dependencies
RUN go mod download

# Copy the rest of the application code to the working directory
COPY . .

# Run the tests
CMD ["go", "test", "-v", "./..."]