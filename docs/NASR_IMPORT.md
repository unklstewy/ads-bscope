# NASR Data Import Guide

## Overview

This guide explains how to import FAA NASR (National Airspace System Resources) data into the ADS-B Scope database. This data includes waypoints, navigation aids (VORs/NDBs), and airways that are essential for enhanced flight path prediction.

## What is NASR?

The FAA NASR database contains authoritative data about the National Airspace System, including:
- **Navigation Fixes**: Named waypoints used in flight plans
- **Navaids**: VORs, NDBs, TACANs for navigation
- **Airways**: Victor airways (low altitude) and Jet routes (high altitude)
- **Airports**: Airport information and coordinates

NASR data is updated every 28 days following the AIRAC (Aeronautical Information Regulation And Control) cycle.

## Downloading NASR Data

### Step 1: Visit the FAA NASR Subscription Page

Go to: https://www.faa.gov/air_traffic/flight_info/aeronav/aero_data/NASR_Subscription/

### Step 2: Download the Current Cycle

1. Click on the current cycle (e.g., "28-Day NASR Subscription - Effective Date: [DATE]")
2. Download the **Text Data** package (not the PDF)
3. The file will be named something like `28DaySubscription_Effective_2026-01-09.zip`

### Step 3: Extract the Data

```bash
# Create directory for NASR data
mkdir -p data/nasr

# Extract the downloaded file
unzip ~/Downloads/28DaySubscription_Effective_*.zip -d data/nasr/
```

### Step 4: Verify Required Files

Ensure these files are present in `data/nasr/`:
- `FIX.txt` - Navigation fixes (~40,000 waypoints)
- `NAV.txt` - VORs, NDBs, and TACANs (~2,000 navaids)
- `AWY.txt` - Airways with waypoint sequences (~50,000 segments)

```bash
ls -lh data/nasr/*.txt
```

## Running the Import

### Prerequisites

1. **Database running**: Ensure PostgreSQL is running
2. **Collector service stopped**: Stop the collector to avoid conflicts during import

```bash
# Check if database is accessible
psql -h localhost -U adsbscope -d adsbscope -c "SELECT version();"
```

### Import Command

```bash
# Run the NASR importer
go run cmd/import-nasr/main.go --nasr-dir data/nasr

# Or build and run
go build -o bin/import-nasr cmd/import-nasr/main.go
./bin/import-nasr --nasr-dir data/nasr
```

### Expected Output

```
===========================================
  NASR Data Importer
===========================================
Connecting to database...
✓ Database connected
✓ Schema initialized

===========================================
Importing Waypoints
===========================================
  Imported 1000 fixes...
  Imported 2000 fixes...
  ...
✓ Imported 40,123 navigation fixes
  Imported 100 navaids...
  Imported 200 navaids...
✓ Imported 1,847 navaids (VORs/NDBs)

===========================================
Importing Airways
===========================================
  Imported 1000 airway segments...
  Imported 2000 airway segments...
  ...
✓ Imported 48,392 airway segments

===========================================
Import Complete
===========================================
Total waypoints: 41,970
Total airway segments: 48,392
```

## Verifying the Import

### Check Waypoint Counts

```bash
psql -h localhost -U adsbscope -d adsbscope
```

```sql
-- Count waypoints by type
SELECT type, COUNT(*) FROM waypoints GROUP BY type;

-- Sample some waypoints
SELECT identifier, name, type, latitude, longitude 
FROM waypoints LIMIT 10;

-- Find specific waypoint (e.g., ATL VOR)
SELECT * FROM waypoints WHERE identifier = 'ATL';
```

### Check Airways

```sql
-- Count airways by type
SELECT type, COUNT(DISTINCT identifier) as airway_count 
FROM airways GROUP BY type;

-- View a specific airway (e.g., J121)
SELECT a.identifier, a.sequence, w.identifier as waypoint, 
       w.latitude, w.longitude
FROM airways a
JOIN waypoints w ON a.waypoint_id = w.id
WHERE a.identifier = 'J121'
ORDER BY a.sequence;
```

## Troubleshooting

### "Failed to open FIX.txt"

**Cause**: Files not in the expected directory

**Solution**: Check the `--nasr-dir` path is correct
```bash
ls -la data/nasr/FIX.txt
```

### "Waypoint not found" during airway import

**Cause**: Some airway waypoints reference fixes not in the FIX.txt file (international fixes, military waypoints)

**Impact**: These airway segments are skipped. This is normal and doesn't affect domestic US airways.

### Database connection errors

**Solution**: Verify database configuration in `configs/config.json`:
```json
{
  "database": {
    "host": "localhost",
    "port": 5432,
    "database": "adsbscope",
    "username": "adsbscope",
    "password": "changeme"
  }
}
```

## Data Updates

NASR data should be updated every 28 days to stay current with the AIRAC cycle.

### Update Procedure

1. Download the new cycle from the FAA website
2. Extract to a temporary directory
3. Run the import (it will update existing waypoints)

```bash
# The importer uses UPSERT, so running it again will update existing data
go run cmd/import-nasr/main.go --nasr-dir data/nasr-new
```

## Next Steps

Once the NASR data is imported:

1. **Phase 2**: Integrate FlightAware API for flight plans
2. **Phase 3**: Implement waypoint-based prediction
3. **Phase 4**: Add airway matching for aircraft without flight plans

See the [Enhanced Prediction Plan](../plans/) for details.

## File Formats

### FIX.txt Format

Fixed-width text file with columns:
- Columns 1-4: Record type ("FIX1")
- Columns 5-34: Fix identifier
- Columns 35-36: Region code
- Columns 67-80: Latitude (DD-MM-SS.SSSH)
- Columns 81-94: Longitude (DD-MM-SS.SSSH)

### NAV.txt Format

Fixed-width text file with columns:
- Columns 1-4: Record type ("NAV1")
- Columns 5-8: Navaid identifier
- Columns 9-28: Navaid type
- Columns 43-72: Navaid name
- Columns 372-385: Latitude
- Columns 397-410: Longitude

### AWY.txt Format

Fixed-width text file with columns:
- Columns 1-4: Record type ("AWY2")
- Columns 5-9: Airway identifier
- Columns 10-14: Sequence number
- Columns 16-45: Waypoint identifier

For complete format specifications, see the NASR Data Format Specification PDF included in the download.

## Storage Requirements

- **Waypoints table**: ~10 MB (40,000 records)
- **Airways table**: ~15 MB (50,000 records)
- **Total**: ~25 MB

With indexes: ~35-40 MB total
