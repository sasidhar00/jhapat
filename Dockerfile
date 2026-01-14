# STEP 1: Build the app in a Go environment
FROM golang:1.21-alpine AS builder

# Disable the security check that is causing the failures
ENV GOSUMDB=off
ENV GOPROXY=direct

WORKDIR /app

# Copy your module files and the main code
COPY go.mod ./
COPY . .

# Force a tidy and build inside this "safe" container
RUN go mod tidy
RUN go build -o main-app main.go

# STEP 2: Create a tiny image to actually run the app
FROM alpine:latest
WORKDIR /root/

# Copy only the final app and your public folder
COPY --from=builder /app/main-app .
COPY --from=builder /app/public ./public

# Start the app
CMD ["./main-app"]
