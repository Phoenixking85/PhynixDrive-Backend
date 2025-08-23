package services

import (
	"context"
	"fmt"
	"phynixdrive/models"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type PermissionService struct {
	fileCollection       *mongo.Collection
	folderCollection     *mongo.Collection
	permissionCollection *mongo.Collection
}

func NewPermissionService(db *mongo.Database) *PermissionService {
	return &PermissionService{
		fileCollection:       db.Collection("files"),
		folderCollection:     db.Collection("folders"),
		permissionCollection: db.Collection("permissions"),
	}
}

func (s *PermissionService) HasFilePermission(userID, fileID, requiredRole string) (bool, error) {
	objID, err := primitive.ObjectIDFromHex(fileID)
	if err != nil {
		return false, fmt.Errorf("invalid file ID: %w", err)
	}

	ctx := context.Background()
	var file models.File

	err = s.fileCollection.FindOne(ctx, bson.M{
		"_id":        objID,
		"deleted_at": nil,
	}).Decode(&file)
	if err != nil {
		return false, fmt.Errorf("file not found: %w", err)
	}

	// Owner always has access
	if file.OwnerID.Hex() == userID {
		return true, nil
	}

	// If file is in a folder, check inherited permissions
	if file.FolderID != nil {
		return s.HasFolderPermission(userID, file.FolderID.Hex(), requiredRole)
	}

	// Check direct permission on file
	return s.checkDirectPermission(userID, fileID, "file", requiredRole)
}

func (s *PermissionService) HasFolderPermission(userID, folderID, requiredRole string) (bool, error) {
	objID, err := primitive.ObjectIDFromHex(folderID)
	if err != nil {
		return false, fmt.Errorf("invalid folder ID: %w", err)
	}

	ctx := context.Background()
	var folder models.Folder

	err = s.folderCollection.FindOne(ctx, bson.M{
		"_id":        objID,
		"deleted_at": nil,
	}).Decode(&folder)
	if err != nil {
		return false, fmt.Errorf("folder not found: %w", err)
	}

	// Owner always has full access
	if folder.OwnerID.Hex() == userID {
		return true, nil
	}

	// Check direct permission on folder
	hasPerm, err := s.checkDirectPermission(userID, folderID, "folder", requiredRole)
	if err != nil {
		return false, err
	}
	if hasPerm {
		return true, nil
	}

	// Check inherited permissions from parent folders
	if folder.ParentID != nil {
		return s.HasFolderPermission(userID, folder.ParentID.Hex(), requiredRole)
	}

	return false, nil
}

func (s *PermissionService) checkDirectPermission(userID, resourceID, resourceType, requiredRole string) (bool, error) {
	ctx := context.Background()

	var permission models.Permission
	err := s.permissionCollection.FindOne(ctx, bson.M{
		"user_id":       userID,
		"resource_id":   resourceID,
		"resource_type": resourceType,
	}).Decode(&permission)

	if err == mongo.ErrNoDocuments {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("permission check failed: %w", err)
	}

	return s.hasRequiredRole(permission.Role, requiredRole), nil
}

func (s *PermissionService) hasRequiredRole(userRole, requiredRole string) bool {
	roleHierarchy := map[string]int{
		"viewer": 1,
		"editor": 2,
		"admin":  3,
	}
	userLevel, ok1 := roleHierarchy[userRole]
	requiredLevel, ok2 := roleHierarchy[requiredRole]

	return ok1 && ok2 && userLevel >= requiredLevel
}

func (s *PermissionService) ShareFolder(folderID, sharedWithUserID, role, sharedByUserID string) error {
	// Only admin can share
	hasPermission, err := s.HasFolderPermission(sharedByUserID, folderID, "admin")
	if err != nil {
		return fmt.Errorf("permission check failed: %w", err)
	}
	if !hasPermission {
		return fmt.Errorf("insufficient permissions to share folder")
	}

	ctx := context.Background()
	// Check if permission already exists
	var existing models.Permission
	err = s.permissionCollection.FindOne(ctx, bson.M{
		"user_id":       sharedWithUserID,
		"resource_id":   folderID,
		"resource_type": "folder",
	}).Decode(&existing)

	if err == mongo.ErrNoDocuments {
		// Create new permission
		permission := models.Permission{
			ID:           primitive.NewObjectID(),
			UserID:       sharedWithUserID,
			Role:         role,
			ResourceID:   folderID,
			ResourceType: "folder",
			GrantedBy:    sharedByUserID,
			GrantedAt:    time.Now(),
		}

		_, err = s.permissionCollection.InsertOne(ctx, permission)
		if err != nil {
			return fmt.Errorf("failed to create permission: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to fetch existing permission: %w", err)
	} else {
		// Update existing permission
		_, err = s.permissionCollection.UpdateOne(ctx, bson.M{
			"_id": existing.ID,
		}, bson.M{
			"$set": bson.M{
				"role":       role,
				"granted_by": sharedByUserID,
				"granted_at": time.Now(),
			},
		})
		if err != nil {
			return fmt.Errorf("failed to update permission: %w", err)
		}
	}

	return nil
}

func (s *PermissionService) GetFolderPermissions(folderID, userID string) ([]models.Permission, error) {
	hasPermission, err := s.HasFolderPermission(userID, folderID, "admin")
	if err != nil {
		return nil, fmt.Errorf("permission check failed: %w", err)
	}
	if !hasPermission {
		return nil, fmt.Errorf("insufficient permissions")
	}

	ctx := context.Background()
	cursor, err := s.permissionCollection.Find(ctx, bson.M{
		"resource_id":   folderID,
		"resource_type": "folder",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch permissions: %w", err)
	}
	defer cursor.Close(ctx)

	var permissions []models.Permission
	if err = cursor.All(ctx, &permissions); err != nil {
		return nil, fmt.Errorf("failed to decode permissions: %w", err)
	}

	return permissions, nil
}
