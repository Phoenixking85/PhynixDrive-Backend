package middleware

import (
	"net/http"
	"phynixdrive/utils"
	"strings"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func AuthMiddleware(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractBearerToken(c)
		if token == "" {
			utils.ErrorResponse(c, http.StatusUnauthorized, "Authorization token required", nil)
			c.Abort()
			return
		}

		claims, err := utils.VerifyJWTTokenWithSecret(token, jwtSecret)
		if err != nil {
			utils.ErrorResponse(c, http.StatusUnauthorized, "Invalid or expired token", nil)
			c.Abort()
			return
		}

		// Validate user ID format
		userID, err := primitive.ObjectIDFromHex(claims.UserID)
		if err != nil {
			utils.ErrorResponse(c, http.StatusUnauthorized, "Invalid user ID in token", nil)
			c.Abort()
			return
		}

		// Set user context
		c.Set("userId", userID)
		c.Set("userIdStr", claims.UserID)
		c.Set("email", claims.Email)
		c.Set("name", claims.Name)
		c.Set("googleId", claims.GoogleID)
		c.Set("role", claims.Role)

		c.Next()
	}
}

func extractBearerToken(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return ""
	}

	const bearerPrefix = "Bearer "
	if !strings.HasPrefix(authHeader, bearerPrefix) {
		return ""
	}

	return strings.TrimSpace(authHeader[len(bearerPrefix):])
}

func RequireRole(requiredRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			utils.ErrorResponse(c, http.StatusUnauthorized, "User role not found", nil)
			c.Abort()
			return
		}

		userRole, ok := role.(string)
		if !ok || userRole != requiredRole {
			utils.ErrorResponse(c, http.StatusForbidden, "Insufficient privileges", nil)
			c.Abort()
			return
		}

		c.Next()
	}
}
