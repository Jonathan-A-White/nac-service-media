# nac-service-media

CLI tool to automate the processing and distribution of church service recordings.

## What It Does

Transforms a raw OBS recording into trimmed video and audio files, uploads them to Google Drive, and sends notification emails with shareable links.

**Before**: 10-15 minutes of manual work every Sunday
**After**: ~1 minute with auto-detection, ~2 minutes with manual timestamps

## Installation (WSL Ubuntu)

### 1. Install System Dependencies

```bash
# Update package list
sudo apt-get update

# Install Go (if not already installed)
sudo apt-get install -y golang-go

# Install ffmpeg for video processing
sudo apt-get install -y ffmpeg

# Install OpenCV for start auto-detection (optional but recommended)
sudo apt-get install -y libopencv-dev libopencv-contrib-dev build-essential

# Install Python for end auto-detection (optional but recommended)
sudo apt-get install -y python3 python3-pip
pip3 install librosa numpy scipy
```

### 2. Clone and Build

```bash
# Clone the repository
mkdir -p ~/src/nac
cd ~/src/nac
git clone https://github.com/Jonathan-A-White/nac-service-media.git
cd nac-service-media

# Build with auto-detection enabled (requires OpenCV)
go build -tags=detection

# Or build without auto-detection (no OpenCV required)
go build
```

### 3. Configure

```bash
# Create config directory
mkdir -p config

# Run interactive setup
./nac-service-media setup
```

You'll also need to copy your Google OAuth credentials:
- `oauth_credentials.json` - From Google Cloud Console
- `drive_token.json` - Generated on first Drive authentication
- `gmail_token.json` - Generated on first Gmail authentication

## Quick Start

```bash
# Process with fully auto-detected timestamps (start + end)
./nac-service-media process --minister henkel --recipient jane

# Process with auto-detected start, manual end
./nac-service-media process --end 01:45:00 --minister henkel --recipient jane

# Process with fully manual timestamps
./nac-service-media process --start 00:05:30 --end 01:45:00 --minister henkel --recipient jane
```

## Commands

### process - Full Workflow

Runs the complete automation: detect/trim → extract audio → upload → email.

```bash
./nac-service-media process \
  --end 01:45:00 \
  --minister henkel \
  --recipient jane

# Options:
#   --input      Source video (defaults to newest in source_directory)
#   --start      Start timestamp HH:MM:SS (auto-detected if omitted)
#   --end        End timestamp HH:MM:SS (auto-detected if omitted)
#   --minister   Minister config key (required)
#   --recipient  Recipient config key (required, repeatable)
#   --cc         Additional CC config key (optional, repeatable)
#   --sender     Sender config key (defaults to config default)
#   --date       Override service date YYYY-MM-DD
```

### config - Manage Configuration

```bash
# List ministers/recipients/senders
./nac-service-media config list ministers
./nac-service-media config list recipients
./nac-service-media config list senders

# Add entries
./nac-service-media config add minister smith "Apostle Smith"
./nac-service-media config add recipient temple "Temple Admin" admin@temple.org
./nac-service-media config add sender avteam "A/V Team"
```

### Individual Commands

```bash
# Trim video only
./nac-service-media trim --source video.mp4 --start 00:05:30 --end 01:45:00

# Extract audio only
./nac-service-media extract-audio --source trimmed.mp4

# Upload to Drive
./nac-service-media upload --video trimmed.mp4 --audio audio.mp3

# Send email
./nac-service-media send-email --to jane --date 2025-12-28 --minister henkel \
  --audio-url "https://..." --video-url "https://..."
```

## Configuration

Example `config/config.yaml`:

```yaml
paths:
  source_directory: /mnt/d/Videos
  trimmed_directory: /mnt/d/Videos/Trimmed
  audio_directory: /mnt/d/Videos/Audio

audio:
  bitrate: 192k

google:
  credentials_file: oauth_credentials.json
  token_file: drive_token.json
  services_folder_id: YOUR_FOLDER_ID

email:
  from_name: Your Church Name
  from_address: church@gmail.com
  default_cc: []
  recipients:
    jane:
      name: Jane Doe
      address: jane@example.com

ministers:
  henkel:
    name: Pr. John Henkel

senders:
  default_sender: avteam
  senders:
    avteam:
      name: A/V Team

detection:
  enabled: true
  templates_dir: config/detection_templates
  thresholds:
    match_score: 0.85
    coarse_step_seconds: 30
  search_range:
    start_minutes: 10
    end_minutes: 70
```

## Google Cloud Setup

### Drive API

1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Create a new project
3. Enable the Google Drive API
4. Create OAuth 2.0 credentials (Desktop app)
5. Download as `oauth_credentials.json`
6. On first run, authorize in browser to generate `drive_token.json`

### Gmail API

1. Enable the Gmail API in the same project
2. Use the same OAuth credentials
3. On first run, authorize to generate `gmail_token.json`

## Auto-Detection

### Start Detection (Visual)

When `--start` is omitted, the tool automatically detects when the cross lights up using template matching. This requires:

1. OpenCV installed (`libopencv-dev libopencv-contrib-dev`)
2. Build with `-tags=detection`
3. `detection.enabled: true` in config
4. Template images in `config/detection_templates/`

The detection uses a 3-phase algorithm:
1. **Coarse scan**: Check every 30 seconds
2. **Binary search**: Narrow down to ~1 second
3. **Refinement**: Find exact frame

Typical accuracy: within 1 second of actual timestamp.

### End Detection (Audio)

When `--end` is omitted, the tool automatically detects the end of the three-fold amen song using audio template matching. This requires:

1. Python 3.8+ with librosa, numpy, scipy (`pip3 install librosa numpy scipy`)
2. Build with `-tags=detection`
3. `detection.enabled: true` in config
4. Audio template in `config/audio_templates/`

The detection uses chromagram cross-correlation:
1. Extract audio from video (FFmpeg)
2. Compute chromagram (pitch-based representation)
3. Cross-correlate with amen template
4. Find best match above confidence threshold

Typical accuracy: within 2 seconds of actual timestamp.

## Development

### Running Tests

```bash
go test ./...
```

### Project Structure

```
nac-service-media/
├── cmd/                    # CLI commands (cobra)
├── domain/                 # Domain models (DDD)
│   ├── video/             # Video processing
│   ├── distribution/      # Google Drive
│   ├── notification/      # Email
│   └── detection/         # Timestamp detection
├── application/           # Application services
├── infrastructure/        # External integrations
│   ├── config/           # YAML config
│   ├── ffmpeg/           # ffmpeg wrapper
│   ├── drive/            # Google Drive client
│   ├── gmail/            # Gmail client
│   └── detection/        # GoCV template matching
├── features/              # BDD tests (godog)
├── scripts/               # Helper scripts (Python detection)
└── config/
    ├── config.yaml        # Your config (gitignored)
    ├── detection_templates/  # Cross templates for start detection
    └── audio_templates/      # Amen template for end detection
```

## License

MIT
