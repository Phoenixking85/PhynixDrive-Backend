package controllers

import (
	"net/http"
	"phynixdrive/services"
	"phynixdrive/utils"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
)

type TrashController struct {
	trashService *services.TrashService
}

// RestoreItem represents an item to be restored (for service layer)
type RestoreItem struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// RestoreItemRequest represents an item in the request with validation
type RestoreItemRequest struct {
	ID   string `json:"id" binding:"required"`
	Type string `json:"type" binding:"required,oneof=file folder"`
}

// RestoreMultipleRequest represents the request body for bulk restore
type RestoreMultipleRequest struct {
	Items []RestoreItemRequest `json:"items" binding:"required,min=1"`
}

// ToRestoreItem converts a request item to a service item
func (r RestoreItemRequest) ToRestoreItem() RestoreItem {
	return RestoreItem{
		ID:   r.ID,
		Type: r.Type,
	}
}

func NewTrashController(db *mongo.Database, b2Service *services.B2Service) *TrashController {
	return &TrashController{
		trashService: services.NewTrashService(db, b2Service),
	}
}

// GetTrashItems retrieves all items in trash for the authenticated user
func (tc *TrashController) GetTrashItems(c *gin.Context) {
	userIdStr := c.GetString("userIdStr")
	if userIdStr == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	// Optional filters
	itemType := c.Query("type") // "file", "folder", or "" for all
	limitStr := c.DefaultQuery("limit", "50")
	offsetStr := c.DefaultQuery("offset", "0")

	// Convert limit and offset to integers
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 0 {
		limit = 50
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	trashItems, err := tc.trashService.GetTrashItems(userIdStr, itemType, limit, offset)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to get trash items", nil)
		return
	}

	utils.SuccessResponse(c, "Trash items retrieved", trashItems)
}

// RestoreFromTrash restores a single item from trash
func (tc *TrashController) RestoreFromTrash(c *gin.Context) {
	itemId := c.Param("id")
	itemType := c.Query("type") // "file" or "folder"
	userIdStr := c.GetString("userIdStr")

	if userIdStr == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	if itemId == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "Item ID is required", nil)
		return
	}

	if itemType == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "Item type is required (file or folder)", nil)
		return
	}

	switch itemType {
	case "file":
		err := tc.trashService.RestoreFile(itemId, userIdStr)
		if err != nil {
			utils.ErrorResponse(c, http.StatusInternalServerError, err.Error(), nil)
			return
		}
		utils.SuccessResponse(c, "File restored successfully", nil)

	case "folder":
		err := tc.trashService.RestoreFolder(itemId, userIdStr)
		if err != nil {
			utils.ErrorResponse(c, http.StatusInternalServerError, err.Error(), nil)
			return
		}
		utils.SuccessResponse(c, "Folder restored successfully", nil)

	default:
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid item type (expected 'file' or 'folder')", nil)
	}
}

// PurgeFromTrash permanently deletes a single item from trash
func (tc *TrashController) PurgeFromTrash(c *gin.Context) {
	itemId := c.Param("id")
	itemType := c.Query("type") // "file" or "folder"
	userIdStr := c.GetString("userIdStr")

	if userIdStr == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	if itemId == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "Item ID is required", nil)
		return
	}

	if itemType == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "Item type is required (file or folder)", nil)
		return
	}

	switch itemType {
	case "file":
		err := tc.trashService.PurgeFile(itemId, userIdStr)
		if err != nil {
			utils.ErrorResponse(c, http.StatusInternalServerError, err.Error(), nil)
			return
		}
		utils.SuccessResponse(c, "File permanently deleted", nil)

	case "folder":
		err := tc.trashService.PurgeFolder(itemId, userIdStr)
		if err != nil {
			utils.ErrorResponse(c, http.StatusInternalServerError, err.Error(), nil)
			return
		}
		utils.SuccessResponse(c, "Folder permanently deleted", nil)

	default:
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid item type (expected 'file' or 'folder')", nil)
	}
}

func (tc *TrashController) RestoreMultipleItems(c *gin.Context) {
	var req RestoreMultipleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid request body", nil)
		return
	}

	userIdStr := c.GetString("userIdStr")
	if userIdStr == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	if len(req.Items) == 0 {
		utils.ErrorResponse(c, http.StatusBadRequest, "No items to restore", nil)
		return
	}

	// Convert request items (RestoreItemRequest) to service items (RestoreItem)
	items := make([]RestoreItem, len(req.Items))
	for i, itemReq := range req.Items {
		items[i] = itemReq.ToRestoreItem()
	}

	results, err := tc.trashService.RestoreMultipleItems(userIdStr, nil)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, err.Error(), nil)
		return
	}

	utils.SuccessResponse(c, "Bulk restore completed", results)
}

// PurgeAllTrash permanently deletes all items in trash
func (tc *TrashController) PurgeAllTrash(c *gin.Context) {
	userIdStr := c.GetString("userIdStr")
	if userIdStr == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	// Optional confirmation parameter
	confirm := c.Query("confirm")
	if confirm != "true" {
		utils.ErrorResponse(c, http.StatusBadRequest, "Confirmation required: add ?confirm=true to purge all items", nil)
		return
	}

	deletedCount, err := tc.trashService.PurgeAllTrash(userIdStr)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, err.Error(), nil)
		return
	}

	response := map[string]interface{}{
		"message":      "All trash items permanently deleted",
		"deletedCount": deletedCount,
	}

	utils.SuccessResponse(c, "Trash purged successfully", response)
}

// EmptyTrash empties the trash (alias for PurgeAllTrash)
func (tc *TrashController) EmptyTrash(c *gin.Context) {
	userIdStr := c.GetString("userIdStr")
	if userIdStr == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	// Optional confirmation parameter
	confirm := c.Query("confirm")
	if confirm != "true" {
		utils.ErrorResponse(c, http.StatusBadRequest, "Confirmation required: add ?confirm=true to empty trash", nil)
		return
	}

	deletedCount, err := tc.trashService.EmptyTrash(userIdStr)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, err.Error(), nil)
		return
	}

	response := map[string]interface{}{
		"message":      "Trash emptied successfully",
		"deletedCount": deletedCount,
	}

	utils.SuccessResponse(c, "Trash emptied successfully", response)
}
