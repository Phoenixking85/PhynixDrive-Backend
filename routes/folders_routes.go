package routes

import (
	"phynixdrive/controllers"
	"phynixdrive/middleware"
	"phynixdrive/services"

	"github.com/gin-gonic/gin"
)

func RegisterFolderRoutes(rg *gin.RouterGroup, jwtSecret string, folderService *services.FolderService, b2Service *services.B2Service) {
	// Initialize the folder controller with both services (passing b2Service as pointer)
	folderController := controllers.NewFolderController(folderService, b2Service)

	folders := rg.Group("/folders")
	folders.Use(middleware.AuthMiddleware(jwtSecret)) // All folder routes require JWT authentication
	{
		// Core folder operations (matching API specification)
		folders.POST("/", folderController.CreateFolder)                 // POST /folders - Create folder
		folders.GET("/", folderController.ListRootFolders)               // GET /folders - List root folders
		folders.GET("/:id/contents", folderController.GetFolderContents) // GET /folders/:id/contents - View folder contents (Google Drive style)
		// POST /folders/:id/share - Share folder with inheritance
		folders.GET("/:id/download", folderController.DownloadFolder) // GET /folders/:id/download - Download folder as ZIP

		// Additional folder operations
		folders.GET("/:id", folderController.GetFolder)             // GET /folders/:id - Get specific folder
		folders.PATCH("/:id/rename", folderController.RenameFolder) // PATCH /folders/:id/rename - Rename folder
		folders.DELETE("/:id", folderController.DeleteFolder)       // DELETE /folders/:id - Delete folder (soft delete)

		// Folder permissions
		// GET /folders/:id/permissions - Get folder permissions

		// GET /folders/:id/files - Get files in folder
		folders.DELETE("/:id/files/:fileId", folderController.DeleteFileFromFolder) // DELETE /folders/:id/files/:fileId - Delete file from folder
	}
}
