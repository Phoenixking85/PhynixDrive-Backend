package utils

import (
	"fmt"
	"mime/multipart"
	"path/filepath"
	"phynixdrive/config"
	"regexp"
	"strings"
	"unicode/utf8"
)

// File validation
func ValidateFileSize(size int64) error {
	if size > config.AppConfig.MaxFileSize {
		return fmt.Errorf("file size %d bytes exceeds maximum allowed size of %d bytes", size, config.AppConfig.MaxFileSize)
	}
	return nil
}

func ValidateFileName(filename string) error {
	if filename == "" {
		return fmt.Errorf("filename cannot be empty")
	}

	if len(filename) > 255 {
		return fmt.Errorf("filename too long (max 255 characters)")
	}

	if !utf8.ValidString(filename) {
		return fmt.Errorf("filename contains invalid UTF-8 characters")
	}

	// Check for invalid characters
	invalidChars := []string{"<", ">", ":", "\"", "|", "?", "*", "\x00"}
	for _, char := range invalidChars {
		if strings.Contains(filename, char) {
			return fmt.Errorf("filename contains invalid character: %s", char)
		}
	}

	// Check for reserved names (Windows)
	reservedNames := []string{"CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9"}
	nameWithoutExt := strings.TrimSuffix(filename, filepath.Ext(filename))
	for _, reserved := range reservedNames {
		if strings.EqualFold(nameWithoutExt, reserved) {
			return fmt.Errorf("filename uses reserved name: %s", reserved)
		}
	}
	return nil
}

func ValidateFileHeader(header *multipart.FileHeader) error {
	if err := ValidateFileName(header.Filename); err != nil {
		return err
	}

	if err := ValidateFileSize(header.Size); err != nil {
		return err
	}

	return nil
}

// Folder validation
func ValidateFolderName(name string) error {
	if name == "" {
		return fmt.Errorf("folder name cannot be empty")
	}

	if len(name) > 255 {
		return fmt.Errorf("folder name too long (max 255 characters)")
	}

	if !utf8.ValidString(name) {
		return fmt.Errorf("folder name contains invalid UTF-8 characters")
	}

	// Check for invalid characters (same as file validation)
	invalidChars := []string{"<", ">", ":", "\"", "|", "?", "*", "\x00", "/", "\\"}
	for _, char := range invalidChars {
		if strings.Contains(name, char) {
			return fmt.Errorf("folder name contains invalid character: %s", char)
		}
	}

	// Check for dots at the end (Windows issue)
	if strings.HasSuffix(name, ".") {
		return fmt.Errorf("folder name cannot end with a dot")
	}

	return nil
}

func ValidateRelativePath(path string) error {
	if path == "" {
		return nil // Empty path is valid (root)
	}

	// Normalize path separators
	path = strings.ReplaceAll(path, "\\", "/")

	// Check for invalid patterns
	if strings.Contains(path, "..") {
		return fmt.Errorf("relative path cannot contain '..' (parent directory references)")
	}

	if strings.HasPrefix(path, "/") {
		return fmt.Errorf("relative path cannot start with '/'")
	}

	// Validate each path segment
	segments := strings.Split(path, "/")
	for _, segment := range segments {
		if segment != "" { // Skip empty segments (from double slashes)
			if err := ValidateFolderName(segment); err != nil {
				return fmt.Errorf("invalid path segment '%s': %v", segment, err)
			}
		}
	}

	return nil
}

// Email validation
func ValidateEmail(email string) error {
	if email == "" {
		return fmt.Errorf("email cannot be empty")
	}

	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(email) {
		return fmt.Errorf("invalid email format")
	}

	return nil
}

// Permission validation
func ValidatePermissionRole(role string) error {
	allowedRoles := []string{"viewer", "editor", "admin"}
	for _, allowedRole := range allowedRoles {
		if role == allowedRole {
			return nil
		}
	}
	return fmt.Errorf("invalid role: %s. Allowed roles: %s", role, strings.Join(allowedRoles, ", "))
}

// Storage validation
func ValidateStorageQuota(currentUsage, additionalSize, maxStorage int64) error {
	if currentUsage+additionalSize > maxStorage {
		return fmt.Errorf("storage quota exceeded. Current: %d bytes, Additional: %d bytes, Max: %d bytes",
			currentUsage, additionalSize, maxStorage)
	}
	return nil
}
