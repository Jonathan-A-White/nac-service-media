Feature: Video Trimming
  As a user
  I want to trim a video to specific timestamps
  So that only the service content is included

  Background:
    Given the trimmed output directory is "/tmp/test-trimmed"

  Scenario: Trim video with valid timestamps
    Given a source video at "/test/videos/2025-12-28 10-06-16.mp4"
    When I trim the video from "00:05:30" to "01:45:00"
    Then the output file should be "/tmp/test-trimmed/2025-12-28.mp4"
    And ffmpeg should have been called with arguments:
      | argument |
      | -i       |
      | -ss      |
      | 00:05:30 |
      | -to      |
      | 01:45:00 |
      | -c       |
      | copy     |

  Scenario: Invalid timestamp format - missing leading zero
    Given a source video at "/test/videos/2025-12-28 10-06-16.mp4"
    When I attempt to trim with start time "5:30:00"
    Then I should receive an error about invalid timestamp format

  Scenario: Invalid timestamp format - wrong separator
    Given a source video at "/test/videos/2025-12-28 10-06-16.mp4"
    When I attempt to trim with start time "00-30-00"
    Then I should receive an error about invalid timestamp format

  Scenario: End time before start time
    Given a source video at "/test/videos/2025-12-28 10-06-16.mp4"
    When I attempt to trim from "01:00:00" to "00:30:00"
    Then I should receive an error about end time before start time

  Scenario: End time equals start time
    Given a source video at "/test/videos/2025-12-28 10-06-16.mp4"
    When I attempt to trim from "00:30:00" to "00:30:00"
    Then I should receive an error about end time before start time

  Scenario: Source file does not exist
    Given no source video exists at "/nonexistent/2025-12-28 10-06-16.mp4"
    When I attempt to trim from "00:05:30" to "01:45:00"
    Then I should receive an error about missing source file

  Scenario: Source filename in wrong format
    Given a source video at "/test/videos/recording.mp4"
    When I attempt to trim from "00:05:30" to "01:45:00"
    Then I should receive an error about invalid source filename
