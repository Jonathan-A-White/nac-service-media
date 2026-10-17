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

# Install Python for end auto-detection (optional but recommended).
# Start auto-detection (the cross) needs only ffmpeg: no OpenCV.
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

# Build with end auto-detection enabled (requires Python with librosa)
go build -tags=detection

# Or build without end auto-detection (start auto-detection is always in)
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

### detect - See what the start detector sees, trimming nothing

```bash
# The search process would run, and the start it would trim from
nac-service-media detect --input "2026-09-27 10-22-05.mp4"

# What every picture of the cross scores in the named frames, saving each frame
nac-service-media detect --input "2026-09-27 10-22-05.mp4" --at 00:02:54,00:08:42 --save-frames /tmp/frames

# One line for every 30th second of the search range, no search
nac-service-media detect --input "2026-09-27 10-22-05.mp4" --every 30
```

Nothing is trimmed, uploaded, emailed or deleted, and no frames are left
beside the video; `--input` defaults to the newest file in the source folder.

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
    match_score: 0.75
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

1. ffmpeg (the frames come out of the video with it; no OpenCV, no build tag)
2. `detection.enabled: true` in config
3. Template images in `config/detection_templates/`

The detection uses a 3-phase algorithm:
1. **Coarse scan**: Check every 30 seconds for the first frame that shows the
   cross lit where it was seen unlit before, and that stays lit there
2. **Binary search**: Narrow down to ~1 second
3. **Refinement**: Find exact frame

One lit frame never decides the start. Once the cross has been seen unlit
somewhere in the frame, a lit cross anywhere else is not the start (on
11/29/2025 a match on the pianist's shot read "lit" for thirteen minutes
while the cross was out of view). And the cross must stay lit through the
three minutes of samples after the frame: no two samples show the cross
unlit anywhere (there is one cross, and one unlit frame is as fallible as
one lit one), and at least half of the readable ones show it lit (the
camera may cut to another shot of it, or look elsewhere meanwhile). On
09/27/2026 the camera panned to the choir rehearsing before the service: the
moving picture of the unlit cross, then a white shirt in the choir close-up,
each matched a lit picture for a frame, and the old search stopped there,
5:48 early. Such frames are logged as `not the start: lit elsewhere than the
cross was seen unlit` or `not the start: the cross was unlit at 00:06:00 and
at 00:06:30`, and a miss counts them. A cross lit before it was ever seen
unlit (a recording that starts late, or a camera parked away from the cross)
is the start unless the rest of the search range shows the cross unlit and
then lit where it was unlit: on 08/09/2026 a static match on the lectern
shot read "lit" for ten minutes, until the wide shot showed the cross unlit
and then lit at 00:12:28. Ten unreadable frames in a row end the scan: the
video is over.

Typical accuracy: within 1 second of actual timestamp.

Each camera shot is a pair of pictures in the templates folder,
`<shot>_lit.png` and `<shot>_unlit.png`; every pair there is used. Each
picture is tried at every scale from half its size to double, in steps of
about 9%, so the camera may zoom in or out by up to 2x from the shot the
picture was taken from. A frame counts as showing a shot when some picture's
match score reaches `detection.thresholds.match_score` (default 0.75: a
screenshot of the 10/11/2026 service scored 0.83 with the `wide` picture at
its own size, under the old 0.85, so a run that missed from 09/13 on most
likely saw the cross just under that line). Whether the cross is lit is then
decided by how much brighter the cross is than the wall around it, set between
the shot's unlit and lit pictures, together with which of the two pictures
fits better.

The log has one line per frame looked at, so a run can be read back:

```
  00:14:00  unlit  wide 1.00x score 0.81 (cross 12% lit) at (808,28)
  00:14:30  lit    wide 1.00x score 0.84 (cross 96% lit) at (808,28)
  00:00:05  not in view  best wide_lit 0.61 at 0.92x (threshold 0.75)
Detected start: 00:14:07 (wide shot at 1.00x, score 0.84)
```

### When the cross is not found

A failed run's email says, for each picture of the cross, the best it scored
over the whole search, at which scale and in which frame, and the threshold:

```
The best each picture of the cross scored (a frame shows the cross from 0.75):
  wide_lit     0.70 at 1.00x in the frame at 00:14:30
  wide_unlit   0.66 at 1.00x in the frame at 00:12:00
  closeup_lit  0.55 at 0.50x in the frame at 00:00:05
```

Read it like this: a lit picture scoring just under the threshold at a sensible
scale, in a frame where the cross was on, means the pictures still fit and
`match_score` is a little too high; every picture far under it means the
camera's shot changed, so no saved picture of the cross matches. The email also
names a folder beside the video (`<video name> cross frames`) holding the
frames it looked at, one every 30 seconds, named by their time in the video
(`00-14-30.jpg`). Find the first one with the cross lit and one just before
it. To look at any other moment:

```bash
nac-service-media templates frame --input "2026-10-11 10-17-09.mp4" --at 00:14:07 --out cross.png
```

Then teach it the new shot from a time with the cross off and a time with it
on (a second or two apart is best):

```bash
nac-service-media templates add --input "2026-10-11 10-17-09.mp4" \
  --unlit-at 00:14:06 --lit-at 00:14:07 --name room
```

It finds the cross as the part of the picture that brightens most, prints the
box it chose and writes `room_lit.png` and `room_unlit.png` into the templates
folder. Look at the two pictures; if the box is wrong, give it yourself with
`--region x,y,w,h` (pixels of the full frame, as in the frame PNG) and
`--force` to replace the shot. Then run `process` again: it finds the cross by
itself.

A run that could not look at the video at all (no frame could be read, no
templates, detection not built in) says "cross search could not run" instead,
and an amen search that could not run (for example, the video is too short)
says "amen search could not run".

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

## Scheduled Automation (Windows)

The tool can be set up to run automatically twice per week via Windows Task Scheduler. This works even when WSL is not actively open.

### Installation

From WSL:

```bash
make install-scheduled-task
```

This will:
1. Install the binary to `~/go/bin/nac-service-media` (with detection enabled)
2. Add `~/go/bin` to PATH in `.profile` (if not already present)
3. Create two Windows Scheduled Tasks:
   - `NAC-Service-Media-Weekly-Sunday` - Runs every Sunday at 12:30 PM
   - `NAC-Service-Media-Weekly-Wednesday` - Runs every Wednesday at 9:30 PM

### Manual Testing

```bash
# Run a scheduled task immediately (from PowerShell)
schtasks /run /tn "NAC-Service-Media-Weekly-Sunday"
schtasks /run /tn "NAC-Service-Media-Weekly-Wednesday"

# View logs (from WSL)
tail -50 logs/scheduled-run-*.log
```

### Uninstallation

```bash
make uninstall-scheduled-task
```

Logs are written to `logs/scheduled-run-YYYY-MM-DD_HH-mm-ss.log`.

### Troubleshooting a Missed Run

A log file is only created once `scheduled-task.ps1` actually starts. If there is
no log for an expected run, the task never launched — that is a Task Scheduler
question, not an application one. From PowerShell:

```powershell
# Did it run? LastTaskResult 0 = success. NumberOfMissedRuns > 0 = skipped triggers.
Get-ScheduledTaskInfo -TaskName "NAC-Service-Media-Weekly-Sunday" |
  Select LastRunTime, LastTaskResult, NextRunTime, NumberOfMissedRuns

# Why was it skipped? Event 332 names the unmet launch condition.
Get-WinEvent -FilterHashtable @{
  LogName='Microsoft-Windows-TaskScheduler/Operational'
  StartTime=(Get-Date).Date
} | Where-Object { $_.Message -match 'NAC-Service-Media' } |
  Select-Object TimeCreated, Id, LevelDisplayName, Message | Format-List

# Was the machine asleep through the trigger? A large forward "Time Delta" here
# means the clock jumped over the scheduled time on resume.
Get-WinEvent -FilterHashtable @{
  LogName='System'; ProviderName='Microsoft-Windows-Kernel-General'; Id=1
  StartTime=(Get-Date).AddDays(-14)
} | Select-Object TimeCreated, Message | Format-List
```

To run a missed week by hand (this goes through the wrapper, so it produces a log):

```powershell
schtasks /run /tn "NAC-Service-Media-Weekly-Sunday"
```

Note that a run whose `.mp4` and `.mp3` are already on Drive exits early (exit
code 0) with `Most recent file (...) has already been processed` and sends no
email — so a partial run that uploaded but failed before emailing cannot be
retried this way.

When a run fails (cross or amen not found, Google sign-in, upload, ffmpeg, ...)
it sends one email to its `--recipient` titled `Service recording NOT processed:
<date>: <reason>`, with the details, the video it used, the last 30 log lines
and the exact `nac-service-media process --input ...` command to finish it by
hand, and exits 1. If that email cannot be sent either, the log says
`FAILURE EMAIL NOT SENT`. A run that finds another run still holding the lock
also exits 1 and emails. In a scheduled run (nobody at the terminal) a Google
sign-in that needs the browser gives up after two minutes; fix it with
`nac-service-media auth status --fix`.

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
│   └── detection/        # Cross template matching (Go, FFT) and the amen wrapper
├── features/              # BDD tests (godog)
├── scripts/               # Helper scripts (Python detection)
└── config/
    ├── config.yaml        # Your config (gitignored)
    ├── detection_templates/  # Cross templates for start detection
    └── audio_templates/      # Amen template for end detection
```

For what each package is responsible for, which files stories collide on, and proposed splits, see [docs/module-map.md](docs/module-map.md).

## License

MIT
