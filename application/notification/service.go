package notification

import (
	"time"

	"nac-service-media/domain/notification"
)

// Service handles email notification operations
type Service struct {
	sender     notification.EmailSender
	churchName string
	senderName string
}

// NewService creates a new notification service
func NewService(sender notification.EmailSender, churchName, senderName string) *Service {
	return &Service{
		sender:     sender,
		churchName: churchName,
		senderName: senderName,
	}
}

// SendRequest contains the parameters for sending a recording notification
type SendRequest struct {
	To           []notification.Recipient
	CC           []notification.Recipient
	ServiceDate  time.Time
	MinisterName string
	AudioURL     string
	VideoURL     string
}

// Send sends a notification email for a service recording
func (s *Service) Send(req SendRequest) error {
	emailReq := &notification.EmailRequest{
		To:           req.To,
		CC:           req.CC,
		ServiceDate:  req.ServiceDate,
		MinisterName: req.MinisterName,
		AudioURL:     req.AudioURL,
		VideoURL:     req.VideoURL,
		ChurchName:   s.churchName,
		SenderName:   s.senderName,
	}

	return s.sender.Send(emailReq)
}
