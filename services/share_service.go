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

type ShareService struct {
	shareCollection   *mongo.Collection
	folderCollection  *mongo.Collection
	fileCollection    *mongo.Collection
	userCollection    *mongo.Collection
	permissionService *PermissionService
}

type ShareRequest struct {
	ResourceID        string `json:"resource_id" validate:"required"`
	ResourceType      string `json:"resource_type" validate:"required,oneof=file folder"`
	Email             string `json:"email" validate:"required,email"`
	Role              string `json:"role" validate:"required,oneof=viewer editor admin"`
	InheritToChildren bool   `json:"inherit_to_children,omitempty"`
}

type ShareResponse struct {
	ID               primitive.ObjectID `json:"id"`
	ResourceID       string             `json:"resource_id"`
	ResourceType     string             `json:"resource_type"`
	ResourceName     string             `json:"resource_name"`
	SharedWith       string             `json:"shared_with"`
	SharedWithName   string             `json:"shared_with_name"`
	Role             string             `json:"role"`
	SharedBy         string             `json:"shared_by"`
	SharedByName     string             `json:"shared_by_name"`
	SharedAt         time.Time          `json:"shared_at"`
	ChildrenAffected int                `json:"children_affected,omitempty"`
}

type SharedResourcesResponse struct {
	SharedByMe   []ShareResponse `json:"shared_by_me"`
	SharedWithMe []ShareResponse `json:"shared_with_me"`
	Total        int             `json:"total"`
}

type ResourceInfo struct {
	ID           primitive.ObjectID `json:"id"`
	Name         string             `json:"name"`
	Type         string             `json:"type"`
	Size         int64              `json:"size,omitempty"`
	CreatedAt    time.Time          `json:"created_at"`
	OwnerName    string             `json:"owner_name"`
	SharedBy     string             `json:"shared_by"`
	SharedByName string             `json:"shared_by_name"`
	Role         string             `json:"role"`
	SharedAt     time.Time          `json:"shared_at"`
}

type PermissionInfo struct {
	ID            primitive.ObjectID `json:"id"`
	ResourceID    string             `json:"resource_id"`
	ResourceType  string             `json:"resource_type"`
	ResourceName  string             `json:"resource_name"`
	UserID        string             `json:"user_id"`
	UserName      string             `json:"user_name"`
	UserEmail     string             `json:"user_email"`
	Role          string             `json:"role"`
	GrantedBy     string             `json:"granted_by"`
	GrantedByName string             `json:"granted_by_name"`
	GrantedAt     time.Time          `json:"granted_at"`
}

func NewShareService(db *mongo.Database, permissionService *PermissionService) *ShareService {
	return &ShareService{
		shareCollection:   db.Collection("shares"),
		folderCollection:  db.Collection("folders"),
		fileCollection:    db.Collection("files"),
		userCollection:    db.Collection("users"),
		permissionService: permissionService,
	}
}

// ShareResource shares a file or folder with a user
func (s *ShareService) ShareResource(ctx context.Context, request ShareRequest, sharerID string) (*ShareResponse, error) {
	// Validate sharer has permission to share
	hasPermission, err := s.validateSharePermission(ctx, request.ResourceID, request.ResourceType, sharerID)
	if err != nil {
		return nil, fmt.Errorf("permission validation failed: %w", err)
	}
	if !hasPermission {
		return nil, fmt.Errorf("insufficient permissions to share resource")
	}

	// Find target user by email
	var targetUser models.User
	err = s.userCollection.FindOne(ctx, bson.M{"email": request.Email}).Decode(&targetUser)
	if err == mongo.ErrNoDocuments {
		return nil, fmt.Errorf("user with email %s not found", request.Email)
	} else if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}

	// Check if already shared
	existingShare, err := s.getExistingShare(ctx, request.ResourceID, request.ResourceType, targetUser.ID.Hex())
	if err != nil && err != mongo.ErrNoDocuments {
		return nil, fmt.Errorf("failed to check existing share: %w", err)
	}
	if existingShare != nil {
		return nil, fmt.Errorf("resource already shared with this user")
	}

	// Get resource info
	resourceName, err := s.getResourceName(ctx, request.ResourceID, request.ResourceType)
	if err != nil {
		return nil, fmt.Errorf("failed to get resource name: %w", err)
	}

	// Get sharer info
	var sharer models.User
	sharerObjID, _ := primitive.ObjectIDFromHex(sharerID)
	err = s.userCollection.FindOne(ctx, bson.M{"_id": sharerObjID}).Decode(&sharer)
	if err != nil {
		return nil, fmt.Errorf("failed to get sharer info: %w", err)
	}

	// Create share record
	share := models.Share{
		ID:           primitive.NewObjectID(),
		ResourceID:   request.ResourceID,
		ResourceType: request.ResourceType,
		SharedWith:   targetUser.ID.Hex(),
		SharedBy:     sharerID,
		Role:         request.Role,
		SharedAt:     time.Now(),
		IsActive:     true,
	}

	_, err = s.shareCollection.InsertOne(ctx, share)
	if err != nil {
		return nil, fmt.Errorf("failed to create share record: %w", err)
	}

	// Grant permission through permission service
	if request.ResourceType == "folder" {
		err = s.permissionService.ShareFolder(ctx, request.ResourceID, targetUser.ID.Hex(), request.Role, sharerID)
	} else {
		err = s.permissionService.ShareFile(ctx, request.ResourceID, targetUser.ID.Hex(), request.Role, sharerID)
	}
	if err != nil {
		// Cleanup share record on permission failure
		s.shareCollection.DeleteOne(ctx, bson.M{"_id": share.ID})
		return nil, fmt.Errorf("failed to grant permission: %w", err)
	}

	childrenAffected := 0
	// Handle folder inheritance
	if request.ResourceType == "folder" && request.InheritToChildren {
		affected, err := s.shareChildFoldersRecursively(ctx, request.ResourceID, targetUser.ID.Hex(), request.Role, sharerID)
		if err != nil {
			return nil, fmt.Errorf("failed to share child folders: %w", err)
		}
		childrenAffected = affected
	}

	response := &ShareResponse{
		ID:               share.ID,
		ResourceID:       request.ResourceID,
		ResourceType:     request.ResourceType,
		ResourceName:     resourceName,
		SharedWith:       request.Email,
		SharedWithName:   targetUser.FirstName + " " + targetUser.LastName,
		Role:             request.Role,
		SharedBy:         sharer.Email,
		SharedByName:     sharer.FirstName + " " + sharer.LastName,
		SharedAt:         share.SharedAt,
		ChildrenAffected: childrenAffected,
	}

	return response, nil
}

// GetSharedByMe returns all resources shared by the current user
func (s *ShareService) GetSharedByMe(ctx context.Context, userID string, resourceType *string) ([]ShareResponse, error) {
	filter := bson.M{
		"shared_by": userID,
		"is_active": true,
	}
	if resourceType != nil && *resourceType != "" {
		filter["resource_type"] = *resourceType
	}

	cursor, err := s.shareCollection.Find(ctx, filter, options.Find().SetSort(bson.M{"shared_at": -1}))
	if err != nil {
		return nil, fmt.Errorf("failed to get shared resources: %w", err)
	}
	defer cursor.Close(ctx)

	var shares []ShareResponse
	for cursor.Next(ctx) {
		var share models.Share
		if err := cursor.Decode(&share); err != nil {
			continue
		}

		response, err := s.buildShareResponse(ctx, share)
		if err != nil {
			continue // Skip invalid shares
		}
		shares = append(shares, *response)
	}

	return shares, nil
}

// GetSharedWithMe returns all resources shared with the current user
func (s *ShareService) GetSharedWithMe(ctx context.Context, userID string, resourceType *string) ([]ResourceInfo, error) {
	filter := bson.M{
		"shared_with": userID,
		"is_active":   true,
	}
	if resourceType != nil && *resourceType != "" {
		filter["resource_type"] = *resourceType
	}

	cursor, err := s.shareCollection.Find(ctx, filter, options.Find().SetSort(bson.M{"shared_at": -1}))
	if err != nil {
		return nil, fmt.Errorf("failed to get shared resources: %w", err)
	}
	defer cursor.Close(ctx)

	var resources []ResourceInfo
	for cursor.Next(ctx) {
		var share models.Share
		if err := cursor.Decode(&share); err != nil {
			continue
		}

		resource, err := s.buildResourceInfo(ctx, share)
		if err != nil {
			continue // Skip invalid resources
		}
		resources = append(resources, *resource)
	}

	return resources, nil
}

// GetAllSharedResources returns both shared by me and shared with me
func (s *ShareService) GetAllSharedResources(ctx context.Context, userID string) (*SharedResourcesResponse, error) {
	sharedByMe, err := s.GetSharedByMe(ctx, userID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get resources shared by user: %w", err)
	}

	sharedWithMe, err := s.GetSharedWithMe(ctx, userID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get resources shared with user: %w", err)
	}

	// Convert ResourceInfo to ShareResponse for consistent response
	var sharedWithMeResponse []ShareResponse
	for _, resource := range sharedWithMe {
		sharedWithMeResponse = append(sharedWithMeResponse, ShareResponse{
			ResourceID:   resource.ID.Hex(),
			ResourceType: resource.Type,
			ResourceName: resource.Name,
			Role:         resource.Role,
			SharedBy:     resource.SharedBy,
			SharedByName: resource.SharedByName,
			SharedAt:     resource.SharedAt,
		})
	}

	response := &SharedResourcesResponse{
		SharedByMe:   sharedByMe,
		SharedWithMe: sharedWithMeResponse,
		Total:        len(sharedByMe) + len(sharedWithMeResponse),
	}

	return response, nil
}

// GetResourcePermissions returns all permissions for a specific resource
func (s *ShareService) GetResourcePermissions(ctx context.Context, resourceID, resourceType, userID string) ([]PermissionInfo, error) {
	// Validate user has permission to view permissions (admin level)
	hasPermission, err := s.validateSharePermission(ctx, resourceID, resourceType, userID)
	if err != nil {
		return nil, fmt.Errorf("permission validation failed: %w", err)
	}
	if !hasPermission {
		return nil, fmt.Errorf("insufficient permissions")
	}

	filter := bson.M{
		"resource_id":   resourceID,
		"resource_type": resourceType,
		"is_active":     true,
	}

	cursor, err := s.shareCollection.Find(ctx, filter, options.Find().SetSort(bson.M{"shared_at": -1}))
	if err != nil {
		return nil, fmt.Errorf("failed to get permissions: %w", err)
	}
	defer cursor.Close(ctx)

	var permissions []PermissionInfo
	for cursor.Next(ctx) {
		var share models.Share
		if err := cursor.Decode(&share); err != nil {
			continue
		}

		permission, err := s.buildPermissionInfo(ctx, share)
		if err != nil {
			continue
		}
		permissions = append(permissions, *permission)
	}

	return permissions, nil
}

// RevokePermission removes a user's access to a resource
func (s *ShareService) RevokePermission(ctx context.Context, shareID, userID string) error {
	shareObjID, err := primitive.ObjectIDFromHex(shareID)
	if err != nil {
		return fmt.Errorf("invalid share ID: %w", err)
	}

	// Get share details
	var share models.Share
	err = s.shareCollection.FindOne(ctx, bson.M{
		"_id":       shareObjID,
		"is_active": true,
	}).Decode(&share)
	if err == mongo.ErrNoDocuments {
		return fmt.Errorf("share not found")
	} else if err != nil {
		return fmt.Errorf("database error: %w", err)
	}

	// Validate user has permission to revoke (must be owner or the person who shared it)
	hasPermission, err := s.validateSharePermission(ctx, share.ResourceID, share.ResourceType, userID)
	if err != nil {
		return fmt.Errorf("permission validation failed: %w", err)
	}
	if !hasPermission && share.SharedBy != userID {
		return fmt.Errorf("insufficient permissions to revoke access")
	}

	// Revoke permission through permission service
	if share.ResourceType == "folder" {
		err = s.permissionService.RevokeFolderPermission(ctx, share.ResourceID, share.SharedWith, userID)
	} else {
		err = s.permissionService.RevokeFilePermission(ctx, share.ResourceID, share.SharedWith, userID)
	}
	if err != nil {
		return fmt.Errorf("failed to revoke permission: %w", err)
	}

	// Deactivate share record
	_, err = s.shareCollection.UpdateOne(
		ctx,
		bson.M{"_id": shareObjID},
		bson.M{
			"$set": bson.M{
				"is_active":  false,
				"revoked_at": time.Now(),
				"revoked_by": userID,
			},
		},
	)
	if err != nil {
		return fmt.Errorf("failed to update share record: %w", err)
	}

	return nil
}

// UpdatePermission changes the role of an existing permission
func (s *ShareService) UpdatePermission(ctx context.Context, shareID, newRole, userID string) (*ShareResponse, error) {
	shareObjID, err := primitive.ObjectIDFromHex(shareID)
	if err != nil {
		return nil, fmt.Errorf("invalid share ID: %w", err)
	}

	// Get share details
	var share models.Share
	err = s.shareCollection.FindOne(ctx, bson.M{
		"_id":       shareObjID,
		"is_active": true,
	}).Decode(&share)
	if err == mongo.ErrNoDocuments {
		return nil, fmt.Errorf("share not found")
	} else if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}

	// Validate user has permission to update
	hasPermission, err := s.validateSharePermission(ctx, share.ResourceID, share.ResourceType, userID)
	if err != nil {
		return nil, fmt.Errorf("permission validation failed: %w", err)
	}
	if !hasPermission {
		return nil, fmt.Errorf("insufficient permissions")
	}

	// Update permission through permission service
	if share.ResourceType == "folder" {
		err = s.permissionService.UpdateFolderPermission(ctx, share.ResourceID, share.SharedWith, newRole, userID)
	} else {
		err = s.permissionService.UpdateFilePermission(ctx, share.ResourceID, share.SharedWith, newRole, userID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to update permission: %w", err)
	}

	// Update share record
	_, err = s.shareCollection.UpdateOne(
		ctx,
		bson.M{"_id": shareObjID},
		bson.M{
			"$set": bson.M{
				"role":       newRole,
				"updated_at": time.Now(),
				"updated_by": userID,
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update share record: %w", err)
	}

	// Return updated share response
	share.Role = newRole
	return s.buildShareResponse(ctx, share)
}

// Helper methods

func (s *ShareService) validateSharePermission(ctx context.Context, resourceID, resourceType, userID string) (bool, error) {
	if s.permissionService == nil {
		return true, nil // Skip validation if no permission service
	}

	if resourceType == "folder" {
		return s.permissionService.HasFolderPermission(ctx, userID, resourceID, "admin")
	}
	return s.permissionService.HasFilePermission(ctx, userID, resourceID, "admin")
}

func (s *ShareService) getExistingShare(ctx context.Context, resourceID, resourceType, sharedWith string) (*models.Share, error) {
	var share models.Share
	err := s.shareCollection.FindOne(ctx, bson.M{
		"resource_id":   resourceID,
		"resource_type": resourceType,
		"shared_with":   sharedWith,
		"is_active":     true,
	}).Decode(&share)

	if err == mongo.ErrNoDocuments {
		return nil, nil // ✅ nothing found → return nil, not an empty struct
	}
	if err != nil {
		return nil, err // other DB error
	}

	return &share, nil // ✅ found → return pointer to actual document
}

func (s *ShareService) getResourceName(ctx context.Context, resourceID, resourceType string) (string, error) {
	objID, err := primitive.ObjectIDFromHex(resourceID)
	if err != nil {
		return "", err
	}

	if resourceType == "folder" {
		var folder models.Folder
		err = s.folderCollection.FindOne(ctx, bson.M{"_id": objID}).Decode(&folder)
		if err != nil {
			return "", err
		}
		return folder.Name, nil
	} else {
		var file models.File
		err = s.fileCollection.FindOne(ctx, bson.M{"_id": objID}).Decode(&file)
		if err != nil {
			return "", err
		}
		return file.Name, nil
	}
}

func (s *ShareService) buildShareResponse(ctx context.Context, share models.Share) (*ShareResponse, error) {
	resourceName, err := s.getResourceName(ctx, share.ResourceID, share.ResourceType)
	if err != nil {
		return nil, err
	}

	// Get shared with user info
	sharedWithObjID, _ := primitive.ObjectIDFromHex(share.SharedWith)
	var sharedWithUser models.User
	err = s.userCollection.FindOne(ctx, bson.M{"_id": sharedWithObjID}).Decode(&sharedWithUser)
	if err != nil {
		return nil, err
	}

	// Get shared by user info
	sharedByObjID, _ := primitive.ObjectIDFromHex(share.SharedBy)
	var sharedByUser models.User
	err = s.userCollection.FindOne(ctx, bson.M{"_id": sharedByObjID}).Decode(&sharedByUser)
	if err != nil {
		return nil, err
	}

	return &ShareResponse{
		ID:             share.ID,
		ResourceID:     share.ResourceID,
		ResourceType:   share.ResourceType,
		ResourceName:   resourceName,
		SharedWith:     sharedWithUser.Email,
		SharedWithName: sharedWithUser.FirstName + " " + sharedWithUser.LastName,
		Role:           share.Role,
		SharedBy:       sharedByUser.Email,
		SharedByName:   sharedByUser.FirstName + " " + sharedByUser.LastName,
		SharedAt:       share.SharedAt,
	}, nil
}

func (s *ShareService) buildResourceInfo(ctx context.Context, share models.Share) (*ResourceInfo, error) {
	objID, err := primitive.ObjectIDFromHex(share.ResourceID)
	if err != nil {
		return nil, err
	}

	var resourceInfo ResourceInfo
	if share.ResourceType == "folder" {
		var folder models.Folder
		err = s.folderCollection.FindOne(ctx, bson.M{"_id": objID}).Decode(&folder)
		if err != nil {
			return nil, err
		}
		resourceInfo = ResourceInfo{
			ID:        folder.ID,
			Name:      folder.Name,
			Type:      "folder",
			CreatedAt: folder.CreatedAt,
		}
	} else {
		var file models.File
		err = s.fileCollection.FindOne(ctx, bson.M{"_id": objID}).Decode(&file)
		if err != nil {
			return nil, err
		}
		resourceInfo = ResourceInfo{
			ID:        file.ID,
			Name:      file.Name,
			Type:      "file",
			Size:      file.Size,
			CreatedAt: file.CreatedAt,
		}
	}

	// Get shared by user info
	sharedByObjID, _ := primitive.ObjectIDFromHex(share.SharedBy)
	var sharedByUser models.User
	err = s.userCollection.FindOne(ctx, bson.M{"_id": sharedByObjID}).Decode(&sharedByUser)
	if err != nil {
		return nil, err
	}

	resourceInfo.SharedBy = share.SharedBy
	resourceInfo.SharedByName = sharedByUser.FirstName + " " + sharedByUser.LastName
	resourceInfo.Role = share.Role
	resourceInfo.SharedAt = share.SharedAt

	return &resourceInfo, nil
}

func (s *ShareService) buildPermissionInfo(ctx context.Context, share models.Share) (*PermissionInfo, error) {
	resourceName, err := s.getResourceName(ctx, share.ResourceID, share.ResourceType)
	if err != nil {
		return nil, err
	}

	// Get user info
	userObjID, _ := primitive.ObjectIDFromHex(share.SharedWith)
	var user models.User
	err = s.userCollection.FindOne(ctx, bson.M{"_id": userObjID}).Decode(&user)
	if err != nil {
		return nil, err
	}

	// Get granted by user info
	grantedByObjID, _ := primitive.ObjectIDFromHex(share.SharedBy)
	var grantedByUser models.User
	err = s.userCollection.FindOne(ctx, bson.M{"_id": grantedByObjID}).Decode(&grantedByUser)
	if err != nil {
		return nil, err
	}

	return &PermissionInfo{
		ID:            share.ID,
		ResourceID:    share.ResourceID,
		ResourceType:  share.ResourceType,
		ResourceName:  resourceName,
		UserID:        share.SharedWith,
		UserName:      user.FirstName + " " + user.LastName,
		UserEmail:     user.Email,
		Role:          share.Role,
		GrantedBy:     share.SharedBy,
		GrantedByName: grantedByUser.FirstName + " " + grantedByUser.LastName,
		GrantedAt:     share.SharedAt,
	}, nil
}

func (s *ShareService) shareChildFoldersRecursively(ctx context.Context, parentID, targetUserID, role, sharerID string) (int, error) {
	parentObjID, err := primitive.ObjectIDFromHex(parentID)
	if err != nil {
		return 0, err
	}

	cursor, err := s.folderCollection.Find(ctx, bson.M{
		"parent_id":  parentObjID,
		"is_deleted": false,
	})
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)

	affected := 0
	for cursor.Next(ctx) {
		var childFolder models.Folder
		if err := cursor.Decode(&childFolder); err != nil {
			continue
		}

		// Create share record for child folder
		share := models.Share{
			ID:           primitive.NewObjectID(),
			ResourceID:   childFolder.ID.Hex(),
			ResourceType: "folder",
			SharedWith:   targetUserID,
			SharedBy:     sharerID,
			Role:         role,
			SharedAt:     time.Now(),
			IsActive:     true,
		}

		_, err = s.shareCollection.InsertOne(ctx, share)
		if err != nil {
			continue
		}

		err = s.permissionService.ShareFolder(ctx, childFolder.ID.Hex(), targetUserID, role, sharerID)
		if err != nil {
			s.shareCollection.DeleteOne(ctx, bson.M{"_id": share.ID})
			continue
		}

		affected++

		// Recursively share grandchildren
		grandchildrenAffected, _ := s.shareChildFoldersRecursively(ctx, childFolder.ID.Hex(), targetUserID, role, sharerID)
		affected += grandchildrenAffected
	}

	return affected, nil
}
