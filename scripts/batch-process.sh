#!/bin/bash
# Batch process all unprocessed source videos
#
# This script scans the source directory for .mp4 files, checks which ones
# haven't been processed yet (by looking for corresponding .mp3 in audio dir),
# and runs the process command for each unprocessed file.
#
# Usage: ./scripts/batch-process.sh [--dry-run]

set -euo pipefail

# Add Go bin to PATH for yq
export PATH="$PATH:$HOME/go/bin"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
CONFIG_FILE="$PROJECT_DIR/config/config.yaml"
BINARY="$PROJECT_DIR/bin/nac-service-media"

# Default options
DRY_RUN=false
RECIPIENT="Jonathan"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --recipient)
            RECIPIENT="$2"
            shift 2
            ;;
        -h|--help)
            echo "Usage: $0 [--dry-run] [--recipient NAME]"
            echo ""
            echo "Options:"
            echo "  --dry-run       Show what would be processed without actually processing"
            echo "  --recipient     Recipient config key (default: Jonathan)"
            echo "  -h, --help      Show this help message"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            exit 1
            ;;
    esac
done

# Check dependencies
if ! command -v yq &> /dev/null; then
    echo "Error: yq is required but not installed."
    echo "Install with: go install github.com/mikefarah/yq/v4@latest"
    exit 1
fi

if [[ ! -f "$CONFIG_FILE" ]]; then
    echo "Error: Config file not found: $CONFIG_FILE"
    exit 1
fi

if [[ ! -x "$BINARY" ]]; then
    echo "Error: Binary not found or not executable: $BINARY"
    echo "Run 'make build' first."
    exit 1
fi

# Read directories from config
SOURCE_DIR=$(yq '.paths.source_directory' "$CONFIG_FILE")
AUDIO_DIR=$(yq '.paths.audio_directory' "$CONFIG_FILE")

if [[ -z "$SOURCE_DIR" ]] || [[ "$SOURCE_DIR" == "null" ]]; then
    echo "Error: source_directory not configured in $CONFIG_FILE"
    exit 1
fi

if [[ -z "$AUDIO_DIR" ]] || [[ "$AUDIO_DIR" == "null" ]]; then
    echo "Error: audio_directory not configured in $CONFIG_FILE"
    exit 1
fi

echo "Source directory: $SOURCE_DIR"
echo "Audio directory: $AUDIO_DIR"
echo "Recipient: $RECIPIENT"
echo ""

# Counters
processed=0
skipped=0
skipped_smaller=0
failed=0
failed_files=()

# Find all mp4 files in source directory
shopt -s nullglob
mp4_files=("$SOURCE_DIR"/*.mp4)
shopt -u nullglob

if [[ ${#mp4_files[@]} -eq 0 ]]; then
    echo "No .mp4 files found in source directory."
    exit 0
fi

echo "Found ${#mp4_files[@]} source file(s)"

# Build associative array: date -> largest file for that date
declare -A largest_file_for_date
declare -A largest_size_for_date

for mp4_file in "${mp4_files[@]}"; do
    filename=$(basename "$mp4_file")

    # Extract date from OBS format: "YYYY-MM-DD HH-MM-SS.mp4" -> "YYYY-MM-DD"
    date_part=$(echo "$filename" | cut -d' ' -f1)

    # Validate date format
    if ! [[ "$date_part" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
        continue
    fi

    # Get file size
    file_size=$(stat -c%s "$mp4_file" 2>/dev/null || stat -f%z "$mp4_file" 2>/dev/null || echo 0)

    # Check if this is the largest file for this date
    if [[ -z "${largest_size_for_date[$date_part]:-}" ]] || [[ "$file_size" -gt "${largest_size_for_date[$date_part]}" ]]; then
        largest_file_for_date[$date_part]="$mp4_file"
        largest_size_for_date[$date_part]="$file_size"
    fi
done

# Get unique dates sorted
dates=($(echo "${!largest_file_for_date[@]}" | tr ' ' '\n' | sort))

echo "Found ${#dates[@]} unique date(s)"
echo ""

for date_part in "${dates[@]}"; do
    mp4_file="${largest_file_for_date[$date_part]}"
    filename=$(basename "$mp4_file")

    # Check if corresponding audio file exists
    audio_file="$AUDIO_DIR/$date_part.mp3"

    if [[ -f "$audio_file" ]]; then
        echo "⏭️  Skipping $date_part (already processed: $date_part.mp3 exists)"
        ((skipped++)) || true
        continue
    fi

    # Process this file
    echo "🎬 Processing: $filename (largest for $date_part)"

    if [[ "$DRY_RUN" == "true" ]]; then
        echo "   [DRY RUN] Would run: $BINARY process --input \"$mp4_file\" --recipient $RECIPIENT --skip-video"
        ((processed++)) || true
    else
        if "$BINARY" process --input "$mp4_file" --recipient "$RECIPIENT" --skip-video; then
            echo "✅ Completed: $filename"
            ((processed++)) || true
        else
            echo "❌ Failed: $filename"
            ((failed++)) || true
            failed_files+=("$filename")
        fi
    fi

    echo ""
done

# Summary
echo "========================================"
echo "Batch Processing Complete"
echo "========================================"
echo "Processed: $processed"
echo "Skipped:   $skipped"
echo "Failed:    $failed"

if [[ ${#failed_files[@]} -gt 0 ]]; then
    echo ""
    echo "Failed files:"
    for f in "${failed_files[@]}"; do
        echo "  - $f"
    done
fi

# Exit with error if any failures
if [[ $failed -gt 0 ]]; then
    exit 1
fi
