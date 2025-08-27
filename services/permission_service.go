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
	userCollection       *mongo.Collection // Added for user validation
}

func NewPermissionService(db *mongo.Database) *PermissionService {
	return &PermissionService{
		fileCollection:       db.Collection("files"),
		folderCollection:     db.Collection("folders"),
		permissionCollection: db.Collection("permissions"),
		userCollection:       db.Collection("users"),
	}
}

// Enhanced permission check that determines resource type automatically
func (s *PermissionService) HasResourcePermission(ctx context.Context, userID, resourceID, requiredRole string) (bool, error) {
	objID, err := primitive.ObjectIDFromHex(resourceID)
	if err != nil {
		return false, fmt.Errorf("invalid resource ID: %w", err)
	}

	// Try to find as file first
	var file models.File
	err = s.fileCollection.FindOne(ctx, bson.M{
		"_id":        objID,
		"deleted_at": nil,
	}).Decode(&file)

	if err == nil {
		// It's a file
		return s.hasFilePermissionInternal(ctx, userID, resourceID, requiredRole, &file)
	} else if err != mongo.ErrNoDocuments {
		return false, fmt.Errorf("error checking file: %w", err)
	}

	// Try to find as folder
	var folder models.Folder
	err = s.folderCollection.FindOne(ctx, bson.M{
		"_id":        objID,
		"deleted_at": nil,
	}).Decode(&folder)

	if err == nil {
		// It's a folder
		return s.hasFolderPermissionInternal(ctx, userID, resourceID, requiredRole, &folder)
	} else if err == mongo.ErrNoDocuments {
		return false, fmt.Errorf("resource not found")
	}

	return false, fmt.Errorf("error checking folder: %w", err)
}

func (s *PermissionService) HasFilePermission(ctx context.Context, userID, fileID, requiredRole string) (bool, error) {
	objID, err := primitive.ObjectIDFromHex(fileID)
	if err != nil {
		return false, fmt.Errorf("invalid file ID: %w", err)
	}

	var file models.File
	err = s.fileCollection.FindOne(ctx, bson.M{
		"_id":        objID,
		"deleted_at": nil,
	}).Decode(&file)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return false, fmt.Errorf("file not found")
		}
		return false, fmt.Errorf("error fetching file: %w", err)
	}

	return s.hasFilePermissionInternal(ctx, userID, fileID, requiredRole, &file)
}

func (s *PermissionService) hasFilePermissionInternal(ctx context.Context, userID, fileID, requiredRole string, file *models.File) (bool, error) {
	// Owner always has access
	if file.OwnerID.Hex() == userID {
		return true, nil
	}

	// If file is in a folder, check inherited permissions
	if file.FolderID != nil {
		return s.HasFolderPermission(ctx, userID, file.FolderID.Hex(), requiredRole)
	}

	// Check direct permission on file
	return s.checkDirectPermission(ctx, userID, fileID, "file", requiredRole)
}

func (s *PermissionService) HasFolderPermission(ctx context.Context, userID, folderID, requiredRole string) (bool, error) {
	objID, err := primitive.ObjectIDFromHex(folderID)
	if err != nil {
		return false, fmt.Errorf("invalid folder ID: %w", err)
	}

	var folder models.Folder
	err = s.folderCollection.FindOne(ctx, bson.M{
		"_id":        objID,
		"deleted_at": nil,
	}).Decode(&folder)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return false, fmt.Errorf("folder not found")
		}
		return false, fmt.Errorf("error fetching folder: %w", err)
	}

	return s.hasFolderPermissionInternal(ctx, userID, folderID, requiredRole, &folder)
}

func (s *PermissionService) hasFolderPermissionInternal(ctx context.Context, userID, folderID, requiredRole string, folder *models.Folder) (bool, error) {
	// Owner always has full access
	if folder.OwnerID.Hex() == userID {
		return true, nil
	}

	// Check direct permission on folder
	hasPerm, err := s.checkDirectPermission(ctx, userID, folderID, "folder", requiredRole)
	if err != nil {
		return false, err
	}
	if hasPerm {
		return true, nil
	}

	// Check inherited permissions from parent folders
	if folder.ParentID != nil {
		return s.HasFolderPermission(ctx, userID, folder.ParentID.Hex(), requiredRole)
	}

	return false, nil
}

func (s *PermissionService) checkDirectPermission(ctx context.Context, userID, resourceID, resourceType, requiredRole string) (bool, error) {
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

// Enhanced ShareFolder with proper validation
func (s *PermissionService) ShareFolder(ctx context.Context, folderID, sharedWithUserID, role, sharedByUserID string) error {
	// Validate role
	validRoles := map[string]bool{
		"viewer": true,
		"editor": true,
		"admin":  true,
	}
	if !validRoles[role] {
		return fmt.Errorf("invalid role: %s", role)
	}

	// Validate that the user being shared with exists
	userObjID, err := primitive.ObjectIDFromHex(sharedWithUserID)
	if err != nil {
		return fmt.Errorf("invalid user ID: %w", err)
	}

	var user models.User
	err = s.userCollection.FindOne(ctx, bson.M{"_id": userObjID}).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return fmt.Errorf("user not found")
		}
		return fmt.Errorf("error validating user: %w", err)
	}

	// Validate that the folder exists and user has admin permission
	hasPermission, err := s.HasFolderPermission(ctx, sharedByUserID, folderID, "admin")
	if err != nil {
		return fmt.Errorf("permission check failed: %w", err)
	}
	if !hasPermission {
		return fmt.Errorf("insufficient permissions to share folder")
	}

	// Don't allow sharing with yourself (optional business logic)
	if sharedWithUserID == sharedByUserID {
		return fmt.Errorf("cannot share with yourself")
	}

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

func (s *PermissionService) GetFolderPermissions(ctx context.Context, folderID, userID string) ([]models.Permission, error) {
	hasPermission, err := s.HasFolderPermission(ctx, userID, folderID, "admin")
	if err != nil {
		return nil, fmt.Errorf("permission check failed: %w", err)
	}
	if !hasPermission {
		return nil, fmt.Errorf("insufficient permissions")
	}

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

// New method to revoke permissions
func (s *PermissionService) RevokePermission(ctx context.Context, folderID, userID, revokedByUserID string) error {
	// Only admin can revoke permissions
	hasPermission, err := s.HasFolderPermission(ctx, revokedByUserID, folderID, "admin")
	if err != nil {
		return fmt.Errorf("permission check failed: %w", err)
	}
	if !hasPermission {
		return fmt.Errorf("insufficient permissions to revoke access")
	}

	// Cannot revoke owner's permissions
	folderObjID, err := primitive.ObjectIDFromHex(folderID)
	if err != nil {
		return fmt.Errorf("invalid folder ID: %w", err)
	}

	var folder models.Folder
	err = s.folderCollection.FindOne(ctx, bson.M{
		"_id":        folderObjID,
		"deleted_at": nil,
	}).Decode(&folder)
	if err != nil {
		return fmt.Errorf("folder not found: %w", err)
	}

	if folder.OwnerID.Hex() == userID {
		return fmt.Errorf("cannot revoke owner's permissions")
	}

	_, err = s.permissionCollection.DeleteOne(ctx, bson.M{
		"user_id":       userID,
		"resource_id":   folderID,
		"resource_type": "folder",
	})
	if err != nil {
		return fmt.Errorf("failed to revoke permission: %w", err)
	}

	return nil
}
