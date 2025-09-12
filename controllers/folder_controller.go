package controllers

import (
	"context"
	"fmt"
	"net/http"
	"phynixdrive/services"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type FolderController struct {
	folderService *services.FolderService
	b2Service     *services.B2Service
}

func NewFolderController(folderService *services.FolderService, b2Service *services.B2Service) *FolderController {
	return &FolderController{
		folderService: folderService,
		b2Service:     b2Service,
	}
}

// ========== Helpers ==========

// Extract and validate user ID from context
func (fc *FolderController) getUserID(c *gin.Context) (string, error) {
	userID, exists := c.Get("userIdStr")
	if !exists {
		return "", fmt.Errorf("user not authenticated")
	}

	userIDStr, ok := userID.(string)
	if !ok || !primitive.IsValidObjectID(userIDStr) {
		return "", fmt.Errorf("invalid user ID format")
	}
	return userIDStr, nil
}

// Unified error handler
func (fc *FolderController) handleError(c *gin.Context, err error, defaultMessage string, defaultStatus int) {
	statusCode := defaultStatus
	message := defaultMessage

	switch err.Error() {
	case "folder not found":
		statusCode, message = http.StatusNotFound, "Folder not found"
	case "insufficient permissions":
		statusCode, message = http.StatusForbidden, "Insufficient permissions"
	case "parent folder not found":
		statusCode, message = http.StatusNotFound, "Parent folder not found"
	case "insufficient permissions to share folder":
		statusCode, message = http.StatusForbidden, "Insufficient permissions to share this folder"
	case "file not found in folder":
		statusCode, message = http.StatusNotFound, "File not found in folder"
	default:
		errorStr := err.Error()
		if len(errorStr) > 25 && errorStr[:19] == "folder with name '" && errorStr[len(errorStr)-15:] == "already exists" {
			statusCode, message = http.StatusConflict, "Folder with this name already exists"
		} else if len(errorStr) > 17 && errorStr[:17] == "user with email " {
			statusCode, message = http.StatusNotFound, "User not found"
		}
	}

	c.JSON(statusCode, gin.H{"success": false, "message": message, "error": err.Error()})
}

// ========== Endpoints ==========

// CreateFolder
func (fc *FolderController) CreateFolder(c *gin.Context) {
	userIDStr, err := fc.getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": err.Error()})
		return
	}

	var req struct {
		Name        string  `json:"name" binding:"required,min=1,max=255"`
		Description string  `json:"description,omitempty"`
		ParentID    *string `json:"parent_id,omitempty"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request data", "error": err.Error()})
		return
	}
	if req.ParentID != nil && *req.ParentID != "" && !primitive.IsValidObjectID(*req.ParentID) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid parent folder ID format"})
		return
	}

	folder, err := fc.folderService.CreateFolder(req.Name, req.ParentID, userIDStr)
	if err != nil {
		fc.handleError(c, err, "Failed to create folder", http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Folder created successfully",
		"data": gin.H{
			"id":         folder.ID,
			"name":       folder.Name,
			"path":       folder.Path,
			"created_at": folder.CreatedAt,
		},
	})
}

// ListRootFolders (FIXED: now includes file_count & subfolder_count)
func (fc *FolderController) ListRootFolders(c *gin.Context) {
	userIDStr, err := fc.getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": err.Error()})
		return
	}

	folders, err := fc.folderService.ListRootFoldersWithCounts(userIDStr)
	if err != nil {
		fc.handleError(c, err, "Failed to retrieve folders", http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": folders})
}

// GetFolderContents (Google Drive style)
func (fc *FolderController) GetFolderContents(c *gin.Context) {
	userIDStr, err := fc.getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": err.Error()})
		return
	}
	folderID := c.Param("id")
	if !primitive.IsValidObjectID(folderID) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid folder ID format"})
		return
	}

	contents, err := fc.folderService.GetFolderContents(folderID, userIDStr)
	if err != nil {
		fc.handleError(c, err, "Failed to retrieve folder contents", http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": contents})
}

// GetFolder
func (fc *FolderController) GetFolder(c *gin.Context) {
	userIDStr, err := fc.getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": err.Error()})
		return
	}
	folderID := c.Param("id")
	if !primitive.IsValidObjectID(folderID) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid folder ID format"})
		return
	}

	folder, err := fc.folderService.GetFolderByID(folderID, userIDStr)
	if err != nil {
		fc.handleError(c, err, "Failed to retrieve folder", http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Folder retrieved successfully", "data": folder})
}

// RenameFolder
func (fc *FolderController) RenameFolder(c *gin.Context) {
	userIDStr, err := fc.getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": err.Error()})
		return
	}
	folderID := c.Param("id")
	if !primitive.IsValidObjectID(folderID) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid folder ID format"})
		return
	}

	var req struct {
		Name string `json:"name" binding:"required,min=1,max=255"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid request data", "error": err.Error()})
		return
	}

	if err := fc.folderService.RenameFolder(folderID, req.Name, userIDStr); err != nil {
		fc.handleError(c, err, "Failed to rename folder", http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Folder renamed successfully"})
}

// DeleteFolder
func (fc *FolderController) DeleteFolder(c *gin.Context) {
	userIDStr, err := fc.getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": err.Error()})
		return
	}
	folderID := c.Param("id")
	if !primitive.IsValidObjectID(folderID) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid folder ID format"})
		return
	}

	if err := fc.folderService.DeleteFolder(c.Request.Context(), folderID, userIDStr); err != nil {
		fc.handleError(c, err, "Failed to delete folder", http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Folder deleted successfully"})
}

// DeleteFileFromFolder
func (fc *FolderController) DeleteFileFromFolder(c *gin.Context) {
	userIDStr, err := fc.getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": err.Error()})
		return
	}
	folderID, fileID := c.Param("id"), c.Param("fileId")
	if !primitive.IsValidObjectID(folderID) || !primitive.IsValidObjectID(fileID) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid ID format"})
		return
	}

	if err := fc.folderService.DeleteFileFromFolder(folderID, fileID, userIDStr); err != nil {
		fc.handleError(c, err, "Failed to delete file", http.StatusInternalServerError)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "File deleted successfully"})
}

// DownloadFolder (streams ZIP)
func (fc *FolderController) DownloadFolder(c *gin.Context) {
	userIDStr, err := fc.getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": err.Error()})
		return
	}
	folderID := c.Param("id")
	if !primitive.IsValidObjectID(folderID) {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Invalid folder ID format"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Minute)
	defer cancel()

	if err := fc.folderService.DownloadFolder(ctx, c.Writer, folderID, userIDStr); err != nil {
		if !c.Writer.Written() {
			fc.handleError(c, err, "Failed to download folder", http.StatusInternalServerError)
		} else {
			fmt.Printf("Error streaming folder zip for %s: %v\n", folderID, err)
		}
	}
}
