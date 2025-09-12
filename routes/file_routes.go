package routes

import (
	"phynixdrive/controllers"
	"phynixdrive/middleware"
	"phynixdrive/services"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
)

func RegisterFileRoutes(rg *gin.RouterGroup, db *mongo.Database, jwtSecret string, folderService *services.FolderService, b2Service *services.B2Service, permissionService *services.PermissionService) {
	// Initialize the file controller
	fileController := controllers.NewFileController(db, folderService, b2Service, permissionService)

	files := rg.Group("/files")
	files.Use(middleware.AuthMiddleware(jwtSecret)) // All file routes require authentication with JWT secret
	{
		// File metadata and operations
		files.GET("/:id", fileController.GetFileMetadata)     // GET /files/:id
		files.DELETE("/:id", fileController.DeleteFile)       // DELETE /files/:id (move to trash)
		files.PATCH("/:id/rename", fileController.RenameFile) // PATCH /files/:id/rename

		// File access URLs
		files.GET("/:id/download", fileController.DownloadFile) // GET /files/:id/download (B2 signed URL for download)
		files.GET("/:id/preview", fileController.PreviewFile)   // GET /files/:id/preview (B2 signed URL for preview)
		// File versions

		// File permissions and sharing
	}

	// File upload and listing routes (separate from /files/:id pattern to avoid conflicts)
	upload := rg.Group("")
	upload.Use(middleware.AuthMiddleware(jwtSecret)) // Use JWT secret for authentication
	{
		upload.POST("/uploadfiles", fileController.UploadFiles) // POST /uploadfiles (with relativePath[] support)
		upload.GET("/allfiles", fileController.GetAllFiles)     // GET /allfiles (root-level files)
	}

}
