package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Permission struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID       string             `bson:"user_id" json:"user_id"`
	Role         string             `bson:"role" json:"role"`                   // viewer, editor, admin
	ResourceID   string             `bson:"resource_id" json:"resource_id"`     // ID of file or folder
	ResourceType string             `bson:"resource_type" json:"resource_type"` // "file" or "folder"
	GrantedBy    string             `bson:"granted_by" json:"granted_by"`       // userID of grantor
	GrantedAt    time.Time          `bson:"granted_at" json:"granted_at"`
	IsActive     bool               `bson:"is_active" json:"is_active"`
}
