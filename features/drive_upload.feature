Feature: Google Drive Upload and Sharing
  As a user
  I want to upload files to Google Drive
  So that recipients can access the recordings

  Background:
    Given the Services folder ID is "test-folder-id"
    And valid Google Drive upload credentials

  Scenario: Upload video file
    Given I have a video file at "/tmp/test-video.mp4"
    When I upload the video to the Services folder
    Then the upload should succeed
    And I should receive a file ID

  Scenario: Upload audio file
    Given I have an audio file at "/tmp/test-audio.mp3"
    When I upload the audio to the Services folder
    Then the upload should succeed
    And I should receive a file ID

  Scenario: Set public sharing permission
    Given I have uploaded a file with ID "test-file-id"
    When I set public sharing permission
    Then the permission should be set successfully

  Scenario: Upload and share video in one operation
    Given I have a video file at "/tmp/test-video.mp4"
    When I upload and share the video
    Then the upload should succeed
    And I should receive a shareable URL in the format "https://drive.google.com/file/d/.../view?usp=sharing"

  Scenario: Upload and share both video and audio
    Given I have a video file at "/tmp/test-video.mp4"
    And I have an audio file at "/tmp/test-audio.mp3"
    When I distribute both files
    Then both uploads should succeed
    And I should receive shareable URLs for both files

  Scenario: Handle upload failure for non-existent file
    Given I have a video file at "/tmp/nonexistent.mp4"
    When I attempt to upload the video
    Then I should receive an error about missing file

  Scenario: Handle permission setting failure
    Given I have uploaded a file with ID "test-file-id"
    And the permission API will fail
    When I attempt to set public sharing permission
    Then I should receive an error about permission failure
