#!/usr/bin/env python3
"""
Detect three-fold amen in a video and return the end timestamp.

This script is called by the Go application to perform audio template matching
for detecting the end of a church service.

Usage:
    python3 detect_amen.py <video_path> [start_offset_minutes] [search_duration_minutes] [template_dir]

Arguments:
    video_path              Path to the video file to analyze
    start_offset_minutes    Minutes from video start to begin searching (default: 20)
    search_duration_minutes Duration in minutes to search (default: 90)
    template_dir            Directory containing amen_chroma.npy (default: config/audio_templates)

Output (JSON to stdout):
    {"detected": true, "amen_end": "01:15:20", "amen_end_seconds": 4520, "confidence": 0.85}
    {"detected": false, "error": "No amen found above confidence threshold"}
"""

import sys
import json
import subprocess
import tempfile
import os
from pathlib import Path

# These imports are deferred to provide better error messages
numpy_available = False
librosa_available = False
scipy_available = False

try:
    import numpy as np
    numpy_available = True
except ImportError:
    pass

try:
    import librosa
    librosa_available = True
except ImportError:
    pass

try:
    from scipy import signal
    scipy_available = True
except ImportError:
    pass


def check_dependencies():
    """Check that all required Python packages are installed."""
    missing = []
    if not numpy_available:
        missing.append("numpy")
    if not librosa_available:
        missing.append("librosa")
    if not scipy_available:
        missing.append("scipy")

    if missing:
        return {
            "detected": False,
            "error": f"Missing Python packages: {', '.join(missing)}. Install with: pip install {' '.join(missing)}"
        }
    return None


def seconds_to_timestamp(secs: float) -> str:
    """Convert seconds to HH:MM:SS format."""
    h = int(secs) // 3600
    m = (int(secs) % 3600) // 60
    s = int(secs) % 60
    return f"{h:02d}:{m:02d}:{s:02d}"


def get_video_duration(video_path: str) -> float:
    """Get video duration in seconds using ffprobe."""
    cmd = [
        "ffprobe", "-v", "error",
        "-show_entries", "format=duration",
        "-of", "default=noprint_wrappers=1:nokey=1",
        video_path
    ]
    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        raise RuntimeError(f"ffprobe failed: {result.stderr}")
    return float(result.stdout.strip())


def extract_audio(video_path: str, start_sec: float, duration_sec: float, output_path: str) -> bool:
    """Extract audio clip from video using ffmpeg."""
    cmd = [
        "ffmpeg", "-y", "-ss", str(start_sec), "-i", video_path,
        "-t", str(duration_sec), "-vn", "-acodec", "pcm_s16le",
        "-ar", "22050", "-ac", "1", output_path
    ]
    result = subprocess.run(cmd, capture_output=True)
    return result.returncode == 0


def compute_chroma(audio_path: str, hop_length: int = 512):
    """
    Compute chromagram - represents pitch content independent of octave.
    Good for matching melodies across different recordings.
    """
    y, sr = librosa.load(audio_path, sr=22050)
    chroma = librosa.feature.chroma_cqt(y=y, sr=sr, hop_length=hop_length)
    return chroma, sr, hop_length


def match_template_chroma(template_chroma, search_chroma):
    """
    Find the best match of template in search audio using cross-correlation on chromagram.

    Returns: (best_position_frames, correlation_score, all_correlations)
    """
    # Normalize both
    template_norm = (template_chroma - template_chroma.mean()) / (template_chroma.std() + 1e-10)
    search_norm = (search_chroma - search_chroma.mean()) / (search_chroma.std() + 1e-10)

    # Cross-correlate each chroma bin and sum
    correlations = np.zeros(search_chroma.shape[1] - template_chroma.shape[1] + 1)

    for i in range(12):  # 12 chroma bins
        corr = signal.correlate(search_norm[i], template_norm[i], mode='valid')
        correlations += corr

    # Normalize by template length
    correlations /= template_chroma.shape[1]

    best_idx = np.argmax(correlations)
    best_score = correlations[best_idx] / 12  # Normalize by number of bins

    return best_idx, best_score


def find_amen(video_path: str, start_offset_minutes: int, search_duration_minutes: int, template_dir: str, min_confidence: float = 0.50):
    """
    Search for the three-fold amen in a video.

    Searches from a start offset (services typically start 5-20 minutes into recording).

    Args:
        video_path: Path to video file
        start_offset_minutes: Minutes from video start to begin searching
        search_duration_minutes: Duration in minutes to search
        template_dir: Directory containing amen_chroma.npy
        min_confidence: Minimum confidence threshold for detection

    Returns:
        dict with detection result
    """
    # Load template chromagram
    template_path = os.path.join(template_dir, "amen_chroma.npy")
    if not os.path.exists(template_path):
        return {
            "detected": False,
            "error": f"Template not found: {template_path}"
        }

    template_chroma = np.load(template_path)

    # Get video duration
    try:
        video_duration = get_video_duration(video_path)
    except Exception as e:
        return {
            "detected": False,
            "error": f"Failed to get video duration: {e}"
        }

    # Search from start_offset for search_duration minutes
    search_start = start_offset_minutes * 60
    search_duration = min(search_duration_minutes * 60, video_duration - search_start)

    if search_duration <= 0:
        return {
            "detected": False,
            "error": f"Video too short: {video_duration:.0f}s, need at least {search_start}s"
        }

    # Extract audio to temp file
    with tempfile.NamedTemporaryFile(suffix=".wav", delete=False) as tmp:
        search_audio_path = tmp.name

    try:
        if not extract_audio(video_path, search_start, search_duration, search_audio_path):
            return {
                "detected": False,
                "error": "Failed to extract audio from video"
            }

        # Compute chromagram of search region
        search_chroma, sr, hop = compute_chroma(search_audio_path)

        # Match template
        best_idx, best_score = match_template_chroma(template_chroma, search_chroma)

        # Convert frame index to time
        best_time_in_search = best_idx * hop / sr
        best_time_absolute = search_start + best_time_in_search

        # End of amen = start + template duration
        template_duration = template_chroma.shape[1] * hop / sr
        amen_end_absolute = best_time_absolute + template_duration

        if best_score < min_confidence:
            return {
                "detected": False,
                "error": f"Best match score {best_score:.2f} below threshold {min_confidence:.2f}",
                "best_score": best_score,
                "best_time": seconds_to_timestamp(amen_end_absolute)
            }

        return {
            "detected": True,
            "amen_start": seconds_to_timestamp(best_time_absolute),
            "amen_start_seconds": int(best_time_absolute),
            "amen_end": seconds_to_timestamp(amen_end_absolute),
            "amen_end_seconds": int(amen_end_absolute),
            "confidence": best_score
        }

    finally:
        if os.path.exists(search_audio_path):
            os.remove(search_audio_path)


def main():
    # Handle help flag
    if len(sys.argv) > 1 and sys.argv[1] in ("-h", "--help"):
        print(__doc__)
        sys.exit(0)

    # Check arguments
    if len(sys.argv) < 2:
        result = {
            "detected": False,
            "error": "Usage: detect_amen.py <video_path> [start_offset_minutes] [search_duration_minutes] [template_dir]"
        }
        print(json.dumps(result))
        sys.exit(1)

    # Check dependencies
    dep_error = check_dependencies()
    if dep_error:
        print(json.dumps(dep_error))
        sys.exit(1)

    video_path = sys.argv[1]
    start_offset_minutes = int(sys.argv[2]) if len(sys.argv) > 2 else 20  # Start looking 20 min in
    search_duration_minutes = int(sys.argv[3]) if len(sys.argv) > 3 else 90  # Search for 90 min

    # Determine template directory
    if len(sys.argv) > 4:
        template_dir = sys.argv[4]
    else:
        # Default: config/audio_templates relative to this script's parent directory
        script_dir = Path(__file__).parent.parent
        template_dir = str(script_dir / "config" / "audio_templates")

    # Check video exists
    if not os.path.exists(video_path):
        result = {
            "detected": False,
            "error": f"Video file not found: {video_path}"
        }
        print(json.dumps(result))
        sys.exit(1)

    # Run detection
    result = find_amen(video_path, start_offset_minutes, search_duration_minutes, template_dir)
    print(json.dumps(result))


if __name__ == "__main__":
    main()
