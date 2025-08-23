package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type User struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	GoogleID     string             `bson:"google_id" json:"google_id"`
	Email        string             `bson:"email" json:"email"`
	Name         string             `bson:"name" json:"name"`
	ProfilePic   string             `bson:"profile_pic" json:"profile_pic"`
	Role         string             `bson:"role" json:"role"`
	UsedStorage  int64              `bson:"used_storage" json:"used_storage"` // in bytes
	MaxStorage   int64              `bson:"max_storage" json:"max_storage"`   // 2GB default
	CreatedAt    time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt    time.Time          `bson:"updated_at" json:"updated_at"`
	RefreshToken string             `json:"refresh_token,omitempty" bson:"refresh_token,omitempty"`
}
