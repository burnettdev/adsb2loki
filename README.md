# FlightAware2Loki

🚀 A Go service that fetches aircraft data from FlightAware's SkyAware and streams it to Grafana Loki for real-time monitoring and analysis.

## Features

- Real-time aircraft data collection from FlightAware SkyAware
- Automatic data streaming to Grafana Loki
- Configurable via environment variables
- Graceful shutdown handling
- Efficient batch processing of aircraft data

## Prerequisites

- Go 1.24 or later
- FlightAware SkyAware instance
- Grafana Alloy
- Grafana Loki instance (Cloud or OSS)

## Configuration

Create a `.env` file in the project root with the following variables:

```env
SKYAWARE_URL=http://your-skyaware-instance/skyaware/data/aircraft.json
ALLOY_URL=http://your-alloy-instance
```

## Installation

1. Clone the repository:
```bash
git clone https://github.com/burnettdev/flightaware2loki.git
cd flightaware2loki
```

2. Install dependencies:
```bash
go mod tidy
```

3. Build the application:
```bash
go build
```

## Usage

Run the application:
```bash
./flightaware2loki
```

The service will:
- Fetch aircraft data every 5 seconds
- Push the data to Loki with appropriate labels
- Log any errors that occur during the process

## Data Structure

Each aircraft entry in Loki includes:
- Timestamp from FlightAware
- Labels for easy querying
- Full aircraft data as JSON

## Contributing

Feel free to open issues or submit pull requests!

## License

MIT License - feel free to use this project for whatever you'd like!
