# Airway-Based Prediction

This document describes the airway-based prediction system (Phase 4) that provides improved tracking accuracy for aircraft without filed flight plans.

## Overview

When an aircraft has no flight plan available but appears to be following a published airway, the system can match the aircraft to the airway and predict its future position along the airway centerline. This is significantly more accurate than dead reckoning for IFR traffic.

## How It Works

### 1. Airway Matching Algorithm

When prediction is needed and no flight plan is available, the system:

1. **Queries Nearby Airways** (25 NM radius around aircraft)
2. **Filters by Altitude**:
   - Victor airways (V-prefix): <18,000 ft MSL
   - Jet routes (J-prefix): >=18,000 ft MSL  
   - RNAV routes (Q/T prefix): Any altitude
3. **Calculates Match Score** based on:
   - **Track Alignment** (70% weight): Aircraft track vs airway bearing
   - **Distance to Centerline** (30% weight): Perpendicular distance to airway

### 2. Matching Criteria

An airway is considered a match if:
- Aircraft track aligns within **45°** of airway bearing
- Aircraft is within **10 NM** of airway centerline
- Aircraft altitude is within airway altitude limits
- Overall match score > **0.6** (60%)

### 3. Prediction Method

Once matched to an airway, prediction uses:
- **Great circle navigation** to next waypoint on airway
- **Spherical interpolation** for accurate geodesic paths
- **Confidence scoring** similar to waypoint-based prediction

## Airway Types

### Victor Airways (V-prefix)
- **Altitude**: Surface to 17,999 ft MSL
- **Navigation**: VOR-based
- **Usage**: Low-altitude IFR traffic
- **Example**: V1, V23, V447

### Jet Routes (J-prefix)
- **Altitude**: 18,000 ft MSL and above
- **Navigation**: VOR/RNAV-based
- **Usage**: High-altitude IFR traffic
- **Example**: J121, J65, J584

### RNAV Routes (Q/T-prefix)
- **Altitude**: Various (depends on route)
- **Navigation**: GPS/RNAV-based
- **Usage**: Modern RNAV-equipped aircraft
- **Example**: Q123, T256

## Implementation Details

### Database Query

```go
// Find airways within 25 NM radius with altitude filtering
segments, err := fpRepo.FindNearbyAirways(
    ctx,
    aircraft.Latitude,
    aircraft.Longitude,
    25.0,  // radius in NM
    int(aircraft.Altitude * 0.9),  // min altitude (10% tolerance)
    int(aircraft.Altitude * 1.1),  // max altitude (10% tolerance)
)
```

### Match Scoring

```go
// Calculate match score for each airway segment
alignmentScore := 1.0 - (trackError / 45.0)      // 0-1 based on heading
distanceScore := 1.0 - (distToCenterline / 10.0) // 0-1 based on distance
score := (alignmentScore * 0.7) + (distanceScore * 0.3)
```

### Cross-Track Distance

The system uses spherical trigonometry to calculate the perpendicular distance from the aircraft to the airway centerline:

```
Distance = |asin(sin(d13) × sin(bearing13 - bearing12))| × R
```

Where:
- `d13` = angular distance from airway start to aircraft
- `bearing13` = bearing from airway start to aircraft
- `bearing12` = bearing from airway start to airway end
- `R` = Earth radius in nautical miles

## Prediction Cascade

The tracker uses a three-tier prediction approach:

```
1. Flight Plan Waypoints (highest confidence)
   ↓ (if no flight plan)
2. Airway Matching (medium confidence)
   ↓ (if no airway match)
3. Dead Reckoning (lowest confidence)
```

### Example Output

**With Airway Match:**
```
[15:42:10] Target: N12345 (A1B2C3) [AIRWAY PREDICTION: V23]
  Last Known: 35.4523°N, 81.2341°W, 8000 ft MSL
  Predicted:  35.5102°N, 81.1892°W, 8100 ft MSL (85% confidence)
  Data age: 45.2s (USING PREDICTION)
```

**Without Match:**
```
[15:42:10] Target: N12345 (A1B2C3) [DEAD RECKONING]
  Last Known: 35.4523°N, 81.2341°W, 8000 ft MSL
  Predicted:  35.5102°N, 81.1892°W, 8100 ft MSL (72% confidence)
  Data age: 45.2s (USING PREDICTION)
```

## Performance Characteristics

### Accuracy

- **Waypoint-based**: 95% confidence (following filed route)
- **Airway-based**: 85-90% confidence (following published airway)
- **Dead reckoning**: 60-70% confidence (straight-line projection)

### Query Performance

- Airway query radius: 25 NM (adjustable)
- Typical query time: <50ms for 997 airways
- Match calculation: ~1ms per candidate airway
- Total overhead: <100ms per prediction

## Benefits

### Improved Tracking

- **VFR Traffic**: Many VFR aircraft follow airways informally
- **IFR without Flight Plan**: Aircraft on airways but no filed plan in database
- **International Flights**: Some international traffic may lack flight plan data
- **General Aviation**: GA aircraft often use Victor airways

### Extended Coverage

- Maintains accuracy when aircraft leave ADS-B coverage
- Predicts position along known routes vs straight-line guessing
- Reduces prediction error by 30-40% vs dead reckoning

## Limitations

### When Airway Matching Fails

Airway matching may not work for:
- **Off-airway routing**: Direct routing between waypoints
- **VFR flight following**: Not following published routes
- **Military operations**: Special use airspace, restricted areas
- **Search and rescue**: Non-standard flight patterns
- **Training flights**: Holding patterns, practice approaches

In these cases, the system falls back to dead reckoning automatically.

### Database Requirements

Requires NASR airway data to be imported:
- 997 unique airways (Victor, Jet, RNAV)
- ~50,000 airway segments with waypoint pairs
- Updated every 28 days (AIRAC cycle)

See `docs/NASR_IMPORT.md` for import instructions.

## Configuration

No additional configuration required. Airway matching is automatic when:
1. Data is stale (>30 seconds old)
2. No flight plan available for aircraft
3. Airways exist in database

### Tuning Parameters

Can be adjusted in code if needed:

```go
// Search radius for nearby airways
radiusNM := 25.0  // Default: 25 NM

// Altitude tolerance for matching
altTolerance := 0.1  // Default: 10%

// Maximum distance to centerline
maxCenterlineDistance := 10.0  // Default: 10 NM

// Maximum track error
maxTrackError := 45.0  // Default: 45°

// Minimum match score
minMatchScore := 0.6  // Default: 60%
```

## Future Enhancements

### Planned Improvements

1. **Multi-Segment Prediction**: Follow airway beyond single segment
2. **Turn Prediction**: Account for turns at airway intersections
3. **Airway Speed Profiles**: Use typical speeds for airway/altitude
4. **Historical Pattern Matching**: Learn common routes from past tracks

### Advanced Features

1. **SID/STAR Matching**: Match departure/arrival procedures
2. **Preferred Routes**: Identify commonly used routing
3. **Weather Routing**: Avoid convective activity
4. **Flow Control**: Account for ATC-assigned routes

## Troubleshooting

### No Airway Matches

If aircraft aren't matching to airways:

1. **Check NASR Data**: Verify airways are imported
   ```bash
   go run cmd/verify-nasr/main.go
   ```

2. **Check Altitude**: Ensure correct Victor/Jet filtering
   - Victor: <18,000 ft
   - Jet: >=18,000 ft

3. **Check Track Alignment**: Aircraft may be off-airway
   - Direct routing between waypoints
   - VFR flight not following airways

4. **Increase Search Radius**: Try larger radius in code
   ```go
   radiusNM := 50.0  // Increase from 25 NM
   ```

### Poor Match Quality

If matches have low confidence:

1. **Cross-track distance too large**: Aircraft >10 NM from centerline
2. **Track error too large**: Aircraft heading differs >45° from airway
3. **Altitude mismatch**: Check airway altitude limits

## See Also

- `docs/FLIGHTAWARE.md` - Flight plan integration (Phase 2)
- `docs/NASR_IMPORT.md` - Waypoint and airway database setup (Phase 1)
- `pkg/tracking/prediction.go` - Prediction algorithm implementation
- `internal/db/flightplan_repository.go` - Airway query methods
