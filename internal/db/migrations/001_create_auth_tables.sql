-- Authentication and Authorization Tables
-- Migration: 001_create_auth_tables
-- Creates users, sessions, and audit_log tables for web authentication

-- Users table: stores user accounts for web interface
CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT 'viewer',  -- admin, observer, viewer, guest
    
    -- Account status
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    email_verified BOOLEAN NOT NULL DEFAULT FALSE,
    
    -- Timestamps
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    last_login TIMESTAMP,
    
    -- Constraints
    CONSTRAINT valid_role CHECK (role IN ('admin', 'observer', 'viewer', 'guest')),
    CONSTRAINT valid_username CHECK (LENGTH(username) >= 3 AND LENGTH(username) <= 50),
    CONSTRAINT valid_email CHECK (email ~* '^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}$')
);

-- Sessions table: stores JWT refresh tokens and session info
CREATE TABLE IF NOT EXISTS sessions (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,  -- SHA256 hash of JWT refresh token
    
    -- Session info
    ip_address INET,
    user_agent TEXT,
    
    -- Timestamps
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP NOT NULL,
    last_activity TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Audit log table: records all significant user actions
CREATE TABLE IF NOT EXISTS audit_log (
    id BIGSERIAL PRIMARY KEY,
    user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
    username TEXT NOT NULL,  -- Stored separately in case user is deleted
    
    -- Action details
    action TEXT NOT NULL,        -- login, logout, telescope_slew, telescope_track, etc.
    resource TEXT,               -- What was acted upon (aircraft ICAO, telescope, etc.)
    resource_id TEXT,
    
    -- Request context
    ip_address INET,
    user_agent TEXT,
    
    -- Result
    success BOOLEAN NOT NULL DEFAULT TRUE,
    error_message TEXT,
    
    -- Additional data (JSON)
    metadata JSONB,
    
    -- Timestamp
    timestamp TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Indexes for performance

-- User lookups
CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);
CREATE INDEX IF NOT EXISTS idx_users_active ON users(is_active) WHERE is_active = TRUE;

-- Session lookups
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_token_hash ON sessions(token_hash);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);

-- Audit log lookups
CREATE INDEX IF NOT EXISTS idx_audit_log_user_id ON audit_log(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_timestamp ON audit_log(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_audit_log_action ON audit_log(action);
CREATE INDEX IF NOT EXISTS idx_audit_log_resource ON audit_log(resource, resource_id);

-- Function to automatically update updated_at timestamp
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger to update updated_at on users table
CREATE TRIGGER update_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- Insert default admin user
-- Password: "admin" (this should be changed immediately in production!)
INSERT INTO users (username, email, password_hash, role, is_active, email_verified)
VALUES (
    'admin',
    'admin@ads-bscope.local',
    '$2a$10$Vy.FcuOzhpnhH4iQXpOLF.NPcs3HJlCRuuK9VtHCWhFeFd5uqCpfC',  -- bcrypt hash of "admin"
    'admin',
    TRUE,
    TRUE
)
ON CONFLICT (username) DO NOTHING;
