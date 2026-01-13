-- Migration: Add collection_region to aircraft table
-- Date: 2026-01-13
-- Purpose: Support multi-region collection tracking

-- Add collection_region column if it doesn't exist
DO $$ 
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'aircraft' 
        AND column_name = 'collection_region'
    ) THEN
        ALTER TABLE aircraft ADD COLUMN collection_region TEXT;
        
        -- Backfill existing records with 'legacy' region
        UPDATE aircraft SET collection_region = 'legacy' WHERE collection_region IS NULL;
        
        -- Create index for region filtering
        CREATE INDEX idx_aircraft_collection_region ON aircraft(collection_region);
        
        COMMENT ON COLUMN aircraft.collection_region IS 'Name of collection region this aircraft was fetched from';
        
        RAISE NOTICE 'Added collection_region column to aircraft table';
    ELSE
        RAISE NOTICE 'collection_region column already exists';
    END IF;
END $$;

-- Verify the migration
SELECT 
    column_name,
    data_type,
    is_nullable
FROM information_schema.columns
WHERE table_name = 'aircraft' AND column_name = 'collection_region';
