Feature: Configuration Setup
  As a new user
  I want to create a config file interactively
  So that I can start using the tool quickly

  Scenario: Create new configuration
    Given no config file exists for setup
    When I run the setup command with inputs:
      | prompt                        | value                    |
      | OBS recordings                | /tmp/recordings          |
      | trimmed videos                | /tmp/trimmed             |
      | audio files                   | /tmp/audio               |
      | audio bitrate                 | 192k                     |
      | credentials file              | credentials.json         |
      | folder ID                     | test-folder-id           |
      | from name                     | Test Church              |
      | from address                  | test@example.com         |
      | Add a CC recipient            | n                        |
      | Add a quick-lookup recipient  | n                        |
    Then a config file should exist
    And the config should have source_directory "/tmp/recordings"
    And the config should have trimmed_directory "/tmp/trimmed"
    And the config should have audio_directory "/tmp/audio"
    And the config should have services_folder_id "test-folder-id"

  Scenario: Create configuration with recipients
    Given no config file exists for setup
    When I run the setup command with inputs:
      | prompt                        | value                    |
      | OBS recordings                | /tmp/recordings          |
      | trimmed videos                | /tmp/trimmed             |
      | audio files                   | /tmp/audio               |
      | audio bitrate                 | 192k                     |
      | credentials file              | credentials.json         |
      | folder ID                     | test-folder-id           |
      | from name                     | Test Church              |
      | from address                  | test@example.com         |
      | Add a CC recipient            | y                        |
      | Full name                     | Admin User               |
      | Email                         | admin@example.com        |
      | Add a CC recipient            | n                        |
      | Add a quick-lookup recipient  | y                        |
      | Nickname                      | mom                      |
      | Full name                     | Mom Smith                |
      | Email                         | mom@example.com          |
      | Add a quick-lookup recipient  | n                        |
    Then a config file should exist
    And the config should have a CC recipient "Admin User"
    And the config should have a quick-lookup recipient "mom"

  Scenario: Warn when config already exists
    Given a config file already exists for setup
    When I run the setup command with confirmation "n"
    Then the setup should be cancelled
    And the existing config should be unchanged

  Scenario: Overwrite existing config when confirmed
    Given a config file already exists for setup
    When I run the setup command with confirmation "y" and inputs:
      | prompt                        | value                    |
      | OBS recordings                | /new/recordings          |
      | trimmed videos                | /new/trimmed             |
      | audio files                   | /new/audio               |
      | audio bitrate                 | 256k                     |
      | credentials file              | new-creds.json           |
      | folder ID                     | new-folder-id            |
      | from name                     | New Church               |
      | from address                  | new@example.com          |
      | Add a CC recipient            | n                        |
      | Add a quick-lookup recipient  | n                        |
    Then a config file should exist
    And the config should have source_directory "/new/recordings"
