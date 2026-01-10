# WARP.md

This file provides guidance to WARP (warp.dev) when working with code in this repository.

## Project Overview

ads-bscope is a PWA (Progressive Web Application) that integrates ADS-B aircraft tracking data with Seestar telescope control to automatically slew telescopes to track aircraft. The system uses the ASCOM Alpaca interface for telescope control.

**Key Technologies:**
- **Language**: Go (install via Homebrew if needed)
- **Telescope Control**: ASCOM Alpaca interface
- **ADS-B Data**: Online services or local SDR (RTL-SDR, HackRF One)
- **Target Hardware**: Seestar S30/S30-Pro/S50 telescopes
- **Mount Types**: Alt/Azimuth and Equatorial
- **Deployment**: Docker containers

## Development Commands

### Go Commands
```bash
# Initialize Go module (if not done)
go mod init github.com/unklstewy/ads-bscope

# Download dependencies
go mod download

# Tidy dependencies
go mod tidy

# Build the application
go build -o bin/ads-bscope ./cmd/ads-bscope

# Run the application
go run ./cmd/ads-bscope

# Run tests
go test ./...

# Run tests with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run a specific test
go test -v -run TestName ./path/to/package

# Run tests in a specific package
go test -v ./pkg/adsb

# Format code
go fmt ./...

# Lint (requires golangci-lint)
golangci-lint run

# Vet code
go vet ./...
```

### Docker Commands
```bash
# Build Docker image
docker build -t ads-bscope:latest .

# Run container
docker run -p 8080:8080 ads-bscope:latest

# Docker Compose (when docker-compose.yml exists)
docker-compose up -d
docker-compose down
docker-compose logs -f
```

## Configuration

**Centralized Configuration**: All application settings are managed through `pkg/config/config.go` and loaded from JSON files in `configs/`. Configuration can be overridden via environment variables for sensitive data (passwords, API keys).

**Environment Variables**:
- `CONFIG_PATH`: Path to config file (default: `configs/config.json`)
- `ADS_BSCOPE_DB_PASSWORD`: Database password
- `ADS_BSCOPE_ADSB_API_KEY`: ADS-B API key
- `ADS_BSCOPE_TELESCOPE_URL`: Telescope Alpaca URL

**Configuration Documentation**: See `configs/README.md` for complete configuration reference.

**Important**: Never hardcode configuration values. All modules that require configuration should accept it through the config system.

## Architecture Guidelines

### Code Organization
The project follows standard Go project layout:
- `cmd/`: Main application entry points
  - `cmd/ads-bscope/`: Main web application
- `pkg/`: Library code (importable by other projects)
  - `pkg/config/`: Centralized configuration management
  - `pkg/adsb/`: ADS-B data acquisition and parsing
  - `pkg/alpaca/`: ASCOM Alpaca client implementation
  - `pkg/telescope/`: Telescope control abstractions (TODO)
  - `pkg/coordinates/`: Astronomical coordinate transformations (TODO)
  - `pkg/tracking/`: Aircraft tracking and prediction algorithms (TODO)
- `internal/`: Private application code
  - `internal/auth/`: User authentication and authorization (TODO)
  - `internal/api/`: HTTP API handlers (TODO)
  - `internal/db/`: Database layer (TODO)
- `web/`: PWA frontend assets (TODO)
- `configs/`: Configuration files and documentation
- `scripts/`: Build and deployment scripts (TODO)
- `test/`: Integration tests (TODO)

### Key Architectural Concepts

**Mount Type Support**: The application must handle both Alt/Azimuth and Equatorial mount types with appropriate coordinate transformations.

**ADS-B Integration**: Support both online ADS-B data services (e.g., ADS-B Exchange API) and local SDR receivers. Abstract the data source to allow switching.

**ASCOM Alpaca Interface**: Use the ASCOM Alpaca REST API for telescope control. The interface is telescope-agnostic but must support Seestar-specific features through the seestar_alp project.

**Seestar Control**: Reference implementation at `git@github.com:smart-underworld/seestar_alp.git`. The Seestar telescopes use the Alpaca protocol but may have specific extensions for camera control.

**Coordinate Transformations**: Aircraft position (lat/lon/altitude) → Local horizontal coordinates (Alt/Az) → Mount coordinates (Alt/Az or RA/Dec depending on mount type).

**Seestar Tracking Limits**: The Seestar Alt-Az mount has specific constraints:
- **Altitude Range**: 20° to 80° (optimal viewing window)
  - Below 20°: atmospheric refraction and practical viewing limits
  - Above 80°: severe field rotation causes tracking issues ("Dobsonian hole")
  - Above 85°: telescope may stop stacking frames entirely
- **Field Rotation**: Most severe near zenith and when pointing N/S, minimal when pointing E/W
- **Azimuth**: Full 360° rotation capability
- **EQ Mode**: When mounted on equatorial wedge, eliminates field rotation but tracking limits still apply

**Security Model**: PWA must follow modern security practices with user authentication, role-based access control, and secure database storage.

## Code Standards

**Commenting Requirements**: All code must be heavily commented:
- Every function, method, interface, and struct must have comprehensive documentation
- Explain the "why" not just the "what"
- Document expected input/output ranges and units (especially for coordinates)
- Include references to algorithms or external documentation where applicable

**Units and Conventions**:
- Angles: Use consistent units (recommend radians internally, degrees for display)
- Coordinates: Document whether using J2000, JNOW, or apparent coordinates
- Time: Use UTC for all astronomical calculations

**Testing**: The ASCOM Alpaca Simulator should be used for testing telescope control without hardware. Install from https://github.com/ASCOMInitiative

## Database

Use an open-source database with strong security models (PostgreSQL recommended). The database must support:
- User accounts and authentication
- Role-based access control
- Session management
- Configuration storage
- Aircraft tracking history (optional, for debugging)

## CI/CD and Project Management

- Use GitHub Actions for CI/CD
- Track work using GitHub Issues
- Document architectural decisions and API documentation in GitHub Wiki
- Follow standard open-source contribution guidelines

## References

- Seestar Alpaca implementation: https://github.com/smart-underworld/seestar_alp
- ASCOM Initiative repos: https://github.com/ASCOMInitiative
- ASCOM Alpaca API Documentation: https://ascom-standards.org/Developer/Alpaca.htm
