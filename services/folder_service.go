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

// FileInfo represents file information with preview/download endpoints
type FileInfo struct {
	ID               primitive.ObjectID `json:"id"`
	Name             string             `json:"name"`
	Type             string             `json:"type"` // Always "file"
	MimeType         string             `json:"mime_type"`
	Size             int64              `json:"size"`
	CreatedAt        time.Time          `json:"created_at"`
	PreviewEndpoint  string             `json:"preview_endpoint"`
	DownloadEndpoint string             `json:"download_endpoint"`
}

// FolderContentsResponse represents the response for GetFolderContents
type FolderContentsResponse struct {
	Folder     FolderInfo      `json:"folder"`
	Subfolders []SubfolderInfo `json:"subfolders"`
	Files      []FileInfo      `json:"files"` // Changed from []models.File to []FileInfo
	Counts     ContentCounts   `json:"counts"`
}
type FolderSummary struct {
	ID             primitive.ObjectID `json:"id"`
	Name           string             `json:"name"`
	Type           string             `json:"type"` // "folder"
	CreatedAt      time.Time          `json:"created_at"`
	FileCount      int                `json:"file_count"`
	SubfolderCount int                `json:"subfolder_count"`
}
type FolderInfo struct {
	ID       primitive.ObjectID `json:"id"`
	Name     string             `json:"name"`
	Type     string             `json:"type"` // Always "folder"
	Path     string             `json:"path"`
	CanEdit  bool               `json:"can_edit"`
	CanShare bool               `json:"can_share"`
}

type SubfolderInfo struct {
	ID        primitive.ObjectID `json:"id"`
	Name      string             `json:"name"`
	Type      string             `json:"type"` // Always "folder"
	Path      string             `json:"path"`
	FileCount int                `json:"file_count"`
	CreatedAt time.Time          `json:"created_at"`
}

type ContentCounts struct {
	Subfolders int `json:"subfolders"`
	Files      int `json:"files"`
}

type FolderService struct {
	folderCollection  *mongo.Collection
	fileCollection    *mongo.Collection
	userCollection    *mongo.Collection
	permissionService *PermissionService
	b2Service         *B2Service
	httpClient        *http.Client
}

func NewFolderService(db *mongo.Database, permissionService *PermissionService, b2Service *B2Service) *FolderService {
	return &FolderService{
		folderCollection:  db.Collection("folders"),
		fileCollection:    db.Collection("files"),
		userCollection:    db.Collection("users"),
		permissionService: permissionService,
		b2Service:         b2Service,
		httpClient:        &http.Client{Timeout: 30 * time.Second},
	}
}

// GetFolderContents retrieves folder contents in Google Drive style (subfolders + files in single view)
func (s *FolderService) GetFolderContents(folderID, userID string) (*FolderContentsResponse, error) {
	ctx := context.Background()

	// Validate folder ID
	folderObjID, err := primitive.ObjectIDFromHex(folderID)
	if err != nil {
		return nil, fmt.Errorf("invalid folder ID: %w", err)
	}

	// Check permissions - viewer level minimum
	if s.permissionService != nil {
		hasPermission, err := s.permissionService.HasFolderPermission(ctx, userID, folderID, "viewer")
		if err != nil {
			return nil, fmt.Errorf("permission check failed: %w", err)
		}
		if !hasPermission {
			return nil, fmt.Errorf("insufficient permissions")
		}
	}

	// Get folder metadata
	var folder models.Folder
	err = s.folderCollection.FindOne(ctx, bson.M{
		"_id":        folderObjID,
		"is_deleted": false,
	}).Decode(&folder)

	if err == mongo.ErrNoDocuments {
		return nil, fmt.Errorf("folder not found")
	} else if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}

	// Determine user permissions for this folder
	canEdit := false
	canShare := false
	if s.permissionService != nil {
		canEdit, _ = s.permissionService.HasFolderPermission(ctx, userID, folderID, "editor")
		canShare, _ = s.permissionService.HasFolderPermission(ctx, userID, folderID, "admin")
	}

	// Get direct child subfolders
	subfolders, err := s.getSubfoldersWithCounts(ctx, folderObjID)
	if err != nil {
		return nil, fmt.Errorf("failed to get subfolders: %w", err)
	}

	// Get files in folder with preview/download endpoints
	files, err := s.getFilesWithEndpoints(ctx, folderObjID)
	if err != nil {
		return nil, fmt.Errorf("failed to get files: %w", err)
	}

	// Build response
	response := &FolderContentsResponse{
		Folder: FolderInfo{
			ID:       folder.ID,
			Name:     folder.Name,
			Type:     "folder",
			Path:     folder.Path,
			CanEdit:  canEdit,
			CanShare: canShare,
		},
		Subfolders: subfolders,
		Files:      files,
		Counts: ContentCounts{
			Subfolders: len(subfolders),
			Files:      len(files),
		},
	}

	return response, nil
}

// getSubfoldersWithCounts gets direct child subfolders with file counts
func (s *FolderService) getSubfoldersWithCounts(ctx context.Context, parentID primitive.ObjectID) ([]SubfolderInfo, error) {
	// Get subfolders
	cursor, err := s.folderCollection.Find(ctx, bson.M{
		"parent_id":  parentID,
		"is_deleted": false,
	}, options.Find().SetSort(bson.M{"name": 1}))

	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var subfolders []SubfolderInfo
	for cursor.Next(ctx) {
		var folder models.Folder
		if err := cursor.Decode(&folder); err != nil {
			continue
		}

		// Count files in this subfolder
		fileCount, err := s.fileCollection.CountDocuments(ctx, bson.M{
			"folder_id":  folder.ID,
			"deleted_at": nil,
		})
		if err != nil {
			fileCount = 0 // Continue with 0 count on error
		}

		subfolders = append(subfolders, SubfolderInfo{
			ID:        folder.ID,
			Name:      folder.Name,
			Type:      "folder",
			Path:      folder.Path,
			FileCount: int(fileCount),
			CreatedAt: folder.CreatedAt,
		})
	}

	return subfolders, nil
}

// getFilesWithEndpoints gets files in folder with preview/download endpoints (not permanent URLs)
func (s *FolderService) getFilesWithEndpoints(ctx context.Context, folderID primitive.ObjectID) ([]FileInfo, error) {
	cursor, err := s.fileCollection.Find(ctx, bson.M{
		"folder_id":  folderID,
		"deleted_at": nil,
	}, options.Find().SetSort(bson.M{"name": 1}))

	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var files []FileInfo
	for cursor.Next(ctx) {
		var file models.File
		if err := cursor.Decode(&file); err != nil {
			continue
		}

		// Convert models.File to FileInfo with endpoints
		fileInfo := FileInfo{
			ID:               file.ID,
			Name:             file.Name,
			Type:             "file",
			MimeType:         file.MimeType,
			Size:             file.Size,
			CreatedAt:        file.CreatedAt,
			PreviewEndpoint:  fmt.Sprintf("/api/files/%s/preview", file.ID.Hex()),
			DownloadEndpoint: fmt.Sprintf("/api/files/%s/download", file.ID.Hex()),
		}

		files = append(files, fileInfo)
	}

	return files, nil
}
func (s *FolderService) ListRootFoldersWithCounts(userID string) ([]FolderSummary, error) {
	ctx := context.Background()

	ownerObjID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID: %w", err)
	}

	// find top-level folders (parent_id == nil)
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

	var results []FolderSummary

	for cursor.Next(ctx) {
		var folder models.Folder
		if err := cursor.Decode(&folder); err != nil {
			continue
		}

		// Count files (use same deleted semantics as elsewhere)
		fileCount, err := s.fileCollection.CountDocuments(ctx, bson.M{
			"folder_id":  folder.ID,
			"deleted_at": nil,
		})
		if err != nil {
			// on error, fall back to zero but log
			fileCount = 0
		}

		// Count direct subfolders
		subfolderCount, err := s.folderCollection.CountDocuments(ctx, bson.M{
			"parent_id":  folder.ID,
			"is_deleted": false,
		})
		if err != nil {
			subfolderCount = 0
		}

		results = append(results, FolderSummary{
			ID:             folder.ID,
			Name:           folder.Name,
			Type:           "folder",
			CreatedAt:      folder.CreatedAt,
			FileCount:      int(fileCount),
			SubfolderCount: int(subfolderCount),
		})
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %w", err)
	}

	return results, nil
}

// CreateFolder creates a new folder
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

func (s *FolderService) DeleteFolder(ctx context.Context, folderID string, userID string) error {
	objID, err := primitive.ObjectIDFromHex(folderID)
	if err != nil {
		return fmt.Errorf("invalid folder ID: %w", err)
	}

	// --- Permission check ---
	if s.permissionService != nil {
		hasPermission, err := s.permissionService.HasFolderPermission(ctx, userID, folderID, "admin")
		if err != nil {
			return fmt.Errorf("permission check failed: %w", err)
		}
		if !hasPermission {
			return fmt.Errorf("insufficient permissions")
		}
	}

	// --- Check if folder exists and is not already deleted ---
	var folder models.Folder
	err = s.folderCollection.FindOne(ctx, bson.M{
		"_id":        objID,
		"is_deleted": false,
	}).Decode(&folder)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return fmt.Errorf("folder not found or already deleted")
		}
		return fmt.Errorf("failed to find folder: %w", err)
	}

	now := time.Now()

	// --- Use transaction for atomicity ---
	callback := func(sessCtx mongo.SessionContext) (interface{}, error) {
		// Mark the main folder as deleted
		update := bson.M{
			"$set": bson.M{
				"is_deleted": true,
				"deleted_at": now,
				"updated_at": now,
			},
		}

		result, err := s.folderCollection.UpdateOne(sessCtx, bson.M{
			"_id":        objID,
			"is_deleted": false,
		}, update)
		if err != nil {
			return nil, fmt.Errorf("failed to delete folder: %w", err)
		}
		if result.MatchedCount == 0 {
			return nil, fmt.Errorf("folder not found or already deleted")
		}

		// Cascade soft-delete subfolders recursively
		if err := s.softDeleteSubfolders(sessCtx, objID, now); err != nil {
			return nil, fmt.Errorf("failed to delete subfolders: %w", err)
		}

		// Soft-delete all files in this folder and subfolders
		if err := s.softDeleteFiles(sessCtx, objID, now); err != nil {
			return nil, fmt.Errorf("failed to delete files: %w", err)
		}

		return nil, nil
	}

	session, err := s.folderCollection.Database().Client().StartSession()
	if err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}
	defer session.EndSession(ctx)

	_, err = session.WithTransaction(ctx, callback)
	if err != nil {
		return err
	}

	return nil
}

// Recursively soft-delete subfolders
func (s *FolderService) softDeleteSubfolders(ctx context.Context, parentID primitive.ObjectID, now time.Time) error {
	// Use bulk operations for better performance
	var bulkOps []mongo.WriteModel

	cursor, err := s.folderCollection.Find(ctx, bson.M{
		"parent_id":  parentID,
		"is_deleted": false,
	})
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)

	var subfolderIDs []primitive.ObjectID
	for cursor.Next(ctx) {
		var subFolder models.Folder
		if err := cursor.Decode(&subFolder); err != nil {
			return err
		}

		subfolderIDs = append(subfolderIDs, subFolder.ID)

		// Prepare bulk update operation
		updateModel := mongo.NewUpdateOneModel().
			SetFilter(bson.M{"_id": subFolder.ID}).
			SetUpdate(bson.M{"$set": bson.M{
				"is_deleted": true,
				"deleted_at": now,
				"updated_at": now,
			}})
		bulkOps = append(bulkOps, updateModel)
	}

	if err := cursor.Err(); err != nil {
		return err
	}

	// Execute bulk operations
	if len(bulkOps) > 0 {
		_, err := s.folderCollection.BulkWrite(ctx, bulkOps)
		if err != nil {
			return err
		}

		// Recursively process subfolders
		for _, subfolderID := range subfolderIDs {
			if err := s.softDeleteSubfolders(ctx, subfolderID, now); err != nil {
				return err
			}
			if err := s.softDeleteFiles(ctx, subfolderID, now); err != nil {
				return err
			}
		}
	}

	return nil
}

// Soft-delete all files inside a folder
func (s *FolderService) softDeleteFiles(ctx context.Context, folderID primitive.ObjectID, now time.Time) error {
	_, err := s.fileCollection.UpdateMany(ctx, bson.M{
		"folder_id":  folderID,
		"is_deleted": false,
	}, bson.M{
		"$set": bson.M{
			"is_deleted": true,
			"deleted_at": now,
			"updated_at": now,
		},
	})
	return err
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
