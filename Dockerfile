FROM golang:1.24.6-bullseye AS builder

WORKDIR /app

COPY . .
RUN go mod tidy

RUN go build -o app .

FROM debian:12.12-slim

# Install ca-certificates for SSL certificate verification
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/app .

# Run the binary
ENTRYPOINT ["./app"]
