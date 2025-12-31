package notification

import (
	"time"
)

// Recipient represents an email recipient with name and address
type Recipient struct {
	Name    string
	Address string
}

// EmailRequest contains all the data needed to send a service recording notification
type EmailRequest struct {
	To           []Recipient // Primary recipients
	CC           []Recipient // Carbon copy recipients
	ServiceDate  time.Time   // Date of the service
	MinisterName string      // Name of the minister (e.g., "Pr. Smith")
	AudioURL     string      // Google Drive URL for audio file
	VideoURL     string      // Google Drive URL for video file
	ChurchName   string      // Name of the church for subject line
	SenderName   string      // Name to sign the email (e.g., "Jonathan")
}

// Validate checks that the email request has all required fields
func (r *EmailRequest) Validate() error {
	if len(r.To) == 0 {
		return ErrNoRecipients
	}
	for _, to := range r.To {
		if to.Address == "" {
			return ErrInvalidRecipient
		}
	}
	if r.ServiceDate.IsZero() {
		return ErrNoServiceDate
	}
	if r.MinisterName == "" {
		return ErrNoMinister
	}
	if r.AudioURL == "" && r.VideoURL == "" {
		return ErrNoMediaURLs
	}
	return nil
}

// EmailSender defines the interface for sending emails
type EmailSender interface {
	Send(req *EmailRequest) error
}
