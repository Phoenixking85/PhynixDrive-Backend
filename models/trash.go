package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type TrashItem struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	ItemID       primitive.ObjectID `bson:"item_id" json:"item_id"`     
	ItemType     string             `bson:"item_type" json:"item_type"` 
	Name         string             `bson:"name" json:"name"`
	OriginalPath string             `bson:"original_path" json:"original_path"`
	OwnerID      primitive.ObjectID `bson:"owner_id" json:"owner_id"`
	Size         int64              `bson:"size" json:"size"` 
	DeletedAt    time.Time          `bson:"deleted_at" json:"deleted_at"`
	AutoPurgeAt  time.Time          `bson:"auto_purge_at" json:"auto_purge_at"`
}
