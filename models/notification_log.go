package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type NotificationLog struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID    primitive.ObjectID `bson:"user_id" json:"user_id"`
	Type      string             `bson:"type" json:"type"` // "file_shared", "folder_shared", "file_uploaded"
	Title     string             `bson:"title" json:"title"`
	Message   string             `bson:"message" json:"message"`
	ItemID    primitive.ObjectID `bson:"item_id" json:"item_id"`
	ItemType  string             `bson:"item_type" json:"item_type"` // "file" or "folder"
	IsRead    bool               `bson:"is_read" json:"is_read"`
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
}
