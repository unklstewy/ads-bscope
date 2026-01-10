# Configuration

The ads-bscope application uses a centralized configuration system that supports both file-based and environment variable-based configuration.

## Configuration Files

The default configuration file is `configs/config.json`. You can specify a different path using the `CONFIG_PATH` environment variable:

```bash
CONFIG_PATH=/path/to/config.json ./bin/ads-bscope
```

## Configuration Structure

### Server Configuration
- `port`: HTTP server port (default: "8080")
- `host`: Server bind address (default: "0.0.0.0")
- `tls_enabled`: Enable HTTPS
- `tls_cert_file`: Path to TLS certificate
- `tls_key_file`: Path to TLS private key

### Database Configuration
- `driver`: Database driver (postgres, mysql, sqlite)
- `host`: Database server hostname
- `port`: Database server port
- `database`: Database name
- `username`: Database username
- `password`: Database password (prefer environment variable)
- `ssl_mode`: PostgreSQL SSL mode (disable, require, verify-ca, verify-full)
- `max_open_conns`: Maximum number of open connections
- `max_idle_conns`: Maximum number of idle connections

### Telescope Configuration
- `base_url`: ASCOM Alpaca server URL (e.g., "http://192.168.1.100:11111")
- `device_number`: Alpaca device number (typically 0)
- `mount_type`: Mount type ("altaz" or "equatorial")
- `slew_rate`: Slew speed in degrees per second
- `tracking_enabled`: Enable telescope tracking
- `model`: Telescope model ("seestar-s30", "seestar-s50", "generic")
- `supports_meridian_flip`: Whether telescope requires meridian flips
  - `false` for Seestar (fork mount with 360° rotation)
  - `true` for German Equatorial Mounts (GEM)
- `max_altitude`: Maximum safe tracking altitude in degrees
  - `0` = auto-detect based on model and mount_type
  - Seestar Alt-Az: 80° (field rotation limit)
  - Seestar Equatorial (with wedge): 85° (physical limit)
  - Generic: 85°
- `min_altitude`: Minimum tracking altitude in degrees
  - `0` = auto-detect based on model and mount_type
  - Seestar Alt-Az: 20° (practical viewing range)
  - Seestar Equatorial: 15° (atmospheric limit)
  - Generic: 15°

### ADS-B Configuration
- `source_type`: Data source type ("online" or "local")
- `online_api_url`: URL for online ADS-B services
- `online_api_key`: API key for online services (prefer environment variable)
- `local_host`: Hostname for local SDR receiver
- `local_port`: Port for local SDR receiver (e.g., 30002 for dump1090)
- `search_radius_nm`: Search radius in nautical miles
- `update_interval_seconds`: Data refresh interval

### Observer Configuration
- `latitude`: Observer latitude in decimal degrees (-90 to +90)
- `longitude`: Observer longitude in decimal degrees (-180 to +180)
- `elevation`: Observer elevation in meters above sea level
- `timezone`: IANA timezone name (e.g., "America/New_York")

## Environment Variables

Sensitive configuration values should be provided via environment variables:

- `CONFIG_PATH`: Path to configuration file
- `ADS_BSCOPE_PORT`: Override server port
- `ADS_BSCOPE_DB_PASSWORD`: Database password
- `ADS_BSCOPE_ADSB_API_KEY`: ADS-B API key
- `ADS_BSCOPE_TELESCOPE_URL`: Telescope Alpaca URL

Environment variables take precedence over configuration file values.

## Example Configuration

```json
{
  "server": {
    "port": "8080",
    "host": "0.0.0.0"
  },
  "database": {
    "driver": "postgres",
    "host": "postgres",
    "port": 5432,
    "database": "adsbscope",
    "username": "adsbscope"
  },
  "telescope": {
    "base_url": "http://seestar.local:11111",
    "device_number": 0,
    "mount_type": "altaz"
  },
  "adsb": {
    "source_type": "local",
    "local_host": "piaware.local",
    "local_port": 30002,
    "search_radius_nm": 50.0
  },
  "observer": {
    "latitude": 40.7128,
    "longitude": -74.0060,
    "elevation": 10.0,
    "timezone": "America/New_York"
  }
}
```

## Docker Environment

When running in Docker, use environment variables to override configuration:

```bash
docker run -p 8080:8080 \
  -e ADS_BSCOPE_DB_PASSWORD=secret \
  -e ADS_BSCOPE_TELESCOPE_URL=http://seestar:11111 \
  ads-bscope:latest
```
