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
	userCollection       *mongo.Collection
}

func NewPermissionService(db *mongo.Database) *PermissionService {
	return &PermissionService{
		fileCollection:       db.Collection("files"),
		folderCollection:     db.Collection("folders"),
		permissionCollection: db.Collection("permissions"),
		userCollection:       db.Collection("users"),
	}
}

// Public API used by ShareService and other services

// HasResourcePermission infers resource type and checks permission for requiredRole
func (s *PermissionService) HasResourcePermission(ctx context.Context, userID, resourceID, requiredRole string) (bool, error) {
	// Try file
	ok, err := s.HasFilePermission(ctx, userID, resourceID, requiredRole)
	if err == nil {
		return ok, nil
	}

	if err.Error() == "file not found" {
		return s.HasFolderPermission(ctx, userID, resourceID, requiredRole)
	}

	return s.HasFolderPermission(ctx, userID, resourceID, requiredRole)
}

// HasFilePermission checks permission on a file (owner, inherited from folder, direct)
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

	// Owner always has full access
	if file.OwnerID.Hex() == userID {
		return true, nil
	}

	// If file is inside a folder, check folder permissions (inheritance)
	if file.FolderID != nil {
		return s.HasFolderPermission(ctx, userID, file.FolderID.Hex(), requiredRole)
	}

	// Check direct permissions on file
	return s.checkDirectPermission(ctx, userID, fileID, "file", requiredRole)
}

// HasFolderPermission checks permission on a folder (owner, direct, inherited from parent)
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

	// Owner always has full access
	if folder.OwnerID.Hex() == userID {
		return true, nil
	}

	// Direct permission on this folder
	ok, err := s.checkDirectPermission(ctx, userID, folderID, "folder", requiredRole)
	if err != nil {
		return false, err
	}
	if ok {
		return true, nil
	}

	// Inherit from parent chain
	if folder.ParentID != nil {
		return s.HasFolderPermission(ctx, userID, folder.ParentID.Hex(), requiredRole)
	}

	return false, nil
}

// ShareFolder grants a permission for a folder to a user (create or update permission doc)
func (s *PermissionService) ShareFolder(ctx context.Context, folderID, sharedWithUserID, role, sharedByUserID string) error {
	// Validate role
	if !isValidRole(role) {
		return fmt.Errorf("invalid role: %s", role)
	}

	// Validate user exists
	sharedWithObjID, err := primitive.ObjectIDFromHex(sharedWithUserID)
	if err != nil {
		return fmt.Errorf("invalid user id: %w", err)
	}
	var u models.User
	if err := s.userCollection.FindOne(ctx, bson.M{"_id": sharedWithObjID}).Decode(&u); err != nil {
		if err == mongo.ErrNoDocuments {
			return fmt.Errorf("user not found")
		}
		return fmt.Errorf("error fetching user: %w", err)
	}

	// Validate folder exists and sharedByUserID has admin on folder
	hasPerm, err := s.HasFolderPermission(ctx, sharedByUserID, folderID, "admin")
	if err != nil {
		return fmt.Errorf("permission check failed: %w", err)
	}
	if !hasPerm {
		return fmt.Errorf("insufficient permissions to share folder")
	}

	// Prevent sharing with self (business rule)
	if sharedWithUserID == sharedByUserID {
		return fmt.Errorf("cannot share with yourself")
	}

	// Insert or update permission doc
	var existing models.Permission
	err = s.permissionCollection.FindOne(ctx, bson.M{
		"user_id":       sharedWithUserID,
		"resource_id":   folderID,
		"resource_type": "folder",
	}).Decode(&existing)

	now := time.Now()
	if err == mongo.ErrNoDocuments {
		perm := models.Permission{
			ID:           primitive.NewObjectID(),
			UserID:       sharedWithUserID,
			Role:         role,
			ResourceID:   folderID,
			ResourceType: "folder",
			GrantedBy:    sharedByUserID,
			GrantedAt:    now,
			IsActive:     true,
		}
		if _, insErr := s.permissionCollection.InsertOne(ctx, perm); insErr != nil {
			return fmt.Errorf("failed to create permission: %w", insErr)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to check existing permission: %w", err)
	}

	// Update existing permission
	_, updErr := s.permissionCollection.UpdateOne(ctx, bson.M{"_id": existing.ID}, bson.M{
		"$set": bson.M{
			"role":       role,
			"granted_by": sharedByUserID,
			"granted_at": now,
			"is_active":  true,
			"updated_at": now,
			"updated_by": sharedByUserID,
		},
	})
	if updErr != nil {
		return fmt.Errorf("failed to update permission: %w", updErr)
	}

	return nil
}

// ShareFile grants permission for a file to a user (create or update permission doc)
func (s *PermissionService) ShareFile(ctx context.Context, fileID, sharedWithUserID, role, sharedByUserID string) error {
	// Validate role
	if !isValidRole(role) {
		return fmt.Errorf("invalid role: %s", role)
	}

	// Validate user exists
	sharedWithObjID, err := primitive.ObjectIDFromHex(sharedWithUserID)
	if err != nil {
		return fmt.Errorf("invalid user id: %w", err)
	}
	var u models.User
	if err := s.userCollection.FindOne(ctx, bson.M{"_id": sharedWithObjID}).Decode(&u); err != nil {
		if err == mongo.ErrNoDocuments {
			return fmt.Errorf("user not found")
		}
		return fmt.Errorf("error fetching user: %w", err)
	}

	// Validate file exists and sharedByUserID has admin on file (or on parent folder)
	hasPerm, err := s.HasFilePermission(ctx, sharedByUserID, fileID, "admin")
	if err != nil {
		return fmt.Errorf("permission check failed: %w", err)
	}
	if !hasPerm {
		return fmt.Errorf("insufficient permissions to share file")
	}

	// Prevent sharing with self
	if sharedWithUserID == sharedByUserID {
		return fmt.Errorf("cannot share with yourself")
	}

	// Insert or update permission doc
	var existing models.Permission
	err = s.permissionCollection.FindOne(ctx, bson.M{
		"user_id":       sharedWithUserID,
		"resource_id":   fileID,
		"resource_type": "file",
	}).Decode(&existing)

	now := time.Now()
	if err == mongo.ErrNoDocuments {
		perm := models.Permission{
			ID:           primitive.NewObjectID(),
			UserID:       sharedWithUserID,
			Role:         role,
			ResourceID:   fileID,
			ResourceType: "file",
			GrantedBy:    sharedByUserID,
			GrantedAt:    now,
			IsActive:     true,
		}
		if _, insErr := s.permissionCollection.InsertOne(ctx, perm); insErr != nil {
			return fmt.Errorf("failed to create permission: %w", insErr)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("failed to check existing permission: %w", err)
	}

	// Update existing permission
	_, updErr := s.permissionCollection.UpdateOne(ctx, bson.M{"_id": existing.ID}, bson.M{
		"$set": bson.M{
			"role":       role,
			"granted_by": sharedByUserID,
			"granted_at": now,
			"is_active":  true,
			"updated_at": now,
			"updated_by": sharedByUserID,
		},
	})
	if updErr != nil {
		return fmt.Errorf("failed to update permission: %w", updErr)
	}

	return nil
}

// RevokeFolderPermission revokes a user's permission on a folder (only admin can revoke)
func (s *PermissionService) RevokeFolderPermission(ctx context.Context, folderID, targetUserID, revokedByUserID string) error {
	// Validate revokedBy has admin on the folder
	hasPerm, err := s.HasFolderPermission(ctx, revokedByUserID, folderID, "admin")
	if err != nil {
		return fmt.Errorf("permission check failed: %w", err)
	}
	if !hasPerm {
		return fmt.Errorf("insufficient permissions to revoke access")
	}

	// Can't revoke owner's permissions
	folderObjID, err := primitive.ObjectIDFromHex(folderID)
	if err != nil {
		return fmt.Errorf("invalid folder ID: %w", err)
	}

	var folder models.Folder
	if err := s.folderCollection.FindOne(ctx, bson.M{"_id": folderObjID, "deleted_at": nil}).Decode(&folder); err != nil {
		if err == mongo.ErrNoDocuments {
			return fmt.Errorf("folder not found")
		}
		return fmt.Errorf("error fetching folder: %w", err)
	}
	if folder.OwnerID.Hex() == targetUserID {
		return fmt.Errorf("cannot revoke owner's permissions")
	}

	// Soft deactivate permission (preferred) so history remains
	now := time.Now()
	res, err := s.permissionCollection.UpdateMany(ctx, bson.M{
		"user_id":       targetUserID,
		"resource_id":   folderID,
		"resource_type": "folder",
		"is_active":     true,
	}, bson.M{
		"$set": bson.M{
			"is_active":  false,
			"revoked_at": now,
			"revoked_by": revokedByUserID,
			"updated_at": now,
			"updated_by": revokedByUserID,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to revoke permission: %w", err)
	}
	if res.MatchedCount == 0 {
		// Nothing matched; interpret as no active permission
		return fmt.Errorf("no active permission found to revoke")
	}
	return nil
}

// RevokeFilePermission revokes a user's permission on a file (only admin can revoke)
func (s *PermissionService) RevokeFilePermission(ctx context.Context, fileID, targetUserID, revokedByUserID string) error {
	// Validate revokedBy has admin on the file (or parent folder)
	hasPerm, err := s.HasFilePermission(ctx, revokedByUserID, fileID, "admin")
	if err != nil {
		return fmt.Errorf("permission check failed: %w", err)
	}
	if !hasPerm {
		return fmt.Errorf("insufficient permissions to revoke access")
	}

	// Can't revoke owner's permissions
	fileObjID, err := primitive.ObjectIDFromHex(fileID)
	if err != nil {
		return fmt.Errorf("invalid file ID: %w", err)
	}

	var file models.File
	if err := s.fileCollection.FindOne(ctx, bson.M{"_id": fileObjID, "deleted_at": nil}).Decode(&file); err != nil {
		if err == mongo.ErrNoDocuments {
			return fmt.Errorf("file not found")
		}
		return fmt.Errorf("error fetching file: %w", err)
	}
	if file.OwnerID.Hex() == targetUserID {
		return fmt.Errorf("cannot revoke owner's permissions")
	}

	now := time.Now()
	res, err := s.permissionCollection.UpdateMany(ctx, bson.M{
		"user_id":       targetUserID,
		"resource_id":   fileID,
		"resource_type": "file",
		"is_active":     true,
	}, bson.M{
		"$set": bson.M{
			"is_active":  false,
			"revoked_at": now,
			"revoked_by": revokedByUserID,
			"updated_at": now,
			"updated_by": revokedByUserID,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to revoke permission: %w", err)
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("no active permission found to revoke")
	}
	return nil
}

// UpdateFolderPermission updates role for a target user's folder permission (only admin can update)
func (s *PermissionService) UpdateFolderPermission(ctx context.Context, folderID, targetUserID, newRole, updatedByUserID string) error {
	if !isValidRole(newRole) {
		return fmt.Errorf("invalid role: %s", newRole)
	}

	// Validate updater is admin
	hasPerm, err := s.HasFolderPermission(ctx, updatedByUserID, folderID, "admin")
	if err != nil {
		return fmt.Errorf("permission check failed: %w", err)
	}
	if !hasPerm {
		return fmt.Errorf("insufficient permissions to update permission")
	}

	// Cannot change owner's implicit permission (owner not stored as permission doc usually)
	folderObjID, err := primitive.ObjectIDFromHex(folderID)
	if err != nil {
		return fmt.Errorf("invalid folder ID: %w", err)
	}
	var folder models.Folder
	if err := s.folderCollection.FindOne(ctx, bson.M{"_id": folderObjID, "deleted_at": nil}).Decode(&folder); err != nil {
		if err == mongo.ErrNoDocuments {
			return fmt.Errorf("folder not found")
		}
		return fmt.Errorf("error fetching folder: %w", err)
	}
	if folder.OwnerID.Hex() == targetUserID {
		return fmt.Errorf("cannot update owner's permissions")
	}

	now := time.Now()
	res, err := s.permissionCollection.UpdateOne(ctx, bson.M{
		"user_id":       targetUserID,
		"resource_id":   folderID,
		"resource_type": "folder",
		"is_active":     true,
	}, bson.M{
		"$set": bson.M{
			"role":       newRole,
			"updated_at": now,
			"updated_by": updatedByUserID,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to update permission: %w", err)
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("no active permission found to update")
	}
	return nil
}

// UpdateFilePermission updates role for a target user's file permission (only admin can update)
func (s *PermissionService) UpdateFilePermission(ctx context.Context, fileID, targetUserID, newRole, updatedByUserID string) error {
	if !isValidRole(newRole) {
		return fmt.Errorf("invalid role: %s", newRole)
	}

	// Validate updater is admin on file (or parent folder)
	hasPerm, err := s.HasFilePermission(ctx, updatedByUserID, fileID, "admin")
	if err != nil {
		return fmt.Errorf("permission check failed: %w", err)
	}
	if !hasPerm {
		return fmt.Errorf("insufficient permissions to update permission")
	}

	// Prevent changing owner's permission
	fileObjID, err := primitive.ObjectIDFromHex(fileID)
	if err != nil {
		return fmt.Errorf("invalid file ID: %w", err)
	}
	var file models.File
	if err := s.fileCollection.FindOne(ctx, bson.M{"_id": fileObjID, "deleted_at": nil}).Decode(&file); err != nil {
		if err == mongo.ErrNoDocuments {
			return fmt.Errorf("file not found")
		}
		return fmt.Errorf("error fetching file: %w", err)
	}
	if file.OwnerID.Hex() == targetUserID {
		return fmt.Errorf("cannot update owner's permissions")
	}

	now := time.Now()
	res, err := s.permissionCollection.UpdateOne(ctx, bson.M{
		"user_id":       targetUserID,
		"resource_id":   fileID,
		"resource_type": "file",
		"is_active":     true,
	}, bson.M{
		"$set": bson.M{
			"role":       newRole,
			"updated_at": now,
			"updated_by": updatedByUserID,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to update permission: %w", err)
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("no active permission found to update")
	}
	return nil
}

// -- Internal helpers --

func (s *PermissionService) checkDirectPermission(ctx context.Context, userID, resourceID, resourceType, requiredRole string) (bool, error) {
	var permission models.Permission
	err := s.permissionCollection.FindOne(ctx, bson.M{
		"user_id":       userID,
		"resource_id":   resourceID,
		"resource_type": resourceType,
		"is_active":     true,
	}).Decode(&permission)

	if err == mongo.ErrNoDocuments {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("permission check failed: %w", err)
	}

	return hasRequiredRole(permission.Role, requiredRole), nil
}

func hasRequiredRole(userRole, requiredRole string) bool {
	roleHierarchy := map[string]int{
		"viewer": 1,
		"editor": 2,
		"admin":  3,
	}
	ur, ok1 := roleHierarchy[userRole]
	rr, ok2 := roleHierarchy[requiredRole]
	return ok1 && ok2 && ur >= rr
}

func isValidRole(role string) bool {
	switch role {
	case "viewer", "editor", "admin":
		return true
	default:
		return false
	}
}
