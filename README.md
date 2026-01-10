# ads-bscope

A Progressive Web Application that integrates ADS-B aircraft tracking data with Seestar telescope control to automatically slew telescopes to track aircraft. Uses the ASCOM Alpaca interface for telescope control.

## Features

- **ADS-B Integration**: Support for online services and local SDR receivers (RTL-SDR, HackRF One)
- **Telescope Control**: ASCOM Alpaca protocol for Seestar S30/S30-Pro/S50 telescopes
- **Mount Support**: Both Alt/Azimuth and Equatorial mounts
- **Secure**: User authentication, role-based access control, PostgreSQL database
- **Containerized**: Easy deployment with Docker Compose

## Quick Start

### Prerequisites
- Docker Desktop installed and running
- Your Seestar telescope IP address
- Your geographic location (latitude/longitude)

### Installation

1. **Clone the repository**
   ```bash
   git clone https://github.com/unklstewy/ads-bscope.git
   cd ads-bscope
   ```

2. **Configure environment**
   ```bash
   cp .env.example .env
   # Edit .env and set DB_PASSWORD
   ```

3. **Configure location and telescope**
   Edit `configs/config.json` and set:
   - Your observer location (latitude, longitude, elevation)
   - Your telescope URL (e.g., `http://192.168.1.100:11111`)

4. **Start the application**
   ```bash
   make run
   ```
   Or:
   ```bash
   docker-compose up -d
   ```

5. **Access the web interface**
   Open http://localhost:8080

## Development

### Local Development (without Docker)
```bash
# Build and run
make dev

# Run tests
make test

# Format code
make fmt
```

### Docker Development
```bash
# Build containers
make build

# View logs
make logs

# Stop containers
make stop

# Clean up everything
make clean
```

## Configuration

See `configs/README.md` for complete configuration documentation.

Key configuration areas:
- **Observer Location**: Required for accurate coordinate transformations
- **Telescope**: ASCOM Alpaca URL and mount type
- **ADS-B Source**: Online API or local SDR receiver
- **Database**: PostgreSQL connection settings

## Architecture

- **Language**: Go 1.25
- **Database**: PostgreSQL 16
- **Telescope Protocol**: ASCOM Alpaca
- **Deployment**: Docker containers

## Documentation

- [Configuration Guide](configs/README.md)
- [Development Guide](WARP.md)
- [Project Goals](INITSTATE.MD)

## License

See [LICENSE](LICENSE) file for details.
