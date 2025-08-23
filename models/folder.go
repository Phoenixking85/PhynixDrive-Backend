package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Folder struct {
	ID          primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	Name        string              `bson:"name" json:"name"`
	ParentID    *primitive.ObjectID `bson:"parent_id,omitempty" json:"parent_id,omitempty"`
	OwnerID     primitive.ObjectID  `bson:"owner_id" json:"owner_id"`
	Path        string              `bson:"path" json:"path"` // Full path for easy lookup
	Permissions []Permission        `bson:"permissions" json:"permissions"`
	IsDeleted   bool                `bson:"is_deleted" json:"is_deleted"`
	DeletedAt   *time.Time          `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedAt   time.Time           `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time           `bson:"updated_at" json:"updated_at"`
}
