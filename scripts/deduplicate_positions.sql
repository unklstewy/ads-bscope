-- Deduplicate aircraft_positions table by removing redundant stationary positions
-- This script identifies and removes consecutive position records where the aircraft
-- hasn't moved, keeping only the first occurrence of each stationary period.

-- Analysis: Find redundant positions before deletion
-- Shows how many redundant positions exist per aircraft
DO $$
DECLARE
    total_positions BIGINT;
    redundant_positions BIGINT;
BEGIN
    SELECT COUNT(*) INTO total_positions FROM aircraft_positions;
    
    RAISE NOTICE 'Total positions before cleanup: %', total_positions;
    RAISE NOTICE 'Analyzing redundant positions...';
    
    -- Count positions that will be removed
    SELECT COUNT(*) INTO redundant_positions
    FROM (
        SELECT 
            ap1.id,
            ap1.icao,
            ap1.timestamp
        FROM aircraft_positions ap1
        JOIN aircraft_positions ap2 ON 
            ap1.icao = ap2.icao
            AND ap2.timestamp < ap1.timestamp
            AND NOT EXISTS (
                -- Check if there's any position between ap2 and ap1
                SELECT 1 FROM aircraft_positions ap3
                WHERE ap3.icao = ap1.icao
                  AND ap3.timestamp > ap2.timestamp
                  AND ap3.timestamp < ap1.timestamp
            )
        WHERE 
            -- Position unchanged (0.000001 degrees ≈ 0.1m)
            ABS(ap1.latitude - ap2.latitude) < 0.000001
            AND ABS(ap1.longitude - ap2.longitude) < 0.000001
            -- Altitude unchanged (1 foot tolerance)
            AND ABS(ap1.altitude_ft - ap2.altitude_ft) < 1.0
            -- Both positions stationary (<1 knot)
            AND COALESCE(ap1.ground_speed_kts, 0) < 1.0
            AND COALESCE(ap2.ground_speed_kts, 0) < 1.0
    ) redundant;
    
    RAISE NOTICE 'Redundant positions found: % (%.1f%%)', 
        redundant_positions, 
        (redundant_positions::FLOAT / total_positions * 100);
END $$;

-- Create temporary table with positions to keep
-- Step 1: Calculate previous position values
CREATE TEMP TABLE positions_with_lag AS
SELECT 
    id,
    icao,
    timestamp,
    latitude,
    longitude,
    altitude_ft,
    ground_speed_kts,
    LAG(latitude) OVER w AS prev_latitude,
    LAG(longitude) OVER w AS prev_longitude,
    LAG(altitude_ft) OVER w AS prev_altitude,
    LAG(ground_speed_kts) OVER w AS prev_speed
FROM aircraft_positions
WINDOW w AS (PARTITION BY icao ORDER BY timestamp);

-- Step 2: Identify position changes
CREATE TEMP TABLE positions_with_changes AS
SELECT 
    id,
    icao,
    timestamp,
    latitude,
    longitude,
    altitude_ft,
    ground_speed_kts,
    CASE 
        WHEN prev_latitude IS NULL THEN 1  -- First position
        WHEN ABS(latitude - prev_latitude) >= 0.000001 THEN 1
        WHEN ABS(longitude - prev_longitude) >= 0.000001 THEN 1
        WHEN ABS(altitude_ft - prev_altitude) >= 1.0 THEN 1
        WHEN (COALESCE(ground_speed_kts, 0) < 1.0) != (COALESCE(prev_speed, 0) < 1.0) THEN 1
        ELSE 0
    END AS position_changed
FROM positions_with_lag;

-- Step 3: Create groups of consecutive identical positions
CREATE TEMP TABLE positions_with_groups AS
SELECT 
    id,
    icao,
    timestamp,
    SUM(position_changed) OVER (PARTITION BY icao ORDER BY timestamp) AS position_group
FROM positions_with_changes;

-- Step 4: Keep only the first position of each group
CREATE TEMP TABLE positions_to_keep AS
SELECT DISTINCT ON (icao, position_group)
    id,
    icao,
    timestamp
FROM positions_with_groups
ORDER BY icao, position_group, timestamp ASC;

-- Show statistics
DO $$
DECLARE
    kept_count BIGINT;
    total_count BIGINT;
BEGIN
    SELECT COUNT(*) INTO kept_count FROM positions_to_keep;
    SELECT COUNT(*) INTO total_count FROM aircraft_positions;
    
    RAISE NOTICE '';
    RAISE NOTICE 'Positions to keep: %', kept_count;
    RAISE NOTICE 'Positions to delete: % (%.1f%%)', 
        (total_count - kept_count),
        ((total_count - kept_count)::FLOAT / total_count * 100);
END $$;

-- Delete redundant positions (keeping the first of each group)
-- SAFETY: This uses a temp table, so it's safe to run
DELETE FROM aircraft_positions
WHERE id NOT IN (SELECT id FROM positions_to_keep);

-- Show results
DO $$
DECLARE
    remaining_positions BIGINT;
BEGIN
    SELECT COUNT(*) INTO remaining_positions FROM aircraft_positions;
    
    RAISE NOTICE '';
    RAISE NOTICE 'Cleanup complete!';
    RAISE NOTICE 'Remaining positions: %', remaining_positions;
END $$;

-- Analyze table to update statistics
ANALYZE aircraft_positions;

-- Optional: Vacuum to reclaim disk space
-- Uncomment the next line to actually reclaim space
-- VACUUM FULL aircraft_positions;

DO $$
BEGIN
    RAISE NOTICE '';
    RAISE NOTICE 'To reclaim disk space, run: VACUUM FULL aircraft_positions;';
END $$;
