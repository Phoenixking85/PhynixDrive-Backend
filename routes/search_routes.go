package routes

import (
	"phynixdrive/controllers"
	"phynixdrive/middleware"
	"phynixdrive/services"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
)

func RegisterSearchRoutes(rg *gin.RouterGroup, db *mongo.Database, permService *services.PermissionService) {
	// Initialize the search controller
	searchController := controllers.NewSearchController(db, permService)

	search := rg.Group("/search")
	search.Use(middleware.AuthMiddleware("your-jwt-secret-here")) // All search routes require authentication
	{
		search.GET("/", searchController.Search)                   // GET /search?q=term
		search.GET("/files", searchController.SearchFilesOnly)     // GET /search/files?q=term
		search.GET("/folders", searchController.SearchFoldersOnly) // GET /search/folders?q=term
		search.GET("/recent", searchController.GetRecentFiles)     // GET /search/recent
		search.GET("/shared", searchController.GetSharedWithMe)    // GET /search/shared
	}
}
