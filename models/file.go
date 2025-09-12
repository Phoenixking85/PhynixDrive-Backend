package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type File struct {
	ID           primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	Name         string              `bson:"name" json:"name"`
	OriginalName string              `bson:"original_name" json:"original_name"`
	Size         int64               `bson:"size" json:"size"`
	MimeType     string              `bson:"mime_type" json:"mime_type"`
	FolderID     *primitive.ObjectID `bson:"folder_id,omitempty" json:"folder_id,omitempty"`
	OwnerID      primitive.ObjectID  `bson:"owner_id" json:"owner_id"`
	B2FileID     string              `bson:"b2_file_id" json:"b2_file_id"`
	B2FileName   string              `bson:"b2_file_name" json:"b2_file_name"`
	B2BucketID   string              `bson:"b2_bucket_id" json:"b2_bucket_id"`
	RelativePath string              `bson:"relative_path" json:"relative_path"` // Original upload path
	Permissions  []Permission        `bson:"permissions" json:"permissions"`
	Versions     []FileVersion       `bson:"versions" json:"versions"`
	IsDeleted    bool                `bson:"is_deleted" json:"is_deleted"`
	DeletedAt    *time.Time          `bson:"deleted_at,omitempty" json:"deleted_at,omitempty"`
	CreatedAt    time.Time           `bson:"created_at" json:"created_at"`
	UpdatedAt    time.Time           `bson:"updated_at" json:"updated_at"`
	Extension    string              `bson:"extension" json:"extension"`
	SHA1Hash     string              `bson:"sha1_hash" json:"sha1_hash"`                     // For file integrity checks
	ContentType  string              `bson:"content_type" json:"content_type"`               // MIME type for file handling
	ParentID     *primitive.ObjectID `bson:"parent_id,omitempty" json:"parent_id,omitempty"` // For nested folders
}

type FileVersion struct {
	VersionID  primitive.ObjectID `bson:"version_id" json:"version_id"`
	B2FileID   string             `bson:"b2_file_id" json:"b2_file_id"`
	B2FileName string             `bson:"b2_file_name" json:"b2_file_name"`
	Size       int64              `bson:"size" json:"size"`
	CreatedAt  time.Time          `bson:"created_at" json:"created_at"`
}
