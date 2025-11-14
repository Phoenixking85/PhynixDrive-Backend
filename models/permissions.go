package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Permission struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID       string             `bson:"user_id" json:"user_id"`
	Role         string             `bson:"role" json:"role"`                   
	ResourceID   string             `bson:"resource_id" json:"resource_id"`     
	ResourceType string             `bson:"resource_type" json:"resource_type"` 
	GrantedBy    string             `bson:"granted_by" json:"granted_by"`       
	GrantedAt    time.Time          `bson:"granted_at" json:"granted_at"`
	IsActive     bool               `bson:"is_active" json:"is_active"`
}
