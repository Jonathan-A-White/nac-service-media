package notification

import (
	"testing"
	"time"
)

func TestEmailRequest_Validate(t *testing.T) {
	validRequest := EmailRequest{
		To:           []Recipient{{Name: "John Doe", Address: "john@example.com"}},
		ServiceDate:  time.Date(2025, 12, 28, 0, 0, 0, 0, time.UTC),
		MinisterName: "Pr. Smith",
		AudioURL:     "https://drive.google.com/file/d/abc/view",
		VideoURL:     "https://drive.google.com/file/d/xyz/view",
	}

	tests := []struct {
		name    string
		modify  func(*EmailRequest)
		wantErr error
	}{
		{
			name:    "valid request",
			modify:  func(r *EmailRequest) {},
			wantErr: nil,
		},
		{
			name:    "no recipients",
			modify:  func(r *EmailRequest) { r.To = nil },
			wantErr: ErrNoRecipients,
		},
		{
			name:    "empty recipients",
			modify:  func(r *EmailRequest) { r.To = []Recipient{} },
			wantErr: ErrNoRecipients,
		},
		{
			name:    "recipient without address",
			modify:  func(r *EmailRequest) { r.To = []Recipient{{Name: "John"}} },
			wantErr: ErrInvalidRecipient,
		},
		{
			name:    "no service date",
			modify:  func(r *EmailRequest) { r.ServiceDate = time.Time{} },
			wantErr: ErrNoServiceDate,
		},
		{
			name:    "no minister",
			modify:  func(r *EmailRequest) { r.MinisterName = "" },
			wantErr: ErrNoMinister,
		},
		{
			name:    "no media URLs",
			modify:  func(r *EmailRequest) { r.AudioURL = ""; r.VideoURL = "" },
			wantErr: ErrNoMediaURLs,
		},
		{
			name:    "audio only is valid",
			modify:  func(r *EmailRequest) { r.VideoURL = "" },
			wantErr: nil,
		},
		{
			name:    "video only is valid",
			modify:  func(r *EmailRequest) { r.AudioURL = "" },
			wantErr: nil,
		},
		{
			name: "multiple recipients",
			modify: func(r *EmailRequest) {
				r.To = []Recipient{
					{Name: "John", Address: "john@example.com"},
					{Name: "Jane", Address: "jane@example.com"},
				}
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validRequest // Copy
			tt.modify(&req)
			err := req.Validate()
			if err != tt.wantErr {
				t.Errorf("Validate() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}
