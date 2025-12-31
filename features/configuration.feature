Feature: Configuration Loading
  As a user
  I want to load configuration from a YAML file
  So that I can customize paths and settings

  Scenario: Load valid configuration file
    Given a configuration file exists at "config/test_config.yaml"
    When I load the configuration
    Then the trimmed directory should be "/test/trimmed"
    And the audio directory should be "/test/audio"
    And the Google services folder ID should be "test-folder-id"

  Scenario: Handle missing configuration file
    Given no configuration file exists at "config/nonexistent.yaml"
    When I attempt to load the configuration
    Then I should receive an error about missing configuration
