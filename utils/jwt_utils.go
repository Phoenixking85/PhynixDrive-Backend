package utils

import (
	"errors"
	"phynixdrive/models"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Claims represents the JWT claims structure
type Claims struct {
	UserID   string `json:"user_id"` // Changed to match middleware expectation
	Email    string `json:"email"`
	Name     string `json:"name"`
	GoogleID string `json:"google_id"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// GenerateJWTToken creates a new JWT token for the given user (uses config for secret)
func GenerateJWTToken(user *models.User) (string, error) {
	expirationTime := time.Now().Add(24 * time.Hour) // Default 24 hours

	claims := &Claims{
		UserID:   user.ID.Hex(),
		Email:    user.Email,
		Name:     user.Name,
		GoogleID: user.GoogleID,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(getJWTSecret()))
}

// GenerateJWTTokenWithSecret creates a new JWT token for the given user with provided secret
func GenerateJWTTokenWithSecret(user *models.User, jwtSecret string, expirationHours int) (string, error) {
	expirationTime := time.Now().Add(time.Duration(expirationHours) * time.Hour)

	claims := &Claims{
		UserID:   user.ID.Hex(),
		Email:    user.Email,
		Name:     user.Name,
		GoogleID: user.GoogleID,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(jwtSecret))
}

// VerifyJWTToken validates and parses a JWT token string
// Note: This version expects the JWT secret to be available from config
func VerifyJWTToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		// You'll need to get the JWT secret from your config or pass it somehow
		// For now, this assumes you have a way to get it
		return []byte(getJWTSecret()), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}

// VerifyJWTTokenWithSecret validates and parses a JWT token string with provided secret
func VerifyJWTTokenWithSecret(tokenString string, jwtSecret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(jwtSecret), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}

// getJWTSecret returns the JWT secret - you need to implement this based on your config
func getJWTSecret() string {
	// Option 1: If you have a config package
	// return config.AppConfig.JWTSecret

	// Option 2: If you use environment variables
	// return os.Getenv("JWT_SECRET")

	// For now, this is a placeholder - you need to implement this
	panic("getJWTSecret not implemented - please implement based on your config system")
}

// GetUserIDFromToken extracts the user ID from a JWT token
func GetUserIDFromToken(tokenString string) (primitive.ObjectID, error) {
	claims, err := VerifyJWTToken(tokenString)
	if err != nil {
		return primitive.NilObjectID, err
	}

	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return primitive.NilObjectID, errors.New("invalid user ID in token")
	}

	return userID, nil
}

// GetUserIDFromTokenWithSecret extracts the user ID from a JWT token with provided secret
func GetUserIDFromTokenWithSecret(tokenString string, jwtSecret string) (primitive.ObjectID, error) {
	claims, err := VerifyJWTTokenWithSecret(tokenString, jwtSecret)
	if err != nil {
		return primitive.NilObjectID, err
	}

	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return primitive.NilObjectID, errors.New("invalid user ID in token")
	}

	return userID, nil
}

// RefreshJWTToken creates a new token from an existing one (if it's close to expiry)
func RefreshJWTToken(tokenString string) (string, error) {
	claims, err := VerifyJWTToken(tokenString)
	if err != nil {
		return "", err
	}

	// Don't allow refresh if token expires in more than 30 minutes
	if time.Until(claims.ExpiresAt.Time) > 30*time.Minute {
		return "", errors.New("token is not expired yet")
	}

	// Generate new token with same claims but new expiration time
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return "", errors.New("invalid user ID in token")
	}

	// You'll need to implement GenerateJWTToken without secret parameter
	// or use GenerateJWTTokenWithSecret
	return GenerateJWTTokenWithSecret(&models.User{
		ID:       userID,
		Email:    claims.Email,
		Name:     claims.Name,
		GoogleID: claims.GoogleID,
		Role:     claims.Role,
	}, getJWTSecret(), 24)
}

// RefreshJWTTokenWithSecret creates a new token from an existing one with provided secret
func RefreshJWTTokenWithSecret(tokenString string, jwtSecret string, expirationHours int) (string, error) {
	claims, err := VerifyJWTTokenWithSecret(tokenString, jwtSecret)
	if err != nil {
		return "", err
	}

	// Don't allow refresh if token expires in more than 30 minutes
	if time.Until(claims.ExpiresAt.Time) > 30*time.Minute {
		return "", errors.New("token is not expired yet")
	}

	// Generate new token with same claims but new expiration time
	userID, err := primitive.ObjectIDFromHex(claims.UserID)
	if err != nil {
		return "", errors.New("invalid user ID in token")
	}

	return GenerateJWTTokenWithSecret(&models.User{
		ID:       userID,
		Email:    claims.Email,
		Name:     claims.Name,
		GoogleID: claims.GoogleID,
		Role:     claims.Role,
	}, jwtSecret, expirationHours)
}
