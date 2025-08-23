package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"phynixdrive/models"
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

type MailgunMessage struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Subject string `json:"subject"`
	Text    string `json:"text"`
	HTML    string `json:"html"`
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

func (s *NotificationService) SendFileSharedNotification(sharedWithUserID, sharedByUserID, fileName string) error {
	var sharedWithUser, sharedByUser models.User

	sharedWithObjID, err := primitive.ObjectIDFromHex(sharedWithUserID)
	if err != nil {
		return fmt.Errorf("invalid shared with user ID: %w", err)
	}

	sharedByObjID, err := primitive.ObjectIDFromHex(sharedByUserID)
	if err != nil {
		return fmt.Errorf("invalid shared by user ID: %w", err)
	}

	err = s.userCollection.FindOne(context.Background(), bson.M{"_id": sharedWithObjID}).Decode(&sharedWithUser)
	if err != nil {
		return fmt.Errorf("shared with user not found: %w", err)
	}

	err = s.userCollection.FindOne(context.Background(), bson.M{"_id": sharedByObjID}).Decode(&sharedByUser)
	if err != nil {
		return fmt.Errorf("shared by user not found: %w", err)
	}

	subject := fmt.Sprintf("File shared with you: %s", fileName)
	text := fmt.Sprintf("Hi %s,\n\n%s has shared a file with you: %s\n\nYou can access it in your PhynixDrive account.\n\nBest regards,\nPhynixDrive Team",
		sharedWithUser.Name, sharedByUser.Name, fileName)

	html := fmt.Sprintf(`
		<h2>File Shared With You</h2>
		<p>Hi %s,</p>
		<p><strong>%s</strong> has shared a file with you: <strong>%s</strong></p>
		<p>You can access it in your PhynixDrive account.</p>
		<p>Best regards,<br>PhynixDrive Team</p>
	`, sharedWithUser.Name, sharedByUser.Name, fileName)

	err = s.sendEmail(sharedWithUser.Email, subject, text, html)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	notification := models.NotificationLog{
		ID:      primitive.NewObjectID(),
		UserID:  sharedWithObjID,
		Type:    "file_shared",
		Message: text,
	}

	_, err = s.notificationCollection.InsertOne(context.Background(), notification)
	if err != nil {
		return fmt.Errorf("failed to log notification: %w", err)
	}

	return nil
}

func (s *NotificationService) SendFolderSharedNotification(sharedWithUserID, sharedByUserID, folderName string) error {
	var sharedWithUser, sharedByUser models.User

	sharedWithObjID, err := primitive.ObjectIDFromHex(sharedWithUserID)
	if err != nil {
		return fmt.Errorf("invalid shared with user ID: %w", err)
	}

	sharedByObjID, err := primitive.ObjectIDFromHex(sharedByUserID)
	if err != nil {
		return fmt.Errorf("invalid shared by user ID: %w", err)
	}

	err = s.userCollection.FindOne(context.Background(), bson.M{"_id": sharedWithObjID}).Decode(&sharedWithUser)
	if err != nil {
		return fmt.Errorf("shared with user not found: %w", err)
	}

	err = s.userCollection.FindOne(context.Background(), bson.M{"_id": sharedByObjID}).Decode(&sharedByUser)
	if err != nil {
		return fmt.Errorf("shared by user not found: %w", err)
	}

	subject := fmt.Sprintf("Folder shared with you: %s", folderName)
	text := fmt.Sprintf("Hi %s,\n\n%s has shared a folder with you: %s\n\nYou can access it in your PhynixDrive account.\n\nBest regards,\nPhynixDrive Team",
		sharedWithUser.Name, sharedByUser.Name, folderName)

	html := fmt.Sprintf(`
		<h2>Folder Shared With You</h2>
		<p>Hi %s,</p>
		<p><strong>%s</strong> has shared a folder with you: <strong>%s</strong></p>
		<p>You can access it in your PhynixDrive account.</p>
		<p>Best regards,<br>PhynixDrive Team</p>
	`, sharedWithUser.Name, sharedByUser.Name, folderName)

	err = s.sendEmail(sharedWithUser.Email, subject, text, html)
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	notification := models.NotificationLog{
		ID:      primitive.NewObjectID(),
		UserID:  sharedWithObjID,
		Type:    "folder_shared",
		Message: text,
	}

	_, err = s.notificationCollection.InsertOne(context.Background(), notification)
	if err != nil {
		return fmt.Errorf("failed to log notification: %w", err)
	}

	return nil
}

func (s *NotificationService) sendEmail(to, subject, text, html string) error {
	url := fmt.Sprintf("https://api.mailgun.net/v3/%s/messages", s.mailgunDomain)

	payload := MailgunMessage{
		From:    s.fromEmail,
		To:      to,
		Subject: subject,
		Text:    text,
		HTML:    html,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal mailgun message: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create mailgun request: %w", err)
	}

	req.SetBasicAuth("api", s.mailgunAPIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send mailgun request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("mailgun responded with status: %s", resp.Status)
	}

	return nil

}
