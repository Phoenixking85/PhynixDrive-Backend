package jobs

import (
	"context"
	"fmt"
	"log"
	"phynixdrive/config"
	"phynixdrive/models"
	"phynixdrive/services"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type TrashCleaner struct {
	db            *mongo.Database
	b2Service     *services.B2Service
	fileService   *services.FileService
	folderService *services.FolderService
	logger        *log.Logger
}

func NewTrashCleaner() *TrashCleaner {
	db := config.DB // Assumes config.DB is set during initialization

	// Use loaded config
	cfg := config.AppConfig

	b2Service, err := services.NewB2Service(cfg.B2ApplicationKeyID, cfg.B2ApplicationKey, cfg.B2BucketName)
	if err != nil {
		log.Fatalf("Failed to initialize B2Service: %v", err)
	}

	permissionService := services.NewPermissionService(db)
	folderService := services.NewFolderService(db, permissionService, b2Service)
	fileService := services.NewFileService(db, folderService, b2Service, permissionService)

	return &TrashCleaner{
		db:            db,
		b2Service:     b2Service,
		fileService:   fileService,
		folderService: folderService,
		logger:        log.New(log.Writer(), "[TRASH_CLEANER] ", log.LstdFlags),
	}
}

// StartTrashCleaner runs the trash cleanup job every hour
func (tc *TrashCleaner) StartTrashCleaner() {
	tc.logger.Println("Starting trash cleaner job...")

	// Run cleanup immediately on start
	tc.runCleanup()

	// Then run every hour
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		tc.runCleanup()
	}

}

func (tc *TrashCleaner) runCleanup() {
	tc.logger.Println("Running trash cleanup...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Calculate cutoff date (30 days ago)
	cutoffDate := time.Now().AddDate(0, 0, -30)

	// Clean up files
	filesDeleted, err := tc.cleanupFiles(ctx, cutoffDate)
	if err != nil {
		tc.logger.Printf("Error cleaning up files: %v", err)
	} else {
		tc.logger.Printf("Cleaned up %d files", filesDeleted)
	}

	// Clean up folders
	foldersDeleted, err := tc.cleanupFolders(ctx, cutoffDate)
	if err != nil {
		tc.logger.Printf("Error cleaning up folders: %v", err)
	} else {
		tc.logger.Printf("Cleaned up %d folders", foldersDeleted)
	}

	tc.logger.Printf("Trash cleanup completed. Files: %d, Folders: %d", filesDeleted, foldersDeleted)
}

func (tc *TrashCleaner) cleanupFiles(ctx context.Context, cutoffDate time.Time) (int, error) {
	filesCollection := tc.db.Collection("files")

	// Find files deleted before cutoff date
	filter := bson.M{
		"deletedAt": bson.M{
			"$ne":  nil,
			"$lte": cutoffDate,
		},
	}

	cursor, err := filesCollection.Find(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("failed to find expired files: %w", err)
	}
	defer cursor.Close(ctx)

	var deletedCount int
	var filesToDelete []models.File

	// Collect files to delete
	for cursor.Next(ctx) {
		var file models.File
		if err := cursor.Decode(&file); err != nil {
			tc.logger.Printf("Error decoding file: %v", err)
			continue
		}
		filesToDelete = append(filesToDelete, file)
	}

	// Delete files from B2 and MongoDB
	for _, file := range filesToDelete {
		// Delete from Backblaze B2
		if err := tc.b2Service.DeleteFile(file.B2FileID); err != nil {
			tc.logger.Printf("Failed to delete file from B2: %s, error: %v", file.B2FileID, err)
			continue
		}

		// Delete all versions from B2
		for _, version := range file.Versions {
			if err := tc.b2Service.DeleteFile(version.B2FileID); err != nil {
				tc.logger.Printf("Failed to delete file version from B2: %s, error: %v", version.B2FileID, err)
			}
		}

		// Delete from MongoDB
		_, err := filesCollection.DeleteOne(ctx, bson.M{"_id": file.ID})
		if err != nil {
			tc.logger.Printf("Failed to delete file from MongoDB: %v", err)
			continue
		}

		// Update user storage usage
		err = tc.updateUserStorage(ctx, file.OwnerID.Hex(), -file.Size)
		if err != nil {
			tc.logger.Printf("Failed to update user storage for file deletion: %v", err)
		}

		deletedCount++
		tc.logger.Printf("Permanently deleted file: %s (%s)", file.Name, file.ID.Hex())
	}

	return deletedCount, nil
}

func (tc *TrashCleaner) cleanupFolders(ctx context.Context, cutoffDate time.Time) (int, error) {
	foldersCollection := tc.db.Collection("folders")

	// Find folders deleted before cutoff date
	filter := bson.M{
		"deletedAt": bson.M{
			"$ne":  nil,
			"$lte": cutoffDate,
		},
	}

	cursor, err := foldersCollection.Find(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("failed to find expired folders: %w", err)
	}
	defer cursor.Close(ctx)

	var deletedCount int

	for cursor.Next(ctx) {
		var folder models.Folder
		if err := cursor.Decode(&folder); err != nil {
			tc.logger.Printf("Error decoding folder: %v", err)
			continue
		}

		// Delete folder from MongoDB
		_, err := foldersCollection.DeleteOne(ctx, bson.M{"_id": folder.ID})
		if err != nil {
			tc.logger.Printf("Failed to delete folder from MongoDB: %v", err)
			continue
		}

		deletedCount++
		tc.logger.Printf("Permanently deleted folder: %s (%s)", folder.Name, folder.ID.Hex())
	}

	return deletedCount, nil
}

func (tc *TrashCleaner) updateUserStorage(ctx context.Context, userID string, sizeChange int64) error {
	usersCollection := tc.db.Collection("users")

	filter := bson.M{"_id": userID}
	update := bson.M{
		"$inc": bson.M{"usedStorage": sizeChange},
	}

	_, err := usersCollection.UpdateOne(ctx, filter, update)
	return err
}
