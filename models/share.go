package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Share represents a sharing relationship between users and resources
type Share struct {
	ID           primitive.ObjectID `bson:"_id" json:"id"`
	ResourceID   string             `bson:"resource_id" json:"resource_id"`     // ID of the shared resource
	ResourceType string             `bson:"resource_type" json:"resource_type"` // "file" or "folder"
	SharedWith   string             `bson:"shared_with" json:"shared_with"`     // User ID who receives access
	SharedBy     string             `bson:"shared_by" json:"shared_by"`         // User ID who granted access
	Role         string             `bson:"role" json:"role"`                   // "viewer", "editor", "admin"
	SharedAt     time.Time          `bson:"shared_at" json:"shared_at"`
	IsActive     bool               `bson:"is_active" json:"is_active"`
	RevokedAt    *time.Time         `bson:"revoked_at,omitempty" json:"revoked_at,omitempty"`
	RevokedBy    string             `bson:"revoked_by,omitempty" json:"revoked_by,omitempty"`
	UpdatedAt    *time.Time         `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
	UpdatedBy    string             `bson:"updated_by,omitempty" json:"updated_by,omitempty"`
	FirstName    string             `bson:"first_name,omitempty" json:"first_name,omitempty"` // Denormalized for quick access
	LastName     string             `bson:"last_name,omitempty" json:"last_name,omitempty"`   // Denormalized for quick access
}

// ShareActivity represents sharing activity logs
type ShareActivity struct {
	ID           primitive.ObjectID     `bson:"_id" json:"id"`
	ShareID      primitive.ObjectID     `bson:"share_id" json:"share_id"`
	ResourceID   string                 `bson:"resource_id" json:"resource_id"`
	ResourceType string                 `bson:"resource_type" json:"resource_type"`
	Action       string                 `bson:"action" json:"action"` // "shared", "updated", "revoked"
	PerformedBy  string                 `bson:"performed_by" json:"performed_by"`
	PerformedAt  time.Time              `bson:"performed_at" json:"performed_at"`
	Details      map[string]interface{} `bson:"details,omitempty" json:"details,omitempty"`
}

// Update existing User model to include sharing-related fields if not already present
// This should be added to your existing User model
type UserSharingInfo struct {
	SharedResourcesCount int        `bson:"shared_resources_count" json:"shared_resources_count"`
	LastSharedAt         *time.Time `bson:"last_shared_at,omitempty" json:"last_shared_at,omitempty"`
}

// Update existing File model to include sharing-related fields if not already present
// This should be added to your existing File model
type FileSharingInfo struct {
	IsShared     bool       `bson:"is_shared" json:"is_shared"`
	SharedCount  int        `bson:"shared_count" json:"shared_count"`
	LastSharedAt *time.Time `bson:"last_shared_at,omitempty" json:"last_shared_at,omitempty"`
}

// Update existing Folder model to include sharing-related fields if not already present
// This should be added to your existing Folder model
type FolderSharingInfo struct {
	IsShared     bool       `bson:"is_shared" json:"is_shared"`
	SharedCount  int        `bson:"shared_count" json:"shared_count"`
	LastSharedAt *time.Time `bson:"last_shared_at,omitempty" json:"last_shared_at,omitempty"`
}
