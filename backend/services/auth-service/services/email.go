package services

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net/smtp"
	"os"
)

//Helper functions for verification code generation and email sending

func GenerateRandomCode(length int) string {
	// logger.Debug(context.Background(), "Generating random code", "length", length)
	code := ""

	for i := 0; i < length; i++ {
		code += fmt.Sprintf("%d", rand.Intn(10))
	}

	return code
}

// Helper function to send verification email
func SendVerificationEmail(to string, code string) error {
	log.Println(context.Background(), "Sending verification email", "to", to)

	from := os.Getenv("SMTP_EMAIL")
	password := os.Getenv("SMTP_PASSWORD")
	smtpServer := "smtp.gmail.com"
	port := "587"

	log.Println(password, "SMTP password loaded from environment")

	if from == "" || password == "" {
		log.Println(context.Background(), "SMTP configuration missing", nil)
		return fmt.Errorf("SMTP configuration is missing")
	}

	// Set up email content
	subject := "Email Verification"
	body := fmt.Sprintf("Your verification code is: %s", code)
	message := []byte("Subject: " + subject + "\r\n" + "To: " + to + "\r\n" + "\r\n" + body)

	// Auth configuration for SMTP
	auth := smtp.PlainAuth("", from, password, smtpServer)
	err := smtp.SendMail(smtpServer+":"+port, auth, from, []string{to}, message)
	if err != nil {
		log.Println(context.Background(), "Failed to send verification email", err)
		return err
	}

	log.Println(context.Background(), "Verification email sent successfully", "to", to)
	return nil
}
