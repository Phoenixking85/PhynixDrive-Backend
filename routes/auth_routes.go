package routes

import (
	"phynixdrive/controllers"
	"phynixdrive/middleware"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
)

func RegisterAuthRoutes(rg *gin.RouterGroup, db *mongo.Database, jwtSecret, googleClientID, googleClientSecret, redirectURL string) {
	authController := controllers.NewAuthController(db, jwtSecret, googleClientID, googleClientSecret, redirectURL)

	auth := rg.Group("/auth")
	{
		// Public OAuth2 routes
		auth.GET("/google", authController.GoogleAuth)
		auth.GET("/google/callback", authController.GoogleCallback)

		// Legacy OAuth route for backward compatibility
		auth.POST("/oauth-login", authController.OAuthLogin)

		// Protected routes requiring authentication
		protected := auth.Group("")
		protected.Use(middleware.AuthMiddleware(jwtSecret))
		{
			protected.GET("/me", authController.GetUserProfile)
			protected.POST("/logout", authController.Logout)
			protected.POST("/refresh", authController.RefreshToken)
			protected.GET("/validate", authController.ValidateToken)
		}
	}
}
