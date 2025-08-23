package services

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"

	"github.com/kurin/blazer/b2"
)

type B2Service struct {
	client     *b2.Client
	bucketName string
	bucket     *b2.Bucket
}

type UploadResult struct {
	FileID      string
	FileName    string
	DownloadURL string // Signed URL for download (longer expiry)
	PreviewURL  string // Signed URL for preview (shorter expiry)
	Size        int64
	SHA1        string
}

type URLType string

const (
	URLTypeDownload URLType = "download"
	URLTypePreview  URLType = "preview"
)

func NewB2Service(keyID, applicationKey, bucketName string) (*B2Service, error) {
	ctx := context.Background()

	client, err := b2.NewClient(ctx, keyID, applicationKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create B2 client: %w", err)
	}

	bucket, err := client.Bucket(ctx, bucketName)
	if err != nil {
		return nil, fmt.Errorf("failed to get bucket %s: %w", bucketName, err)
	}

	return &B2Service{
		client:     client,
		bucketName: bucketName,
		bucket:     bucket,
	}, nil
}

func (s *B2Service) UploadFile(file multipart.File, filename string, userID string, relativePath string) (*UploadResult, error) {
	ctx := context.Background()

	// Create object path
	cleanPath := strings.TrimPrefix(relativePath, "/")
	objectName := fmt.Sprintf("users/%s/%s", userID, cleanPath)
	if cleanPath == "" {
		objectName = fmt.Sprintf("users/%s/%s", userID, filename)
	}

	// Create a B2 writer
	obj := s.bucket.Object(objectName)
	writer := obj.NewWriter(ctx)
	// writer.ContentType = s.getContentType(filename)

	// Instead of reading into memory, stream directly
	hasher := sha1.New()
	multiWriter := io.MultiWriter(writer, hasher)

	// Copy from request → B2 → hash calculator
	if _, err := io.Copy(multiWriter, file); err != nil {
		writer.Close()
		return nil, fmt.Errorf("failed to upload file to B2: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close B2 writer: %w", err)
	}

	sha1Hash := hex.EncodeToString(hasher.Sum(nil))

	downloadURL, err := s.GetSignedURL(objectName, URLTypeDownload)
	if err != nil {
		return nil, err
	}
	previewURL, err := s.GetSignedURL(objectName, URLTypePreview)
	if err != nil {
		return nil, err
	}

	return &UploadResult{
		FileID:      objectName,
		FileName:    filename,
		DownloadURL: downloadURL,
		PreviewURL:  previewURL,
		SHA1:        sha1Hash,
	}, nil
}

// GetSignedURL generates a signed URL based on the type (download or preview)
func (s *B2Service) GetSignedURL(objectName string, urlType URLType) (string, error) {
	var duration time.Duration

	switch urlType {
	case URLTypeDownload:
		duration = 24 * time.Hour // 24 hours for download
	case URLTypePreview:
		duration = 1 * time.Hour // 1 hour for preview
	default:
		duration = 1 * time.Hour
	}

	return s.GetDownloadURL(objectName, duration)
}

// GetDownloadURL generates a signed download URL for private buckets
func (s *B2Service) GetDownloadURL(objectName string, duration time.Duration) (string, error) {
	ctx := context.Background()
	obj := s.bucket.Object(objectName)

	// Generate signed URL for GET requests
	urlObj, err := obj.AuthURL(ctx, duration, "GET")
	if err != nil {
		return "", fmt.Errorf("failed to generate signed URL: %w", err)
	}

	return urlObj.String(), nil
}

// GetDownloadURLWithHeaders generates a signed URL with custom headers for download
func (s *B2Service) GetDownloadURLWithHeaders(objectName, filename string, duration time.Duration) (string, error) {
	ctx := context.Background()
	obj := s.bucket.Object(objectName)

	// For download, we want to force download with proper filename
	// Note: B2 doesn't support custom response headers in signed URLs directly
	// This would need to be handled at the application level
	urlObj, err := obj.AuthURL(ctx, duration, "GET")
	if err != nil {
		return "", fmt.Errorf("failed to generate signed download URL: %w", err)
	}

	return urlObj.String(), nil
}

// GetPreviewURL generates a signed URL optimized for preview (inline display)
func (s *B2Service) GetPreviewURL(objectName string) (string, error) {
	return s.GetSignedURL(objectName, URLTypePreview)
}

// GetDownloadURLForFile generates a download URL optimized for file download
func (s *B2Service) GetDownloadURLForFile(objectName string) (string, error) {
	return s.GetSignedURL(objectName, URLTypeDownload)
}

// RefreshURLs generates fresh URLs for both download and preview
func (s *B2Service) RefreshURLs(objectName string) (downloadURL, previewURL string, err error) {
	downloadURL, err = s.GetSignedURL(objectName, URLTypeDownload)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate download URL: %w", err)
	}

	previewURL, err = s.GetSignedURL(objectName, URLTypePreview)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate preview URL: %w", err)
	}

	return downloadURL, previewURL, nil
}

func (s *B2Service) DeleteFile(objectName string) error {
	ctx := context.Background()
	obj := s.bucket.Object(objectName)

	if err := obj.Delete(ctx); err != nil {
		return fmt.Errorf("failed to delete file from B2: %w", err)
	}
	return nil
}

// IsPreviewableFile checks if a file can be previewed in browser
func (s *B2Service) IsPreviewableFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	previewableExts := map[string]bool{
		".jpg":  true,
		".jpeg": true,
		".png":  true,
		".gif":  true,
		".pdf":  true,
		".txt":  true,
		".mp4":  true,
		".mp3":  true,
	}
	return previewableExts[ext]
}
