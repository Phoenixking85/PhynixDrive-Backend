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
	b2Service     *services.B2Service // Changed to pointer to match service signature
}

func NewFolderController(folderService *services.FolderService, b2Service *services.B2Service) *FolderController {
	return &FolderController{
		folderService: folderService,
		b2Service:     b2Service, // No dereference needed now
	}
}

// Helper function to extract and validate user ID
func (fc *FolderController) getUserID(c *gin.Context) (string, error) {
	userID, exists := c.Get("userIdStr")
	if !exists {
		return "", fmt.Errorf("user not authenticated")
	}

	userIDStr, ok := userID.(string)
	if !ok {
		return "", fmt.Errorf("invalid user ID format")
	}

	if !primitive.IsValidObjectID(userIDStr) {
		return "", fmt.Errorf("invalid user ID format")
	}

	return userIDStr, nil
}

// Helper function to handle common error responses
func (fc *FolderController) handleError(c *gin.Context, err error, defaultMessage string, defaultStatus int) {
	statusCode := defaultStatus
	message := defaultMessage

	// Handle specific error cases
	switch err.Error() {
	case "folder not found":
		statusCode = http.StatusNotFound
		message = "Folder not found"
	case "insufficient permissions":
		statusCode = http.StatusForbidden
		message = "Insufficient permissions"
	case "parent folder not found":
		statusCode = http.StatusNotFound
		message = "Parent folder not found"
	case "insufficient permissions to share folder":
		statusCode = http.StatusForbidden
		message = "Insufficient permissions to share this folder"
	case "file not found in folder":
		statusCode = http.StatusNotFound
		message = "File not found in folder"
	default:
		// Check for specific error patterns
		errorStr := err.Error()
		if len(errorStr) > 25 && errorStr[:19] == "folder with name '" && errorStr[len(errorStr)-15:] == "already exists" {
			statusCode = http.StatusConflict
			message = "Folder with this name already exists"
		} else if len(errorStr) > 17 && errorStr[:17] == "user with email " {
			statusCode = http.StatusNotFound
			message = "User not found"
		}
	}

	c.JSON(statusCode, gin.H{
		"success": false,
		"message": message,
		"error":   err.Error(),
	})
}

// CreateFolder creates a new folder
func (fc *FolderController) CreateFolder(c *gin.Context) {
	userIDStr, err := fc.getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	// Parse request body
	var req struct {
		Name        string  `json:"name" binding:"required,min=1,max=255"`
		Description string  `json:"description,omitempty"`
		ParentID    *string `json:"parent_id,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request data",
			"error":   err.Error(),
		})
		return
	}

	// Validate parent ID if provided
	if req.ParentID != nil && *req.ParentID != "" && !primitive.IsValidObjectID(*req.ParentID) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid parent folder ID format",
		})
		return
	}

	// Create folder
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

// ListRootFolders lists all root folders for the authenticated user
func (fc *FolderController) ListRootFolders(c *gin.Context) {
	userIDStr, err := fc.getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	folders, err := fc.folderService.ListRootFolders(userIDStr)
	if err != nil {
		fc.handleError(c, err, "Failed to retrieve folders", http.StatusInternalServerError)
		return
	}

	// Transform data to match API specification
	var folderData []gin.H
	for _, folder := range folders {
		// Get file count using the service method
		files, fileErr := fc.folderService.GetFilesInFolder(folder.ID.Hex(), userIDStr)
		fileCount := 0
		if fileErr == nil {
			fileCount = len(files)
		}

		// You could add subfolder count logic here if needed
		subfolderCount := 0

		folderData = append(folderData, gin.H{
			"id":              folder.ID,
			"name":            folder.Name,
			"type":            "folder",
			"file_count":      fileCount,
			"subfolder_count": subfolderCount,
			"created_at":      folder.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    folderData,
	})
}

// GetFolderContents retrieves folder contents in Google Drive style (subfolders + files)
func (fc *FolderController) GetFolderContents(c *gin.Context) {
	userIDStr, err := fc.getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	folderID := c.Param("id")
	if !primitive.IsValidObjectID(folderID) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid folder ID format",
		})
		return
	}

	contents, err := fc.folderService.GetFolderContents(folderID, userIDStr)
	if err != nil {
		fc.handleError(c, err, "Failed to retrieve folder contents", http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    contents,
	})
}

// GetFolder retrieves a specific folder by ID
func (fc *FolderController) GetFolder(c *gin.Context) {
	userIDStr, err := fc.getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	folderID := c.Param("id")
	if !primitive.IsValidObjectID(folderID) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid folder ID format",
		})
		return
	}

	folder, err := fc.folderService.GetFolderByID(folderID, userIDStr)
	if err != nil {
		fc.handleError(c, err, "Failed to retrieve folder", http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Folder retrieved successfully",
		"data":    folder,
	})
}

// RenameFolder renames an existing folder
func (fc *FolderController) RenameFolder(c *gin.Context) {
	userIDStr, err := fc.getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	folderID := c.Param("id")
	if !primitive.IsValidObjectID(folderID) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid folder ID format",
		})
		return
	}

	var req struct {
		Name string `json:"name" binding:"required,min=1,max=255"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request data",
			"error":   err.Error(),
		})
		return
	}

	err = fc.folderService.RenameFolder(folderID, req.Name, userIDStr)
	if err != nil {
		fc.handleError(c, err, "Failed to rename folder", http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Folder renamed successfully",
	})
}

// DeleteFolder soft deletes a folder
func (fc *FolderController) DeleteFolder(c *gin.Context) {
	userIDStr, err := fc.getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	folderID := c.Param("id")
	if !primitive.IsValidObjectID(folderID) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid folder ID format",
		})
		return
	}

	err = fc.folderService.DeleteFolder(c.Request.Context(), folderID, userIDStr)
	if err != nil {
		fc.handleError(c, err, "Failed to delete folder", http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Folder deleted successfully",
	})
}

// ShareFolder shares a folder with another user with inheritance option
func (fc *FolderController) ShareFolder(c *gin.Context) {
	userIDStr, err := fc.getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	folderID := c.Param("id")
	if !primitive.IsValidObjectID(folderID) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid folder ID format",
		})
		return
	}

	var req struct {
		Email             string `json:"email" binding:"required,email"`
		Role              string `json:"role" binding:"required,oneof=viewer editor admin"`
		InheritToChildren bool   `json:"inherit_to_children,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid request data",
			"error":   err.Error(),
		})
		return
	}

	// Use ShareFolderWithInheritance method
	response, err := fc.folderService.ShareFolderWithInheritance(folderID, userIDStr, req.Email, req.Role, req.InheritToChildren)
	if err != nil {
		fc.handleError(c, err, "Failed to share folder", http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Folder shared successfully",
		"data":    response,
	})
}

// GetFolderPermissions retrieves permissions for a folder
func (fc *FolderController) GetFolderPermissions(c *gin.Context) {
	userIDStr, err := fc.getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	folderID := c.Param("id")
	if !primitive.IsValidObjectID(folderID) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid folder ID format",
		})
		return
	}

	permissions, err := fc.folderService.GetFolderPermissions(folderID, userIDStr)
	if err != nil {
		fc.handleError(c, err, "Failed to retrieve folder permissions", http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Folder permissions retrieved successfully",
		"data":    permissions,
	})
}

// GetFilesInFolder retrieves all files within a folder
func (fc *FolderController) GetFilesInFolder(c *gin.Context) {
	userIDStr, err := fc.getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	folderID := c.Param("id")
	if !primitive.IsValidObjectID(folderID) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid folder ID format",
		})
		return
	}

	files, err := fc.folderService.GetFilesInFolder(folderID, userIDStr)
	if err != nil {
		fc.handleError(c, err, "Failed to retrieve files", http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Files retrieved successfully",
		"data":    files,
		"count":   len(files),
	})
}

// DeleteFileFromFolder removes a file from a folder
func (fc *FolderController) DeleteFileFromFolder(c *gin.Context) {
	userIDStr, err := fc.getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	folderID := c.Param("id")
	fileID := c.Param("fileId")

	if !primitive.IsValidObjectID(folderID) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid folder ID format",
		})
		return
	}

	if !primitive.IsValidObjectID(fileID) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid file ID format",
		})
		return
	}

	err = fc.folderService.DeleteFileFromFolder(folderID, fileID, userIDStr)
	if err != nil {
		fc.handleError(c, err, "Failed to delete file", http.StatusInternalServerError)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "File deleted successfully",
	})
}

// DownloadFolder streams the folder contents as ZIP directly to the client
func (fc *FolderController) DownloadFolder(c *gin.Context) {
	userIDStr, err := fc.getUserID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	folderID := c.Param("id")
	if !primitive.IsValidObjectID(folderID) {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid folder ID format",
		})
		return
	}

	// Set longer timeout for large folder downloads
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Minute)
	defer cancel()

	// Stream folder as ZIP directly to response (memory efficient)
	err = fc.folderService.DownloadFolder(ctx, c.Writer, folderID, userIDStr)
	if err != nil {
		// Check if we haven't started writing to the response yet
		if !c.Writer.Written() {
			fc.handleError(c, err, "Failed to download folder", http.StatusInternalServerError)
		} else {
			// If streaming has started, we can only log the error
			fmt.Printf("Error streaming folder zip for folder %s: %v\n", folderID, err)
		}
		return
	}
}
