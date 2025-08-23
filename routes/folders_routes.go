package routes

import (
	"phynixdrive/controllers"
	"phynixdrive/middleware"
	"phynixdrive/services"

	"github.com/gin-gonic/gin"
)

func RegisterFolderRoutes(rg *gin.RouterGroup, jwtSecret string, folderService *services.FolderService, b2Service *services.B2Service) {
	// Initialize the folder controller with both services (dereference b2Service since controller expects value)
	folderController := controllers.NewFolderController(folderService, *b2Service)

	folders := rg.Group("/folders")
	folders.Use(middleware.AuthMiddleware(jwtSecret)) // All folder routes require authentication with JWT secret
	{
		// Folder CRUD operations
		folders.POST("/", folderController.CreateFolder)              // POST /folders
		folders.GET("/", folderController.ListRootFolders)            // GET /folders
		folders.GET("/:id", folderController.GetFolder)               // GET /folders/:id
		folders.PATCH("/:id/rename", folderController.RenameFolder)   // PATCH /folders/:id/rename
		folders.DELETE("/:id", folderController.DeleteFolder)         // DELETE /folders/:id (soft delete to trash)
		folders.GET("/:id/download", folderController.DownloadFolder) // GET /folders/:id/download

		// Folder permissions
		folders.POST("/:id/share", folderController.ShareFolder)               // POST /folders/:id/share
		folders.GET("/:id/permissions", folderController.GetFolderPermissions) // GET /folders/:id/permissions

		// Files within folders - Using consistent :id parameter for folder ID
		folders.GET("/:id/files", folderController.GetFilesInFolder)                // GET /folders/:id/files
		folders.DELETE("/:id/files/:fileId", folderController.DeleteFileFromFolder) // DELETE /folders/:id/files/:fileId
	}
}
