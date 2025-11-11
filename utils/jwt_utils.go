package utils

import (
	"errors"
	"phynixdrive/models"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Claims struct {
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	GoogleID string `json:"google_id"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

func GenerateJWTToken(user *models.User) (string, error) {
	expirationTime := time.Now().Add(24 * time.Hour)

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

func VerifyJWTToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
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

func getJWTSecret() string {
	panic("getJWTSecret not implemented - please implement based on your config system")
}

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

func RefreshJWTToken(tokenString string) (string, error) {
	claims, err := VerifyJWTToken(tokenString)
	if err != nil {
		return "", err
	}

	if time.Until(claims.ExpiresAt.Time) > 30*time.Minute {
		return "", errors.New("token is not expired yet")
	}

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
	}, getJWTSecret(), 24)
}

func RefreshJWTTokenWithSecret(tokenString string, jwtSecret string, expirationHours int) (string, error) {
	claims, err := VerifyJWTTokenWithSecret(tokenString, jwtSecret)
	if err != nil {
		return "", err
	}

	if time.Until(claims.ExpiresAt.Time) > 30*time.Minute {
		return "", errors.New("token is not expired yet")
	}

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
