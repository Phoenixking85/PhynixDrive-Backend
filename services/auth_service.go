package services

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"phynixdrive/models"
	"phynixdrive/utils"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	ErrInvalidToken     = errors.New("invalid or expired token")
	ErrUserNotFound     = errors.New("user not found")
	ErrEmailNotVerified = errors.New("email not verified")
	ErrInvalidProvider  = errors.New("unsupported authentication provider")
	ErrInvalidState     = errors.New("invalid or expired OAuth state")
)

type AuthService struct {
	userCollection     *mongo.Collection
	jwtSecret          string
	googleClientID     string
	googleClientSecret string
	redirectURL        string
	stateManager       *StateManager
}

type StateManager struct {
	states map[string]StateInfo
	mu     sync.RWMutex
}

type StateInfo struct {
	CreatedAt time.Time
	ExpiresAt time.Time
	Used      bool
}

func NewStateManager() *StateManager {
	sm := &StateManager{
		states: make(map[string]StateInfo),
	}

	// Start cleanup routine
	go sm.startCleanupRoutine()
	return sm
}

func (sm *StateManager) Store(state string, duration time.Duration) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	sm.states[state] = StateInfo{
		CreatedAt: now,
		ExpiresAt: now.Add(duration),
		Used:      false,
	}

	log.Printf("[StateManager] Stored state: %s, expires at: %s", state, now.Add(duration).Format(time.RFC3339))
}

func (sm *StateManager) Validate(state string) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	stateInfo, exists := sm.states[state]
	if !exists {
		log.Printf("[StateManager] State not found: %s", state)
		return false
	}

	now := time.Now()
	if now.After(stateInfo.ExpiresAt) {
		log.Printf("[StateManager] State expired: %s (expired at: %s)", state, stateInfo.ExpiresAt.Format(time.RFC3339))
		delete(sm.states, state)
		return false
	}

	if stateInfo.Used {
		log.Printf("[StateManager] State already used: %s", state)
		delete(sm.states, state)
		return false
	}

	// Mark as used and remove (one-time use)
	delete(sm.states, state)
	log.Printf("[StateManager] State validated and removed: %s", state)
	return true
}

func (sm *StateManager) GetStoredStates() map[string]StateInfo {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// Return a copy for debugging purposes
	states := make(map[string]StateInfo)
	for k, v := range sm.states {
		states[k] = v
	}
	return states
}

func (sm *StateManager) startCleanupRoutine() {
	ticker := time.NewTicker(2 * time.Minute) // More frequent cleanup for debugging
	defer ticker.Stop()

	for range ticker.C {
		sm.cleanup()
	}
}

func (sm *StateManager) cleanup() {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	now := time.Now()
	expired := 0
	for state, stateInfo := range sm.states {
		if now.After(stateInfo.ExpiresAt) {
			delete(sm.states, state)
			expired++
		}
	}

	if expired > 0 {
		log.Printf("[StateManager] Cleaned up %d expired states", expired)
	}
}

type GoogleTokenInfo struct {
	ID            string       `json:"sub"`
	Email         string       `json:"email"`
	EmailVerified FlexibleBool `json:"email_verified"`
	Name          string       `json:"name"`
	Picture       string       `json:"picture"`
	GivenName     string       `json:"given_name"`
	FamilyName    string       `json:"family_name"`
}

type GoogleTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	Error        string `json:"error,omitempty"`
}

type FlexibleBool bool

func (fb *FlexibleBool) UnmarshalJSON(data []byte) error {
	str := strings.Trim(string(data), `"`)
	*fb = str == "true"
	return nil
}

func NewAuthService(db *mongo.Database, jwtSecret, googleClientID, googleClientSecret, redirectURL string) *AuthService {
	service := &AuthService{
		userCollection:     db.Collection("users"),
		jwtSecret:          jwtSecret,
		googleClientID:     googleClientID,
		googleClientSecret: googleClientSecret,
		redirectURL:        redirectURL,
		stateManager:       NewStateManager(),
	}

	// Create index on email for better performance
	service.createIndexes()
	log.Printf("[AuthService] Initialized with redirect URL: %s", redirectURL)
	return service
}

func (s *AuthService) createIndexes() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create unique index on email
	emailIndex := mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	}

	// Create index on google_id
	googleIDIndex := mongo.IndexModel{
		Keys: bson.D{{Key: "google_id", Value: 1}},
	}

	_, err := s.userCollection.Indexes().CreateMany(ctx, []mongo.IndexModel{emailIndex, googleIDIndex})
	if err != nil {
		// Log error but don't fail - indexes might already exist
		log.Printf("Warning: Failed to create indexes: %v", err)
	}
}

const OAuthStateExpiration = 10 * time.Minute // Duration for which OAuth state is valid; adjust as needed

func (s *AuthService) GenerateState() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random state: %w", err)
	}

	state := base64.RawURLEncoding.EncodeToString(bytes)

	// Store with configurable expiration
	duration := OAuthStateExpiration
	s.stateManager.Store(state, duration)

	log.Printf("[AuthService] Generated new state: %s", state)
	return state, nil
}

func (s *AuthService) ValidateState(state string) bool {
	log.Printf("[AuthService] Validating state: %s", state)

	// Debug: Show current stored states
	stored := s.stateManager.GetStoredStates()
	log.Printf("[AuthService] Current stored states count: %d", len(stored))
	for storedState, info := range stored {
		log.Printf("[AuthService] Stored state: %s, expires: %s, used: %t",
			storedState, info.ExpiresAt.Format(time.RFC3339), info.Used)
	}

	isValid := s.stateManager.Validate(state)
	log.Printf("[AuthService] State validation result: %t", isValid)
	return isValid
}

func (s *AuthService) GetGoogleAuthURL(state string) string {
	params := url.Values{
		"client_id":     {s.googleClientID},
		"redirect_uri":  {s.redirectURL},
		"scope":         {"openid email profile https://www.googleapis.com/auth/drive"},
		"response_type": {"code"},
		"state":         {state},
		"access_type":   {"offline"},
		"prompt":        {"consent"},
	}

	authURL := "https://accounts.google.com/o/oauth2/auth?" + params.Encode()
	log.Printf("[AuthService] Generated Google Auth URL: %s", authURL)
	return authURL
}

func (s *AuthService) ExchangeCodeForTokens(code string) (*GoogleTokenResponse, error) {
	log.Printf("[AuthService] Exchanging code for tokens...")

	data := url.Values{
		"client_id":     {s.googleClientID},
		"client_secret": {s.googleClientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {s.redirectURL},
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.PostForm("https://oauth2.googleapis.com/token", data)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code for tokens: %w", err)
	}
	defer resp.Body.Close()

	var tokenResponse GoogleTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	if tokenResponse.Error != "" {
		return nil, fmt.Errorf("OAuth token exchange error: %s", tokenResponse.Error)
	}

	if tokenResponse.AccessToken == "" {
		return nil, errors.New("no access token received")
	}

	log.Printf("[AuthService] Successfully exchanged code for tokens")
	return &tokenResponse, nil
}

func (s *AuthService) ValidateGoogleIDToken(idToken string) (*GoogleTokenInfo, error) {
	if idToken == "" {
		return nil, errors.New("ID token is required")
	}

	log.Printf("[AuthService] Validating Google ID token...")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get("https://oauth2.googleapis.com/tokeninfo?id_token=" + url.QueryEscape(idToken))
	if err != nil {
		return nil, fmt.Errorf("failed to validate ID token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("invalid ID token: HTTP %d", resp.StatusCode)
	}

	var tokenInfo GoogleTokenInfo
	if err := json.NewDecoder(resp.Body).Decode(&tokenInfo); err != nil {
		return nil, fmt.Errorf("failed to decode token info: %w", err)
	}

	if tokenInfo.Email == "" {
		return nil, errors.New("email missing in token")
	}

	if !bool(tokenInfo.EmailVerified) {
		return nil, ErrEmailNotVerified
	}

	log.Printf("[AuthService] ID token validated for user: %s", tokenInfo.Email)
	return &tokenInfo, nil
}

func (s *AuthService) HandleGoogleCallback(code string) (*models.User, string, error) {
	log.Printf("[AuthService] Handling Google callback with code: %s...", code[:10])

	// Exchange code for tokens
	tokenResponse, err := s.ExchangeCodeForTokens(code)
	if err != nil {
		return nil, "", err
	}

	// Validate ID token
	googleInfo, err := s.ValidateGoogleIDToken(tokenResponse.IDToken)
	if err != nil {
		return nil, "", err
	}

	// Create or update user
	user, err := s.createOrUpdateUser(googleInfo, tokenResponse.RefreshToken)
	if err != nil {
		return nil, "", err
	}

	// Generate JWT
	jwtToken, err := s.GenerateJWT(user.ID.Hex(), user.Email)
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate JWT: %w", err)
	}

	log.Printf("[AuthService] Google callback handled successfully for user: %s", user.Email)
	return user, jwtToken, nil
}

func (s *AuthService) LoginWithIDToken(idToken, provider string) (*models.User, string, error) {
	if provider != "google" {
		return nil, "", ErrInvalidProvider
	}

	googleInfo, err := s.ValidateGoogleIDToken(idToken)
	if err != nil {
		return nil, "", err
	}

	user, err := s.createOrUpdateUser(googleInfo, "")
	if err != nil {
		return nil, "", err
	}

	jwtToken, err := s.GenerateJWT(user.ID.Hex(), user.Email)
	if err != nil {
		return nil, "", fmt.Errorf("failed to generate JWT: %w", err)
	}

	return user, jwtToken, nil
}

func (s *AuthService) createOrUpdateUser(googleInfo *GoogleTokenInfo, refreshToken string) (*models.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var user models.User

	// Try to find existing user by email
	err := s.userCollection.FindOne(ctx, bson.M{"email": googleInfo.Email}).Decode(&user)

	if err == mongo.ErrNoDocuments {
		// Create new user
		user = models.User{
			ID:           primitive.NewObjectID(),
			GoogleID:     googleInfo.ID,
			Email:        googleInfo.Email,
			Name:         googleInfo.Name,
			ProfilePic:   googleInfo.Picture,
			Role:         "user",
			UsedStorage:  0,
			MaxStorage:   2 * 1024 * 1024 * 1024, // 2GB
			RefreshToken: refreshToken,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
		}

		_, err = s.userCollection.InsertOne(ctx, user)
		if err != nil {
			return nil, fmt.Errorf("failed to create user: %w", err)
		}
		log.Printf("[AuthService] Created new user: %s", user.Email)
	} else if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	} else {
		// Update existing user
		updateFields := bson.M{
			"google_id":   googleInfo.ID,
			"name":        googleInfo.Name,
			"profile_pic": googleInfo.Picture,
			"updated_at":  time.Now(),
		}

		if refreshToken != "" {
			updateFields["refresh_token"] = refreshToken
		}

		_, err = s.userCollection.UpdateOne(
			ctx,
			bson.M{"_id": user.ID},
			bson.M{"$set": updateFields},
		)
		if err != nil {
			return nil, fmt.Errorf("failed to update user: %w", err)
		}

		// Fetch updated user data
		err = s.userCollection.FindOne(ctx, bson.M{"_id": user.ID}).Decode(&user)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch updated user: %w", err)
		}
		log.Printf("[AuthService] Updated existing user: %s", user.Email)
	}

	return &user, nil
}

func (s *AuthService) GenerateJWT(userID, email string) (string, error) {
	user, err := s.GetUserProfile(userID)
	if err != nil {
		return "", err
	}

	return utils.GenerateJWTTokenWithSecret(user, s.jwtSecret, 24)
}

func (s *AuthService) ValidateJWT(tokenString string) (string, string, error) {
	claims, err := utils.VerifyJWTTokenWithSecret(tokenString, s.jwtSecret)
	if err != nil {
		return "", "", ErrInvalidToken
	}

	return claims.UserID, claims.Email, nil
}

func (s *AuthService) GetUserProfile(userID string) (*models.User, error) {
	objID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID format: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user models.User
	err = s.userCollection.FindOne(ctx, bson.M{"_id": objID}).Decode(&user)
	if err == mongo.ErrNoDocuments {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}

	return &user, nil
}
