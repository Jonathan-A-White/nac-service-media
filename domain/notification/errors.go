package notification

import "errors"

var (
	// ErrNoRecipients is returned when no To recipients are provided
	ErrNoRecipients = errors.New("at least one recipient is required")

	// ErrInvalidRecipient is returned when a recipient has no email address
	ErrInvalidRecipient = errors.New("recipient must have an email address")

	// ErrNoServiceDate is returned when the service date is missing
	ErrNoServiceDate = errors.New("service date is required")

	// ErrNoMinister is returned when the minister name is missing
	ErrNoMinister = errors.New("minister name is required")

	// ErrNoMediaURLs is returned when neither audio nor video URL is provided
	ErrNoMediaURLs = errors.New("at least one media URL (audio or video) is required")

	// ErrRecipientNotFound is returned when a recipient lookup fails
	ErrRecipientNotFound = errors.New("recipient not found")

	// ErrAmbiguousRecipient is returned when multiple recipients match a query
	ErrAmbiguousRecipient = errors.New("multiple recipients match query")

	// ErrSendFailed is returned when the email fails to send
	ErrSendFailed = errors.New("failed to send email")
)
