package services

import (
	"context"
	"fmt"
	"mime/multipart"
	"path/filepath"
	"phynixdrive/models"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type FileService struct {
	fileCollection    *mongo.Collection
	userCollection    *mongo.Collection
	folderService     *FolderService
	b2Service         *B2Service
	permissionService *PermissionService
}

type FileUploadRequest struct {
	File         multipart.File
	Filename     string
	RelativePath string
	UserID       string
}

func NewFileService(db *mongo.Database, folderService *FolderService, b2Service *B2Service, permissionService *PermissionService) *FileService {
	return &FileService{
		fileCollection:    db.Collection("files"),
		userCollection:    db.Collection("users"),
		folderService:     folderService,
		b2Service:         b2Service,
		permissionService: permissionService,
	}
}

// CheckStorageQuota checks if user can upload additional files
func (s *FileService) CheckStorageQuota(userID string, additionalSize int64) (bool, error) {
	const maxUserStorage = 2 * 1024 * 1024 * 1024 // 2GB

	ctx := context.Background()
	userObjID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return false, fmt.Errorf("invalid user ID: %w", err)
	}

	var user models.User
	err = s.userCollection.FindOne(ctx, bson.M{"_id": userObjID}).Decode(&user)
	if err != nil {
		return false, fmt.Errorf("user not found: %w", err)
	}

	return user.UsedStorage+additionalSize <= maxUserStorage, nil
}

// UploadFiles handles file uploads with proper multipart form handling
func (s *FileService) UploadFiles(userID string, files []*multipart.FileHeader, relativePaths []string) ([]models.File, error) {
	const maxFileSize = 100 * 1024 * 1024         // 100MB
	const maxUserStorage = 2 * 1024 * 1024 * 1024 // 2GB

	if len(files) == 0 {
		return nil, fmt.Errorf("no files to upload")
	}

	ctx := context.Background()

	// Get user's current storage usage
	var user models.User
	userObjID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID: %w", err)
	}

	err = s.userCollection.FindOne(ctx, bson.M{"_id": userObjID}).Decode(&user)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	// Calculate total upload size
	var totalSize int64
	for _, file := range files {
		totalSize += file.Size
		if file.Size > maxFileSize {
			return nil, fmt.Errorf("file %s exceeds maximum size of 100MB", file.Filename)
		}
	}

	// Check total storage limit
	if user.UsedStorage+totalSize > maxUserStorage {
		return nil, fmt.Errorf("upload would exceed storage limit of 2GB")
	}

	var uploadedFiles []models.File
	var uploadedSize int64

	// Process each file
	for i, fileHeader := range files {
		// Open the file
		file, err := fileHeader.Open()
		if err != nil {
			s.cleanupUploadedFiles(uploadedFiles)
			return nil, fmt.Errorf("failed to open file %s: %w", fileHeader.Filename, err)
		}
		defer file.Close()

		// Extract folder path from relative path
		relativePath := relativePaths[i]
		folderPath := filepath.Dir(relativePath)
		if folderPath == "." {
			folderPath = ""
		}

		// Get or create folder structure
		var folderID *primitive.ObjectID
		if folderPath != "" {
			folderIDStr, err := s.folderService.GetOrCreateFolderPath(folderPath, userID)
			if err != nil {
				s.cleanupUploadedFiles(uploadedFiles)
				return nil, fmt.Errorf("failed to create folder structure for %s: %w", relativePath, err)
			}
			if folderIDStr != nil {
				folderID = folderIDStr
			}
		}

		// Upload file to B2
		uploadResult, err := s.b2Service.UploadFile(file, fileHeader.Filename, userID, relativePath)
		if err != nil {
			s.cleanupUploadedFiles(uploadedFiles)
			return nil, fmt.Errorf("failed to upload %s to B2: %w", fileHeader.Filename, err)
		}

		// Create file metadata
		fileDoc := models.File{
			ID:           primitive.NewObjectID(),
			Name:         fileHeader.Filename,
			OriginalName: fileHeader.Filename,
			Size:         fileHeader.Size,
			MimeType:     s.getMimeType(fileHeader.Filename),
			ContentType:  s.getMimeType(fileHeader.Filename),
			Extension:    strings.ToLower(filepath.Ext(fileHeader.Filename)),
			OwnerID:      userObjID,
			B2FileID:     uploadResult.FileID,
			B2FileName:   uploadResult.FileName,
			SHA1Hash:     uploadResult.SHA1,
			FolderID:     folderID,
			RelativePath: relativePath,
			CreatedAt:    time.Now(),
			UpdatedAt:    time.Now(),
			IsDeleted:    false,
		}

		// Save file metadata to database
		_, err = s.fileCollection.InsertOne(ctx, fileDoc)
		if err != nil {
			s.cleanupUploadedFiles(append(uploadedFiles, fileDoc))
			return nil, fmt.Errorf("failed to save file metadata for %s: %w", fileHeader.Filename, err)
		}

		uploadedFiles = append(uploadedFiles, fileDoc)
		uploadedSize += fileHeader.Size
	}

	// Update user's storage usage
	_, err = s.userCollection.UpdateOne(
		ctx,
		bson.M{"_id": userObjID},
		bson.M{"$inc": bson.M{"used_storage": uploadedSize}},
	)
	if err != nil {
		return uploadedFiles, fmt.Errorf("files uploaded but failed to update storage usage: %w", err)
	}

	return uploadedFiles, nil
}

// GetRootFiles gets files in the root directory (no folder)
func (s *FileService) GetRootFiles(userID string) ([]models.File, error) {
	return s.GetFilesByFolder(nil, userID)
}

// GetFilesByFolder gets files by folder ID (internal method)
func (s *FileService) GetFilesByFolder(folderID *string, userID string) ([]models.File, error) {
	ctx := context.Background()

	userObjID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID: %w", err)
	}

	filter := bson.M{
		"owner_id":   userObjID,
		"deleted_at": nil,
	}

	if folderID != nil && *folderID != "" {
		// Check folder permissions if service is available
		if s.permissionService != nil {
			hasPermission, err := s.permissionService.HasFolderPermission(ctx, userID, *folderID, "viewer")
			if err != nil {
				return nil, fmt.Errorf("permission check failed: %w", err)
			}
			if !hasPermission {
				return nil, fmt.Errorf("insufficient permissions")
			}
		}

		folderObjID, err := primitive.ObjectIDFromHex(*folderID)
		if err != nil {
			return nil, fmt.Errorf("invalid folder ID: %w", err)
		}
		filter["folder_id"] = folderObjID
	} else {
		filter["folder_id"] = nil
	}

	cursor, err := s.fileCollection.Find(ctx, filter, options.Find().SetSort(bson.M{"name": 1}))
	if err != nil {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}
	defer cursor.Close(ctx)

	var files []models.File
	if err = cursor.All(ctx, &files); err != nil {
		return nil, fmt.Errorf("failed to decode files: %w", err)
	}

	return files, nil
}

func (s *FileService) GetFileByID(fileID string, userID string) (*models.File, error) {
	objID, err := primitive.ObjectIDFromHex(fileID)
	if err != nil {
		return nil, fmt.Errorf("invalid file ID: %w", err)
	}

	ctx := context.Background()
	var file models.File

	err = s.fileCollection.FindOne(ctx, bson.M{
		"_id":        objID,
		"deleted_at": nil,
	}).Decode(&file)

	if err == mongo.ErrNoDocuments {
		return nil, fmt.Errorf("file not found")
	} else if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}

	// Check permissions if service is available
	if s.permissionService != nil {
		hasPermission, err := s.permissionService.HasFilePermission(ctx, userID, fileID, "viewer")
		if err != nil {
			return nil, fmt.Errorf("permission check failed: %w", err)
		}
		if !hasPermission {
			return nil, fmt.Errorf("insufficient permissions")
		}
	}

	return &file, nil
}

// Add these methods to your FileService struct

// GetDownloadURL generates a download URL with longer expiry
func (s *FileService) GetDownloadURL(fileID string, userID string) (string, error) {
	file, err := s.GetFileByID(fileID, userID)
	if err != nil {
		return "", err
	}

	// Generate download URL from B2
	url, err := s.b2Service.GetDownloadURLForFile(file.B2FileID)
	if err != nil {
		return "", fmt.Errorf("failed to generate download URL: %w", err)
	}

	return url, nil
}

// GetPreviewURL generates a preview URL with shorter expiry
func (s *FileService) GetPreviewURL(fileID string, userID string) (string, error) {
	file, err := s.GetFileByID(fileID, userID)
	if err != nil {
		return "", err
	}

	// Check if file is previewable
	if !s.b2Service.IsPreviewableFile(file.Name) {
		return "", fmt.Errorf("file type not previewable")
	}

	// Generate preview URL from B2
	url, err := s.b2Service.GetPreviewURL(file.B2FileID)
	if err != nil {
		return "", fmt.Errorf("failed to generate preview URL: %w", err)
	}

	return url, nil
}

func (s *FileService) DeleteFile(fileID string, userID string) error {
	objID, err := primitive.ObjectIDFromHex(fileID)
	if err != nil {
		return fmt.Errorf("invalid file ID: %w", err)
	}

	// Check permissions if service is available
	ctx := context.Background()
	if s.permissionService != nil {
		hasPermission, err := s.permissionService.HasFilePermission(ctx, userID, fileID, "admin")
		if err != nil {
			return fmt.Errorf("permission check failed: %w", err)
		}
		if !hasPermission {
			return fmt.Errorf("insufficient permissions")
		}
	}

	// Get file info before deletion
	var file models.File
	err = s.fileCollection.FindOne(ctx, bson.M{
		"_id":        objID,
		"deleted_at": nil,
	}).Decode(&file)

	if err == mongo.ErrNoDocuments {
		return fmt.Errorf("file not found")
	} else if err != nil {
		return fmt.Errorf("database error: %w", err)
	}

	// Soft delete file
	now := time.Now()
	update := bson.M{
		"$set": bson.M{
			"deleted_at": &now,
			"updated_at": now,
			"is_deleted": true,
		},
	}

	_, err = s.fileCollection.UpdateOne(ctx, bson.M{"_id": objID}, update)
	if err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	// Update user's storage usage
	userObjID, _ := primitive.ObjectIDFromHex(userID)
	_, err = s.userCollection.UpdateOne(
		ctx,
		bson.M{"_id": userObjID},
		bson.M{"$inc": bson.M{"used_storage": -file.Size}},
	)
	if err != nil {
		return fmt.Errorf("file deleted but failed to update storage usage: %w", err)
	}

	return nil
}

func (s *FileService) cleanupUploadedFiles(files []models.File) {
	ctx := context.Background()
	for _, file := range files {
		// Delete from B2
		if s.b2Service != nil {
			s.b2Service.DeleteFile(file.B2FileID)
		}
		// Delete from database
		s.fileCollection.DeleteOne(ctx, bson.M{"_id": file.ID})
	}
}

func (s *FileService) getMimeType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".pdf":
		return "application/pdf"
	case ".txt":
		return "text/plain"
	case ".doc":
		return "application/msword"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xls":
		return "application/vnd.ms-excel"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".zip":
		return "application/zip"
	case ".mp4":
		return "video/mp4"
	case ".mp3":
		return "audio/mpeg"
	default:
		return "application/octet-stream"
	}
}
