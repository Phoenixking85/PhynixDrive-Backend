package services

import (
	"context"
	"fmt"
	"log"
	"phynixdrive/models"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type TrashService struct {
	fileCollection   *mongo.Collection
	folderCollection *mongo.Collection
	userCollection   *mongo.Collection
	b2Service        *B2Service
}

// RestoreItem represents an item to be restored
type RestoreItem struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

type RestoreResult struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

func NewTrashService(db *mongo.Database, b2Service *B2Service) *TrashService {
	return &TrashService{
		fileCollection:   db.Collection("files"),
		folderCollection: db.Collection("folders"),
		userCollection:   db.Collection("users"),
		b2Service:        b2Service,
	}
}

func (s *TrashService) GetTrashItems(userID, itemType string, limit, offset int) ([]models.TrashItem, error) {
	ctx := context.Background()
	var trashItems []models.TrashItem

	// Convert userID string to ObjectID
	userObjID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID: %w", err)
	}

	// Build filters
	baseFilter := bson.M{
		"owner_id":   userObjID,
		"deleted_at": bson.M{"$ne": nil},
	}

	// Set up find options with limit and offset
	findOptions := options.Find().
		SetSort(bson.M{"deleted_at": -1}).
		SetLimit(int64(limit)).
		SetSkip(int64(offset))

	// Get deleted files if itemType is empty or "file"
	if itemType == "" || itemType == "file" {
		fileCursor, err := s.fileCollection.Find(ctx, baseFilter, findOptions)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch deleted files: %w", err)
		}
		defer fileCursor.Close(ctx)

		var deletedFiles []models.File
		if err = fileCursor.All(ctx, &deletedFiles); err != nil {
			return nil, fmt.Errorf("failed to decode deleted files: %w", err)
		}

		for _, file := range deletedFiles {
			var deletedAt, autoPurgeAt time.Time
			if file.DeletedAt != nil {
				deletedAt = *file.DeletedAt
				autoPurgeAt = deletedAt.AddDate(0, 0, 30)
			}

			trashItems = append(trashItems, models.TrashItem{
				ItemID:       file.ID,
				ItemType:     "file",
				Name:         file.Name,
				OriginalPath: file.RelativePath,
				OwnerID:      file.OwnerID,
				Size:         file.Size,
				DeletedAt:    deletedAt,
				AutoPurgeAt:  autoPurgeAt,
			})
		}
	}

	// Get deleted folders if itemType is empty or "folder"
	if itemType == "" || itemType == "folder" {
		folderCursor, err := s.folderCollection.Find(ctx, baseFilter, findOptions)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch deleted folders: %w", err)
		}
		defer folderCursor.Close(ctx)

		var deletedFolders []models.Folder
		if err = folderCursor.All(ctx, &deletedFolders); err != nil {
			return nil, fmt.Errorf("failed to decode deleted folders: %w", err)
		}

		for _, folder := range deletedFolders {
			var deletedAt, autoPurgeAt time.Time
			if folder.DeletedAt != nil {
				deletedAt = *folder.DeletedAt
				autoPurgeAt = deletedAt.AddDate(0, 0, 30)
			}

			trashItems = append(trashItems, models.TrashItem{
				ItemID:       folder.ID,
				ItemType:     "folder",
				Name:         folder.Name,
				OriginalPath: folder.Path,
				OwnerID:      folder.OwnerID,
				Size:         0,
				DeletedAt:    deletedAt,
				AutoPurgeAt:  autoPurgeAt,
			})
		}
	}

	return trashItems, nil
}

func (s *TrashService) RestoreFile(fileID, userID string) error {
	ctx := context.Background()

	// Convert IDs to ObjectID
	fileObjID, err := primitive.ObjectIDFromHex(fileID)
	if err != nil {
		return fmt.Errorf("invalid file ID: %w", err)
	}

	userObjID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return fmt.Errorf("invalid user ID: %w", err)
	}

	// Find the file
	var file models.File
	err = s.fileCollection.FindOne(ctx, bson.M{
		"_id":        fileObjID,
		"owner_id":   userObjID,
		"deleted_at": bson.M{"$ne": nil},
	}).Decode(&file)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return fmt.Errorf("file not found in trash")
		}
		return fmt.Errorf("failed to find file: %w", err)
	}

	// Check if parent folder exists and is not deleted
	if file.ParentID != nil {
		var parentFolder models.Folder
		err = s.folderCollection.FindOne(ctx, bson.M{
			"_id":        file.ParentID,
			"owner_id":   userObjID,
			"deleted_at": nil,
		}).Decode(&parentFolder)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				return fmt.Errorf("cannot restore file: parent folder no longer exists or is deleted")
			}
			return fmt.Errorf("failed to check parent folder: %w", err)
		}
	}

	// Restore the file
	update := bson.M{
		"$unset": bson.M{"deleted_at": ""},
	}

	result, err := s.fileCollection.UpdateOne(ctx, bson.M{
		"_id":      fileObjID,
		"owner_id": userObjID,
	}, update)
	if err != nil {
		return fmt.Errorf("failed to restore file: %w", err)
	}

	if result.ModifiedCount == 0 {
		return fmt.Errorf("file not found or already restored")
	}

	return nil
}

func (s *TrashService) RestoreFolder(folderID, userID string) error {
	ctx := context.Background()

	// Convert IDs to ObjectID
	folderObjID, err := primitive.ObjectIDFromHex(folderID)
	if err != nil {
		return fmt.Errorf("invalid folder ID: %w", err)
	}

	userObjID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return fmt.Errorf("invalid user ID: %w", err)
	}

	// Find the folder
	var folder models.Folder
	err = s.folderCollection.FindOne(ctx, bson.M{
		"_id":        folderObjID,
		"owner_id":   userObjID,
		"deleted_at": bson.M{"$ne": nil},
	}).Decode(&folder)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return fmt.Errorf("folder not found in trash")
		}
		return fmt.Errorf("failed to find folder: %w", err)
	}

	// Check if parent folder exists and is not deleted
	if folder.ParentID != nil {
		var parentFolder models.Folder
		err = s.folderCollection.FindOne(ctx, bson.M{
			"_id":        folder.ParentID,
			"owner_id":   userObjID,
			"deleted_at": nil,
		}).Decode(&parentFolder)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				return fmt.Errorf("cannot restore folder: parent folder no longer exists or is deleted")
			}
			return fmt.Errorf("failed to check parent folder: %w", err)
		}
	}

	// Start a session for transaction
	session, err := s.folderCollection.Database().Client().StartSession()
	if err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}
	defer session.EndSession(ctx)

	// Use transaction to restore folder and its contents
	_, err = session.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {
		// Restore the folder
		update := bson.M{
			"$unset": bson.M{"deleted_at": ""},
		}

		result, err := s.folderCollection.UpdateOne(sc, bson.M{
			"_id":      folderObjID,
			"owner_id": userObjID,
		}, update)
		if err != nil {
			return nil, fmt.Errorf("failed to restore folder: %w", err)
		}

		if result.ModifiedCount == 0 {
			return nil, fmt.Errorf("folder not found or already restored")
		}

		// Restore all child folders recursively
		_, err = s.folderCollection.UpdateMany(sc, bson.M{
			"path":     bson.M{"$regex": "^" + folder.Path + "/"},
			"owner_id": userObjID,
		}, update)
		if err != nil {
			return nil, fmt.Errorf("failed to restore child folders: %w", err)
		}

		// Restore all files in this folder and subfolders
		_, err = s.fileCollection.UpdateMany(sc, bson.M{
			"relative_path": bson.M{"$regex": "^" + folder.Path + "/"},
			"owner_id":      userObjID,
		}, update)
		if err != nil {
			return nil, fmt.Errorf("failed to restore files in folder: %w", err)
		}

		return nil, nil
	})

	return err
}

func (s *TrashService) RestoreMultipleItems(userID string, items []RestoreItem) ([]RestoreResult, error) {
	var results []RestoreResult

	for _, item := range items {
		result := RestoreResult{
			ID:   item.ID,
			Type: item.Type,
		}

		switch item.Type {
		case "file":
			err := s.RestoreFile(item.ID, userID)
			if err != nil {
				result.Success = false
				result.Error = err.Error()
			} else {
				result.Success = true
			}
		case "folder":
			err := s.RestoreFolder(item.ID, userID)
			if err != nil {
				result.Success = false
				result.Error = err.Error()
			} else {
				result.Success = true
			}
		default:
			result.Success = false
			result.Error = "Invalid item type"
		}

		results = append(results, result)
	}

	return results, nil
}

func (s *TrashService) PurgeFile(fileID, userID string) error {
	ctx := context.Background()

	// Convert IDs to ObjectID
	fileObjID, err := primitive.ObjectIDFromHex(fileID)
	if err != nil {
		return fmt.Errorf("invalid file ID: %w", err)
	}

	userObjID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return fmt.Errorf("invalid user ID: %w", err)
	}

	// Find the file to get its B2 file ID
	var file models.File
	err = s.fileCollection.FindOne(ctx, bson.M{
		"_id":        fileObjID,
		"owner_id":   userObjID,
		"deleted_at": bson.M{"$ne": nil},
	}).Decode(&file)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return fmt.Errorf("file not found in trash")
		}
		return fmt.Errorf("failed to find file: %w", err)
	}

	// Delete from B2 storage
	if s.b2Service != nil && file.B2FileID != "" {
		err = s.b2Service.DeleteFile(file.B2FileID)
		if err != nil {
			// Log the error but don't fail the operation
			fmt.Printf("Warning: failed to delete file from B2 storage: %v\n", err)
		}
	}

	// Delete from database
	result, err := s.fileCollection.DeleteOne(ctx, bson.M{
		"_id":      fileObjID,
		"owner_id": userObjID,
	})
	if err != nil {
		return fmt.Errorf("failed to delete file from database: %w", err)
	}

	if result.DeletedCount == 0 {
		return fmt.Errorf("file not found")
	}

	return nil
}

func (s *TrashService) PurgeFolder(folderID, userID string) error {
	ctx := context.Background()

	// Convert IDs to ObjectID
	folderObjID, err := primitive.ObjectIDFromHex(folderID)
	if err != nil {
		return fmt.Errorf("invalid folder ID: %w", err)
	}

	userObjID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return fmt.Errorf("invalid user ID: %w", err)
	}

	// Find the folder
	var folder models.Folder
	err = s.folderCollection.FindOne(ctx, bson.M{
		"_id":        folderObjID,
		"owner_id":   userObjID,
		"deleted_at": bson.M{"$ne": nil},
	}).Decode(&folder)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return fmt.Errorf("folder not found in trash")
		}
		return fmt.Errorf("failed to find folder: %w", err)
	}

	// Start a session for transaction
	session, err := s.folderCollection.Database().Client().StartSession()
	if err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}
	defer session.EndSession(ctx)

	// Use transaction to delete folder and its contents
	_, err = session.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {
		// Get all files in this folder and subfolders for B2 deletion
		if s.b2Service != nil {
			fileCursor, err := s.fileCollection.Find(sc, bson.M{
				"relative_path": bson.M{"$regex": "^" + folder.Path + "/"},
				"owner_id":      userObjID,
			})
			if err == nil {
				defer fileCursor.Close(sc)

				var files []models.File
				if err = fileCursor.All(sc, &files); err == nil {
					for _, file := range files {
						if file.B2FileID != "" {
							err = s.b2Service.DeleteFile(file.B2FileID)
							if err != nil {
								fmt.Printf("Warning: failed to delete file %s from B2 storage: %v\n", file.Name, err)
							}
						}
					}
				}
			}
		}

		// Delete all files in this folder and subfolders
		_, err = s.fileCollection.DeleteMany(sc, bson.M{
			"relative_path": bson.M{"$regex": "^" + folder.Path + "/"},
			"owner_id":      userObjID,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to delete files in folder: %w", err)
		}

		// Delete all child folders
		_, err = s.folderCollection.DeleteMany(sc, bson.M{
			"path":     bson.M{"$regex": "^" + folder.Path + "/"},
			"owner_id": userObjID,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to delete child folders: %w", err)
		}

		// Delete the folder itself
		result, err := s.folderCollection.DeleteOne(sc, bson.M{
			"_id":      folderObjID,
			"owner_id": userObjID,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to delete folder: %w", err)
		}

		if result.DeletedCount == 0 {
			return nil, fmt.Errorf("folder not found")
		}

		return nil, nil
	})

	return err
}

func (s *TrashService) PurgeAllTrash(userID string) (int64, error) {
	ctx := context.Background()

	// Convert userID to ObjectID
	userObjID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return 0, fmt.Errorf("invalid user ID: %w", err)
	}

	var totalDeleted int64

	// Start a session for transaction
	session, err := s.fileCollection.Database().Client().StartSession()
	if err != nil {
		return 0, fmt.Errorf("failed to start session: %w", err)
	}
	defer session.EndSession(ctx)

	// Use transaction to delete all trash items
	_, err = session.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {
		// Get all deleted files for B2 cleanup
		if s.b2Service != nil {
			fileCursor, err := s.fileCollection.Find(sc, bson.M{
				"owner_id":   userObjID,
				"deleted_at": bson.M{"$ne": nil},
			})
			if err == nil {
				defer fileCursor.Close(sc)

				var files []models.File
				if err = fileCursor.All(sc, &files); err == nil {
					for _, file := range files {
						if file.B2FileID != "" {
							err = s.b2Service.DeleteFile(file.B2FileID)
							if err != nil {
								fmt.Printf("Warning: failed to delete file %s from B2 storage: %v\n", file.Name, err)
							}
						}
					}
				}
			}
		}

		// Delete all deleted files
		fileResult, err := s.fileCollection.DeleteMany(sc, bson.M{
			"owner_id":   userObjID,
			"deleted_at": bson.M{"$ne": nil},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to delete files from trash: %w", err)
		}

		// Delete all deleted folders
		folderResult, err := s.folderCollection.DeleteMany(sc, bson.M{
			"owner_id":   userObjID,
			"deleted_at": bson.M{"$ne": nil},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to delete folders from trash: %w", err)
		}

		totalDeleted = fileResult.DeletedCount + folderResult.DeletedCount
		return nil, nil
	})

	return totalDeleted, err
}

func (s *TrashService) EmptyTrash(userID string) (int64, error) {
	// EmptyTrash is an alias for PurgeAllTrash
	return s.PurgeAllTrash(userID)
}

// AutoPurgeExpiredItems removes items that have been in trash for more than 30 days
func (s *TrashService) AutoPurgeExpiredItems() error {
	ctx := context.Background()
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30)

	// Start a session for transaction
	session, err := s.fileCollection.Database().Client().StartSession()
	if err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}
	defer session.EndSession(ctx)

	_, err = session.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {
		// Get expired files for B2 cleanup
		if s.b2Service != nil {
			fileCursor, err := s.fileCollection.Find(sc, bson.M{
				"deleted_at": bson.M{
					"$ne":  nil,
					"$lte": thirtyDaysAgo,
				},
			})
			if err == nil {
				defer fileCursor.Close(sc)

				var files []models.File
				if err = fileCursor.All(sc, &files); err == nil {
					for _, file := range files {
						if file.B2FileID != "" {
							err = s.b2Service.DeleteFile(file.B2FileID)
							if err != nil {
								fmt.Printf("Warning: failed to delete expired file %s from B2 storage: %v\n", file.Name, err)
							}
						}
					}
				}
			}
		}

		// Delete expired files
		_, err = s.fileCollection.DeleteMany(sc, bson.M{
			"deleted_at": bson.M{
				"$ne":  nil,
				"$lte": thirtyDaysAgo,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to auto-purge expired files: %w", err)
		}

		// Delete expired folders
		_, err = s.folderCollection.DeleteMany(sc, bson.M{
			"deleted_at": bson.M{
				"$ne":  nil,
				"$lte": thirtyDaysAgo,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to auto-purge expired folders: %w", err)
		}

		return nil, nil
	})

	return err
}

// StartTrashCleanupJob initializes a background job that periodically purges expired trash items
func StartTrashCleanupJob(trashService *TrashService, interval time.Duration) {
	ticker := time.NewTicker(interval)
	quit := make(chan struct{})

	go func() {
		for {
			select {
			case <-ticker.C:
				log.Println("Running trash cleanup job...")
				err := trashService.AutoPurgeExpiredItems()
				if err != nil {
					log.Printf("Trash cleanup job failed: %v", err)
				} else {
					log.Println("Trash cleanup job completed successfully")
				}
			case <-quit:
				ticker.Stop()
				return
			}
		}
	}()

}
