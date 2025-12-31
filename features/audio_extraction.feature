Feature: Audio Extraction
  As a user
  I want to extract audio from trimmed videos
  So that recipients can download a smaller file

  Background:
    Given the audio output directory is "/tmp/test-audio"
    And the default audio bitrate is "192k"

  Scenario: Extract audio from trimmed video with default bitrate
    Given a trimmed video at "/test/trimmed/2025-12-28.mp4"
    When I extract audio for service date "2025-12-28"
    Then the audio output file should be "/tmp/test-audio/2025-12-28.mp3"
    And ffmpeg should have been called with audio arguments:
      | argument     |
      | -i           |
      | -vn          |
      | -acodec      |
      | libmp3lame   |
      | -ab          |
      | 192k         |

  Scenario: Extract audio with custom bitrate
    Given a trimmed video at "/test/trimmed/2025-12-28.mp4"
    When I extract audio with bitrate "128k" for service date "2025-12-28"
    Then ffmpeg should have been called with audio arguments:
      | argument     |
      | -ab          |
      | 128k         |

  Scenario: Extract audio with high quality bitrate
    Given a trimmed video at "/test/trimmed/2025-12-28.mp4"
    When I extract audio with bitrate "320k" for service date "2025-12-28"
    Then ffmpeg should have been called with audio arguments:
      | argument     |
      | -ab          |
      | 320k         |

  Scenario: Source video does not exist
    Given no trimmed video exists at "/test/trimmed/2025-12-28.mp4"
    When I attempt to extract audio for service date "2025-12-28"
    Then I should receive an error about missing source video

  Scenario: Audio extraction preserves service date in filename
    Given a trimmed video at "/test/trimmed/2025-01-15.mp4"
    When I extract audio for service date "2025-01-15"
    Then the audio output file should be "/tmp/test-audio/2025-01-15.mp3"
