package utils

import (
	"fmt"

	"github.com/rs/zerolog/log"
)

// EmailService handles sending emails
type EmailService struct {
	// For now, we'll just log emails to console
	// Later this can be extended to use SMTP, SendGrid, etc.
	enabled bool
}

// NewEmailService creates a new email service
func NewEmailService(enabled bool) *EmailService {
	return &EmailService{
		enabled: enabled,
	}
}

// SendVerificationEmail sends an email verification email
func (e *EmailService) SendVerificationEmail(email, firstName, verificationURL string) error {
	if !e.enabled {
		log.Info().
			Str("email", email).
			Str("verification_url", verificationURL).
			Msg("Email service disabled - verification email not sent")
		return nil
	}

	// For development, just log the email content
	// In production, this would send an actual email
	subject := "Verify your SREootb account"
	body := fmt.Sprintf(`
Hi %s,

Thank you for registering with SREootb! Please verify your email address by clicking the link below:

%s

This link will expire in 24 hours.

If you didn't create an account with SREootb, please ignore this email.

Best regards,
The SREootb Team
`, firstName, verificationURL)

	log.Info().
		Str("to", email).
		Str("subject", subject).
		Str("verification_url", verificationURL).
		Msg("📧 Email verification sent (logged to console for development)")

	// Print the email content to console for development
	fmt.Printf("\n=== EMAIL VERIFICATION ===\n")
	fmt.Printf("To: %s\n", email)
	fmt.Printf("Subject: %s\n", subject)
	fmt.Printf("Body:\n%s\n", body)
	fmt.Printf("========================\n\n")

	return nil
}

// SendPasswordResetEmail sends a password reset email
func (e *EmailService) SendPasswordResetEmail(email, firstName, resetURL string) error {
	if !e.enabled {
		log.Info().
			Str("email", email).
			Str("reset_url", resetURL).
			Msg("Email service disabled - password reset email not sent")
		return nil
	}

	subject := "Reset your SREootb password"
	body := fmt.Sprintf(`
Hi %s,

You requested to reset your password for your SREootb account. Click the link below to reset your password:

%s

This link will expire in 1 hour.

If you didn't request a password reset, please ignore this email and your password will remain unchanged.

Best regards,
The SREootb Team
`, firstName, resetURL)

	log.Info().
		Str("to", email).
		Str("subject", subject).
		Str("reset_url", resetURL).
		Msg("📧 Password reset email sent (logged to console for development)")

	// Print the email content to console for development
	fmt.Printf("\n=== PASSWORD RESET ===\n")
	fmt.Printf("To: %s\n", email)
	fmt.Printf("Subject: %s\n", subject)
	fmt.Printf("Body:\n%s\n", body)
	fmt.Printf("===================\n\n")

	return nil
}

// SendWelcomeEmail sends a welcome email after email verification
func (e *EmailService) SendWelcomeEmail(email, firstName string) error {
	if !e.enabled {
		log.Info().
			Str("email", email).
			Msg("Email service disabled - welcome email not sent")
		return nil
	}

	subject := "Welcome to SREootb!"
	body := fmt.Sprintf(`
Hi %s,

Welcome to SREootb! Your email has been verified and your account is now active.

You can now start monitoring your websites and services with our powerful monitoring platform.

Getting started:
- Add your first website to monitor
- Set up monitoring agents for distributed checking
- Configure alerts and notifications

If you have any questions, please don't hesitate to reach out.

Best regards,
The SREootb Team
`, firstName)

	log.Info().
		Str("to", email).
		Str("subject", subject).
		Msg("📧 Welcome email sent (logged to console for development)")

	// Print the email content to console for development
	fmt.Printf("\n=== WELCOME EMAIL ===\n")
	fmt.Printf("To: %s\n", email)
	fmt.Printf("Subject: %s\n", subject)
	fmt.Printf("Body:\n%s\n", body)
	fmt.Printf("==================\n\n")

	return nil
}
