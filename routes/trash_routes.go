package routes

import (
	"phynixdrive/controllers"
	"phynixdrive/middleware"
	"phynixdrive/services"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
)

func RegisterTrashRoutes(rg *gin.RouterGroup, db *mongo.Database, jwtSecret string, b2Service *services.B2Service) {
	// Initialize the trash controller
	trashController := controllers.NewTrashController(db, b2Service)

	trash := rg.Group("/trash")
	trash.Use(middleware.AuthMiddleware(jwtSecret)) // All trash routes require authentication with JWT secret
	{
		trash.GET("/", trashController.GetTrashItems)                 // GET /trash
		trash.PATCH("/:id/restore", trashController.RestoreFromTrash) // PATCH /trash/:id/restore
		trash.DELETE("/:id/purge", trashController.PurgeFromTrash)    // DELETE /trash/:id/purge (permanent delete)

		// Bulk operations
		trash.POST("/restore-multiple", trashController.RestoreMultipleItems) // POST /trash/restore-multiple
		trash.DELETE("/purge-all", trashController.PurgeAllTrash)             // DELETE /trash/purge-all

	}
}
