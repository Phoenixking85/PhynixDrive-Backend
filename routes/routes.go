// routes/routes.go
package routes

import (
	"phynixdrive/controllers"
	"phynixdrive/services"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
)

// B2Config holds the B2 service configuration
type B2Config struct {
	KeyID          string
	ApplicationKey string
	BucketName     string
}

// GoogleConfig holds the Google OAuth2 configuration
type GoogleConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// SetupRoutes configures all API routes for the application
// This function is called from main.go after middleware is already set up
func SetupRoutes(api *gin.RouterGroup, db *mongo.Database, jwtSecret string, b2Config B2Config, googleConfig GoogleConfig) error {
	// Initialize B2 service first (required by folder service)
	b2Service, err := services.NewB2Service(b2Config.KeyID, b2Config.ApplicationKey, b2Config.BucketName)
	if err != nil {
		return err
	}

	// Initialize permission service (required by folder + share service)
	permissionService := services.NewPermissionService(db)

	// Initialize folder service
	folderService := services.NewFolderService(db, permissionService, b2Service)

	// Initialize share service + controller ✅ (only db + permissionService)
	shareService := services.NewShareService(db, permissionService)
	shareController := controllers.NewShareController(shareService)

	// Register all route groups
	RegisterAuthRoutes(api, db, jwtSecret, googleConfig.ClientID, googleConfig.ClientSecret, googleConfig.RedirectURL)
	RegisterFolderRoutes(api, jwtSecret, folderService, b2Service)
	RegisterFileRoutes(api, db, jwtSecret, folderService, b2Service, permissionService)
	RegisterTrashRoutes(api, db, jwtSecret, b2Service)
	RegisterSearchRoutes(api, db, permissionService)
	RegisterShareRoutes(api, jwtSecret, shareController)

	return nil
}

// Alternative approach: If you want to initialize services elsewhere
func SetupRoutesWithServices(api *gin.RouterGroup,
	db *mongo.Database,
	jwtSecret string,
	folderService *services.FolderService,
	b2Service *services.B2Service,
	permissionService *services.PermissionService,
	googleConfig GoogleConfig) {

	shareService := services.NewShareService(db, permissionService)
	shareController := controllers.NewShareController(shareService)

	RegisterAuthRoutes(api, db, jwtSecret, googleConfig.ClientID, googleConfig.ClientSecret, googleConfig.RedirectURL)
	RegisterFolderRoutes(api, jwtSecret, folderService, b2Service)
	RegisterFileRoutes(api, db, jwtSecret, folderService, b2Service, permissionService)
	RegisterTrashRoutes(api, db, jwtSecret, b2Service)
	RegisterSearchRoutes(api, db, permissionService)
	RegisterShareRoutes(api, jwtSecret, shareController)
}

// ServiceContainer holds all services and dependencies
type ServiceContainer struct {
	DB                *mongo.Database
	JWTSecret         string
	FolderService     *services.FolderService
	B2Service         *services.B2Service
	PermissionService *services.PermissionService
	GoogleConfig      GoogleConfig
}

// NewServiceContainer creates a new service container with all dependencies initialized
func NewServiceContainer(db *mongo.Database, jwtSecret string, b2Config B2Config, googleConfig GoogleConfig) (*ServiceContainer, error) {
	// Initialize B2 service first
	b2Service, err := services.NewB2Service(b2Config.KeyID, b2Config.ApplicationKey, b2Config.BucketName)
	if err != nil {
		return nil, err
	}

	// Initialize permission service
	permissionService := services.NewPermissionService(db)

	// Initialize folder service
	folderService := services.NewFolderService(db, permissionService, b2Service)

	return &ServiceContainer{
		DB:                db,
		JWTSecret:         jwtSecret,
		FolderService:     folderService,
		B2Service:         b2Service,
		PermissionService: permissionService,
		GoogleConfig:      googleConfig,
	}, nil
}

// SetupRoutesWithContainer configures all API routes using a service container
func SetupRoutesWithContainer(api *gin.RouterGroup, container *ServiceContainer) {

	shareService := services.NewShareService(container.DB, container.PermissionService)
	shareController := controllers.NewShareController(shareService)

	RegisterAuthRoutes(api, container.DB, container.JWTSecret,
		container.GoogleConfig.ClientID,
		container.GoogleConfig.ClientSecret,
		container.GoogleConfig.RedirectURL)

	RegisterFolderRoutes(api, container.JWTSecret, container.FolderService, container.B2Service)
	RegisterFileRoutes(api, container.DB, container.JWTSecret, container.FolderService, container.B2Service, container.PermissionService)
	RegisterTrashRoutes(api, container.DB, container.JWTSecret, container.B2Service)
	RegisterSearchRoutes(api, container.DB, container.PermissionService)
	RegisterShareRoutes(api, container.JWTSecret, shareController)
}
