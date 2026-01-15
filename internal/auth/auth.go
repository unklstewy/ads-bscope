// Package auth provides authentication and authorization functionality for the web server.
// It handles password hashing, JWT token generation/validation, and user authentication.
package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// User roles for role-based access control (RBAC)
const (
	RoleAdmin    = "admin"    // Full system access
	RoleObserver = "observer" // Telescope control and viewing
	RoleViewer   = "viewer"   // Read-only access
	RoleGuest    = "guest"    // Limited public access
)

var (
	// ErrInvalidCredentials is returned when authentication fails
	ErrInvalidCredentials = errors.New("invalid credentials")
	// ErrInvalidToken is returned when token validation fails
	ErrInvalidToken = errors.New("invalid or expired token")
	// ErrUnauthorized is returned when user lacks required permissions
	ErrUnauthorized = errors.New("unauthorized access")
)

// Claims represents the JWT claims for a user session
type Claims struct {
	UserID   int    `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// Config holds authentication configuration
type Config struct {
	JWTSecret     string        // Secret key for signing JWTs
	TokenDuration time.Duration // How long tokens are valid
	BCryptCost    int           // BCrypt hashing cost (default: 12)
}

// Service provides authentication operations
type Service struct {
	config Config
}

// NewService creates a new authentication service
func NewService(cfg Config) *Service {
	// Set default BCrypt cost if not specified
	if cfg.BCryptCost == 0 {
		cfg.BCryptCost = bcrypt.DefaultCost
	}
	
	// Set default token duration if not specified (24 hours)
	if cfg.TokenDuration == 0 {
		cfg.TokenDuration = 24 * time.Hour
	}
	
	return &Service{
		config: cfg,
	}
}

// HashPassword hashes a plaintext password using bcrypt
func (s *Service) HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.config.BCryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// ComparePassword compares a plaintext password with a hashed password
func (s *Service) ComparePassword(hashedPassword, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}

// GenerateToken generates a JWT token for a user
func (s *Service) GenerateToken(userID int, username, role string) (string, error) {
	// Create claims
	claims := &Claims{
		UserID:   userID,
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.config.TokenDuration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "ads-bscope",
		},
	}
	
	// Create token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	
	// Sign token with secret
	tokenString, err := token.SignedString([]byte(s.config.JWTSecret))
	if err != nil {
		return "", err
	}
	
	return tokenString, nil
}

// ValidateToken validates a JWT token and returns the claims
func (s *Service) ValidateToken(tokenString string) (*Claims, error) {
	// Parse token
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return []byte(s.config.JWTSecret), nil
	})
	
	if err != nil {
		return nil, ErrInvalidToken
	}
	
	// Extract claims
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}
	
	return nil, ErrInvalidToken
}

// HasRole checks if a user has a specific role or higher
// Role hierarchy: Admin > Observer > Viewer > Guest
func HasRole(userRole, requiredRole string) bool {
	roleLevel := map[string]int{
		RoleAdmin:    3,
		RoleObserver: 2,
		RoleViewer:   1,
		RoleGuest:    0,
	}
	
	userLevel, ok1 := roleLevel[userRole]
	requiredLevel, ok2 := roleLevel[requiredRole]
	
	if !ok1 || !ok2 {
		return false
	}
	
	return userLevel >= requiredLevel
}

// CanControlTelescope checks if a role can control the telescope
func CanControlTelescope(role string) bool {
	return HasRole(role, RoleObserver)
}

// CanViewTelemetry checks if a role can view telemetry
func CanViewTelemetry(role string) bool {
	return HasRole(role, RoleViewer)
}

// CanManageUsers checks if a role can manage users
func CanManageUsers(role string) bool {
	return role == RoleAdmin
}
