package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	appdetection "nac-service-media/application/detection"
	appprocess "nac-service-media/application/process"
	"nac-service-media/domain/distribution"
	domainfs "nac-service-media/domain/filesystem"
	"nac-service-media/domain/notification"
	"nac-service-media/domain/video"
	"nac-service-media/infrastructure/config"
	infradetection "nac-service-media/infrastructure/detection"
	"nac-service-media/infrastructure/drive"
	"nac-service-media/infrastructure/ffmpeg"
	"nac-service-media/infrastructure/filesystem"
	"nac-service-media/infrastructure/gmail"
	"nac-service-media/infrastructure/lock"

	"github.com/spf13/cobra"
)

// processLockFileName lives in os.TempDir() and serializes concurrent
// `process` invocations. Without this, two scheduled tasks firing at once
// race past the already-processed check and upload duplicate copies.
const processLockFileName = "nac-service-media-process.lock"

var (
	processInputPath     string
	processStartTime     string
	processEndTime       string
	processMinisterKey   string
	processRecipientKeys []string
	processCCKeys        []string
	processDateOverride  string
	processSenderKey     string
	processSkipVideo     bool
)

var processCmd = &cobra.Command{
	Use:   "process",
	Short: "Process a service recording through the complete workflow",
	Long: `Process a service recording through the complete automated workflow:
1. Detect or specify start timestamp (auto-detection when --start omitted)
2. Detect or specify end timestamp (auto-detection when --end omitted)
3. Trim video to specified timestamps
4. Extract audio as MP3
5. Clean up old files from Google Drive if needed
6. Upload video and audio to Google Drive
7. Share files publicly
8. Send email notification with links

The source video can be specified with --input, or the newest file in the
source directory will be used by default.

Timestamps can be auto-detected when detection.enabled is true in config:
  --start: Detects when the cross lights up (visual template matching)
  --end: Detects the three-fold amen song (audio template matching)

The service date is inferred from the filename (OBS format: YYYY-MM-DD HH-MM-SS.mp4),
or can be specified with --date.

Ministers, recipients, and CCs are looked up by their config keys.

If the run fails, it sends one email to the --recipient (subject "Service
recording NOT processed: <date>: <reason>") with what failed, the video, the
last lines of the log and the process command that finishes it by hand, and
exits non-zero. A newest video that is already on Drive is not a failure: one
log line, no email, exit 0. When nobody is at the terminal (the scheduled
task), a Google sign-in that needs the browser gives up after two minutes.

Example:
  # Fully automatic - detect both start and end
  nac-service-media process --minister smith --recipient jane

  # Auto-detect start, specify end manually
  nac-service-media process --end 01:45:00 --minister smith --recipient jane

  # Specify both timestamps manually
  nac-service-media process --start 00:05:30 --end 01:45:00 --minister smith --recipient jane

  nac-service-media process \
    --input "2025-12-28 10-06-16.mp4" \
    --start 00:05:30 \
    --end 01:45:00 \
    --minister smith \
    --recipient jane --recipient john \
    --sender avteam

  # Audio-only mode (skip video trimming and upload)
  nac-service-media process --skip-video --start 00:05:30 --end 01:45:00 --minister smith --recipient jane`,
	RunE: runProcess,
}

func init() {
	rootCmd.AddCommand(processCmd)
	processCmd.Flags().StringVar(&processInputPath, "input", "", "Path to source video file (defaults to newest in source directory)")
	processCmd.Flags().StringVar(&processStartTime, "start", "", "Start timestamp in HH:MM:SS format (auto-detected if omitted)")
	processCmd.Flags().StringVar(&processEndTime, "end", "", "End timestamp in HH:MM:SS format (auto-detected if omitted)")
	processCmd.Flags().StringVar(&processMinisterKey, "minister", "", "Minister config key (optional, omit to exclude from email)")
	processCmd.Flags().StringArrayVar(&processRecipientKeys, "recipient", nil, "Recipient config key(s) (required, can be repeated)")
	processCmd.Flags().StringArrayVar(&processCCKeys, "cc", nil, "Additional CC config key(s) (optional)")
	processCmd.Flags().StringVar(&processDateOverride, "date", "", "Override service date (YYYY-MM-DD)")
	processCmd.Flags().StringVar(&processSenderKey, "sender", "", "Sender config key (defaults to config default_sender)")
	processCmd.Flags().BoolVar(&processSkipVideo, "skip-video", false, "Skip video trimming and upload; extract audio directly from source using timestamps")

	// --start and --end are now optional (auto-detected when omitted)
	// --minister is optional (email will omit minister section if not provided)
	processCmd.MarkFlagRequired("recipient")
}

// browserAuthWait bounds the Google browser sign-in in a run nobody is
// watching (the scheduled task pipes the output into its log), so a dead token
// fails the run and reaches the failure email instead of waiting forever.
const browserAuthWait = 2 * time.Minute

// failureLogLines is how many log lines the failure email carries.
const failureLogLines = 30

func runProcess(cmd *cobra.Command, args []string) error {
	cfg := GetConfig()

	ctx := cmd.Context()
	tail := appprocess.NewLogTail(failureLogLines)
	out := io.MultiWriter(os.Stdout, tail)
	lockPath := filepath.Join(os.TempDir(), processLockFileName)
	authWait := browserAuthTimeout(os.Stdin, os.Stdout)
	workDir, _ := os.Getwd()

	trimmer := ffmpeg.NewTrimmer()
	extractor := ffmpeg.NewExtractor()
	fileChecker := filesystem.NewChecker()
	fileFinder := &ProductionFileFinder{}

	run := &processRun{
		cfg:      cfg,
		input:    processInputFromFlags(),
		out:      out,
		tail:     tail,
		lockPath: lockPath,
		workDir:  workDir,
		acquireLock: func() (func(), error) {
			fl, err := lock.Acquire(lockPath)
			if err != nil {
				return nil, err
			}
			return func() { fl.Release() }, nil
		},
		findNewest: func(dir string) (string, error) {
			return fileFinder.FindNewestFile(dir, ".mp4")
		},
		newDrive: func(ctx context.Context) (distribution.DriveClient, error) {
			return drive.NewClientWithOAuth(ctx, cfg.Google.CredentialsFile, cfg.Google.TokenFile, drive.WithBrowserAuthTimeout(authWait))
		},
		newMailer: func(ctx context.Context) (processMailer, error) {
			from := notification.Recipient{
				Name:    cfg.Email.FromName,
				Address: cfg.Email.FromAddress,
			}
			return gmail.NewClientWithOAuth(ctx, gmail.OAuthConfig{
				CredentialsFile: cfg.Google.CredentialsFile,
				TokenFile:       cfg.Google.GmailTokenFile,
			}, from, gmail.WithBrowserAuthTimeout(authWait))
		},
		detectStart: func(ctx context.Context, videoPath string) (string, error) {
			return detectStartTimestamp(ctx, cfg, videoPath, out)
		},
		detectEnd: func(ctx context.Context, videoPath string, startSeconds int) (string, error) {
			return detectEndTimestamp(ctx, cfg, videoPath, startSeconds, out)
		},
		workflow: func(ctx context.Context, driveClient distribution.DriveClient, mailer notification.EmailSender, input ProcessInput) error {
			return runProcessWithClients(ctx, cfg, trimmer, extractor, fileChecker, driveClient, mailer, fileFinder, input, out)
		},
	}
	return run.run(ctx)
}

// browserAuthTimeout is zero (wait for the person at the keyboard) when both
// stdin and stdout are terminals, and browserAuthWait otherwise.
func browserAuthTimeout(stdin, stdout *os.File) time.Duration {
	if isTerminal(stdin) && isTerminal(stdout) {
		return 0
	}
	return browserAuthWait
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// processInputFromFlags gathers the command-line flags of one 'process' run.
func processInputFromFlags() ProcessInput {
	return ProcessInput{
		InputPath:     processInputPath,
		StartTime:     processStartTime,
		EndTime:       processEndTime,
		MinisterKey:   processMinisterKey,
		RecipientKeys: processRecipientKeys,
		CCKeys:        processCCKeys,
		DateOverride:  processDateOverride,
		SenderKey:     processSenderKey,
		SkipVideo:     processSkipVideo,
	}
}

// processMailer sends the links email and the failure email.
type processMailer interface {
	notification.EmailSender
	appprocess.FailureMailer
}

// processRun is one 'process' run with its outside world behind seams, so
// every way it can stop can be tested with fakes.
type processRun struct {
	cfg      *config.Config
	input    ProcessInput
	out      io.Writer
	tail     *appprocess.LogTail
	lockPath string
	workDir  string

	acquireLock func() (release func(), err error)
	findNewest  func(dir string) (string, error)
	newDrive    func(ctx context.Context) (distribution.DriveClient, error)
	newMailer   func(ctx context.Context) (processMailer, error)
	detectStart func(ctx context.Context, videoPath string) (string, error)
	detectEnd   func(ctx context.Context, videoPath string, startSeconds int) (string, error)
	workflow    func(ctx context.Context, driveClient distribution.DriveClient, mailer notification.EmailSender, input ProcessInput) error

	// What the run knew when it stopped, for the failure email.
	videoPath   string
	serviceDate time.Time
	startTime   string
	endTime     string

	driveClient distribution.DriveClient
	driveErr    error
	mailer      processMailer
	mailerErr   error
}

// processFailure is an error from one stage of the run, with the short reason
// the failure email puts in its subject. Its text is the stage's own error.
type processFailure struct {
	reason string
	hint   string
	err    error
}

func (e *processFailure) Error() string { return e.err.Error() }
func (e *processFailure) Unwrap() error { return e.err }

func fail(reason string, err error) error {
	return &processFailure{reason: reason, err: err}
}

func failWithHint(reason, hint string, err error) error {
	return &processFailure{reason: reason, hint: hint, err: err}
}

const signInHint = "sign in to Google again: in WSL, in the nac-service-media folder, run 'nac-service-media auth status --fix'."

// run executes the process command. Every error it returns has gone through
// notifyFailure, the one failure notifier.
func (r *processRun) run(ctx context.Context) error {
	err := r.execute(ctx)
	if err != nil {
		r.notifyFailure(ctx, err)
	}
	return err
}

// drive creates the Drive client once per run.
func (r *processRun) drive(ctx context.Context) (distribution.DriveClient, error) {
	if r.driveClient == nil && r.driveErr == nil {
		r.driveClient, r.driveErr = r.newDrive(ctx)
	}
	return r.driveClient, r.driveErr
}

// gmail creates the Gmail client once per run, so a dead token is waited on
// once, not again for the failure email.
func (r *processRun) gmail(ctx context.Context) (processMailer, error) {
	if r.mailer == nil && r.mailerErr == nil {
		r.mailer, r.mailerErr = r.newMailer(ctx)
	}
	return r.mailer, r.mailerErr
}

func (r *processRun) execute(ctx context.Context) error {
	cfg := r.cfg
	if cfg == nil {
		return fail("configuration not loaded", fmt.Errorf("configuration not loaded; ensure config/config.yaml exists"))
	}

	release, err := r.acquireLock()
	if err != nil {
		if errors.Is(err, lock.ErrAlreadyHeld) {
			return failWithHint("another run is still going",
				"that run sends its own email when it ends. If none comes, it is stuck: in WSL run 'pgrep -af nac-service-media' to see it and 'pkill -f \"nac-service-media process\"' to stop it, then run the command below.",
				fmt.Errorf("another nac-service-media process run is still going (lock: %s), so this run did nothing", r.lockPath))
		}
		return fail("could not take the run lock", fmt.Errorf("failed to acquire process lock: %w", err))
	}
	defer release()

	// Resolve video path once (used for both detection types)
	videoPath := r.input.InputPath
	if videoPath == "" {
		// Find newest file
		newest, err := r.findNewest(cfg.Paths.SourceDirectory)
		if err != nil {
			return fail("no video file found", fmt.Errorf("failed to find video file: %w", err))
		}
		videoPath = newest
	} else if !filepath.IsAbs(videoPath) {
		videoPath = filepath.Join(cfg.Paths.SourceDirectory, videoPath)
	}
	r.videoPath = videoPath
	if date, err := time.Parse("2006-01-02", r.input.DateOverride); err == nil {
		r.serviceDate = date
	} else if date, err := inferDateFromFilename(filepath.Base(videoPath)); err == nil {
		r.serviceDate = date
	}

	// Check if file was already processed (only in auto-detect mode, before running expensive detection)
	if r.input.InputPath == "" {
		// Infer date from filename to check if already processed
		serviceDate, err := inferDateFromFilename(filepath.Base(videoPath))
		if err == nil {
			// Create Drive client early to check for existing files
			driveClient, err := r.drive(ctx)
			if err != nil {
				return failWithHint("Google Drive sign-in failed", signInHint, fmt.Errorf("failed to create Google Drive client: %w", err))
			}

			// Check if both mp4 and mp3 already exist on Drive
			dateStr := serviceDate.Format("2006-01-02")
			mp4File, mp4Err := driveClient.FindFileByName(ctx, cfg.Google.ServicesFolderID, dateStr+".mp4")
			if mp4Err != nil {
				return fail("Google Drive check failed", fmt.Errorf("failed to check Drive for existing files: %w", mp4Err))
			}
			mp3File, mp3Err := driveClient.FindFileByName(ctx, cfg.Google.ServicesFolderID, dateStr+".mp3")
			if mp3Err != nil {
				return fail("Google Drive check failed", fmt.Errorf("failed to check Drive for existing files: %w", mp3Err))
			}
			if mp4File != nil && mp3File != nil {
				// No new video: not a failure, so no email and exit 0.
				fmt.Fprintf(r.out, "Most recent file (%s) has already been processed; nothing to do. Use --input to process a different file.\n", dateStr)
				return nil
			}
		}
	}

	// Detect start timestamp if not provided
	r.startTime = r.input.StartTime
	if r.startTime == "" {
		// Check if detection is enabled
		if !cfg.Detection.Enabled {
			return fail("no start time", fmt.Errorf("--start flag is required (auto-detection is disabled in config)"))
		}

		// Run detection
		detectedTime, err := r.detectStart(ctx, videoPath)
		if err != nil {
			return crossSearchFailure(videoPath, err)
		}
		r.startTime = detectedTime
	}

	// Detect end timestamp if not provided
	r.endTime = r.input.EndTime
	if r.endTime == "" {
		// Check if detection is enabled
		if !cfg.Detection.Enabled {
			return fail("no end time", fmt.Errorf("--end flag is required (auto-detection is disabled in config)"))
		}

		// Parse start time to get seconds for search window calculation
		startTimestamp, err := video.ParseTimestamp(r.startTime)
		if err != nil {
			r.startTime = ""
			return fail("bad start time", fmt.Errorf("invalid start timestamp: %w", err))
		}

		// Run detection, searching from (startTime + offset) minutes into video
		detectedTime, err := r.detectEnd(ctx, videoPath, startTimestamp.TotalSeconds())
		if err != nil {
			if errors.Is(err, infradetection.ErrNothingAnalysed) {
				return fail("amen search could not run", err)
			}
			return fail("amen not found", err)
		}
		r.endTime = detectedTime
	}

	driveClient, err := r.drive(ctx)
	if err != nil {
		return failWithHint("Google Drive sign-in failed", signInHint, fmt.Errorf("failed to create Google Drive client: %w", err))
	}

	gmailClient, err := r.gmail(ctx)
	if err != nil {
		return failWithHint("Gmail sign-in failed", signInHint, fmt.Errorf("failed to create Gmail client: %w", err))
	}

	input := r.input
	input.StartTime = r.startTime
	input.EndTime = r.endTime
	return r.workflow(ctx, driveClient, gmailClient, input)
}

// crossSearchFailure names a failed cross search: one that could not look at
// the video is not "cross not found"; a miss that left its sampled frames
// says where they are and how to teach the detector the shot.
func crossSearchFailure(videoPath string, err error) error {
	if errors.Is(err, infradetection.ErrNothingAnalysed) {
		return fail("cross search could not run", err)
	}
	var miss *infradetection.CrossNotFoundError
	if errors.As(err, &miss) && miss.FramesDir != "" {
		return failWithHint("cross not found", fmt.Sprintf(
			"the camera's shot may have changed. The frames it looked at are in %q, named by their time in the video. "+
				"Find the first one with the cross lit and one just before it, then in WSL, in the nac-service-media folder, run "+
				"'nac-service-media templates add --input %q --lit-at HH:MM:SS --unlit-at HH:MM:SS --name <shot>' with those two times "+
				"(the shot's name is yours to choose, e.g. room). Then run the command below with --start replaced by the lit time, "+
				"or without --start and --end to let it find them.",
			miss.FramesDir, videoPath), err)
	}
	return fail("cross not found", err)
}

// notifyFailure is the one failure notifier of the process command: it sends
// one email to the run's --recipient saying what failed and the command that
// finishes it by hand. When that email cannot be sent it says so loudly in the
// log; the run exits non-zero either way.
func (r *processRun) notifyFailure(ctx context.Context, err error) {
	reason, hint := failureReason(err)
	failure := appprocess.Failure{
		Reason:      reason,
		Hint:        hint,
		Err:         err,
		When:        time.Now(),
		ServiceDate: r.serviceDate,
		VideoPath:   r.videoPath,
		StartTime:   r.startTime,
		EndTime:     r.endTime,
		Input:       appprocess.Input{InputPath: r.input.InputPath, MinisterKey: r.input.MinisterKey, RecipientKeys: r.input.RecipientKeys, CCKeys: r.input.CCKeys, DateOverride: r.input.DateOverride, SenderKey: r.input.SenderKey, SkipVideo: r.input.SkipVideo},
		WorkDir:     r.workDir,
	}
	if r.tail != nil {
		failure.LogTail = r.tail.Lines()
	}

	if r.cfg == nil {
		r.failureEmailNotSent(errors.New("no configuration, so no recipient and no Gmail sign-in"))
		return
	}
	to, lookupErr := config.NewRecipientLookup(r.cfg, "").LookupRecipients(r.input.RecipientKeys)
	if lookupErr == nil && len(to) == 0 {
		lookupErr = errors.New("no --recipient given")
	}
	if lookupErr != nil {
		r.failureEmailNotSent(fmt.Errorf("cannot find the --recipient to tell: %w", lookupErr))
		return
	}
	mailer, mailerErr := r.gmail(ctx)
	if mailerErr != nil {
		r.failureEmailNotSent(fmt.Errorf("Gmail sign-in failed: %w", mailerErr))
		return
	}
	if sendErr := appprocess.SendFailureEmail(mailer, to, failure); sendErr != nil {
		r.failureEmailNotSent(sendErr)
		return
	}
	for _, rcpt := range to {
		fmt.Fprintf(r.out, "Failure email sent to %s <%s>: %s\n", rcpt.Name, rcpt.Address, failure.Subject())
	}
}

// failureEmailNotSent says loudly in the log that nobody was told.
func (r *processRun) failureEmailNotSent(err error) {
	fmt.Fprintln(r.out)
	fmt.Fprintln(r.out, "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
	fmt.Fprintf(r.out, "!!! FAILURE EMAIL NOT SENT: %v\n", err)
	fmt.Fprintln(r.out, "!!! This run failed and nobody has been told. Read this log.")
	fmt.Fprintln(r.out, "!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!!")
	fmt.Fprintln(r.out)
}

// failureReason gives the short reason for the subject: the stage's own for
// the command's stages, else the workflow error's lead ("trim failed", ...).
func failureReason(err error) (reason, hint string) {
	var pf *processFailure
	if errors.As(err, &pf) {
		return pf.reason, pf.hint
	}
	reason = strings.SplitN(strings.TrimSpace(err.Error()), "\n", 2)[0]
	if i := strings.Index(reason, ": "); i > 0 {
		reason = reason[:i]
	}
	if len(reason) > 60 {
		reason = reason[:60]
	}
	return reason, ""
}

// detectStartTimestamp runs the detection algorithm and returns the detected timestamp
func detectStartTimestamp(ctx context.Context, cfg *config.Config, videoPath string, out io.Writer) (string, error) {
	// Create detection service
	detectionService := appdetection.NewService(cfg.Detection, out)

	// Run detection
	result, err := detectionService.DetectStart(ctx, appdetection.DetectInput{
		VideoPath: videoPath,
	})
	if err != nil {
		return "", fmt.Errorf("auto-detection failed: %w\nUse --start to specify manually", err)
	}

	fmt.Fprintf(out, "Using detected timestamp: %s\n\n", result.Timestamp)
	return result.Timestamp, nil
}

// detectEndTimestamp runs the amen detection algorithm and returns the detected end timestamp
// startTimeSeconds is the service start time used to calculate where to begin searching
func detectEndTimestamp(ctx context.Context, cfg *config.Config, videoPath string, startTimeSeconds int, out io.Writer) (string, error) {
	// Create detection service
	detectionService := appdetection.NewService(cfg.Detection, out)

	// Run detection, passing start time so it searches from (start + offset) minutes
	result, err := detectionService.DetectEnd(ctx, videoPath, startTimeSeconds)
	if err != nil {
		return "", fmt.Errorf("end detection failed: %w\nUse --end to specify manually", err)
	}

	fmt.Fprintf(out, "Using detected end timestamp: %s\n\n", result.Timestamp)
	return result.Timestamp, nil
}

// ProcessInput contains the input parameters for process command
type ProcessInput struct {
	InputPath     string
	StartTime     string
	EndTime       string
	MinisterKey   string
	RecipientKeys []string
	CCKeys        []string
	DateOverride  string
	SenderKey     string
	SkipVideo     bool
}

// FileFinder interface for finding files (allows testing)
type FileFinder interface {
	FindNewestFile(dir, ext string) (string, error)
	ListFiles(dir, ext string) ([]string, error)
}

// ProductionFileFinder implements FileFinder for production use
type ProductionFileFinder struct{}

func (f *ProductionFileFinder) FindNewestFile(dir, ext string) (string, error) {
	files, err := f.ListFiles(dir, ext)
	if err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", fmt.Errorf("no video files found in %s", dir)
	}

	// Sort by filename descending (newest by date in filename)
	sort.Slice(files, func(i, j int) bool {
		return files[i] > files[j]
	})

	return files[0], nil
}

func (f *ProductionFileFinder) ListFiles(dir, ext string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", dir, err)
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) == ext {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	return files, nil
}

// runProcessWithClients runs the process with the high-level clients (production path)
func runProcessWithClients(
	ctx context.Context,
	cfg *config.Config,
	trimmer video.Trimmer,
	extractor video.AudioExtractor,
	fileChecker video.FileChecker,
	driveClient distribution.DriveClient,
	gmailClient notification.EmailSender,
	fileFinder FileFinder,
	input ProcessInput,
	output io.Writer,
) error {
	// Verify ffmpeg is available
	if verifiable, ok := trimmer.(interface{ VerifyInstalled(context.Context) error }); ok {
		verifyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := verifiable.VerifyInstalled(verifyCtx); err != nil {
			return fmt.Errorf("ffmpeg verification failed: %w", err)
		}
	}

	// Create file sizer
	fileSizer := &productionFileSizer{}

	// Create disk checker and file remover for local cleanup
	diskChecker := filesystem.NewDiskUsageChecker()
	fileRemover := filesystem.NewRemover()

	// Create process service
	service := appprocess.NewService(
		trimmer,
		extractor,
		fileChecker,
		fileSizer,
		driveClient,
		gmailClient,
		&fileFinderAdapter{finder: fileFinder},
		cfg,
		output,
		diskChecker,
		fileRemover,
	)

	// Build input
	processInput := appprocess.Input{
		InputPath:     input.InputPath,
		StartTime:     input.StartTime,
		EndTime:       input.EndTime,
		MinisterKey:   input.MinisterKey,
		RecipientKeys: input.RecipientKeys,
		CCKeys:        input.CCKeys,
		DateOverride:  input.DateOverride,
		SenderKey:     input.SenderKey,
		SkipVideo:     input.SkipVideo,
	}

	_, err := service.Process(ctx, processInput)
	return err
}

// RunProcessWithDependencies runs the process command with injected dependencies (for testing)
// This version accepts low-level service interfaces for mocking
func RunProcessWithDependencies(
	ctx context.Context,
	cfg *config.Config,
	trimmer video.Trimmer,
	extractor video.AudioExtractor,
	fileChecker video.FileChecker,
	driveService drive.DriveService,
	gmailService gmail.GmailService,
	fileFinder FileFinder,
	input ProcessInput,
	output io.Writer,
	diskChecker domainfs.DiskChecker,
	fileRemover domainfs.FileRemover,
) error {
	// Verify ffmpeg is available
	if verifiable, ok := trimmer.(interface{ VerifyInstalled(context.Context) error }); ok {
		verifyCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if err := verifiable.VerifyInstalled(verifyCtx); err != nil {
			return fmt.Errorf("ffmpeg verification failed: %w", err)
		}
	}

	// Create Drive client wrapper
	driveClient, err := drive.NewClient(ctx, "", drive.WithDriveService(driveService))
	if err != nil {
		return fmt.Errorf("failed to create drive client: %w", err)
	}

	// Create Gmail client wrapper
	from := notification.Recipient{
		Name:    cfg.Email.FromName,
		Address: cfg.Email.FromAddress,
	}
	gmailClient := gmail.NewClient(from, gmail.WithGmailService(gmailService))

	// Create file sizer that uses the mock file checker
	fileSizer := &mockFileSizer{fileChecker: fileChecker}

	// Create process service
	service := appprocess.NewService(
		trimmer,
		extractor,
		fileChecker,
		fileSizer,
		driveClient,
		gmailClient,
		&fileFinderAdapter{finder: fileFinder},
		cfg,
		output,
		diskChecker,
		fileRemover,
	)

	// Build input
	processInput := appprocess.Input{
		InputPath:     input.InputPath,
		StartTime:     input.StartTime,
		EndTime:       input.EndTime,
		MinisterKey:   input.MinisterKey,
		RecipientKeys: input.RecipientKeys,
		CCKeys:        input.CCKeys,
		DateOverride:  input.DateOverride,
		SenderKey:     input.SenderKey,
		SkipVideo:     input.SkipVideo,
	}

	_, err = service.Process(ctx, processInput)
	return err
}

// productionFileSizer provides file sizes using os.Stat
type productionFileSizer struct{}

func (s *productionFileSizer) Size(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// mockFileSizer provides file sizes for testing using a SizeProvider interface
type mockFileSizer struct {
	fileChecker video.FileChecker
}

func (s *mockFileSizer) Size(path string) int64 {
	// Check if the file checker also implements a Size method
	if sizer, ok := s.fileChecker.(interface{ Size(string) int64 }); ok {
		return sizer.Size(path)
	}
	// Default to a reasonable size for testing
	return 1200000000 // ~1.2GB
}

// fileFinderAdapter adapts the FileFinder interface to appprocess.FileFinder
type fileFinderAdapter struct {
	finder FileFinder
}

func (a *fileFinderAdapter) FindNewestFile(dir, ext string) (string, error) {
	return a.finder.FindNewestFile(dir, ext)
}

func (a *fileFinderAdapter) ListFiles(dir, ext string) ([]string, error) {
	return a.finder.ListFiles(dir, ext)
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

// Ensure distribution.DriveClient is implemented
var _ distribution.DriveClient = (*drive.Client)(nil)
