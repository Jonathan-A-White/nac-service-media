Feature: Google Drive Storage Cleanup
  As a user
  I want old recordings automatically deleted
  So that there's always space for new uploads

  Background:
    Given the Services folder ID is "test-folder-id"
    And valid Google Drive credentials

  Scenario: Sufficient space available - no deletion needed
    Given there is 2 GB of available storage
    And the Services folder contains mp4 files:
      | name             | size        |
      | 2025-11-10.mp4   | 1073741824  |
      | 2025-11-17.mp4   | 1073741824  |
    When I ensure 1 GB of space is available
    Then no files should be deleted
    And the cleanup result should show 0 bytes freed

  Scenario: Delete oldest trimmed file to make space
    Given there is 500 MB of available storage
    And the Services folder contains mp4 files:
      | name             | size        |
      | 2025-11-10.mp4   | 1073741824  |
      | 2025-11-17.mp4   | 1073741824  |
      | 2025-11-24.mp4   | 1073741824  |
    When I ensure 1 GB of space is available
    Then "2025-11-10.mp4" should be deleted
    And the cleanup result should show 1 file deleted

  Scenario: Delete oldest file with raw upload format
    Given there is 500 MB of available storage
    And the Services folder contains mp4 files:
      | name                       | size        |
      | 2025-11-03 10-06-16.mp4    | 1073741824  |
      | 2025-11-10.mp4             | 1073741824  |
      | 2025-11-17.mp4             | 1073741824  |
    When I ensure 1 GB of space is available
    Then "2025-11-03 10-06-16.mp4" should be deleted
    And the cleanup result should show 1 file deleted

  Scenario: Delete multiple files including raw uploads to make space
    Given there is 200 MB of available storage
    And the Services folder contains mp4 files:
      | name                       | size        |
      | 2025-10-01 09-30-00.mp4    | 536870912   |
      | 2025-10-15.mp4             | 536870912   |
      | 2025-11-01.mp4             | 1073741824  |
    When I ensure 1 GB of space is available
    Then "2025-10-01 09-30-00.mp4" should be deleted
    And "2025-10-15.mp4" should be deleted
    And the cleanup result should show 2 files deleted

  Scenario: No mp4 files available to delete
    Given there is 100 MB of available storage
    And the Services folder contains mp4 files:
      | name | size |
    When I ensure 1 GB of space is available
    Then I should receive an error about insufficient storage

  Scenario: Files sorted correctly with mixed formats
    Given the Services folder contains mp4 files:
      | name                       | size        |
      | 2025-11-24.mp4             | 1073741824  |
      | 2025-11-10 08-00-00.mp4    | 1073741824  |
      | 2025-11-17.mp4             | 1073741824  |
      | 2025-11-03.mp4             | 1073741824  |
    When I list mp4 files sorted by date
    Then the files should be in order:
      | name                       |
      | 2025-11-03.mp4             |
      | 2025-11-10 08-00-00.mp4    |
      | 2025-11-17.mp4             |
      | 2025-11-24.mp4             |
