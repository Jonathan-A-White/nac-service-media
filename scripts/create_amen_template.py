#!/usr/bin/env python3
"""
Create amen audio template from a video.

Extracts the three-fold amen audio and computes a chromagram for template matching.

Usage:
    python3 create_amen_template.py <video_path> <start_timestamp> <end_timestamp> [output_dir]

Arguments:
    video_path       Path to the video file
    start_timestamp  Start of amen in HH:MM:SS format (e.g., 01:54:04)
    end_timestamp    End of amen in HH:MM:SS format (e.g., 01:54:22)
    output_dir       Output directory (default: config/audio_templates)

Example:
    python3 create_amen_template.py /path/to/video.mp4 01:54:04 01:54:22

Output:
    Creates amen_template.wav and amen_chroma.npy in the output directory.
"""

import sys
import os
import subprocess
from pathlib import Path

try:
    import numpy as np
except ImportError:
    print("Error: numpy is required. Install with: pip install numpy")
    sys.exit(1)

try:
    import librosa
except ImportError:
    print("Error: librosa is required. Install with: pip install librosa")
    sys.exit(1)


def parse_timestamp(ts: str) -> int:
    """Parse HH:MM:SS or MM:SS to seconds."""
    parts = ts.split(":")
    if len(parts) == 3:
        return int(parts[0]) * 3600 + int(parts[1]) * 60 + int(parts[2])
    elif len(parts) == 2:
        return int(parts[0]) * 60 + int(parts[1])
    else:
        raise ValueError(f"Invalid timestamp format: {ts}")


def extract_audio(video_path: str, start_sec: int, duration_sec: int, output_path: str) -> bool:
    """Extract audio clip from video using ffmpeg."""
    cmd = [
        "ffmpeg", "-y",
        "-ss", str(start_sec),
        "-i", video_path,
        "-t", str(duration_sec),
        "-vn",
        "-acodec", "pcm_s16le",
        "-ar", "22050",
        "-ac", "1",
        output_path
    ]
    result = subprocess.run(cmd, capture_output=True)
    if result.returncode != 0:
        print(f"FFmpeg error: {result.stderr.decode()}")
        return False
    return True


def compute_chroma(audio_path: str, hop_length: int = 512):
    """Compute chromagram for the audio file."""
    y, sr = librosa.load(audio_path, sr=22050)
    chroma = librosa.feature.chroma_cqt(y=y, sr=sr, hop_length=hop_length)
    return chroma, sr, hop_length


def main():
    if len(sys.argv) < 4:
        print(__doc__)
        sys.exit(1)

    video_path = sys.argv[1]
    start_ts = sys.argv[2]
    end_ts = sys.argv[3]

    # Determine output directory
    if len(sys.argv) > 4:
        output_dir = sys.argv[4]
    else:
        script_dir = Path(__file__).parent.parent
        output_dir = str(script_dir / "config" / "audio_templates")

    # Validate inputs
    if not os.path.exists(video_path):
        print(f"Error: Video file not found: {video_path}")
        sys.exit(1)

    # Parse timestamps
    try:
        start_sec = parse_timestamp(start_ts)
        end_sec = parse_timestamp(end_ts)
    except ValueError as e:
        print(f"Error: {e}")
        sys.exit(1)

    duration = end_sec - start_sec
    if duration <= 0:
        print(f"Error: End timestamp must be after start timestamp")
        sys.exit(1)

    print(f"Creating amen template from {video_path}")
    print(f"  Start: {start_ts} ({start_sec}s)")
    print(f"  End: {end_ts} ({end_sec}s)")
    print(f"  Duration: {duration}s")
    print(f"  Output: {output_dir}")

    # Create output directory
    os.makedirs(output_dir, exist_ok=True)

    # Extract audio
    wav_path = os.path.join(output_dir, "amen_template.wav")
    print(f"\nExtracting audio...")
    if not extract_audio(video_path, start_sec, duration, wav_path):
        print("Failed to extract audio")
        sys.exit(1)
    print(f"  Created: {wav_path}")

    # Compute and save chromagram
    print(f"\nComputing chromagram...")
    chroma, sr, hop = compute_chroma(wav_path)
    chroma_path = os.path.join(output_dir, "amen_chroma.npy")
    np.save(chroma_path, chroma)
    print(f"  Created: {chroma_path}")
    print(f"  Shape: {chroma.shape}")
    print(f"  Computed duration: {chroma.shape[1] * hop / sr:.2f}s")

    print(f"\nTemplate created successfully!")
    print(f"Files:")
    print(f"  - {wav_path}")
    print(f"  - {chroma_path}")


if __name__ == "__main__":
    main()
