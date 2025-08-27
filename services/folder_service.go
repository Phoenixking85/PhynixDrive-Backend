package services

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"phynixdrive/models"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type FolderService struct {
	folderCollection  *mongo.Collection
	fileCollection    *mongo.Collection
	userCollection    *mongo.Collection
	permissionService *PermissionService
	b2Service         *B2Service // Add B2Service dependency
	httpClient        *http.Client
}

func NewFolderService(db *mongo.Database, permissionService *PermissionService, b2Service *B2Service) *FolderService {
	return &FolderService{
		folderCollection:  db.Collection("folders"),
		fileCollection:    db.Collection("files"),
		userCollection:    db.Collection("users"),
		permissionService: permissionService,
		b2Service:         b2Service, // Initialize B2Service
		httpClient:        &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *FolderService) CreateFolder(name string, parentID *string, ownerID string) (*models.Folder, error) {
	ctx := context.Background()

	// Validate owner ID
	ownerObjID, err := primitive.ObjectIDFromHex(ownerID)
	if err != nil {
		return nil, fmt.Errorf("invalid owner ID: %w", err)
	}

	var parentObjID *primitive.ObjectID

	// Validate parent folder exists and user has permission
	if parentID != nil && *parentID != "" {
		parentObjIDTemp, err := primitive.ObjectIDFromHex(*parentID)
		if err != nil {
			return nil, fmt.Errorf("invalid parent ID: %w", err)
		}
		parentObjID = &parentObjIDTemp

		var parentFolder models.Folder
		err = s.folderCollection.FindOne(ctx, bson.M{
			"_id":        *parentObjID,
			"is_deleted": false,
		}).Decode(&parentFolder)

		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("parent folder not found")
		} else if err != nil {
			return nil, fmt.Errorf("database error: %w", err)
		}

		// Check permissions if service is available
		if s.permissionService != nil {
			hasPermission, err := s.permissionService.HasFolderPermission(context.Background(), ownerID, *parentID, "editor")
			if err != nil {
				return nil, fmt.Errorf("permission check failed: %w", err)
			}
			if !hasPermission {
				return nil, fmt.Errorf("insufficient permissions")
			}
		}
	}

	// Check if folder with same name exists in same parent
	filter := bson.M{
		"name":       name,
		"owner_id":   ownerObjID,
		"is_deleted": false,
	}
	if parentObjID != nil {
		filter["parent_id"] = *parentObjID
	} else {
		filter["parent_id"] = nil
	}

	var existingFolder models.Folder
	err = s.folderCollection.FindOne(ctx, filter).Decode(&existingFolder)
	if err == nil {
		return nil, fmt.Errorf("folder with name '%s' already exists", name)
	} else if err != mongo.ErrNoDocuments {
		return nil, fmt.Errorf("database error: %w", err)
	}

	// Build path
	path := name
	if parentObjID != nil {
		parentPath, err := s.getFolderPath(*parentObjID)
		if err != nil {
			return nil, fmt.Errorf("failed to get parent path: %w", err)
		}
		path = parentPath + "/" + name
	}

	// Create folder
	folder := models.Folder{
		ID:          primitive.NewObjectID(),
		Name:        name,
		OwnerID:     ownerObjID,
		ParentID:    parentObjID,
		Path:        path,
		Permissions: []models.Permission{},
		IsDeleted:   false,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	_, err = s.folderCollection.InsertOne(ctx, folder)
	if err != nil {
		return nil, fmt.Errorf("failed to create folder: %w", err)
	}

	return &folder, nil
}

func (s *FolderService) getFolderPath(folderID primitive.ObjectID) (string, error) {
	ctx := context.Background()
	var folder models.Folder

	err := s.folderCollection.FindOne(ctx, bson.M{
		"_id":        folderID,
		"is_deleted": false,
	}).Decode(&folder)

	if err != nil {
		return "", err
	}

	if folder.ParentID == nil {
		return folder.Name, nil
	}

	parentPath, err := s.getFolderPath(*folder.ParentID)
	if err != nil {
		return "", err
	}

	return parentPath + "/" + folder.Name, nil
}

func (s *FolderService) GetOrCreateFolderPath(path string, ownerID string) (*primitive.ObjectID, error) {
	if path == "" || path == "/" {
		return nil, nil // Root folder
	}

	// Validate owner ID
	ownerObjID, err := primitive.ObjectIDFromHex(ownerID)
	if err != nil {
		return nil, fmt.Errorf("invalid owner ID: %w", err)
	}

	// Clean and split path
	path = strings.Trim(path, "/")
	parts := strings.Split(path, "/")

	var currentParentID *primitive.ObjectID
	ctx := context.Background()

	for _, part := range parts {
		if part == "" {
			continue
		}

		// Check if folder exists
		filter := bson.M{
			"name":       part,
			"owner_id":   ownerObjID,
			"is_deleted": false,
		}
		if currentParentID != nil {
			filter["parent_id"] = *currentParentID
		} else {
			filter["parent_id"] = nil
		}

		var folder models.Folder
		err := s.folderCollection.FindOne(ctx, filter).Decode(&folder)

		if err == mongo.ErrNoDocuments {
			// Build path for new folder
			currentPath := part
			if currentParentID != nil {
				parentPath, err := s.getFolderPath(*currentParentID)
				if err != nil {
					return nil, fmt.Errorf("failed to get parent path: %w", err)
				}
				currentPath = parentPath + "/" + part
			}

			// Create folder
			newFolder := models.Folder{
				ID:          primitive.NewObjectID(),
				Name:        part,
				OwnerID:     ownerObjID,
				ParentID:    currentParentID,
				Path:        currentPath,
				Permissions: []models.Permission{},
				IsDeleted:   false,
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}

			_, err = s.folderCollection.InsertOne(ctx, newFolder)
			if err != nil {
				return nil, fmt.Errorf("failed to create folder '%s': %w", part, err)
			}

			currentParentID = &newFolder.ID
		} else if err != nil {
			return nil, fmt.Errorf("database error: %w", err)
		} else {
			currentParentID = &folder.ID
		}
	}

	return currentParentID, nil
}

func (s *FolderService) ListRootFolders(userID string) ([]models.Folder, error) {
	ctx := context.Background()

	ownerObjID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID: %w", err)
	}

	filter := bson.M{
		"owner_id":   ownerObjID,
		"parent_id":  nil,
		"is_deleted": false,
	}

	cursor, err := s.folderCollection.Find(ctx, filter, options.Find().SetSort(bson.M{"name": 1}))
	if err != nil {
		return nil, fmt.Errorf("failed to list folders: %w", err)
	}
	defer cursor.Close(ctx)

	var folders []models.Folder
	if err = cursor.All(ctx, &folders); err != nil {
		return nil, fmt.Errorf("failed to decode folders: %w", err)
	}

	return folders, nil
}

func (s *FolderService) GetFolderByID(folderID string, userID string) (*models.Folder, error) {
	objID, err := primitive.ObjectIDFromHex(folderID)
	if err != nil {
		return nil, fmt.Errorf("invalid folder ID: %w", err)
	}

	ctx := context.Background()
	var folder models.Folder

	err = s.folderCollection.FindOne(ctx, bson.M{
		"_id":        objID,
		"is_deleted": false,
	}).Decode(&folder)

	if err == mongo.ErrNoDocuments {
		return nil, fmt.Errorf("folder not found")
	} else if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}

	// Check permissions if service is available
	if s.permissionService != nil {
		hasPermission, err := s.permissionService.HasFolderPermission(context.Background(), userID, folderID, "viewer")
		if err != nil {
			return nil, fmt.Errorf("permission check failed: %w", err)
		}
		if !hasPermission {
			return nil, fmt.Errorf("insufficient permissions")
		}
	}

	return &folder, nil
}

func (s *FolderService) RenameFolder(folderID string, newName string, userID string) error {
	objID, err := primitive.ObjectIDFromHex(folderID)
	if err != nil {
		return fmt.Errorf("invalid folder ID: %w", err)
	}

	// Check permissions if service is available
	if s.permissionService != nil {
		hasPermission, err := s.permissionService.HasFolderPermission(context.Background(), userID, folderID, "editor")
		if err != nil {
			return fmt.Errorf("permission check failed: %w", err)
		}
		if !hasPermission {
			return fmt.Errorf("insufficient permissions")
		}
	}

	ctx := context.Background()

	// Get current folder to update path
	var currentFolder models.Folder
	err = s.folderCollection.FindOne(ctx, bson.M{
		"_id":        objID,
		"is_deleted": false,
	}).Decode(&currentFolder)

	if err == mongo.ErrNoDocuments {
		return fmt.Errorf("folder not found")
	} else if err != nil {
		return fmt.Errorf("database error: %w", err)
	}

	// Update path
	newPath := newName
	if currentFolder.ParentID != nil {
		parentPath, err := s.getFolderPath(*currentFolder.ParentID)
		if err != nil {
			return fmt.Errorf("failed to get parent path: %w", err)
		}
		newPath = parentPath + "/" + newName
	}

	update := bson.M{
		"$set": bson.M{
			"name":       newName,
			"path":       newPath,
			"updated_at": time.Now(),
		},
	}

	result, err := s.folderCollection.UpdateOne(ctx, bson.M{
		"_id":        objID,
		"is_deleted": false,
	}, update)

	if err != nil {
		return fmt.Errorf("failed to rename folder: %w", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("folder not found")
	}

	return nil
}

func (s *FolderService) DeleteFolder(folderID string, userID string) error {
	objID, err := primitive.ObjectIDFromHex(folderID)
	if err != nil {
		return fmt.Errorf("invalid folder ID: %w", err)
	}

	// Check permissions if service is available
	if s.permissionService != nil {

		hasPermission, err := s.permissionService.HasFolderPermission(context.Background(), userID, folderID, "admin")
		if err != nil {
			return fmt.Errorf("permission check failed: %w", err)
		}
		if !hasPermission {
			return fmt.Errorf("insufficient permissions")
		}
	}

	ctx := context.Background()
	now := time.Now()

	update := bson.M{
		"$set": bson.M{
			"is_deleted": true,
			"deleted_at": now,
			"updated_at": now,
		},
	}

	result, err := s.folderCollection.UpdateOne(ctx, bson.M{
		"_id":        objID,
		"is_deleted": false,
	}, update)

	if err != nil {
		return fmt.Errorf("failed to delete folder: %w", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("folder not found")
	}

	return nil
}

func (s *FolderService) GetFilesInFolder(folderID string, userID string) ([]models.File, error) {
	ctx := context.Background()

	// Validate folder ID
	folderObjID, err := primitive.ObjectIDFromHex(folderID)
	if err != nil {
		return nil, fmt.Errorf("invalid folder ID: %w", err)
	}

	// Check if folder exists and user has permission
	if s.permissionService != nil {
		hasPermission, err := s.permissionService.HasFolderPermission(ctx, userID, folderID, "viewer")
		if err != nil {
			return nil, fmt.Errorf("permission check failed: %w", err)
		}
		if !hasPermission {
			return nil, fmt.Errorf("insufficient permissions")
		}
	}

	// Get files in the folder
	filter := bson.M{
		"folder_id":  folderObjID,
		"deleted_at": nil,
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

func (s *FolderService) ShareFolder(folderID string, userID string, email string, role string) error {
	// Check if user has admin permissions on the folder
	if s.permissionService != nil {
		hasPermission, err := s.permissionService.HasFolderPermission(context.Background(), userID, folderID, "admin")
		if err != nil {
			return fmt.Errorf("permission check failed: %w", err)
		}
		if !hasPermission {
			return fmt.Errorf("insufficient permissions to share folder")
		}

		// Find user by email
		ctx := context.Background()
		var targetUser models.User
		err = s.userCollection.FindOne(ctx, bson.M{"email": email}).Decode(&targetUser)
		if err == mongo.ErrNoDocuments {
			return fmt.Errorf("user with email %s not found", email)
		} else if err != nil {
			return fmt.Errorf("database error: %w", err)
		}

		// Add permission for the shared user
		err = s.permissionService.ShareFolder(ctx, folderID, targetUser.ID.Hex(), role, userID)
		if err != nil {
			return fmt.Errorf("failed to add folder permission: %w", err)
		}
	}

	return nil
}

func (s *FolderService) GetFolderPermissions(folderID string, userID string) ([]models.Permission, error) {
	// Check if folder exists and user has permission to view it
	folder, err := s.GetFolderByID(folderID, userID)
	if err != nil {
		return nil, err
	}

	// Return the permissions from the folder
	return folder.Permissions, nil
}

func (s *FolderService) DeleteFileFromFolder(folderID string, fileID string, userID string) error {
	// Check if user has permission to modify the folder
	if s.permissionService != nil {
		hasPermission, err := s.permissionService.HasFolderPermission(context.Background(), userID, folderID, "editor")
		if err != nil {
			return fmt.Errorf("permission check failed: %w", err)
		}
		if !hasPermission {
			return fmt.Errorf("insufficient permissions")
		}
	}

	ctx := context.Background()
	fileObjID, err := primitive.ObjectIDFromHex(fileID)
	if err != nil {
		return fmt.Errorf("invalid file ID: %w", err)
	}

	folderObjID, err := primitive.ObjectIDFromHex(folderID)
	if err != nil {
		return fmt.Errorf("invalid folder ID: %w", err)
	}

	// Soft delete the file
	now := time.Now()
	update := bson.M{
		"$set": bson.M{
			"deleted_at": &now,
			"updated_at": now,
			"is_deleted": true,
		},
	}

	result, err := s.fileCollection.UpdateOne(ctx, bson.M{
		"_id":        fileObjID,
		"folder_id":  folderObjID,
		"deleted_at": nil,
	}, update)

	if err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("file not found in folder")
	}

	return nil
}

// DownloadFolder streams folder contents directly as ZIP to HTTP response - memory efficient
func (s *FolderService) DownloadFolder(ctx context.Context, w http.ResponseWriter, folderID string, userID string) error {
	// Validate folder ID and check permissions
	folderObjID, err := primitive.ObjectIDFromHex(folderID)
	if err != nil {
		return fmt.Errorf("invalid folder ID: %w", err)
	}

	if s.permissionService != nil {
		hasPermission, err := s.permissionService.HasFolderPermission(context.Background(), userID, folderID, "viewer")
		if err != nil {
			return fmt.Errorf("permission check failed: %w", err)
		}
		if !hasPermission {
			return fmt.Errorf("insufficient permissions")
		}
	}

	// Get folder info
	var folder models.Folder
	err = s.folderCollection.FindOne(ctx, bson.M{
		"_id":        folderObjID,
		"is_deleted": false,
	}).Decode(&folder)

	if err == mongo.ErrNoDocuments {
		return fmt.Errorf("folder not found")
	} else if err != nil {
		return fmt.Errorf("database error: %w", err)
	}

	// Set headers for zip download
	zipFileName := fmt.Sprintf("%s_%d.zip", strings.ReplaceAll(folder.Name, " ", "_"), time.Now().Unix())
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", zipFileName))
	w.Header().Set("Cache-Control", "no-cache")

	// Create zip writer that writes directly to HTTP response
	zipWriter := zip.NewWriter(w)
	defer zipWriter.Close()

	// Recursively add folder contents
	return s.AddFolderContentsToZip(ctx, zipWriter, folderObjID, "")
}

// AddFolderContentsToZip recursively adds all files and subfolders to the zip, streaming from B2
func (s *FolderService) AddFolderContentsToZip(ctx context.Context, zipWriter *zip.Writer, folderID primitive.ObjectID, currentPath string) error {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Add all files in current folder
	fileFilter := bson.M{
		"folder_id":  folderID,
		"deleted_at": nil,
	}

	fileCursor, err := s.fileCollection.Find(ctx, fileFilter)
	if err != nil {
		return fmt.Errorf("failed to get files: %w", err)
	}
	defer fileCursor.Close(ctx)

	var files []models.File
	if err = fileCursor.All(ctx, &files); err != nil {
		return fmt.Errorf("failed to decode files: %w", err)
	}

	// Add each file to zip by streaming from B2
	for _, file := range files {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		zipPath := path.Join(currentPath, file.Name)
		zipEntry, err := zipWriter.Create(zipPath)
		if err != nil {
			fmt.Printf("Failed to create zip entry for %s: %v\n", file.Name, err)
			continue
		}

		// Stream file from B2 directly to ZIP
		err = s.downloadB2FileToZip(ctx, file, zipEntry)
		if err != nil {
			fmt.Printf("Failed to download B2 file %s: %v\n", file.Name, err)
			continue
		}
	}

	// Get all subfolders
	folderFilter := bson.M{
		"parent_id":  folderID,
		"is_deleted": false,
	}

	folderCursor, err := s.folderCollection.Find(ctx, folderFilter)
	if err != nil {
		return fmt.Errorf("failed to get subfolders: %w", err)
	}
	defer folderCursor.Close(ctx)

	var subFolders []models.Folder
	if err = folderCursor.All(ctx, &subFolders); err != nil {
		return fmt.Errorf("failed to decode subfolders: %w", err)
	}

	// Recursively add each subfolder
	for _, subFolder := range subFolders {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		subFolderPath := path.Join(currentPath, subFolder.Name)

		// Create folder entry in zip (helps with empty folders)
		_, err = zipWriter.Create(subFolderPath + "/")
		if err != nil {
			fmt.Printf("Warning: failed to create folder entry for %s\n", subFolderPath)
		}

		err = s.AddFolderContentsToZip(ctx, zipWriter, subFolder.ID, subFolderPath)
		if err != nil {
			return fmt.Errorf("failed to process subfolder %s: %w", subFolder.Name, err)
		}
	}

	return nil
}

// downloadB2FileToZip downloads a file from B2 storage and streams it directly to the zip entry
func (s *FolderService) downloadB2FileToZip(ctx context.Context, file models.File, zipEntry io.Writer) error {
	if s.b2Service == nil {
		return fmt.Errorf("B2 service not available")
	}

	// Get download URL from B2
	downloadURL, err := s.b2Service.GetDownloadURL(file.B2FileID, 5*time.Minute)
	if err != nil {
		return fmt.Errorf("failed to generate B2 download URL for file %s: %w", file.Name, err)
	}

	// Create HTTP request with context
	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Use optimized HTTP client
	client := &http.Client{
		Timeout: 10 * time.Minute,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     30 * time.Second,
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download from B2: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("B2 download failed with status: %d", resp.StatusCode)
	}

	// Stream file directly from B2 response to ZIP entry with buffering
	buffer := make([]byte, 32*1024) // 32KB buffer for efficient streaming
	_, err = io.CopyBuffer(zipEntry, resp.Body, buffer)
	if err != nil {
		return fmt.Errorf("failed to copy B2 file to zip: %w", err)
	}

	return nil
}
