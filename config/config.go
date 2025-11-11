package config

import (
	"context"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
)

type Config struct {
	Port string
	Env  string

	MongoURI     string
	DatabaseName string

	FrontendRedirectURL string

	JWTSecret     string
	JWTExpiration time.Duration

	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string

	B2ApplicationKeyID string
	B2ApplicationKey   string
	B2BucketName       string
	B2BucketID         string

	MaxFileSize    int64
	MaxUserStorage int64

	MailgunAPIKey  string
	MailgunDomain  string
	SendGridAPIKey string
	FromEmail      string

	TrashCleanupInterval time.Duration

	AllowedOrigins []string

	JWTIssuer string
}

var AppConfig *Config
var DB *mongo.Database

func LoadConfig() {
	AppConfig = &Config{
		Port: getEnv("PORT", "8080"),
		Env:  getEnv("ENV", "development"),

		MongoURI:     getMongoURI(),
		DatabaseName: getEnv("DATABASE_NAME", "phynixdrive"),

		JWTSecret:     getEnv("JWT_SECRET", "your-super-secret-jwt-key"),
		JWTExpiration: parseDuration(getEnv("JWT_EXPIRATION", "24h")),
		JWTIssuer:     getEnv("JWT_ISSUER", "phynixdrive"),

		FrontendRedirectURL: getEnv("FRONTEND_REDIRECT_URL", ""),

		GoogleClientID:     getEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret: getEnv("GOOGLE_CLIENT_SECRET", ""),
		GoogleRedirectURL:  getEnv("GOOGLE_REDIRECT_URL", "http://localhost:8080/api/auth/google/callback"),

		B2ApplicationKeyID: getB2KeyID(),
		B2ApplicationKey:   getB2AppKey(),
		B2BucketName:       getB2BucketName(),
		B2BucketID:         getEnv("B2_BUCKET_ID", ""),

		MaxFileSize:    parseInt64(getEnv("MAX_FILE_SIZE", "104857600")),
		MaxUserStorage: parseInt64(getEnv("MAX_USER_STORAGE", "2147483648")),

		MailgunAPIKey:  getEnv("MAILGUN_API_KEY", ""),
		MailgunDomain:  getEnv("MAILGUN_DOMAIN", ""),
		SendGridAPIKey: getEnv("SENDGRID_API_KEY", ""),
		FromEmail:      getEnv("FROM_EMAIL", "noreply@phynixdrive.com"),

		TrashCleanupInterval: parseDuration(getEnv("TRASH_CLEANUP_INTERVAL", "24h")),

		AllowedOrigins: parseStringSlice(getEnv("ALLOWED_ORIGINS", "http://localhost:3000,http://localhost:5173")),
	}

	logConfig()
	validateConfig()
}

func getMongoURI() string {
	if uri := os.Getenv("MONGO_URI"); uri != "" {
		return uri
	}
	return "mongodb://localhost:27017"
}

func getB2KeyID() string {
	possibleKeys := []string{"B2_APPLICATION_KEY_ID", "B2_KEY_ID", "BACKBLAZE_KEY_ID"}
	for _, key := range possibleKeys {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
}

func getB2AppKey() string {
	possibleKeys := []string{"B2_APPLICATION_KEY", "B2_APP_KEY", "BACKBLAZE_APP_KEY"}
	for _, key := range possibleKeys {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
}

func getB2BucketName() string {
	possibleKeys := []string{"B2_BUCKET_NAME", "B2_BUCKET", "BACKBLAZE_BUCKET"}
	for _, key := range possibleKeys {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
}

func logConfig() {
	log.Println("Configuration loaded:")
	log.Printf("  Port: %s", AppConfig.Port)
	log.Printf("  Environment: %s", AppConfig.Env)
	log.Printf("  Database: %s", AppConfig.DatabaseName)
	log.Printf("  MongoDB URI: %s", maskConnectionString(AppConfig.MongoURI))
	log.Printf("  JWT Secret: %s", maskSecret(AppConfig.JWTSecret))
	log.Printf("  JWT Expiration: %v", AppConfig.JWTExpiration)
	log.Printf("  Google Client ID: %s", maskSecret(AppConfig.GoogleClientID))
	log.Printf("  Google Redirect URL: %s", AppConfig.GoogleRedirectURL)
	log.Printf("  B2 Key ID: %s", maskSecret(AppConfig.B2ApplicationKeyID))
	log.Printf("  B2 Bucket: %s", AppConfig.B2BucketName)
	log.Printf("  Max File Size: %d bytes", AppConfig.MaxFileSize)
	log.Printf("  Max User Storage: %d bytes", AppConfig.MaxUserStorage)
	log.Printf("  Allowed Origins: %v", AppConfig.AllowedOrigins)
	log.Printf("  Trash Cleanup Interval: %v", AppConfig.TrashCleanupInterval)
}

func maskSecret(secret string) string {
	if secret == "" {
		return "[NOT SET]"
	}
	if len(secret) <= 8 {
		return "[HIDDEN]"
	}
	return secret[:4] + "***" + secret[len(secret)-4:]
}

func maskConnectionString(uri string) string {
	if uri == "" {
		return "[NOT SET]"
	}
	if strings.Contains(uri, "@") {
		parts := strings.Split(uri, "@")
		if len(parts) >= 2 {
			return "[CREDENTIALS_HIDDEN]@" + parts[len(parts)-1]
		}
	}
	return uri
}

func validateConfig() {
	var missingVars []string

	required := map[string]string{
		"MONGO_URI/MONGODB_URI": AppConfig.MongoURI,
		"JWT_SECRET":            AppConfig.JWTSecret,
		"GOOGLE_CLIENT_ID":      AppConfig.GoogleClientID,
		"GOOGLE_CLIENT_SECRET":  AppConfig.GoogleClientSecret,
		"GOOGLE_REDIRECT_URL":   AppConfig.GoogleRedirectURL,
		"B2_APPLICATION_KEY_ID": AppConfig.B2ApplicationKeyID,
		"B2_APPLICATION_KEY":    AppConfig.B2ApplicationKey,
		"B2_BUCKET_NAME":        AppConfig.B2BucketName,
	}

	for key, value := range required {
		if value == "" {
			missingVars = append(missingVars, key)
		}
	}

	if len(missingVars) > 0 {
		log.Printf("Missing required environment variables: %v", missingVars)
		log.Fatal("Please set all required environment variables")
	}

	log.Println("All required environment variables are set")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func parseInt64(s string) int64 {
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		log.Fatalf("Failed to parse int64: %s", s)
	}
	return i
}

func parseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		log.Fatalf("Failed to parse duration: %s", s)
	}
	return d
}

func CreateContext(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), timeout)
}

func parseStringSlice(s string) []string {
	if s == "" {
		return []string{}
	}

	parts := strings.Split(s, ",")
	var result []string
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
