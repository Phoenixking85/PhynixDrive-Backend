package services

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"phynixdrive/models"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type NotificationService struct {
	notificationCollection *mongo.Collection
	userCollection         *mongo.Collection
	mailgunAPIKey          string
	mailgunDomain          string
	fromEmail              string
}

func NewNotificationService(db *mongo.Database, mailgunAPIKey, mailgunDomain, fromEmail string) *NotificationService {
	return &NotificationService{
		notificationCollection: db.Collection("notification_logs"),
		userCollection:         db.Collection("users"),
		mailgunAPIKey:          mailgunAPIKey,
		mailgunDomain:          mailgunDomain,
		fromEmail:              fromEmail,
	}
}

// --- Public API ---

func (s *NotificationService) SendFileSharedNotification(ctx context.Context, sharedWithUserID, sharedByUserID, fileName string) error {
	subject := fmt.Sprintf("File shared with you: %s", fileName)
	text := fmt.Sprintf("A file has been shared with you: %s", fileName)
	html := fmt.Sprintf("<h2>File Shared With You</h2><p>A file has been shared with you: <b>%s</b></p>", fileName)

	return s.sendSharedNotification(ctx, sharedWithUserID, sharedByUserID, subject, text, html, "file_shared")
}

func (s *NotificationService) SendFolderSharedNotification(ctx context.Context, sharedWithUserID, sharedByUserID, folderName string) error {
	subject := fmt.Sprintf("Folder shared with you: %s", folderName)
	text := fmt.Sprintf("A folder has been shared with you: %s", folderName)
	html := fmt.Sprintf("<h2>Folder Shared With You</h2><p>A folder has been shared with you: <b>%s</b></p>", folderName)

	return s.sendSharedNotification(ctx, sharedWithUserID, sharedByUserID, subject, text, html, "folder_shared")
}

// --- Private Helpers ---

func (s *NotificationService) sendSharedNotification(ctx context.Context, sharedWithUserID, sharedByUserID, subject, text, html, notifType string) error {
	var sharedWithUser, sharedByUser models.User

	// Parse ObjectIDs
	sharedWithObjID, err := primitive.ObjectIDFromHex(sharedWithUserID)
	if err != nil {
		return fmt.Errorf("invalid sharedWith user ID: %w", err)
	}
	sharedByObjID, err := primitive.ObjectIDFromHex(sharedByUserID)
	if err != nil {
		return fmt.Errorf("invalid sharedBy user ID: %w", err)
	}

	// Lookup users
	if err := s.userCollection.FindOne(ctx, bson.M{"_id": sharedWithObjID}).Decode(&sharedWithUser); err != nil {
		return fmt.Errorf("sharedWith user not found: %w", err)
	}
	if err := s.userCollection.FindOne(ctx, bson.M{"_id": sharedByObjID}).Decode(&sharedByUser); err != nil {
		return fmt.Errorf("sharedBy user not found: %w", err)
	}

	// Personalize message
	textBody := fmt.Sprintf("Hi %s,\n\n%s has shared something with you: %s\n\nBest,\nPhynixDrive Team",
		sharedWithUser.Name, sharedByUser.Name, text)
	htmlBody := fmt.Sprintf("<p>Hi %s,</p><p><strong>%s</strong> has shared something with you.</p>%s<p>Best regards,<br>PhynixDrive Team</p>",
		sharedWithUser.Name, sharedByUser.Name, html)

	// Send email
	if err := s.sendEmail(ctx, sharedWithUser.Email, subject, textBody, htmlBody); err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	// Log notification
	notification := models.NotificationLog{
		ID:        primitive.NewObjectID(),
		UserID:    sharedWithObjID,
		Type:      notifType,
		Message:   textBody,
		CreatedAt: time.Now(),
	}

	if _, err := s.notificationCollection.InsertOne(ctx, notification); err != nil {
		return fmt.Errorf("failed to log notification: %w", err)
	}

	return nil
}

func (s *NotificationService) sendEmail(ctx context.Context, to, subject, text, html string) error {
	apiURL := fmt.Sprintf("https://api.mailgun.net/v3/%s/messages", s.mailgunDomain)

	data := url.Values{}
	data.Set("from", s.fromEmail)
	data.Set("to", to)
	data.Set("subject", subject)
	data.Set("text", text)
	data.Set("html", html)

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.SetBasicAuth("api", s.mailgunAPIKey)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request to Mailgun: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("mailgun error: %s - %s", resp.Status, string(body))
	}

	return nil
}
