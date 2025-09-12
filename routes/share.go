package routes

import (
	"phynixdrive/controllers"
	"phynixdrive/middleware"

	"github.com/gin-gonic/gin"
)

// RegisterShareRoutes registers all share-related endpoints
func RegisterShareRoutes(api *gin.RouterGroup, jwtSecret string, shareController *controllers.ShareController) {
	// Apply authentication middleware to all share routes
	shareGroup := api.Group("/share")
	shareGroup.Use(middleware.AuthMiddleware(jwtSecret))

	// Core sharing endpoints
	shareGroup.POST("/", shareController.ShareResource) // Share a resource
	shareGroup.POST("/bulk", shareController.BulkShare) // Bulk share resources

	// Get shared resources
	shareGroup.GET("/by-me", shareController.GetSharedByMe)
	shareGroup.GET("/with-me", shareController.GetSharedWithMe)
	shareGroup.GET("/all", shareController.GetAllSharedResources)

	// Permission management (fixed routes to avoid conflicts)
	shareGroup.GET("/resource/:resource_type/:resource_id/permissions", shareController.GetResourcePermissions)
	shareGroup.GET("/details/:share_id", shareController.GetShareDetails)
	shareGroup.DELETE("/:share_id/revoke", shareController.RevokePermission)
	shareGroup.PUT("/:share_id/update", shareController.UpdatePermission)
}
