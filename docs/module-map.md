# Module map

What each module of nac-service-media is for, where stories collide, and how to
split the code so two stories can change two modules at once. Written against
base commit `4bf0760` (2026-10-10). Docs only: nothing here changes behaviour
and no source file has moved.

Why this exists: the rule "Draw the boundaries so two stories can change two
modules at once" (27-modular-boundaries.md in the builder seat's PWA best
practices, from the Governor's words in Postern general, 2026-10-09 18:19Z).
Stories on one rig run in series when they touch the same file; this map names
those files and the cuts that would stop it. Library candidates follow the
library best practices (when code becomes a library, public and nothing
personal, versioning).

Conventions used below. This is a Go repo, so "module" means a Go package, and
"src/" is the repo root (there is no src folder). Paths in backticks exist
today; modules and files that are only proposed are written in plain text.
`docs/check-module-map.sh` checks the section list, every backticked path and
the collision table.

## 1. Modules

The layout is domain-driven: `domain/` holds types and ports (interfaces) and
imports nothing from the app; `application/` holds use-case services;
`infrastructure/` holds adapters; `cmd/` is the cobra CLI and the wiring. The
Go module name is `nac-service-media`; imports below are the in-repo ones.

| Package | One responsibility | Entry file and exported API | Imports (in-repo) |
|---|---|---|---|
| `main.go` | Start the CLI | `main()` calls `cmd.Execute()` | `cmd` |
| `cmd` | Parse flags, build real clients, run one command | `cmd/root.go`: `Execute()`, `GetConfig()`; one `Run<Name>WithDependencies(...)` per command (`RunTrimWithDependencies`, `RunProcessWithDependencies`, `RunSendEmailWithDependencies`, ...); `cmd/detect.go` runs the start detector alone (`detect --input`, `--at`, `--every`), trimming nothing; `cmd/truth.go` adds a recording's true start to the truth table with its frames (`truth add <recording> <hh:mm:ss>`); `Prompter`, `OutputWriter`, `FileFinder` interfaces | everything below |
| `domain/video` | Timestamps and the requests to trim or extract a recording | `domain/video/timestamp.go`: `ParseTimestamp(string) (Timestamp, error)`; `domain/video/trim_request.go`: `NewTrimRequest`; `domain/video/audio_request.go`: `NewAudioExtractionRequest[WithTimestamps]`; ports `Trimmer`, `AudioExtractor`, `FileChecker` (`domain/video/trimmer.go`, `domain/video/extractor.go`) | none |
| `domain/distribution` | What a Drive upload, quota and cleanup look like | `domain/distribution/drive_client.go`: port `DriveClient` (list, find, quota, delete, upload, share); `FileInfo`, `StorageInfo.HasSpaceFor`, `UploadRequest`, `UploadResult`, `CleanupResult` | none |
| `domain/notification` | The email: recipients, validation, wording | `domain/notification/email.go`: `Recipient`, `EmailRequest.Validate`, port `EmailSender`; `domain/notification/template.go`: `DefaultTemplate`, `FormatGreeting`, `FormatServiceRef`, `EmailTemplate.Render*` | none |
| `domain/detection` | Ports and results for finding the service start and end | `domain/detection/detector.go`: ports `StartDetector`, `EndDetector`; `DetectionResult`, `EndDetectionResult`, `FrameAnalysis` | `domain/video` |
| `domain/filesystem` | Two small ports for local cleanup | `domain/filesystem/interfaces.go`: `DiskChecker`, `FileRemover` | none |
| `application/video` | Trim a recording; extract audio from it | `application/video/service.go`: `NewTrimService(...).Trim`; `application/video/extract_service.go`: `NewExtractService(...).Extract` and `.ExtractWithTimestamps` | `domain/video` |
| `application/distribution` | Upload and share files; free Drive space | `application/distribution/upload_service.go`: `NewUploadService(...)`, `.UploadVideo`, `.UploadAudio`, `.Distribute`; `application/distribution/cleanup_service.go`: `NewCleanupService(...).EnsureSpaceAvailable` | `domain/distribution` |
| `application/notification` | Turn a send request into an `EmailRequest` and send it | `application/notification/service.go`: `NewService(sender, churchName, senderName).Send(SendRequest)` | `domain/notification` |
| `application/detection` | Run start and end detection and report a timestamp string | `application/detection/service.go`: `NewService(cfg, out)`, `.DetectStart`, `.DetectEnd`, `.IsEnabled` | `infrastructure/config`, `infrastructure/detection` |
| `application/process` | The end-to-end workflow: validate, trim, extract, check space, upload, email, clean local files, print recovery commands | `application/process/service.go`: `NewService(11 ports and settings)`, `.Process(ctx, Input) (*Result, error)`, `ValidationError`, `GetSteps()` | `application/{distribution,notification,video}`, `domain/{distribution,filesystem,notification,video}`, `infrastructure/config` |
| `infrastructure/config` | Load, save and edit the YAML config | `infrastructure/config/loader.go`: `Load`, `Save`, `Config` and its section structs; `infrastructure/config/manager.go`: `NewConfigManager(cfg, path)` with Add/List/Get/Remove/Update for ministers, recipients, CCs and senders; `infrastructure/config/recipients.go`: `NewRecipientLookup` | `domain/notification` |
| `infrastructure/drive` | Google Drive adapter and its OAuth flow | `infrastructure/drive/client.go`: `NewClient`, `Client` (implements `DriveClient`), `DriveService` seam; `infrastructure/drive/oauth.go`: `NewClientWithOAuth` | `domain/distribution` |
| `infrastructure/gmail` | Gmail adapter, its OAuth flow, MIME building and template rendering | `infrastructure/gmail/client.go`: `NewClient`, `Client.Send`; `infrastructure/gmail/oauth.go`: `NewClientWithOAuth`, `OAuthConfig` | `domain/notification` |
| `infrastructure/ffmpeg` | Run ffmpeg to trim and extract audio | `infrastructure/ffmpeg/trimmer.go`: `NewTrimmer`, `CommandRunner`; `infrastructure/ffmpeg/extractor.go`: `NewExtractor` | `domain/video` |
| `infrastructure/filesystem` | Local file checks: exists, disk usage, remove | `infrastructure/filesystem/checker.go`: `NewChecker`; `infrastructure/filesystem/disk.go`: `NewDiskUsageChecker`; `infrastructure/filesystem/remover.go`: `NewRemover` | `domain/filesystem`, `domain/video` |
| `infrastructure/detection` | Find the start (template match in Go, every picture of the cross at every scale from half to double) and end (Python audio match); the amen detector is a stub when built without `-tags=detection`, the cross detector is untagged and gate-tested | `infrastructure/detection/template_detector.go`: `NewTemplateDetector`, `.LoadTemplates`, `.DetectStart`; `infrastructure/detection/match.go` (the FFT template match, `DefaultMatchScore`, lit or unlit by contrast and score) and `infrastructure/detection/fft.go`; `infrastructure/detection/scan.go` (the cross search: a start must stay lit in place for `confirmSeconds`; `ErrNothingAnalysed`, `CrossNotFoundError` with each picture's best score); `infrastructure/detection/inspect.go` (`.Inspect` one frame for `cmd/detect.go`, `WithSamplesDir`); `infrastructure/detection/regression_test.go` replays real frames from `infrastructure/detection/testdata/` against `config/detection_templates`; `infrastructure/detection/truth.go` (the truth table `testdata/truth/truth.tsv`: `TruthCase`, `ReadTruthTable`, `WriteTruthCase`, `TruthFrameTimes`, `ReplayFrames`, `TruthMiss`), replayed row by row by `infrastructure/detection/truth_test.go`; `infrastructure/detection/template_capture.go` (`AddTemplate`, `FindCrossBox`, `TemplateFiles`, `FFmpegFrames`, used by `cmd/templates.go`); `infrastructure/detection/amen_detector.go`: `NewAmenDetector`, `.DetectEnd` (passes the service start on to `scripts/detect_amen.py`, whose pure `amen_search_window` falls back to the last part of a video that ends before the offset; `scripts/test_detect_amen.py` runs in the gate through `infrastructure/detection/amen_window_test.go`), and `infrastructure/detection/amen_detector_stub.go` | `domain/detection`, `domain/video`, `infrastructure/config` |
| `infrastructure/lock` | One running instance at a time (advisory flock) | `infrastructure/lock/lock.go`: `Acquire(path) (*FileLock, error)`, `ErrAlreadyHeld`, `.Release` | none |
| `features` (with `features/steps`) | BDD scenarios (godog), built only with the `integration` tag, so `go test ./...` does not run them | `features/features_test.go`: `initializeScenarios` registers ten `Initialize*Scenario` functions | `features/steps` and the packages under test |

Other file groups: `scripts/` (batch and Windows scheduled-task scripts, the
Python end detector `scripts/detect_amen.py`), `config/` (example config and the
detection templates), `Makefile`.

Dependency direction as built: `cmd` to `application` to `domain`, and
`infrastructure` to `domain`. Three arrows break the pattern (see section 4):
`application/process` and `application/detection` import `infrastructure/config`,
and `application/detection` imports `infrastructure/detection`.

## 2. Collisions

The command from the story, run at the base commit:

```
git log --since=30.days --name-only --format= -- "*.go" | sort | uniq -c | sort -rn | head -15
```

printed nothing on 2026-10-10: the last commit (`4bf0760`) is dated 2026-08-09,
62 days earlier, so no commit falls in the window. The table below is the same
pipeline over the whole history up to `4bf0760` (57 commits, 2025-12-29 to
2026-08-09), which is the only data there is. Line counts are `wc -l` at the
base commit. Test files (the ones ending in _test.go) are listed because they collide too.

| Commits | File | Lines |
|---|---|---|
| 10 | `features/features_test.go` | 44 |
| 10 | `cmd/process.go` | 541 |
| 9 | `infrastructure/config/loader.go` | 148 |
| 7 | `application/process/service.go` | 682 |
| 5 | `features/steps/process_steps.go` | 998 |
| 5 | `domain/notification/template.go` | 136 |
| 4 | `infrastructure/drive/client_test.go` | 611 |
| 4 | `infrastructure/drive/client.go` | 312 |
| 4 | `infrastructure/detection/template_detector.go` | 333 |
| 4 | `domain/notification/template_test.go` | 280 |
| 4 | `domain/distribution/drive_client.go` | 47 |
| 4 | `application/process/service_test.go` | 725 |
| 3 | `infrastructure/drive/integration_test.go` | 166 |
| 3 | `features/steps/drive_steps.go` | 361 |
| 3 | `cmd/trim.go` | 169 |

What the table says:

- `cmd/process.go` and `application/process/service.go` (1,223 lines between
  them, 17 commits) are the hot pair. Every feature that touches the workflow
  (skip-video, already-processed check, detection, local cleanup, locking)
  edited both. Both are over the 500-line rule.
- `features/features_test.go` is 44 lines and 10 commits only because it is the
  registry: each new feature adds an `Initialize*Scenario` line. Any two stories
  that add a scenario collide there.
- `infrastructure/config/loader.go` changes whenever a setting is added, and
  five packages import it, so a config change fans out.
- `domain/notification/template.go` mixes wording with date logic; the
  weekday bug fix (`09e6eab`) landed there.
- Test files collide with the code they test; `application/process/service_test.go`
  (725 lines) and `features/steps/process_steps.go` (998 lines, the largest file
  in the repo) each define their own copies of the same fakes (section 3, R5).

## 3. Proposed refactors

Ranked by how much parallel work each frees. Each is behaviour-preserving
(move and rename, tests stay green) and is meant to be filed as its own story.
R7 is first in execution order because R1 and R2 use it, but it ranks seventh
because it frees little by itself.

### R1. Take the composition root and the "already processed" check out of the process command

**Responsibility to carve out:** `cmd/process.go` does six jobs: flag parsing, the process lock, building real Drive and Gmail clients, resolving the video, the Drive "already processed" check, and running detection. Only the first two are CLI work. A story that changes detection, Drive or email wiring currently edits this file; after the cut they edit three different files.

**New module and API:** a new `bootstrap` package at the repo root (wiring only, no logic), and a new method on the existing `application/process` service.

```go
// package bootstrap
type ProcessDeps struct {
	Trimmer     video.Trimmer
	Extractor   video.AudioExtractor
	FileChecker video.FileChecker
	Drive       distribution.DriveClient
	Mail        notification.EmailSender
	Files       process.FileFinder
	Sizer       process.FileSizer
	Disk        domainfs.DiskChecker
	Remover     domainfs.FileRemover
}
func NewProcessDeps(ctx context.Context, cfg *config.Config) (*ProcessDeps, error)

// package process (application/process)
func (s *Service) AlreadyProcessed(ctx context.Context, serviceDate time.Time) (bool, error)

// package detection (application/detection), see R8
func (s *Service) ResolveWindow(ctx context.Context, videoPath, start, end string) (startTS, endTS string, err error)
```

**Files it touches:** `cmd/process.go` (shrinks to about 120 lines), `application/process/service.go`, `application/detection/service.go`; new `bootstrap` package.

**Risk:** medium. `runProcessWithClients` and `RunProcessWithDependencies` are the seam the BDD steps and `application/process/service_test.go` use; keep both signatures for the first story. The Drive check currently runs before detection on purpose (commit `99870ad`); the order must be kept.

**Size:** does not fit one story. Two: (a) move client building into `bootstrap` and `AlreadyProcessed` into the service, (b) move detection resolution behind `ResolveWindow` (needs R8's injected detectors).

### R2. Split `application/process/service.go` by responsibility

**Responsibility to carve out:** the file holds the workflow (`Process`, `processFullWorkflow`, `processAudioOnly`), input resolution (`validateInputs`, which reads the concrete config), local-file cleanup (`computeCleanupInput`, `cleanupLocalFiles`, `deleteOldestFile`, lines 569 to 654) and the recovery-command text (`showRecoveryCommands*`, lines 482 to 552). The full and audio-only workflows repeat the storage, upload, email and cleanup steps in two copies.

**New module and API:** three new packages under `application/process/` and one shared step list.

```go
// package localcleanup (application/process/localcleanup)
type Dirs struct{ Source, Trimmed, Audio string }
type Cleaner struct{ /* disk, remover, finder, dirs, out */ }
func New(disk domainfs.DiskChecker, rm domainfs.FileRemover, finder FileFinder, dirs Dirs, out io.Writer) *Cleaner
func (c *Cleaner) Plan(skipVideo bool, source string, date time.Time) Plan
func (c *Cleaner) Run(p Plan, thresholdPercent float64, label string)

// package recovery (application/process/recovery)
func Commands(failedStep int, in process.Input, source string, date time.Time, audioOnly bool) []string

// package resolve (application/process/resolve), consumes the port from R3
func Request(ctx context.Context, in process.Input, dir config.Directory, files process.FileFinder, check video.FileChecker) (Resolved, error)
```

**Files it touches:** `application/process/service.go` (682 lines to about 250), `application/process/service_test.go` (split with it).

**Risk:** low to medium. Pure moves, but the 90 percent and 70 percent thresholds and the order "compute cleanup state before new files exist" must stay (see the comment at `service.go:141`).

**Size:** does not fit one story. Two: (a) `localcleanup` and `recovery` (independent of other refactors), (b) `resolve` (after R3).

### R3. Give each consumer its own settings and put the config editing behind a port

**Responsibility to carve out:** `infrastructure/config` is both the YAML schema and a 450-line editor for four entity kinds, and `application/process` and `application/detection` import it, so every config change recompiles and re-reviews the application layer. Each consumer should declare the few settings it needs; `cmd` maps the YAML onto them.

**New module and API:** consumer-owned types in the packages that use them, and one lookup port.

```go
// package detection (domain/detection)
type Settings struct {
	Enabled           bool
	TemplatesDir      string
	AudioTemplatesDir string
	Thresholds        Thresholds
	SearchRange       SearchRange
}

// package process (application/process)
type Settings struct {
	SourceDir, TrimmedDir, AudioDir string
	ServicesFolderID, FromName      string
	AudioBitrate                    string
}

// package config (infrastructure/config), implements the port
type Directory interface {
	Minister(key string) (name string, err error)
	Recipients(keys []string) ([]notification.Recipient, error)
	DefaultCC() []notification.Recipient
	Sender(key string) (name string, err error) // empty key means the default sender
}
func (c *Config) Settings() (process.Settings, detection.Settings)
```

**Files it touches:** `infrastructure/config/loader.go`, `infrastructure/config/manager.go` (split into ministers, recipients, senders files), `application/process/service.go`, `application/detection/service.go`, `infrastructure/detection/template_detector.go` and `infrastructure/detection/amen_detector.go`, `cmd/process.go`.

**Risk:** medium. The config structs carry YAML tags and the example file `config/config.example.yaml` must still load; keep the YAML shape unchanged and add a round-trip test first.

**Size:** does not fit one story. Two: (a) `Settings` for detection and process, (b) the `Directory` port and splitting `infrastructure/config/manager.go`.

### R4. One Google OAuth module for Drive, Gmail and `auth status`

**Responsibility to carve out:** the installed-app OAuth flow (load token, open browser, local callback server, save token) is written twice, in `infrastructure/drive/oauth.go` and `infrastructure/gmail/oauth.go`; a `diff` of `getToken` through `openBrowser` in the two files shows only the callback port (8085 and 8086), a private `ServeMux` in the Gmail copy, the HTML title and the printed lines differing. `cmd/auth.go` then re-implements token checking against the Google libraries directly. A change to the flow has to be made in three places.

**New module and API:** a new package `googleauth` under `infrastructure/`.

```go
// package googleauth
type Config struct {
	CredentialsFile string
	TokenFile       string
	Scopes          []string
	CallbackPort    int    // 0 picks a free port
	Label           string // shown to the user: "Drive", "Gmail"
}
func HTTPClient(ctx context.Context, cfg Config) (*http.Client, error)
func Check(ctx context.Context, cfg Config) TokenStatus // missing, corrupt, no refresh token, expired, ok
func Reauthorize(ctx context.Context, cfg Config) error
```

**Files it touches:** `infrastructure/drive/oauth.go`, `infrastructure/gmail/oauth.go`, `cmd/auth.go`; tests in `infrastructure/drive/client_test.go` and `infrastructure/gmail/client_test.go` stay as they are.

**Risk:** low. Keep the two callback ports as defaults so the redirect URIs registered in the Google Cloud console do not change.

**Size:** fits one story. It is also the first step of library candidate L2.

### R5. One shared set of test fakes

**Responsibility to carve out:** the same fakes are written separately in `application/process/service_test.go` (`mockTrimmer`, `mockExtractor`, `mockFileChecker`, `mockFileSizer`, `mockDriveClient`, `mockEmailSender`, `mockFileFinder`, `mockDiskChecker`, `mockFileRemover`), in `features/steps/*.go` (`mockTrimmer`, `mockFileChecker`, `mockExtractor`, `mockDriveService`, `mockGmailService`) and in `infrastructure/drive/client_test.go`, `infrastructure/gmail/client_test.go`. Adding a method to a port means editing three or four copies, and that is why `features/steps/process_steps.go` is 998 lines.

**New module and API:** a new package `testsupport` (an `internal` package, so it is not a public API).

```go
// package fakes (internal/testsupport/fakes)
type Trimmer struct{ Calls []*video.TrimRequest; Err error }
func (t *Trimmer) Trim(ctx context.Context, req *video.TrimRequest, out string) error
type DriveClient struct{ Files []distribution.FileInfo; Quota distribution.StorageInfo; Err error }
type Mail struct{ Sent []*notification.EmailRequest; Err error }
type Files struct{ Existing map[string]int64 } // implements FileChecker, FileSizer, FileFinder
```

Also split the godog registry so a new feature adds a file, not a line in `features/features_test.go` (each `features/steps/*_steps.go` registers itself from an `init()` into a list the test reads).

**Files it touches:** `application/process/service_test.go`, `features/steps/*.go`, `features/features_test.go`; a new internal package testsupport/fakes.

**Risk:** low (tests only), but the rule is never to weaken a test: each migrated test must assert the same things as before.

**Size:** does not fit one story. Two: (a) the shared fakes used by `application/process/service_test.go` and the steps, (b) the self-registering step files.

### R6. Email wording as data, and the Gmail adapter only sends

**Responsibility to carve out:** the church wording is Go code in `domain/notification/template.go` (`DefaultTemplate`, "Recording of Service", `FormatGreeting` with "Dear" and "Hey Everyone!"). `infrastructure/gmail/client.go` renders the template and also builds the MIME message, so a wording story and a transport story edit the same adapter. `cmd/send-email.go:167` prints a second copy of the subject format ("Recording of Service on ...") that can drift from the template.

**New module and API:** the template becomes a file loaded from config, with the built-in text as the fallback; rendering moves into the application layer.

```go
// package notification (domain/notification)
func LoadTemplate(path string) (EmailTemplate, error) // empty path returns DefaultTemplate
type Message struct {
	From, To, CC    []Recipient
	Subject         string
	PlainText, HTML string
}
type MessageSender interface{ SendMessage(ctx context.Context, m Message) error }

// package notification (application/notification)
func (s *Service) Render(req SendRequest, now time.Time) (Message, error)
```

A new setting `email.template_file` points at the template; the language, greeting words and subject line come from that file, not from constants.

**Files it touches:** `domain/notification/template.go`, `domain/notification/email.go`, `infrastructure/gmail/client.go`, `application/notification/service.go`, `cmd/send-email.go`, `config/config.example.yaml`.

**Risk:** medium. The rendered text must stay byte-identical for the default template; `domain/notification/template_test.go` (280 lines) is the guard and must pass unchanged.

**Size:** does not fit one story. Two: (a) move rendering out of the Gmail adapter behind `Message`, (b) load the template from a file.

### R7. One home for service-date parsing and file lookup

**Responsibility to carve out:** the filename-to-date rule is written three times: `inferDateFromFilename` in `cmd/process.go` (line 524) and in `application/process/service.go` (line 553), and `parseDateFromFilename` in `cmd/extract-audio.go` (line 103). Finding a newest file is written twice, and the two differ: `ProductionFileFinder.FindNewestFile` in `cmd/process.go` sorts by file name, `findLatestFile` in `cmd/upload.go` (line 101) goes by modification time; the `FileFinder` interface is declared in both `cmd/process.go` and `application/process/service.go`. The literals `".mp4"`, `".mp3"` and `"2006-01-02"` appear 39 times in non-test code outside `features/`.

**New module and API:** the date rule goes to the domain, the finder to the filesystem adapter.

```go
// package video (domain/video)
func ServiceDateFromFilename(name string) (time.Time, error) // "2025-12-28 10-06-16.mp4" or "2025-12-28.mp4"
func (d ServiceDate) TrimmedName() string // "2025-12-28.mp4"
func (d ServiceDate) AudioName() string   // "2025-12-28.mp3"

// package filesystem (infrastructure/filesystem)
type Finder struct{}
func (Finder) NewestByName(dir, ext string) (string, error) // today's FindNewestFile
func (Finder) LatestModified(dir, ext string) (string, error) // today's findLatestFile
func (Finder) List(dir, ext string) ([]string, error)
```

**Files it touches:** `cmd/process.go`, `cmd/extract-audio.go`, `cmd/upload.go`, `application/process/service.go`, `domain/video/trim_request.go`, `domain/video/audio_request.go`, `infrastructure/filesystem/`.

**Risk:** low. No unit test names `inferDateFromFilename` or `findLatestFile` directly (grep of the test files, 2026-10-10), so add a table test for `ServiceDateFromFilename` and for both finders first. Keep both finder behaviours: they are different rules, not copies, and merging them into one would change behaviour.

**Size:** fits one story. Do it before R1 and R2 (it removes the duplicates they would otherwise have to move twice).

### R8. Inject the detectors into the detection service

**Responsibility to carve out:** `application/detection/service.go` builds the concrete GoCV and Python detectors itself (`infradetection.NewTemplateDetector(s.config)` inside `DetectStart`), although `domain/detection` already defines `StartDetector` and `EndDetector` for this. The "is detection enabled" check is also written in `cmd/process.go` (lines 181 and 197) beside the service's `IsEnabled`. Tests cannot run the service without OpenCV or Python, and a detection story edits both `cmd/process.go` and the service.

**New module and API:** the service takes the ports; `bootstrap` (R1) picks the real or stub detector.

```go
// package detection (application/detection)
func NewService(start detection.StartDetector, end detection.EndDetector, set detection.Settings, out io.Writer) *Service
func (s *Service) DetectStart(ctx context.Context, in DetectInput) (*DetectResult, error)
func (s *Service) DetectEnd(ctx context.Context, videoPath string, startSeconds int) (*DetectResult, error)
```

**Files it touches:** `application/detection/service.go`, `cmd/process.go`, `infrastructure/detection/` (constructors only).

**Risk:** low. The build-tag split (`-tags=detection` real, no tag stub) stays as is for the amen detector; the cross detector is untagged since 2026-10-16, so the gate exercises it.

**Size:** fits one story.

## 4. Rule breaks

Against 27-modular-boundaries.md. The rule is written for PWAs; the Go equivalents are used: "screen" is `cmd`, "vendor" is Google's client libraries, ffmpeg and OpenCV. There is no Go code in bsv-kit (it is TypeScript: packages bsv, grist, composer and tips), so there is no duplicate of bsv-kit code in this repo and nothing here can be moved into it.

1. **A screen calling a vendor directly.** `cmd/auth.go` imports golang.org/x/oauth2, golang.org/x/oauth2/google and the Drive and Gmail API packages for their scopes, and checks tokens itself. `cmd/process.go` builds `drive.NewClientWithOAuth` and `gmail.NewClientWithOAuth` inline, twice for Drive (once for the already-processed check, once for the run). `RunProcessWithDependencies` in `cmd/process.go` takes the vendor-shaped `drive.DriveService` and `gmail.GmailService` instead of the domain ports. Fix: R1, R4.
2. **Business rule in the screen.** The "already processed" decision (both `.mp4` and `.mp3` on Drive) lives in `cmd/process.go`, not in the application layer. Fix: R1.
3. **Test double in production code.** `mockFileSizer` in `cmd/process.go` (line 496) ships in the binary and returns a made-up 1.2 GB when the checker has no size. Fix: R1 (use the real sizer) and R5.
4. **Inner layer importing an outer one.** `application/process` and `application/detection` import `infrastructure/config`; `application/detection` imports `infrastructure/detection` and constructs the concrete detector, bypassing `domain/detection`'s ports. `infrastructure/config` imports `domain/notification` and returns domain types. Fix: R3, R8.
5. **Constants where settings belong.** Cleanup thresholds `90.0` and `70.0` (`application/process/service.go`, lines 145, 231 and 295); OAuth callback ports `8085` and `8086`; the English wording and the "Recording of Service" subject in Go code; the "service" idea itself (`ChurchName` in `domain/notification/template.go` and `application/notification/service.go`, while `cmd/send-email.go:124` fills it from `cfg.Email.FromName`, a field named for the sender, with the comment "Church name"). Commit `09e6eab` already fixed one such constant (a hard-coded "Sunday"). Fix: R6, R3, R4.
6. **A duplicate in two places.** The OAuth flow (R4), the filename-to-date rule and the file finders (R7), the subject format (R6), the test fakes (R5).
7. **A file past about 500 lines, edited by many stories.** `application/process/service.go` (682 lines, 7 commits), `cmd/process.go` (541 lines, 10 commits), `features/steps/process_steps.go` (998 lines). Fix: R1, R2, R5.
8. **Not breaks, worth keeping:** `domain/*` imports nothing outside the standard library and `domain/video`; `infrastructure/ffmpeg` takes a `CommandRunner` so tests do not run ffmpeg; every Drive and Gmail call goes through the `DriveService` and `GmailService` seams.

## 5. Library candidates

bsv-kit is TypeScript, so none of this belongs in it. Each candidate would be a NEW public Go module under github.com/Jonathan-A-White/, one repo per candidate (or one repo with several packages, if they ship together). Public means nothing personal in code, tests, fixtures or history: this repo's `config/config.example.yaml` already uses placeholders (commit `64ffaf1`), but grep each lift before the first push. All use Go modules' semantic versions: tag `v0.1.0`, keep a CHANGELOG file with "breaking" first, call a removed or changed export a breaking change (in 0.x a minor bump, after 1.0 a new major, which in Go also means a /v2 module path), pin apps to a tag, and ship a `LICENSE` file (this repo's README says MIT, but the repo has no `LICENSE` file; add one here too). Dependency licences are checked before the first release.

### L1. `filelock`: lift now (a second copy already exists)

`infrastructure/lock/lock.go` (56 lines) is an advisory flock that makes a command single-instance. Other Go projects of his hold their own flock code: millwright's `hostlock`, `eventlog`, `stampqueue`, `rig` and `handsroot` packages call `syscall.Flock` directly (found by grep on 2026-10-10; whether they could use the same API has not been checked). It is the cleanest lift: no dependencies, no personal data, already tested (`infrastructure/lock/lock_test.go`).

```go
// module github.com/Jonathan-A-White/filelock
var ErrAlreadyHeld error
type Lock struct{ /* unexported */ }
func Acquire(path string) (*Lock, error)            // non-blocking exclusive lock; ErrAlreadyHeld if held
func AcquireTimeout(path string, d time.Duration) (*Lock, error) // new, for callers that wait
func (l *Lock) Release() error                       // safe to call twice
```

Versioning: start at `v0.1.0`; moving the app onto it changes only the import path (one story in this repo that deletes `infrastructure/lock` in the same commit). Unix only (`syscall.Flock`): say so in the README and add a build-tagged Windows file before `v1.0.0`; adding it is not breaking. A change to `ErrAlreadyHeld` semantics is breaking.

### L2. `googleauth`: lift after R4

The installed-app OAuth loopback flow with a token file and a token health check, from R4 (today two copies in `infrastructure/drive/oauth.go` and `infrastructure/gmail/oauth.go`, 220 and 224 lines, plus the checks in `cmd/auth.go`). Any CLI that talks to a Google API needs exactly this.

```go
// module github.com/Jonathan-A-White/googleauth
type Config struct {
	CredentialsFile, TokenFile string
	Scopes                     []string
	CallbackPort               int
	OpenBrowser                func(url string) error // injectable; default opens the system browser
	Out                        io.Writer
}
func HTTPClient(ctx context.Context, cfg Config) (*http.Client, error)
func Check(ctx context.Context, cfg Config) (Status, error)
func Reauthorize(ctx context.Context, cfg Config) error
type Status int // StatusOK, StatusMissing, StatusCorrupt, StatusNoRefreshToken, StatusExpired
```

Versioning: `v0.x` until a second app has used it. The token file format (the `oauth2.Token` JSON) is a wire format: a change to what is written is breaking and the changelog must say how to migrate. Tests use a fake token endpoint and no network; ship the fake for consumers.

### L3. `gmailsend`: after R6

Building a multipart plain-and-HTML message and sending it through the Gmail API (`buildMIMEMessage` and `Send` in `infrastructure/gmail/client.go`, 173 lines). Nothing church-specific remains after R6 moves the wording out. Check before lifting: the builder writes the subject and display names without RFC 2047 encoding and uses a fixed boundary string, so a non-ASCII name or subject may be mangled (UNVERIFIED: not tested here); fix it in the lift, with a test.

```go
// module github.com/Jonathan-A-White/gmailsend
type Address struct{ Name, Email string }
type Message struct {
	From     Address
	To, CC   []Address
	Subject  string
	Text     string
	HTML     string
}
func New(httpClient *http.Client) *Sender
func (s *Sender) Send(ctx context.Context, m Message) error
func Build(m Message) ([]byte, error) // the RFC 2822 bytes, for tests and other transports
```

Versioning: `v0.x`; the message bytes are output other systems parse, so changing `Build` output is a minor bump with a changelog line, and a change to `Message` fields is breaking.

### L4. `drivepublish`: later, when a second consumer exists

Upload a file to a Drive folder, replace a same-named file, make it public, report quota and delete oldest files to make room (`infrastructure/drive/client.go` plus the two services in `application/distribution`, about 490 lines). Reusable by anything that publishes recordings or exports. Not yet: it still speaks this repo's `domain/distribution` types, so R3 and R4 come first, and no second consumer has been found (only millwright and fellowship were checked as other Go modules; Python and web apps were not).

```go
// module github.com/Jonathan-A-White/drivepublish
type Folder struct{ ID string }
type File struct{ ID, Name string; Size int64; Created time.Time; URL string }
func New(httpClient *http.Client) *Client
func (c *Client) Upload(ctx context.Context, f Folder, localPath, mimeType string, replace bool) (File, error)
func (c *Client) MakePublic(ctx context.Context, fileID string) error
func (c *Client) FreeSpace(ctx context.Context, f Folder, needBytes int64, ext string) ([]File, error)
```

Versioning: `v0.x`; the Drive API calls are the law, so keep a recorded-response fixture per call.

### Not candidates

`ffmpeg` trim and extract wrappers are 185 lines of thin `exec` calls with domain-typed arguments and no second user found. The GoCV and Python detectors are specific to this church's camera and cross; they stay in the app.
