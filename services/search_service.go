package services

import (
	"context"
	"fmt"
	"phynixdrive/models"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type SearchService struct {
	fileCollection       *mongo.Collection
	folderCollection     *mongo.Collection
	permissionCollection *mongo.Collection
	permissionService    *PermissionService
}

type SearchResult struct {
	Files   []models.File   `json:"files"`
	Folders []models.Folder `json:"folders"`
}

type SharedItem struct {
	Type     string      `json:"type"` // "file" or "folder"
	Item     interface{} `json:"item"`
	SharedBy string      `json:"sharedBy"`
	Role     string      `json:"role"`
	SharedAt time.Time   `json:"sharedAt"`
}

func NewSearchService(db *mongo.Database, permissionService *PermissionService) *SearchService {
	return &SearchService{
		fileCollection:       db.Collection("files"),
		folderCollection:     db.Collection("folders"),
		permissionCollection: db.Collection("permissions"),
		permissionService:    permissionService,
	}
}

// Search - Fixed method signature to match controller call
func (s *SearchService) Search(userID string, query string, limit int, offset int) (*SearchResult, error) {
	if query == "" {
		return &SearchResult{Files: []models.File{}, Folders: []models.Folder{}}, nil
	}

	ctx := context.Background()
	userObjID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID: %w", err)
	}

	// Create regex search filter (fallback if text index doesn't exist)
	searchRegex := bson.M{"$regex": query, "$options": "i"}

	// Search files
	fileFilter := bson.M{
		"$and": []bson.M{
			{
				"$or": []bson.M{
					{"name": searchRegex},
					{"original_name": searchRegex},
				},
			},
			{"deleted_at": nil},
			{"owner_id": userObjID}, // For now, only search user's own files
		},
	}

	findOptions := options.Find().SetLimit(int64(limit)).SetSkip(int64(offset))
	fileCursor, err := s.fileCollection.Find(ctx, fileFilter, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to search files: %w", err)
	}
	defer fileCursor.Close(ctx)

	var files []models.File
	if err = fileCursor.All(ctx, &files); err != nil {
		return nil, fmt.Errorf("failed to decode files: %w", err)
	}

	// Search folders
	folderFilter := bson.M{
		"$and": []bson.M{
			{"name": searchRegex},
			{"is_deleted": false},
			{"owner_id": userObjID}, // For now, only search user's own folders
		},
	}

	folderCursor, err := s.folderCollection.Find(ctx, folderFilter, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to search folders: %w", err)
	}
	defer folderCursor.Close(ctx)

	var folders []models.Folder
	if err = folderCursor.All(ctx, &folders); err != nil {
		return nil, fmt.Errorf("failed to decode folders: %w", err)
	}

	return &SearchResult{
		Files:   files,
		Folders: folders,
	}, nil
}

// SearchFilesOnly - New method for file-only search
func (s *SearchService) SearchFilesOnly(userID string, query string, limit int, offset int) ([]models.File, error) {
	if query == "" {
		return []models.File{}, nil
	}

	ctx := context.Background()
	userObjID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID: %w", err)
	}

	searchRegex := bson.M{"$regex": query, "$options": "i"}

	fileFilter := bson.M{
		"$and": []bson.M{
			{
				"$or": []bson.M{
					{"name": searchRegex},
					{"original_name": searchRegex},
				},
			},
			{"deleted_at": nil},
			{"owner_id": userObjID},
		},
	}

	findOptions := options.Find().SetLimit(int64(limit)).SetSkip(int64(offset))
	cursor, err := s.fileCollection.Find(ctx, fileFilter, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to search files: %w", err)
	}
	defer cursor.Close(ctx)

	var files []models.File
	if err = cursor.All(ctx, &files); err != nil {
		return nil, fmt.Errorf("failed to decode files: %w", err)
	}

	return files, nil
}

// SearchFoldersOnly - New method for folder-only search
func (s *SearchService) SearchFoldersOnly(userID string, query string, limit int, offset int) ([]models.Folder, error) {
	if query == "" {
		return []models.Folder{}, nil
	}

	ctx := context.Background()
	userObjID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID: %w", err)
	}

	searchRegex := bson.M{"$regex": query, "$options": "i"}

	folderFilter := bson.M{
		"$and": []bson.M{
			{"name": searchRegex},
			{"is_deleted": false},
			{"owner_id": userObjID},
		},
	}

	findOptions := options.Find().SetLimit(int64(limit)).SetSkip(int64(offset))
	cursor, err := s.folderCollection.Find(ctx, folderFilter, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to search folders: %w", err)
	}
	defer cursor.Close(ctx)

	var folders []models.Folder
	if err = cursor.All(ctx, &folders); err != nil {
		return nil, fmt.Errorf("failed to decode folders: %w", err)
	}

	return folders, nil
}

// GetRecentFiles - New method for recent files
func (s *SearchService) GetRecentFiles(userID string, limit int, days int) ([]models.File, error) {
	ctx := context.Background()
	userObjID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID: %w", err)
	}

	// Calculate date threshold
	dateThreshold := time.Now().AddDate(0, 0, -days)

	filter := bson.M{
		"owner_id":   userObjID,
		"deleted_at": nil,
		"$or": []bson.M{
			{"updated_at": bson.M{"$gte": dateThreshold}},
			{"created_at": bson.M{"$gte": dateThreshold}},
		},
	}

	findOptions := options.Find().
		SetLimit(int64(limit)).
		SetSort(bson.D{
			{Key: "updated_at", Value: -1},
			{Key: "created_at", Value: -1},
		})

	cursor, err := s.fileCollection.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent files: %w", err)
	}
	defer cursor.Close(ctx)

	var files []models.File
	if err = cursor.All(ctx, &files); err != nil {
		return nil, fmt.Errorf("failed to decode files: %w", err)
	}

	return files, nil
}

// GetSharedWithMe - New method for shared items
func (s *SearchService) GetSharedWithMe(userID string, itemType string, limit int, offset int) ([]SharedItem, error) {
	ctx := context.Background()

	// Get permissions where user is granted access
	filter := bson.M{
		"user_id": userID,
	}

	if itemType != "all" {
		switch itemType {
		case "files":
			filter["resource_type"] = "file"
		case "folders":
			filter["resource_type"] = "folder"
		}
	}

	findOptions := options.Find().SetLimit(int64(limit)).SetSkip(int64(offset))
	cursor, err := s.permissionCollection.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to get shared permissions: %w", err)
	}
	defer cursor.Close(ctx)

	var permissions []models.Permission
	if err = cursor.All(ctx, &permissions); err != nil {
		return nil, fmt.Errorf("failed to decode permissions: %w", err)
	}

	var sharedItems []SharedItem
	for _, perm := range permissions {
		var item interface{}
		var itemType string

		if perm.ResourceType == "file" {
			// Get file details
			fileObjID, err := primitive.ObjectIDFromHex(perm.ResourceID)
			if err != nil {
				continue
			}

			var file models.File
			err = s.fileCollection.FindOne(ctx, bson.M{
				"_id":        fileObjID,
				"deleted_at": nil,
			}).Decode(&file)
			if err == nil {
				item = file
				itemType = "file"
			}
		} else if perm.ResourceType == "folder" {
			// Get folder details
			folderObjID, err := primitive.ObjectIDFromHex(perm.ResourceID)
			if err != nil {
				continue
			}

			var folder models.Folder
			err = s.folderCollection.FindOne(ctx, bson.M{
				"_id":        folderObjID,
				"is_deleted": false,
			}).Decode(&folder)
			if err == nil {
				item = folder
				itemType = "folder"
			}
		}

		if item != nil {
			sharedItems = append(sharedItems, SharedItem{
				Type:     itemType,
				Item:     item,
				SharedBy: perm.GrantedBy,
				Role:     perm.Role,
				SharedAt: perm.GrantedAt,
			})
		}
	}
	return sharedItems, nil
}

// CreateSearchIndexes - Enhanced version
func (s *SearchService) CreateSearchIndexes() error {
	ctx := context.Background()

	// Create text index for files
	fileIndexModel := mongo.IndexModel{
		Keys: bson.D{
			{Key: "name", Value: "text"},
			{Key: "original_name", Value: "text"},
		},
		Options: options.Index().SetName("file_search_index"),
	}

	_, err := s.fileCollection.Indexes().CreateOne(ctx, fileIndexModel)
	if err != nil {
		return fmt.Errorf("failed to create file search index: %w", err)
	}

	// Create text index for folders
	folderIndexModel := mongo.IndexModel{
		Keys: bson.D{
			{Key: "name", Value: "text"},
		},
		Options: options.Index().SetName("folder_search_index"),
	}

	_, err = s.folderCollection.Indexes().CreateOne(ctx, folderIndexModel)
	if err != nil {
		return fmt.Errorf("failed to create folder search index: %w", err)
	}

	// Create indexes for better performance
	additionalIndexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "owner_id", Value: 1}, {Key: "deleted_at", Value: 1}},
			Options: options.Index().SetName("owner_deleted_index"),
		},
		{
			Keys:    bson.D{{Key: "updated_at", Value: -1}},
			Options: options.Index().SetName("updated_at_desc_index"),
		},
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "resource_type", Value: 1}},
			Options: options.Index().SetName("permission_lookup_index"),
		},
	}

	for _, indexModel := range additionalIndexes {
		_, err := s.fileCollection.Indexes().CreateOne(ctx, indexModel)
		if err != nil {
			// Continue if index already exists
			continue
		}
	}

	return nil
}
