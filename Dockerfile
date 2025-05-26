FROM golang:1.24-bullseye AS builder

WORKDIR /app

COPY . .
RUN go mod tidy

RUN go build -o app .

FROM debian:bullseye-slim

WORKDIR /app

COPY --from=builder /app/app .

# Run the binary
ENTRYPOINT ["./app"]
