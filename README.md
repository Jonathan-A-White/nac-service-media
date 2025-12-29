# nac-service-media

CLI tool to automate the processing and distribution of church service recordings.

## What It Does

Transforms a raw OBS recording into trimmed video and audio files, uploads them to Google Drive, and sends notification emails with shareable links.

**Before**: 10-15 minutes of manual work every Sunday
**After**: ~2 minutes (mostly entering timestamps)

## Current Status: In Development

This project is being built incrementally. See the [Roadmap](#roadmap) below.

## Quick Start

```bash
# First time: interactive setup creates config.yaml
nac-service-media setup

# Process a recording
nac-service-media process \
  --source "/path/to/2025-12-28 10-06-16.mp4" \
  --start "00:05:30" \
  --end "01:45:00" \
  --priest "Pr. Smith" \
  --to jane
```

## Installation

```bash
go install github.com/Jonathan-A-White/nac-service-media@latest
```

Or build from source:

```bash
git clone https://github.com/Jonathan-A-White/nac-service-media.git
cd nac-service-media
go build -o bin/nac-service-media .
```

## Configuration

On first run without a config, the CLI will prompt for setup:

```bash
$ nac-service-media setup

Welcome to nac-service-media setup!

Where does OBS save recordings? /mnt/c/Users/jonat/Videos
Where should trimmed videos go? /mnt/c/Users/jonat/Videos/Trimmed
Where should audio files go? /mnt/c/Users/jonat/Videos/Audio

Google Drive folder ID for Services: 1dPV078FlLsWUFGjjoq3-epJiY_tBGXC8
Path to Google credentials file: credentials.json

Gmail address to send from: whiteplainsnac@gmail.com
Display name: White Plains NAC
Gmail App Password: ********

Default CC recipient name: Your Name
Default CC recipient email: you@example.com

Add a quick-lookup recipient? (y/n): y
  Nickname: jane
  Full name: Jane Doe
  Email: jane.doe@example.com
Add another? (y/n): n

Configuration saved to config.yaml
```

Or copy and edit the example:

```bash
cp config/config.example.yaml config.yaml
# Edit config.yaml with your values
```

## Google Drive Setup

1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Create a new project
3. Enable the Google Drive API
4. Create a Service Account (IAM & Admin > Service Accounts)
5. Download the JSON key as `credentials.json`
6. Share your Google Drive Services folder with the service account email

## Gmail Setup

For sending emails, use a Gmail App Password:

1. Go to [Google Account Security](https://myaccount.google.com/security)
2. Enable 2-Step Verification if not already enabled
3. Go to [App Passwords](https://myaccount.google.com/apppasswords)
4. Generate a new app password for "Mail"
5. Use this in your config (not your regular Gmail password)

## Roadmap

### Phase 1: Foundation
- [ ] **Project Scaffolding** - Go module, godog BDD setup, config loading
- [ ] **Video Trimming** - ffmpeg integration, manual timestamp input

### Phase 2: Core Processing
- [ ] **Audio Extraction** - Extract mp3 from trimmed video

### Phase 3: Distribution
- [ ] **Google Drive Auth** - Service account authentication
- [ ] **Google Drive Upload** - Upload files, set sharing permissions
- [ ] **Google Drive Cleanup** - Auto-delete oldest videos when storage is full

### Phase 4: Notification
- [ ] **Email Notification** - Send templated emails with Drive links

### Phase 5: Future Enhancement
- [ ] **Timestamp Detection** - Automatically detect service start/end

See `thoughts/shared/issues/` for detailed specifications of each feature.

## Development

### Prerequisites

- Go 1.21+
- ffmpeg installed and in PATH
- Google Cloud credentials (for Drive/Gmail features)

### Running Tests

```bash
# Unit tests
go test ./...

# Integration tests (requires credentials)
go test -tags=integration ./features/...
```

### Project Structure

```
nac-service-media/
├── cmd/                    # CLI commands (cobra)
├── domain/                 # Domain models (DDD)
│   ├── video/             # Video processing domain
│   ├── distribution/      # Google Drive domain
│   └── notification/      # Email domain
├── application/           # Application services
├── infrastructure/        # External integrations
│   ├── config/           # YAML config loading
│   ├── ffmpeg/           # ffmpeg wrapper
│   ├── drive/            # Google Drive client
│   └── gmail/            # Gmail client
├── features/              # BDD tests (godog)
│   ├── *.feature         # Gherkin scenarios
│   └── steps/            # Step definitions
└── config/
    └── config.example.yaml
```

## License

MIT
