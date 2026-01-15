-- Migration: Create observation points table
-- Description: Store user-defined observation points for multi-user support
-- Each user can have multiple observation points, one of which is active

-- Create observation_points table
CREATE TABLE IF NOT EXISTS observation_points (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    latitude DOUBLE PRECISION NOT NULL CHECK (latitude >= -90 AND latitude <= 90),
    longitude DOUBLE PRECISION NOT NULL CHECK (longitude >= -180 AND longitude <= 180),
    elevation_meters DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (elevation_meters >= -500 AND elevation_meters <= 10000),
    is_active BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    
    -- Ensure unique names per user
    UNIQUE(user_id, name)
);

-- Create index for faster lookups
CREATE INDEX IF NOT EXISTS idx_observation_points_user_id ON observation_points(user_id);
CREATE INDEX IF NOT EXISTS idx_observation_points_active ON observation_points(user_id, is_active);

-- Function to ensure only one active point per user
CREATE OR REPLACE FUNCTION ensure_single_active_point()
RETURNS TRIGGER AS $$
BEGIN
    -- If setting a point to active, deactivate all other points for this user
    IF NEW.is_active = TRUE THEN
        UPDATE observation_points
        SET is_active = FALSE
        WHERE user_id = NEW.user_id AND id != NEW.id;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger to maintain single active point constraint
DROP TRIGGER IF EXISTS trg_single_active_point ON observation_points;
CREATE TRIGGER trg_single_active_point
    BEFORE INSERT OR UPDATE ON observation_points
    FOR EACH ROW
    EXECUTE FUNCTION ensure_single_active_point();

-- Add default observation point for admin user (Joplin, Missouri from config)
INSERT INTO observation_points (user_id, name, latitude, longitude, elevation_meters, is_active)
VALUES (1, 'CLT Primary Observatory', 37.1401, -94.4912, 299, TRUE)
ON CONFLICT (user_id, name) DO NOTHING;
