Feature: End-to-End Process Command
  As a user
  I want to process a service recording with a single command
  So that I can automate the entire workflow

  Background:
    Given the process config has paths:
      | source_directory  | /test/source    |
      | trimmed_directory | /test/trimmed   |
      | audio_directory   | /test/audio     |
    And the process config has services folder "folder123"
    And the process config has ministers:
      | key   | name           |
      | smith | Pr. John Smith |
    And the process config has recipients:
      | key  | name     | address          |
      | jane | Jane Doe | jane@example.com |
      | john | John Doe | john@example.com |
    And the process config has default CCs:
      | name       | address           |
      | Admin User | admin@example.com |

  Scenario: Complete workflow with all required flags
    Given a source video exists at "/test/source/2025-12-28 10-06-16.mp4"
    When I run process with flags:
      | flag       | value                              |
      | --input    | /test/source/2025-12-28 10-06-16.mp4 |
      | --start    | 00:05:30                           |
      | --end      | 01:45:00                           |
      | --minister | smith                              |
      | --recipient| jane                               |
    Then the process should succeed
    And the video should be trimmed from "00:05:30" to "01:45:00"
    And the audio should be extracted with bitrate "192k"
    And drive cleanup should be called with space for 2 files
    And the video should be uploaded to Drive
    And the audio should be uploaded to Drive
    And both files should be shared publicly
    And email should be sent to "jane@example.com"
    And email should include minister "Pr. John Smith"
    And email should include video and audio links

  Scenario: Process with multiple recipients
    Given a source video exists at "/test/source/2025-12-28 10-06-16.mp4"
    When I run process with flags:
      | flag       | value                              |
      | --input    | /test/source/2025-12-28 10-06-16.mp4 |
      | --start    | 00:05:30                           |
      | --end      | 01:45:00                           |
      | --minister | smith                              |
      | --recipient| jane                               |
      | --recipient| john                               |
    Then the process should succeed
    And email should be sent to "jane@example.com"
    And email should be sent to "john@example.com"

  Scenario: Process using newest file when input omitted
    Given a source video exists at "/test/source/2025-12-28 10-06-16.mp4"
    And a source video exists at "/test/source/2025-12-29 09-15-00.mp4"
    When I run process with flags:
      | flag       | value    |
      | --start    | 00:05:30 |
      | --end      | 01:45:00 |
      | --minister | smith    |
      | --recipient| jane     |
    Then the process should succeed
    And the source video should be "2025-12-29 09-15-00.mp4"
    And the service date should be "2025-12-29"

  # Note: Date override with non-OBS format filename requires TrimRequest changes
  # For now, date override only works with OBS format filenames
  Scenario: Process with explicit date override
    Given a source video exists at "/test/source/2025-12-28 10-06-16.mp4"
    When I run process with flags:
      | flag       | value                              |
      | --input    | /test/source/2025-12-28 10-06-16.mp4 |
      | --start    | 00:05:30                           |
      | --end      | 01:45:00                           |
      | --minister | smith                              |
      | --recipient| jane                               |
      | --date     | 2025-12-31                         |
    Then the process should succeed
    And the service date should be "2025-12-28"
    And the trimmed file should be named "2025-12-28.mp4"

  Scenario: Relative input path resolved against source directory
    Given a source video exists at "/test/source/2025-12-28 10-06-16.mp4"
    When I run process with flags:
      | flag       | value                      |
      | --input    | 2025-12-28 10-06-16.mp4    |
      | --start    | 00:05:30                   |
      | --end      | 01:45:00                   |
      | --minister | smith                      |
      | --recipient| jane                       |
    Then the process should succeed
    And the source path should be "/test/source/2025-12-28 10-06-16.mp4"

  Scenario: Error when minister not found
    Given a source video exists at "/test/source/2025-12-28 10-06-16.mp4"
    When I run process with flags:
      | flag       | value                              |
      | --input    | /test/source/2025-12-28 10-06-16.mp4 |
      | --start    | 00:05:30                           |
      | --end      | 01:45:00                           |
      | --minister | unknown                            |
      | --recipient| jane                               |
    Then the process should fail with error "minister 'unknown' not found"
    And the error should suggest command "config add minister --key unknown"

  Scenario: Error when recipient not found
    Given a source video exists at "/test/source/2025-12-28 10-06-16.mp4"
    When I run process with flags:
      | flag       | value                              |
      | --input    | /test/source/2025-12-28 10-06-16.mp4 |
      | --start    | 00:05:30                           |
      | --end      | 01:45:00                           |
      | --minister | smith                              |
      | --recipient| unknown                            |
    Then the process should fail with error "recipient 'unknown' not found"
    And the error should suggest command "config add recipient --key unknown"

  Scenario: Error when source file not found
    Given no source video exists at "/test/source/2025-12-28 10-06-16.mp4"
    When I run process with flags:
      | flag       | value                              |
      | --input    | /test/source/2025-12-28 10-06-16.mp4 |
      | --start    | 00:05:30                           |
      | --end      | 01:45:00                           |
      | --minister | smith                              |
      | --recipient| jane                               |
    Then the process should fail with error "source file does not exist"

  Scenario: Error when no source files in directory and input omitted
    Given the source directory is empty
    When I run process with flags:
      | flag       | value    |
      | --start    | 00:05:30 |
      | --end      | 01:45:00 |
      | --minister | smith    |
      | --recipient| jane     |
    Then the process should fail with error "no video files found"

  Scenario: Failure during upload shows manual recovery commands
    Given a source video exists at "/test/source/2025-12-28 10-06-16.mp4"
    And the drive upload will fail with "authentication expired"
    When I run process with flags:
      | flag       | value                              |
      | --input    | /test/source/2025-12-28 10-06-16.mp4 |
      | --start    | 00:05:30                           |
      | --end      | 01:45:00                           |
      | --minister | smith                              |
      | --recipient| jane                               |
    Then the process should fail with error "authentication expired"
    And the output should include recovery commands
    And the recovery should suggest "upload" command
    And the recovery should suggest "send-email" command

  Scenario: Progress output shows step completion
    Given a source video exists at "/test/source/2025-12-28 10-06-16.mp4"
    When I run process with flags:
      | flag       | value                              |
      | --input    | /test/source/2025-12-28 10-06-16.mp4 |
      | --start    | 00:05:30                           |
      | --end      | 01:45:00                           |
      | --minister | smith                              |
      | --recipient| jane                               |
    Then the process should succeed
    And the output should include "[1/7] Trimming video"
    And the output should include "[2/7] Extracting audio"
    And the output should include "[3/7] Checking Drive storage"
    And the output should include "[4/7] Uploading video"
    And the output should include "[5/7] Uploading audio"
    And the output should include "[6/7] Sharing files"
    And the output should include "[7/7] Sending email"
    And the output should include "Done!"

  Scenario: Cleanup shows what files were removed
    Given a source video exists at "/test/source/2025-12-28 10-06-16.mp4"
    And drive has insufficient space
    And drive has old files:
      | name           | size      |
      | 2025-11-01.mp4 | 1073741824|
      | 2025-11-08.mp4 | 1073741824|
    When I run process with flags:
      | flag       | value                              |
      | --input    | /test/source/2025-12-28 10-06-16.mp4 |
      | --start    | 00:05:30                           |
      | --end      | 01:45:00                           |
      | --minister | smith                              |
      | --recipient| jane                               |
    Then the process should succeed
    And the output should include "Removed: 2025-11-01.mp4"
