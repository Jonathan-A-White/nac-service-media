package process

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"time"

	appdist "nac-service-media/application/distribution"
	appnotif "nac-service-media/application/notification"
	appvideo "nac-service-media/application/video"
	"nac-service-media/domain/distribution"
	"nac-service-media/domain/notification"
	"nac-service-media/domain/video"
	"nac-service-media/infrastructure/config"
)

// FileFinder abstracts file system operations for finding files
type FileFinder interface {
	FindNewestFile(dir, ext string) (string, error)
	ListFiles(dir, ext string) ([]string, error)
}

// FileSizer provides file size information
type FileSizer interface {
	Size(path string) int64
}

// Service orchestrates the complete processing workflow
type Service struct {
	trimmer     video.Trimmer
	extractor   video.AudioExtractor
	fileChecker video.FileChecker
	fileSizer   FileSizer
	driveClient distribution.DriveClient
	emailSender notification.EmailSender
	fileFinder  FileFinder
	cfg         *config.Config
	output      io.Writer
}

// NewService creates a new process service
func NewService(
	trimmer video.Trimmer,
	extractor video.AudioExtractor,
	fileChecker video.FileChecker,
	fileSizer FileSizer,
	driveClient distribution.DriveClient,
	emailSender notification.EmailSender,
	fileFinder FileFinder,
	cfg *config.Config,
	output io.Writer,
) *Service {
	return &Service{
		trimmer:     trimmer,
		extractor:   extractor,
		fileChecker: fileChecker,
		fileSizer:   fileSizer,
		driveClient: driveClient,
		emailSender: emailSender,
		fileFinder:  fileFinder,
		cfg:         cfg,
		output:      output,
	}
}

// Input contains all input parameters for the process command
type Input struct {
	InputPath     string   // Source video path (optional if using newest)
	StartTime     string   // Start timestamp HH:MM:SS
	EndTime       string   // End timestamp HH:MM:SS
	MinisterKey   string   // Minister config key
	RecipientKeys []string // Recipient config keys
	CCKeys        []string // CC config keys (optional)
	DateOverride  string   // Override service date (YYYY-MM-DD)
	SenderKey     string   // Sender config key (optional, uses default if empty)
	SkipVideo     bool     // Skip video trimming and upload; extract audio from source
}

// Result contains the results of a successful process run
type Result struct {
	TrimmedPath string
	AudioPath   string
	VideoURL    string
	AudioURL    string
	ServiceDate time.Time
}

// ValidationError contains details about a validation failure with suggestions
type ValidationError struct {
	Message    string
	Suggestion string
}

func (e *ValidationError) Error() string {
	if e.Suggestion != "" {
		return fmt.Sprintf("%s\n\nTo fix this, run:\n  %s", e.Message, e.Suggestion)
	}
	return e.Message
}

// Process runs the complete end-to-end workflow
func (s *Service) Process(ctx context.Context, input Input) (*Result, error) {
	startTime := time.Now()

	// Step 0: Validate all inputs before starting
	sourcePath, serviceDate, recipients, ccRecipients, ministerName, senderName, err := s.validateInputs(input)
	if err != nil {
		return nil, err
	}

	fmt.Fprintf(s.output, "Using source: %s\n", filepath.Base(sourcePath))
	fmt.Fprintf(s.output, "Service date: %s\n", serviceDate.Format("2006-01-02"))
	if ministerName != "" {
		fmt.Fprintf(s.output, "Minister: %s\n", ministerName)
	}
	if input.SkipVideo {
		fmt.Fprintf(s.output, "Mode: Audio-only (--skip-video)\n")
	}
	fmt.Fprintln(s.output)

	// Route to appropriate workflow
	if input.SkipVideo {
		return s.processAudioOnly(ctx, input, sourcePath, serviceDate, recipients, ccRecipients, ministerName, senderName, startTime)
	}
	return s.processFullWorkflow(ctx, input, sourcePath, serviceDate, recipients, ccRecipients, ministerName, senderName, startTime)
}

// processFullWorkflow handles the standard video+audio workflow
func (s *Service) processFullWorkflow(ctx context.Context, input Input, sourcePath string, serviceDate time.Time, recipients, ccRecipients []notification.Recipient, ministerName, senderName string, processStartTime time.Time) (*Result, error) {
	// Step 1: Trim video
	fmt.Fprintf(s.output, "[1/7] Trimming video...\n")
	trimResult, err := s.trimVideo(ctx, sourcePath, input.StartTime, input.EndTime)
	if err != nil {
		s.showRecoveryCommands(1, input, sourcePath, serviceDate)
		return nil, fmt.Errorf("trim failed: %w", err)
	}
	fmt.Fprintf(s.output, "      Created: %s\n\n", trimResult.OutputPath)

	// Step 2: Extract audio
	fmt.Fprintf(s.output, "[2/7] Extracting audio...\n")
	audioResult, err := s.extractAudio(ctx, trimResult.OutputPath, serviceDate)
	if err != nil {
		s.showRecoveryCommands(2, input, sourcePath, serviceDate)
		return nil, fmt.Errorf("audio extraction failed: %w", err)
	}
	fmt.Fprintf(s.output, "      Created: %s\n\n", audioResult.OutputPath)

	// Step 3: Ensure Drive storage
	fmt.Fprintf(s.output, "[3/7] Checking Drive storage...\n")
	videoSize := s.fileSizer.Size(trimResult.OutputPath)
	audioSize := s.fileSizer.Size(audioResult.OutputPath)
	neededSpace := videoSize + audioSize
	cleanupResult, err := s.ensureStorage(ctx, neededSpace)
	if err != nil {
		s.showRecoveryCommands(3, input, sourcePath, serviceDate)
		return nil, fmt.Errorf("storage check failed: %w", err)
	}
	for _, df := range cleanupResult.DeletedFiles {
		fmt.Fprintf(s.output, "      Removed: %s (%.1f MB)\n", df.Name, float64(df.Size)/1024/1024)
	}
	if len(cleanupResult.DeletedFiles) == 0 {
		fmt.Fprintf(s.output, "      Storage OK\n")
	}
	fmt.Fprintln(s.output)

	// Step 4: Upload video
	fmt.Fprintf(s.output, "[4/7] Uploading video...\n")
	videoUploadResult, err := s.uploadVideo(ctx, trimResult.OutputPath)
	if err != nil {
		s.showRecoveryCommands(4, input, sourcePath, serviceDate)
		return nil, fmt.Errorf("video upload failed: %w", err)
	}
	fmt.Fprintf(s.output, "      Uploaded: %s\n\n", filepath.Base(trimResult.OutputPath))

	// Step 5: Upload audio
	fmt.Fprintf(s.output, "[5/7] Uploading audio...\n")
	audioUploadResult, err := s.uploadAudio(ctx, audioResult.OutputPath)
	if err != nil {
		s.showRecoveryCommands(5, input, sourcePath, serviceDate)
		return nil, fmt.Errorf("audio upload failed: %w", err)
	}
	fmt.Fprintf(s.output, "      Uploaded: %s\n\n", filepath.Base(audioResult.OutputPath))

	// Step 6: Share files
	fmt.Fprintf(s.output, "[6/7] Sharing files...\n")
	fmt.Fprintf(s.output, "      Video link: %s\n", videoUploadResult.ShareableURL)
	fmt.Fprintf(s.output, "      Audio link: %s\n\n", audioUploadResult.ShareableURL)

	// Step 7: Send email
	fmt.Fprintf(s.output, "[7/7] Sending email...\n")
	err = s.sendEmail(recipients, ccRecipients, serviceDate, ministerName, senderName, audioUploadResult.ShareableURL, videoUploadResult.ShareableURL)
	if err != nil {
		s.showRecoveryCommands(7, input, sourcePath, serviceDate)
		return nil, fmt.Errorf("email failed: %w", err)
	}
	for _, r := range recipients {
		fmt.Fprintf(s.output, "      Sent to: %s <%s>\n", r.Name, r.Address)
	}
	fmt.Fprintln(s.output)

	elapsed := time.Since(processStartTime)
	fmt.Fprintf(s.output, "Done! Completed in %s\n", formatDuration(elapsed))

	return &Result{
		TrimmedPath: trimResult.OutputPath,
		AudioPath:   audioResult.OutputPath,
		VideoURL:    videoUploadResult.ShareableURL,
		AudioURL:    audioUploadResult.ShareableURL,
		ServiceDate: serviceDate,
	}, nil
}

// processAudioOnly handles the audio-only workflow (--skip-video mode)
func (s *Service) processAudioOnly(ctx context.Context, input Input, sourcePath string, serviceDate time.Time, recipients, ccRecipients []notification.Recipient, ministerName, senderName string, processStartTime time.Time) (*Result, error) {
	// Step 1: Extract audio directly from source with timestamps
	fmt.Fprintf(s.output, "[1/4] Extracting audio...\n")
	audioResult, err := s.extractAudioWithTimestamps(ctx, sourcePath, serviceDate, input.StartTime, input.EndTime)
	if err != nil {
		s.showRecoveryCommandsAudioOnly(1, input, sourcePath, serviceDate)
		return nil, fmt.Errorf("audio extraction failed: %w", err)
	}
	fmt.Fprintf(s.output, "      Created: %s\n\n", audioResult.OutputPath)

	// Step 2: Ensure Drive storage (still run cleanup for mp4s)
	fmt.Fprintf(s.output, "[2/4] Checking Drive storage...\n")
	audioSize := s.fileSizer.Size(audioResult.OutputPath)
	cleanupResult, err := s.ensureStorage(ctx, audioSize)
	if err != nil {
		s.showRecoveryCommandsAudioOnly(2, input, sourcePath, serviceDate)
		return nil, fmt.Errorf("storage check failed: %w", err)
	}
	for _, df := range cleanupResult.DeletedFiles {
		fmt.Fprintf(s.output, "      Removed: %s (%.1f MB)\n", df.Name, float64(df.Size)/1024/1024)
	}
	if len(cleanupResult.DeletedFiles) == 0 {
		fmt.Fprintf(s.output, "      Storage OK\n")
	}
	fmt.Fprintln(s.output)

	// Step 3: Upload audio
	fmt.Fprintf(s.output, "[3/4] Uploading audio...\n")
	audioUploadResult, err := s.uploadAudio(ctx, audioResult.OutputPath)
	if err != nil {
		s.showRecoveryCommandsAudioOnly(3, input, sourcePath, serviceDate)
		return nil, fmt.Errorf("audio upload failed: %w", err)
	}
	fmt.Fprintf(s.output, "      Uploaded: %s\n", filepath.Base(audioResult.OutputPath))
	fmt.Fprintf(s.output, "      Audio link: %s\n\n", audioUploadResult.ShareableURL)

	// Step 4: Send email (audio only)
	fmt.Fprintf(s.output, "[4/4] Sending email...\n")
	err = s.sendEmail(recipients, ccRecipients, serviceDate, ministerName, senderName, audioUploadResult.ShareableURL, "")
	if err != nil {
		s.showRecoveryCommandsAudioOnly(4, input, sourcePath, serviceDate)
		return nil, fmt.Errorf("email failed: %w", err)
	}
	for _, r := range recipients {
		fmt.Fprintf(s.output, "      Sent to: %s <%s>\n", r.Name, r.Address)
	}
	fmt.Fprintln(s.output)

	elapsed := time.Since(processStartTime)
	fmt.Fprintf(s.output, "Done! Completed in %s\n", formatDuration(elapsed))

	return &Result{
		TrimmedPath: "", // No trimmed video
		AudioPath:   audioResult.OutputPath,
		VideoURL:    "", // No video URL
		AudioURL:    audioUploadResult.ShareableURL,
		ServiceDate: serviceDate,
	}, nil
}

func (s *Service) validateInputs(input Input) (sourcePath string, serviceDate time.Time, recipients, ccRecipients []notification.Recipient, ministerName, senderName string, err error) {
	// Resolve source path
	sourcePath = input.InputPath
	if sourcePath == "" {
		// Find newest file in source directory
		newest, findErr := s.fileFinder.FindNewestFile(s.cfg.Paths.SourceDirectory, ".mp4")
		if findErr != nil {
			err = findErr
			return
		}
		sourcePath = newest
	} else if !filepath.IsAbs(sourcePath) {
		// Resolve relative paths against source directory
		sourcePath = filepath.Join(s.cfg.Paths.SourceDirectory, sourcePath)
	}

	// Verify source file exists
	if !s.fileChecker.Exists(sourcePath) {
		err = fmt.Errorf("source file does not exist: %s", sourcePath)
		return
	}

	// Determine service date
	if input.DateOverride != "" {
		serviceDate, err = time.Parse("2006-01-02", input.DateOverride)
		if err != nil {
			err = fmt.Errorf("invalid date format (use YYYY-MM-DD): %w", err)
			return
		}
	} else {
		// Try to infer from filename
		serviceDate, err = inferDateFromFilename(filepath.Base(sourcePath))
		if err != nil {
			err = fmt.Errorf("cannot infer date from filename %q. Use --date to specify: %w", filepath.Base(sourcePath), err)
			return
		}
	}

	// Lookup minister (optional - if key provided)
	if input.MinisterKey != "" {
		minister, ministerErr := config.NewConfigManager(s.cfg, "").GetMinister(input.MinisterKey)
		if ministerErr != nil {
			err = &ValidationError{
				Message:    fmt.Sprintf("minister '%s' not found in config", input.MinisterKey),
				Suggestion: config.SuggestAddMinisterCommand(input.MinisterKey),
			}
			return
		}
		ministerName = minister.Name
	}

	// Lookup recipients
	lookup := config.NewRecipientLookup(s.cfg, "")
	recipients, err = lookup.LookupRecipients(input.RecipientKeys)
	if err != nil {
		key := input.RecipientKeys[0]
		if len(input.RecipientKeys) > 1 {
			key = "recipients"
		}
		err = &ValidationError{
			Message:    fmt.Sprintf("recipient '%s' not found in config", key),
			Suggestion: config.SuggestAddRecipientCommand(key),
		}
		return
	}

	// Get default CC recipients
	ccRecipients = lookup.GetDefaultCC()

	// Add any additional CC recipients from flags
	for _, ccKey := range input.CCKeys {
		ccMatches, ccErr := lookup.LookupRecipient(ccKey)
		if ccErr != nil {
			err = &ValidationError{
				Message:    fmt.Sprintf("cc recipient '%s' not found in config", ccKey),
				Suggestion: config.SuggestAddCCCommand(ccKey),
			}
			return
		}
		ccRecipients = append(ccRecipients, ccMatches...)
	}

	// Lookup sender
	mgr := config.NewConfigManager(s.cfg, "")
	if input.SenderKey != "" {
		sender, senderErr := mgr.GetSender(input.SenderKey)
		if senderErr != nil {
			err = &ValidationError{
				Message:    fmt.Sprintf("sender '%s' not found in config", input.SenderKey),
				Suggestion: config.SuggestAddSenderCommand(input.SenderKey),
			}
			return
		}
		senderName = sender.Name
	} else {
		sender, senderErr := mgr.GetDefaultSender()
		if senderErr != nil {
			err = &ValidationError{
				Message:    "no default sender configured",
				Suggestion: "Set senders.default_sender in config or use --sender flag",
			}
			return
		}
		senderName = sender.Name
	}

	return
}

func (s *Service) trimVideo(ctx context.Context, sourcePath, startTime, endTime string) (*appvideo.TrimResult, error) {
	trimService := appvideo.NewTrimService(s.trimmer, s.fileChecker, s.cfg.Paths.TrimmedDirectory)
	return trimService.Trim(ctx, appvideo.TrimInput{
		SourcePath: sourcePath,
		StartTime:  startTime,
		EndTime:    endTime,
	})
}

func (s *Service) extractAudio(ctx context.Context, videoPath string, serviceDate time.Time) (*appvideo.ExtractResult, error) {
	bitrate := s.cfg.Audio.Bitrate
	if bitrate == "" {
		bitrate = video.DefaultAudioBitrate
	}
	extractService := appvideo.NewExtractService(s.extractor, s.fileChecker, s.cfg.Paths.AudioDirectory, bitrate)
	return extractService.Extract(ctx, appvideo.ExtractInput{
		SourcePath:  videoPath,
		ServiceDate: serviceDate,
		Bitrate:     bitrate,
	})
}

func (s *Service) extractAudioWithTimestamps(ctx context.Context, sourcePath string, serviceDate time.Time, startTime, endTime string) (*appvideo.ExtractResult, error) {
	bitrate := s.cfg.Audio.Bitrate
	if bitrate == "" {
		bitrate = video.DefaultAudioBitrate
	}
	extractService := appvideo.NewExtractService(s.extractor, s.fileChecker, s.cfg.Paths.AudioDirectory, bitrate)
	return extractService.ExtractWithTimestamps(ctx, appvideo.ExtractWithTimestampsInput{
		SourcePath:  sourcePath,
		ServiceDate: serviceDate,
		Bitrate:     bitrate,
		StartTime:   startTime,
		EndTime:     endTime,
	})
}

func (s *Service) ensureStorage(ctx context.Context, neededBytes int64) (*distribution.CleanupResult, error) {
	cleanupService := appdist.NewCleanupService(s.driveClient, s.cfg.Google.ServicesFolderID)
	return cleanupService.EnsureSpaceAvailable(ctx, neededBytes)
}

func (s *Service) uploadVideo(ctx context.Context, videoPath string) (*distribution.UploadResult, error) {
	uploadService := appdist.NewUploadService(s.driveClient, s.cfg.Google.ServicesFolderID, s.output)
	return uploadService.UploadVideo(ctx, videoPath)
}

func (s *Service) uploadAudio(ctx context.Context, audioPath string) (*distribution.UploadResult, error) {
	uploadService := appdist.NewUploadService(s.driveClient, s.cfg.Google.ServicesFolderID, s.output)
	return uploadService.UploadAudio(ctx, audioPath)
}

func (s *Service) sendEmail(recipients, ccRecipients []notification.Recipient, serviceDate time.Time, ministerName, senderName, audioURL, videoURL string) error {
	notifService := appnotif.NewService(s.emailSender, s.cfg.Email.FromName, senderName)
	return notifService.Send(appnotif.SendRequest{
		To:           recipients,
		CC:           ccRecipients,
		ServiceDate:  serviceDate,
		MinisterName: ministerName,
		AudioURL:     audioURL,
		VideoURL:     videoURL,
	})
}

func (s *Service) showRecoveryCommands(failedStep int, input Input, sourcePath string, serviceDate time.Time) {
	fmt.Fprintln(s.output)
	fmt.Fprintln(s.output, "To complete manually:")

	dateStr := serviceDate.Format("2006-01-02")
	trimmedPath := filepath.Join(s.cfg.Paths.TrimmedDirectory, dateStr+".mp4")
	audioPath := filepath.Join(s.cfg.Paths.AudioDirectory, dateStr+".mp3")

	step := 1
	if failedStep <= 1 {
		fmt.Fprintf(s.output, "  %d. Trim:       nac-service-media trim --source %q --start %s --end %s\n", step, sourcePath, input.StartTime, input.EndTime)
		step++
	}
	if failedStep <= 2 {
		fmt.Fprintf(s.output, "  %d. Extract:    nac-service-media extract-audio --source %q\n", step, trimmedPath)
		step++
	}
	if failedStep <= 3 {
		fmt.Fprintf(s.output, "  %d. Auth:       nac-service-media auth drive\n", step)
		step++
		fmt.Fprintf(s.output, "  %d. Cleanup:    nac-service-media cleanup --ensure-space 2GB\n", step)
		step++
	}
	if failedStep <= 4 {
		fmt.Fprintf(s.output, "  %d. Upload:     nac-service-media upload --video %q --audio %q\n", step, trimmedPath, audioPath)
		step++
	}
	if failedStep <= 7 {
		recipientArgs := ""
		for _, r := range input.RecipientKeys {
			recipientArgs += fmt.Sprintf(" --to %s", r)
		}
		fmt.Fprintf(s.output, "  %d. Email:      nac-service-media send-email%s --date %s --minister %q --audio-url <URL> --video-url <URL>\n", step, recipientArgs, dateStr, input.MinisterKey)
	}
	fmt.Fprintln(s.output)
}

func (s *Service) showRecoveryCommandsAudioOnly(failedStep int, input Input, sourcePath string, serviceDate time.Time) {
	fmt.Fprintln(s.output)
	fmt.Fprintln(s.output, "To complete manually:")

	dateStr := serviceDate.Format("2006-01-02")
	audioPath := filepath.Join(s.cfg.Paths.AudioDirectory, dateStr+".mp3")

	step := 1
	if failedStep <= 1 {
		fmt.Fprintf(s.output, "  %d. Extract:    nac-service-media extract-audio --source %q --start %s --end %s\n", step, sourcePath, input.StartTime, input.EndTime)
		step++
	}
	if failedStep <= 2 {
		fmt.Fprintf(s.output, "  %d. Auth:       nac-service-media auth drive\n", step)
		step++
		fmt.Fprintf(s.output, "  %d. Cleanup:    nac-service-media cleanup --ensure-space 200MB\n", step)
		step++
	}
	if failedStep <= 3 {
		fmt.Fprintf(s.output, "  %d. Upload:     nac-service-media upload --audio %q\n", step, audioPath)
		step++
	}
	if failedStep <= 4 {
		recipientArgs := ""
		for _, r := range input.RecipientKeys {
			recipientArgs += fmt.Sprintf(" --to %s", r)
		}
		fmt.Fprintf(s.output, "  %d. Email:      nac-service-media send-email%s --date %s --minister %q --audio-url <URL>\n", step, recipientArgs, dateStr, input.MinisterKey)
	}
	fmt.Fprintln(s.output)
}

// inferDateFromFilename extracts date from OBS-style filenames
// Supports: "2025-12-28 10-06-16.mp4" or "2025-12-28.mp4"
func inferDateFromFilename(filename string) (time.Time, error) {
	// Pattern for OBS format: YYYY-MM-DD HH-MM-SS.mp4
	obsPattern := regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})\s+\d{2}-\d{2}-\d{2}\.mp4$`)
	if matches := obsPattern.FindStringSubmatch(filename); len(matches) > 1 {
		return time.Parse("2006-01-02", matches[1])
	}

	// Pattern for trimmed format: YYYY-MM-DD.mp4
	trimmedPattern := regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})\.mp4$`)
	if matches := trimmedPattern.FindStringSubmatch(filename); len(matches) > 1 {
		return time.Parse("2006-01-02", matches[1])
	}

	return time.Time{}, fmt.Errorf("filename does not match expected format")
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	m := d / time.Minute
	s := (d % time.Minute) / time.Second
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// StepInfo provides information about a workflow step
type StepInfo struct {
	Number      int
	Description string
}

// GetSteps returns the list of workflow steps
func GetSteps() []StepInfo {
	return []StepInfo{
		{1, "Trimming video"},
		{2, "Extracting audio"},
		{3, "Checking Drive storage"},
		{4, "Uploading video"},
		{5, "Uploading audio"},
		{6, "Sharing files"},
		{7, "Sending email"},
	}
}
