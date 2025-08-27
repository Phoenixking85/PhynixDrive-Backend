package middleware

import (
	"net/http"
	"phynixdrive/services"
	"phynixdrive/utils"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
)

func PermissionMiddleware(requiredRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("userID")
		if userID == "" {
			utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated", nil)
			c.Abort()
			return
		}

		resourceID := c.Param("id")
		// Fallback to other common parameter names
		if resourceID == "" {
			resourceID = c.Param("folderId")
		}
		if resourceID == "" {
			resourceID = c.Param("fileId")
		}

		if resourceID == "" {
			utils.ErrorResponse(c, http.StatusBadRequest, "Missing resource ID", nil)
			c.Abort()
			return
		}

		db, ok := c.MustGet("db").(*mongo.Database)
		if !ok {
			utils.ErrorResponse(c, http.StatusInternalServerError, "Database connection not found", nil)
			c.Abort()
			return
		}

		permissionService := services.NewPermissionService(db)

		// Use the enhanced method that auto-detects resource type
		hasPermission, err := permissionService.HasResourcePermission(c.Request.Context(), userID, resourceID, requiredRole)
		if err != nil {
			if err.Error() == "resource not found" {
				utils.ErrorResponse(c, http.StatusNotFound, "Resource not found", nil)
			} else {
				utils.ErrorResponse(c, http.StatusInternalServerError, "Permission check failed", err.Error())
			}
			c.Abort()
			return
		}

		if !hasPermission {
			utils.ErrorResponse(c, http.StatusForbidden, "Insufficient permissions", nil)
			c.Abort()
			return
		}

		c.Next()
	}
}

// Specific middleware for file operations
func FilePermissionMiddleware(requiredRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("userID")
		if userID == "" {
			utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated", nil)
			c.Abort()
			return
		}

		fileID := c.Param("id")
		if fileID == "" {
			fileID = c.Param("fileId")
		}

		if fileID == "" {
			utils.ErrorResponse(c, http.StatusBadRequest, "Missing file ID", nil)
			c.Abort()
			return
		}

		db, ok := c.MustGet("db").(*mongo.Database)
		if !ok {
			utils.ErrorResponse(c, http.StatusInternalServerError, "Database connection not found", nil)
			c.Abort()
			return
		}

		permissionService := services.NewPermissionService(db)
		hasPermission, err := permissionService.HasFilePermission(c.Request.Context(), userID, fileID, requiredRole)
		if err != nil {
			if err.Error() == "file not found" {
				utils.ErrorResponse(c, http.StatusNotFound, "File not found", nil)
			} else {
				utils.ErrorResponse(c, http.StatusInternalServerError, "Permission check failed", err.Error())
			}
			c.Abort()
			return
		}

		if !hasPermission {
			utils.ErrorResponse(c, http.StatusForbidden, "Insufficient permissions", nil)
			c.Abort()
			return
		}

		c.Next()
	}
}

func FolderPermissionMiddleware(requiredRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("userID")
		if userID == "" {
			utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated", nil)
			c.Abort()
			return
		}

		folderID := c.Param("id")
		if folderID == "" {
			folderID = c.Param("folderId")
		}

		if folderID == "" {
			utils.ErrorResponse(c, http.StatusBadRequest, "Missing folder ID", nil)
			c.Abort()
			return
		}

		db, ok := c.MustGet("db").(*mongo.Database)
		if !ok {
			utils.ErrorResponse(c, http.StatusInternalServerError, "Database connection not found", nil)
			c.Abort()
			return
		}

		permissionService := services.NewPermissionService(db)
		hasPermission, err := permissionService.HasFolderPermission(c.Request.Context(), userID, folderID, requiredRole)
		if err != nil {
			if err.Error() == "folder not found" {
				utils.ErrorResponse(c, http.StatusNotFound, "Folder not found", nil)
			} else {
				utils.ErrorResponse(c, http.StatusInternalServerError, "Permission check failed", err.Error())
			}
			c.Abort()
			return
		}

		if !hasPermission {
			utils.ErrorResponse(c, http.StatusForbidden, "Insufficient permissions", nil)
			c.Abort()
			return
		}

		c.Next()
	}

}
