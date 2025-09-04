package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"phynixdrive/config"
	"phynixdrive/routes"
	"phynixdrive/services"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	// Load .env file with proper path handling (do this BEFORE config.LoadConfig)
	loadEnvFile()

	// Initialize configuration
	config.LoadConfig()
	cfg := config.AppConfig

	// Add CORS debug logging
	log.Printf("=== CORS Configuration Debug ===")
	log.Printf("AllowedOrigins: %v", cfg.AllowedOrigins)
	log.Printf("AllowedOrigins length: %d", len(cfg.AllowedOrigins))
	for i, origin := range cfg.AllowedOrigins {
		log.Printf("  [%d]: '%s'", i, origin)
	}
	log.Printf("=== End CORS Debug ===")

	// Initialize MongoDB client
	ctx, cancel := config.CreateContext(10 * time.Second)
	defer cancel()

	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI(cfg.MongoURI))
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}

	// Create separate context for disconnection
	defer func() {
		disconnectCtx, disconnectCancel := config.CreateContext(5 * time.Second)
		defer disconnectCancel()
		if err = mongoClient.Disconnect(disconnectCtx); err != nil {
			log.Printf("Failed to disconnect MongoDB: %v", err)
		}
	}()

	// Verify MongoDB connection
	if err = mongoClient.Ping(ctx, nil); err != nil {
		log.Fatalf("Failed to ping MongoDB: %v", err)
	}

	log.Println("Connected to MongoDB successfully")

	// Initialize Backblaze B2 service
	b2Config := routes.B2Config{
		KeyID:          cfg.B2ApplicationKeyID,
		ApplicationKey: cfg.B2ApplicationKey,
		BucketName:     cfg.B2BucketName,
	}

	googleConfig := routes.GoogleConfig{
		ClientID:     cfg.GoogleClientID,
		ClientSecret: cfg.GoogleClientSecret,
		RedirectURL:  cfg.GoogleRedirectURL,
	}

	// Initialize services container
	serviceContainer, err := routes.NewServiceContainer(
		mongoClient.Database(cfg.DatabaseName),
		cfg.JWTSecret,
		b2Config,
		googleConfig,
	)
	if err != nil {
		log.Fatalf("Failed to initialize services: %v", err)
	}

	// Set up Gin router
	router := gin.Default()

	// Set up CORS with fixed middleware
	router.Use(corsMiddleware(cfg.AllowedOrigins))

	// Set up API routes
	api := router.Group("/api")
	routes.SetupRoutesWithContainer(api, serviceContainer)

	// Health check endpoint
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"time":   time.Now().UTC(),
		})
	})

	// Start the cron job for trash cleanup
	if cfg.TrashCleanupInterval > 0 {
		trashService := services.NewTrashService(
			mongoClient.Database(cfg.DatabaseName),
			serviceContainer.B2Service,
		)
		services.StartTrashCleanupJob(trashService, cfg.TrashCleanupInterval)
		log.Printf("Started trash cleanup job running every %v", cfg.TrashCleanupInterval)
	}

	// Start the server
	log.Printf("Starting PhynixDrive server on port %s", cfg.Port)
	if err := router.Run(":" + cfg.Port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// loadEnvFile handles loading .env file from multiple possible locations
func loadEnvFile() {
	// Get current working directory
	pwd, err := os.Getwd()
	if err != nil {
		log.Printf("Could not get working directory: %v", err)
		return
	}

	log.Printf("Current working directory: %s", pwd)

	// Define possible .env file locations
	envPaths := []string{
		".env",                                   // Current directory
		"../.env",                                // Parent directory
		"../../.env",                             // Grandparent directory
		"cmd/../.env",                            // If running from cmd directory
		filepath.Join(pwd, ".env"),               // Absolute path to current dir
		filepath.Join(filepath.Dir(pwd), ".env"), // Absolute path to parent dir
	}

	loaded := false
	for _, envPath := range envPaths {
		absPath, _ := filepath.Abs(envPath)
		log.Printf("Trying to load .env from: %s", absPath)

		if _, err := os.Stat(envPath); err == nil {
			if err := godotenv.Load(envPath); err == nil {
				log.Printf("Successfully loaded environment variables from: %s", absPath)
				loaded = true
				break
			} else {
				log.Printf("Failed to load .env from %s: %v", absPath, err)
			}
		}
	}

	if !loaded {
		log.Println("No .env file found in any expected location, using system environment variables")
		log.Println("Expected locations checked:")
		for _, path := range envPaths {
			absPath, _ := filepath.Abs(path)
			log.Printf("  - %s", absPath)
		}
	}

	// Debug: Print some environment variables to verify loading
	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		mongoURI = os.Getenv("MONGO_URI") // Check alternative name
	}
	log.Printf("MONGODB_URI/MONGO_URI set: %t", mongoURI != "")
	log.Printf("JWT_SECRET set: %t", os.Getenv("JWT_SECRET") != "")
	log.Printf("B2_APPLICATION_KEY_ID set: %t", os.Getenv("B2_APPLICATION_KEY_ID") != "")
	log.Printf("ALLOWED_ORIGINS set: %t", os.Getenv("ALLOWED_ORIGINS") != "")
	log.Printf("ALLOWED_ORIGINS value: '%s'", os.Getenv("ALLOWED_ORIGINS"))
}

// Fixed CORS middleware
func corsMiddleware(allowedOrigins []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestOrigin := c.Request.Header.Get("Origin")

		// Debug logging
		log.Printf("CORS Request - Origin: '%s', Method: %s, Path: %s", requestOrigin, c.Request.Method, c.Request.URL.Path)
		log.Printf("CORS - Allowed Origins: %v", allowedOrigins)

		var allowOrigin string

		// If no allowed origins specified, allow all
		if len(allowedOrigins) == 0 {
			log.Printf("CORS - No allowed origins specified, allowing all")
			allowOrigin = "*"
		} else {
			// Check if request origin is in allowed list
			found := false
			for _, allowedOrigin := range allowedOrigins {
				if allowedOrigin == "*" {
					allowOrigin = "*"
					found = true
					log.Printf("CORS - Wildcard found, allowing all")
					break
				} else if allowedOrigin == requestOrigin {
					allowOrigin = requestOrigin
					found = true
					log.Printf("CORS - Origin '%s' found in allowed list", requestOrigin)
					break
				}
			}

			// If origin not found in allowed list
			if !found {
				if requestOrigin == "" {
					// No origin header (like from Postman), use first allowed
					allowOrigin = allowedOrigins[0]
					log.Printf("CORS - No origin header, using first allowed: %s", allowOrigin)
				} else {
					// For now, allow the requesting origin (you can change this for production)
					allowOrigin = requestOrigin
					log.Printf("CORS - Origin '%s' not in allowed list, but allowing for debugging", requestOrigin)
					// To deny: allowOrigin = "null" // This will cause CORS to fail
				}
			}
		}

		log.Printf("CORS - Setting Access-Control-Allow-Origin to: %s", allowOrigin)

		// Set CORS headers
		c.Writer.Header().Set("Access-Control-Allow-Origin", allowOrigin)
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, PATCH, DELETE")
		c.Writer.Header().Set("Access-Control-Max-Age", "86400") // Cache preflight for 24 hours

		// Handle preflight requests
		if c.Request.Method == "OPTIONS" {
			log.Printf("CORS - Handling OPTIONS preflight request")
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
