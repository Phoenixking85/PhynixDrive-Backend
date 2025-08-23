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
		userID := c.GetString("userID") // User ID extracted from context (e.g., from JWT)
		resourceID := c.Param("id")     // Try getting resource from "id"

		// Fallback to "folderId" if "id" is not present
		if resourceID == "" {
			resourceID = c.Param("folderId")
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
		hasPermission, err := permissionService.HasFilePermission(userID, resourceID, requiredRole)
		if err != nil {
			utils.ErrorResponse(c, http.StatusInternalServerError, "Permission check failed", err.Error())
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
