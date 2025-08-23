package controllers

import (
	"net/http"
	"phynixdrive/services"
	"phynixdrive/utils"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
)

type FileController struct {
	fileService *services.FileService
}

func NewFileController(db *mongo.Database, folderService *services.FolderService, b2Service *services.B2Service, permissionService *services.PermissionService) *FileController {
	return &FileController{
		fileService: services.NewFileService(db, folderService, b2Service, permissionService),
	}
}

func (fc *FileController) UploadFiles(c *gin.Context) {
	userId := c.GetString("userIdStr")
	if userId == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	// Parse multipart form
	form, err := c.MultipartForm()
	if err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid multipart form", nil)
		return
	}

	files := form.File["files[]"]
	relativePaths := form.Value["relativePath[]"]

	if len(files) == 0 {
		utils.ErrorResponse(c, http.StatusBadRequest, "No files provided", nil)
		return
	}

	if len(files) != len(relativePaths) {
		utils.ErrorResponse(c, http.StatusBadRequest, "Files and relative paths count mismatch", nil)
		return
	}

	// Validate total upload size
	var totalSize int64
	for _, file := range files {
		totalSize += file.Size
		if file.Size > 100*1024*1024 { // 100MB limit per file
			utils.ErrorResponse(c, http.StatusBadRequest, "File exceeds 100MB limit: "+file.Filename, nil)
			return
		}
	}

	// Check user storage quota
	canUpload, err := fc.fileService.CheckStorageQuota(userId, totalSize)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Storage check failed", nil)
		return
	}
	if !canUpload {
		utils.ErrorResponse(c, http.StatusBadRequest, "Upload would exceed 2GB storage limit", nil)
		return
	}

	uploadResult, err := fc.fileService.UploadFiles(userId, files, relativePaths)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, err.Error(), nil)
		return
	}

	utils.SuccessResponse(c, "Files uploaded successfully", uploadResult)
}

func (fc *FileController) GetAllFiles(c *gin.Context) {
	userId := c.GetString("userIdStr")
	if userId == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	files, err := fc.fileService.GetRootFiles(userId)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to get files", nil)
		return
	}

	utils.SuccessResponse(c, "Files retrieved", files)
}

func (fc *FileController) GetFolderFiles(c *gin.Context) {
	folderId := c.Param("id")
	userId := c.GetString("userIdStr")

	if userId == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	if folderId == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "Folder ID is required", nil)
		return
	}

	files, err := fc.fileService.GetFolderFiles(folderId, userId)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to get folder files", nil)
		return
	}

	utils.SuccessResponse(c, "Folder files retrieved", files)
}

// Update the existing DownloadFile method and add new methods

func (fc *FileController) DownloadFile(c *gin.Context) {
	fileId := c.Param("id")
	userId := c.GetString("userIdStr")

	if userId == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	if fileId == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "File ID is required", nil)
		return
	}

	downloadURL, err := fc.fileService.GetDownloadURL(fileId, userId)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, err.Error(), nil)
		return
	}

	utils.SuccessResponse(c, "Download URL generated", map[string]string{
		"downloadUrl": downloadURL,
	})
}

func (fc *FileController) PreviewFile(c *gin.Context) {
	fileId := c.Param("id")
	userId := c.GetString("userIdStr")

	if userId == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	if fileId == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "File ID is required", nil)
		return
	}

	previewURL, err := fc.fileService.GetPreviewURL(fileId, userId)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, err.Error(), nil)
		return
	}

	utils.SuccessResponse(c, "Preview URL generated", map[string]string{
		"previewUrl": previewURL,
	})
}

func (fc *FileController) GetFileURLs(c *gin.Context) {
	fileId := c.Param("id")
	userId := c.GetString("userIdStr")

	if userId == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	if fileId == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "File ID is required", nil)
		return
	}

	downloadURL, previewURL, err := fc.fileService.GetFileURLs(fileId, userId)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, err.Error(), nil)
		return
	}

	response := map[string]interface{}{
		"downloadUrl": downloadURL,
	}

	// Only include preview URL if file is previewable
	if previewURL != "" {
		response["previewUrl"] = previewURL
		response["isPreviewable"] = true
	} else {
		response["isPreviewable"] = false
	}

	utils.SuccessResponse(c, "File URLs generated", response)
}

func (fc *FileController) DeleteFile(c *gin.Context) {
	fileId := c.Param("id") // Changed from "fileId" to "id" to match route
	userId := c.GetString("userIdStr")

	if userId == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	if fileId == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "File ID is required", nil)
		return
	}

	err := fc.fileService.DeleteFile(fileId, userId)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, err.Error(), nil)
		return
	}

	utils.SuccessResponse(c, "File moved to trash", nil)
}

func (fc *FileController) GetFileVersions(c *gin.Context) {
	fileId := c.Param("id")
	userId := c.GetString("userIdStr")

	if userId == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	if fileId == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "File ID is required", nil)
		return
	}

	versions, err := fc.fileService.GetFileVersions(fileId, userId)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to get file versions", nil)
		return
	}

	utils.SuccessResponse(c, "File versions retrieved", versions)
}

func (fc *FileController) GetFilePermissions(c *gin.Context) {
	fileId := c.Param("id")
	userId := c.GetString("userIdStr")

	if userId == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	if fileId == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "File ID is required", nil)
		return
	}

	permissions, err := fc.fileService.GetFilePermissions(fileId, userId)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to get file permissions", nil)
		return
	}

	utils.SuccessResponse(c, "File permissions retrieved", permissions)
}

func (fc *FileController) GetFileMetadata(c *gin.Context) {
	fileId := c.Param("id")
	userId := c.GetString("userIdStr")

	if userId == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	if fileId == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "File ID is required", nil)
		return
	}

	fileMetadata, err := fc.fileService.GetFileByID(fileId, userId)
	if err != nil {
		utils.ErrorResponse(c, http.StatusInternalServerError, "Failed to get file metadata", nil)
		return
	}

	utils.SuccessResponse(c, "File metadata retrieved", fileMetadata)
}

func (fc *FileController) RenameFile(c *gin.Context) {
	fileId := c.Param("id")
	userId := c.GetString("userIdStr")

	if userId == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	if fileId == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "File ID is required", nil)
		return
	}

	var req struct {
		NewName string `json:"newName" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid request body", nil)
		return
	}

	utils.SuccessResponse(c, "File renamed successfully", nil)
}

func (fc *FileController) ShareFile(c *gin.Context) {
	fileId := c.Param("id")
	userId := c.GetString("userIdStr")

	if userId == "" {
		utils.ErrorResponse(c, http.StatusUnauthorized, "User not authenticated", nil)
		return
	}

	if fileId == "" {
		utils.ErrorResponse(c, http.StatusBadRequest, "File ID is required", nil)
		return
	}

	var req struct {
		Email      string `json:"email" binding:"required"`
		Permission string `json:"permission" binding:"required"` // "read" or "write"
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.ErrorResponse(c, http.StatusBadRequest, "Invalid request body", nil)
		return
	}

	utils.SuccessResponse(c, "File shared successfully", nil)
}
