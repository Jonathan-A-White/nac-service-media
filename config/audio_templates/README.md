# Audio Templates for End Detection

## Three-Fold Amen Template

The `amen_template.wav` and `amen_chroma.npy` files are used to detect
the end of service by finding the three-fold amen song.

### How It Works

The three-fold amen is a consistent, identifiable audio marker:
- Always same tune, same notes, same key
- ~18 seconds duration
- Sung by congregation with organ
- Chromagram cross-correlation reliably detects it

The detection searches from 20 minutes into the video for up to 90 minutes,
which covers services that are typically 45-60 minutes long.

### Creating a new template

If you need to create a template for a different congregation (slightly different amen):

1. Find a clear recording of the three-fold amen
2. Note the start and end timestamps (e.g., 01:54:04 to 01:54:22)
3. Run:
   ```bash
   python3 scripts/create_amen_template.py <video_path> <start_timestamp> <end_timestamp>
   ```

Example:
```bash
python3 scripts/create_amen_template.py /path/to/video.mp4 01:54:04 01:54:22
```

This will create both the WAV and chromagram files in this directory.

### Manual Detection Testing

To test detection on a specific video:
```bash
# Default: search from 20 min to 110 min (90 min duration)
python3 scripts/detect_amen.py /path/to/video.mp4

# Custom: search from 15 min for 60 min duration
python3 scripts/detect_amen.py /path/to/video.mp4 15 60
```

### Template Files

- `amen_template.wav` - Raw audio of three-fold amen (~18s)
- `amen_chroma.npy` - Pre-computed chromagram for fast matching

### Python Dependencies

```bash
pip install librosa numpy scipy
```
