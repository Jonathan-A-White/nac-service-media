Feature: Google Drive Authentication
  As a user
  I want to authenticate with Google Drive
  So that I can upload and manage service recordings

  Background:
    Given the Services folder ID is "test-folder-id"

  Scenario: Successful authentication with valid credentials
    Given valid Google Drive credentials
    When I initialize the Drive client
    Then the client should be authenticated
    And I should be able to list files in the Services folder

  Scenario: List files returns expected files
    Given valid Google Drive credentials
    And the Services folder contains files:
      | name            | mimeType  | size   |
      | 2025-12-01.mp4  | video/mp4 | 100000 |
      | 2025-12-08.mp4  | video/mp4 | 200000 |
      | 2025-12-15.mp3  | audio/mp3 | 50000  |
    When I list files in the Services folder
    Then I should see 3 files

  Scenario: Missing credentials file
    Given no credentials file exists
    When I attempt to initialize the Drive client
    Then I should receive an error about missing credentials

  Scenario: Invalid credentials file
    Given an invalid credentials file
    When I attempt to initialize the Drive client
    Then I should receive an error about invalid credentials

  Scenario: Folder not accessible
    Given valid Google Drive credentials
    And the Services folder is not accessible
    When I attempt to list files in the Services folder
    Then I should receive a permission denied error
