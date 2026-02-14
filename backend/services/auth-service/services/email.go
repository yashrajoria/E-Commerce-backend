package services

import (
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"net/smtp"
	"os"
	"strings"
)

// Helper functions for verification code generation and email sending

func GenerateRandomCode(length int) string {
	code := ""
	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			// Fallback to 0 in the unlikely event of entropy failure
			code += "0"
			continue
		}
		code += n.String()
	}
	return code
}

// EmailConfig holds SMTP configuration
type EmailConfig struct {
	SmtpServer  string
	SmtpPort    string
	SenderEmail string
	SenderPass  string
	SenderName  string
}

// LoadEmailConfig loads email configuration from environment variables
func LoadEmailConfig() (*EmailConfig, error) {
	config := &EmailConfig{
		SmtpServer:  os.Getenv("SMTP_SERVER"),
		SmtpPort:    os.Getenv("SMTP_PORT"),
		SenderEmail: os.Getenv("SMTP_EMAIL"),
		SenderPass:  os.Getenv("SMTP_PASSWORD"),
		SenderName:  os.Getenv("SMTP_SENDER_NAME"),
	}

	// Set defaults
	if config.SmtpServer == "" {
		config.SmtpServer = "smtp.gmail.com"
	}
	if config.SmtpPort == "" {
		config.SmtpPort = "587"
	}
	if config.SenderName == "" {
		config.SenderName = "ShopSwift"
	}

	// Validate required fields
	if config.SenderEmail == "" {
		return nil, fmt.Errorf("SMTP_EMAIL environment variable not set")
	}
	if config.SenderPass == "" {
		return nil, fmt.Errorf("SMTP_PASSWORD environment variable not set")
	}

	return config, nil
}

// SendVerificationEmail sends a verification code email to the user
func SendVerificationEmail(to string, code string) error {
	// Load email config
	emailConfig, err := LoadEmailConfig()
	if err != nil {
		log.Printf("Failed to load email config: %v", err)
		return err
	}

	// Build email message with HTML content
	subject := "Email Verification - ShopSwift"
	htmlBody := buildVerificationEmailHTML(code)
	from := fmt.Sprintf("%s <%s>", emailConfig.SenderName, emailConfig.SenderEmail)

	// Create MIME headers
	headers := map[string]string{
		"From":                      from,
		"To":                        to,
		"Subject":                   subject,
		"MIME-Version":              "1.0",
		"Content-Type":              "text/html; charset=UTF-8",
		"Content-Transfer-Encoding": "8bit",
	}

	// Build message with headers
	message := ""
	for key, value := range headers {
		message += fmt.Sprintf("%s: %s\r\n", key, value)
	}
	message += "\r\n" + htmlBody

	// Set up SMTP authentication
	auth := smtp.PlainAuth("", emailConfig.SenderEmail, emailConfig.SenderPass, emailConfig.SmtpServer)

	// Send email
	err = smtp.SendMail(
		emailConfig.SmtpServer+":"+emailConfig.SmtpPort,
		auth,
		emailConfig.SenderEmail,
		[]string{to},
		[]byte(message),
	)

	if err != nil {
		log.Printf("Failed to send verification email to %s: %v", to, err)
		return fmt.Errorf("failed to send verification email: %w", err)
	}

	log.Printf("Verification email sent successfully to %s", to)
	return nil
}

// buildVerificationEmailHTML creates an HTML email template for verification
func buildVerificationEmailHTML(code string) string {
	html := `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <style>
        body {
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
            background-color: #f4f4f4;
            margin: 0;
            padding: 0;
        }
        .container {
            max-width: 600px;
            margin: 50px auto;
            background-color: #ffffff;
            border-radius: 8px;
            box-shadow: 0 2px 10px rgba(0,0,0,0.1);
            overflow: hidden;
        }
        .header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            padding: 30px;
            text-align: center;
            color: white;
        }
        .header h1 {
            margin: 0;
            font-size: 28px;
        }
        .content {
            padding: 30px;
            color: #333;
        }
        .content p {
            font-size: 16px;
            line-height: 1.6;
            margin: 10px 0;
        }
        .code-box {
            background-color: #f8f9fa;
            border-left: 4px solid #667eea;
            padding: 20px;
            margin: 30px 0;
            border-radius: 4px;
        }
        .code-box .label {
            font-size: 12px;
            color: #666;
            text-transform: uppercase;
            letter-spacing: 1px;
            margin-bottom: 10px;
        }
        .code-box .code {
            font-size: 32px;
            font-weight: bold;
            color: #667eea;
            letter-spacing: 4px;
            text-align: center;
            font-family: 'Courier New', monospace;
        }
        .footer {
            background-color: #f4f4f4;
            padding: 20px;
            text-align: center;
            font-size: 12px;
            color: #999;
            border-top: 1px solid #e0e0e0;
        }
        .warning {
            background-color: #fff3cd;
            border: 1px solid #ffc107;
            padding: 15px;
            border-radius: 4px;
            margin: 20px 0;
            font-size: 14px;
            color: #856404;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>ShopSwift</h1>
        </div>
        <div class="content">
            <p>Hello,</p>
            <p>Thank you for creating an account with <strong>ShopSwift</strong>. To complete your registration, please verify your email address using the code below:</p>
            
            <div class="code-box">
                <div class="label">Your Verification Code</div>
                <div class="code">` + code + `</div>
            </div>
            
            <p>This code will expire in <strong>15 minutes</strong>.</p>
            
            <div class="warning">
                <strong>⚠️ Security Notice:</strong> Never share this code with anyone. ShopSwift staff will never ask for this code.
            </div>
            
            <p>If you did not create this account, please ignore this email.</p>
            
            <p>Best regards,<br><strong>The ShopSwift Team</strong></p>
        </div>
        <div class="footer">
            <p>© 2026 ShopSwift. All rights reserved.</p>
            <p>This is an automated message, please do not reply directly to this email.</p>
        </div>
    </div>
</body>
</html>
`
	return strings.TrimSpace(html)
}

// SendPasswordResetEmail sends a password reset link to the user (for future use)
func SendPasswordResetEmail(to, resetToken string) error {
	emailConfig, err := LoadEmailConfig()
	if err != nil {
		log.Printf("Failed to load email config: %v", err)
		return err
	}

	subject := "Password Reset Request - ShopSwift"
	resetLink := fmt.Sprintf("%s/reset-password?token=%s", os.Getenv("FRONTEND_URL"), resetToken)

	htmlBody := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <style>
        body { font-family: Arial, sans-serif; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .button { background-color: #667eea; color: white; padding: 12px 30px; text-decoration: none; border-radius: 4px; }
    </style>
</head>
<body>
    <div class="container">
        <h2>Password Reset Request</h2>
        <p>We received a request to reset your password. Click the button below to set a new password:</p>
        <p><a href="%s" class="button">Reset Password</a></p>
        <p>This link expires in 1 hour.</p>
        <p>If you didn't request this, please ignore this email.</p>
    </div>
</body>
</html>
`, resetLink)

	from := fmt.Sprintf("%s <%s>", emailConfig.SenderName, emailConfig.SenderEmail)
	headers := map[string]string{
		"From":         from,
		"To":           to,
		"Subject":      subject,
		"MIME-Version": "1.0",
		"Content-Type": "text/html; charset=UTF-8",
	}

	message := ""
	for key, value := range headers {
		message += fmt.Sprintf("%s: %s\r\n", key, value)
	}
	message += "\r\n" + htmlBody

	auth := smtp.PlainAuth("", emailConfig.SenderEmail, emailConfig.SenderPass, emailConfig.SmtpServer)

	err = smtp.SendMail(
		emailConfig.SmtpServer+":"+emailConfig.SmtpPort,
		auth,
		emailConfig.SenderEmail,
		[]string{to},
		[]byte(message),
	)

	if err != nil {
		log.Printf("Failed to send password reset email to %s: %v", to, err)
		return fmt.Errorf("failed to send password reset email: %w", err)
	}

	log.Printf("Password reset email sent successfully to %s", to)
	return nil
}
