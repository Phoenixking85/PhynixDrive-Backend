package controllers

import (
	"fmt"
	"log"
	"net/http"
	"phynixdrive/config"
	"phynixdrive/services"
	"phynixdrive/utils"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
)

type AuthController struct {
	authService *services.AuthService
}

func NewAuthController(db *mongo.Database, jwtSecret, googleClientID, googleClientSecret, redirectURL string) *AuthController {
	return &AuthController{
		authService: services.NewAuthService(db, jwtSecret, googleClientID, googleClientSecret, redirectURL),
	}
}

type OAuthLoginRequest struct {
	IDToken  string `json:"idToken" binding:"required"`
	Provider string `json:"provider" binding:"required,oneof=google"`
}

const (
	stateCookieName = "oauth_state"
	cookieMaxAge    = 10 * 60 // 10 minutes
	cookiePath      = "/"
	cookieDomain    = ""
)

// GoogleAuth returns Google OAuth URL
func (ac *AuthController) GoogleAuth(c *gin.Context) {
	state, err := ac.authService.GenerateState()
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to generate authentication state", nil)
		return
	}

	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(stateCookieName, state, cookieMaxAge, cookiePath, cookieDomain, false, true)

	authURL := ac.authService.GetGoogleAuthURL(state)

	log.Printf("[AuthController] Generated OAuth state and URL - State: %s", state)

	utils.SuccessResponse(c, "Google OAuth URL generated", gin.H{
		"auth_url": authURL,
		"state":    state,
	})
}

func (ac *AuthController) GoogleCallback(c *gin.Context) {
	state := c.Query("state")
	code := c.Query("code")

	// Validate state
	if !ac.authService.ValidateState(state) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid or expired authentication state"})
		return
	}

	// Handle callback
	_, token, err := ac.authService.HandleGoogleCallback(code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
		return
	}

	// Redirect to frontend with token
	redirectURL := fmt.Sprintf("%s/auth/callback?token=%s", config.AppConfig.FrontendRedirectURL, token)
	c.Redirect(http.StatusTemporaryRedirect, redirectURL)
}

func (ac *AuthController) OAuthLogin(c *gin.Context) {
	var req OAuthLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid request format", err.Error())
		return
	}

	user, token, err := ac.authService.LoginWithIDToken(req.IDToken, req.Provider)
	if err != nil {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Authentication failed", err.Error())
		return
	}

	utils.SuccessResponse(c, "Authentication successful", gin.H{
		"user":  user,
		"token": token,
	})
}

// GetUserProfile retrieves the authenticated user's profile
func (ac *AuthController) GetUserProfile(c *gin.Context) {
	userID := ac.extractUserID(c)
	if userID == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	user, err := ac.authService.GetUserProfile(userID)
	if err != nil {
		utils.ErrorResponse(c, http.StatusNotFound, "User profile not found", err.Error())
		return
	}

	utils.SuccessResponse(c, "Profile retrieved successfully", user)
}

// Logout
func (ac *AuthController) Logout(c *gin.Context) {
	utils.SuccessResponse(c, "Logout successful", nil)
}

// RefreshToken generates a new JWT token
func (ac *AuthController) RefreshToken(c *gin.Context) {
	userID := ac.extractUserID(c)
	email := ac.extractEmail(c)

	if userID == "" || email == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Invalid authentication context", nil)
		return
	}

	newToken, err := ac.authService.GenerateJWT(userID, email)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Token refresh failed", err.Error())
		return
	}

	utils.SuccessResponse(c, "Token refreshed successfully", gin.H{
		"token": newToken,
	})
}

// ValidateToken validates the current JWT token
func (ac *AuthController) ValidateToken(c *gin.Context) {
	userID := ac.extractUserID(c)
	email := ac.extractEmail(c)

	if userID == "" || email == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "Token validation failed", nil)
		return
	}

	utils.SuccessResponse(c, "Token is valid", gin.H{
		"user_id": userID,
		"email":   email,
	})
}

// DebugStates endpoint for debugging OAuth state issues
func (ac *AuthController) DebugStates(c *gin.Context) {
	// This is a debug endpoint - remove in production
	log.Printf("[AuthController] Debug states endpoint called")

	utils.SuccessResponse(c, "Debug info", gin.H{
		"message": "Check server logs for state information",
		"note":    "This endpoint is for debugging only",
	})
}

func (ac *AuthController) extractUserID(c *gin.Context) string {
	if userID, exists := c.Get("userIdStr"); exists {
		if id, ok := userID.(string); ok {
			return id
		}
	}
	return ""
}

func (ac *AuthController) extractEmail(c *gin.Context) string {
	if email, exists := c.Get("email"); exists {
		if emailStr, ok := email.(string); ok {
			return emailStr
		}
	}
	return ""
}
