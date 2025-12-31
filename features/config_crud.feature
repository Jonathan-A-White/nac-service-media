Feature: Config CRUD Commands
  As a user of the service media tool
  I want to manage ministers, recipients, and CCs via CLI commands
  So that I can easily add visiting ministers and new recipients

  Background:
    Given a config file exists with initial data

  # Minister CRUD

  Scenario: Add a minister
    When I run config add minister with key "smith" and name "Rev. John Smith"
    Then the command should succeed
    And the config should contain minister "smith" with name "Rev. John Smith"

  Scenario: Add duplicate minister fails
    Given minister "jones" exists with name "Rev. Mary Jones"
    When I run config add minister with key "jones" and name "Rev. Mary Jones Jr"
    Then the command should fail with "key already exists"

  Scenario: List ministers
    Given minister "smith" exists with name "Rev. John Smith"
    And minister "jones" exists with name "Rev. Mary Jones"
    When I run config list ministers
    Then the command should succeed
    And the output should contain "smith"
    And the output should contain "Rev. John Smith"
    And the output should contain "jones"
    And the output should contain "Rev. Mary Jones"

  Scenario: List ministers when none exist
    When I run config list ministers
    Then the command should succeed
    And the output should contain "No ministers configured"

  Scenario: Remove a minister
    Given minister "smith" exists with name "Rev. John Smith"
    When I run config remove minister "smith"
    Then the command should succeed
    And the config should not contain minister "smith"

  Scenario: Remove non-existent minister fails
    When I run config remove minister "notfound"
    Then the command should fail with "minister not found"

  Scenario: Update a minister
    Given minister "smith" exists with name "Rev. John Smith"
    When I run config update minister "smith" with name "Rev. John H. Smith"
    Then the command should succeed
    And the config should contain minister "smith" with name "Rev. John H. Smith"

  Scenario: Update non-existent minister fails
    When I run config update minister "notfound" with name "New Name"
    Then the command should fail with "minister not found"

  # Recipient CRUD

  Scenario: Add a recipient
    When I run config add recipient with key "jane" name "Jane Doe" and email "jane@example.com"
    Then the command should succeed
    And the config should contain recipient "jane" with name "Jane Doe" and email "jane@example.com"

  Scenario: Add recipient with invalid email fails
    When I run config add recipient with key "bad" name "Bad Email" and email "notanemail"
    Then the command should fail with "invalid email"

  Scenario: Add duplicate recipient fails
    Given recipient "john" exists with name "John Doe" and email "john@example.com"
    When I run config add recipient with key "john" name "John Smith" and email "john.smith@example.com"
    Then the command should fail with "key already exists"

  Scenario: List recipients
    Given recipient "jane" exists with name "Jane Doe" and email "jane@example.com"
    And recipient "john" exists with name "John Doe" and email "john@example.com"
    When I run config list recipients
    Then the command should succeed
    And the output should contain "jane"
    And the output should contain "Jane Doe"
    And the output should contain "jane@example.com"

  Scenario: List recipients when none exist
    When I run config list recipients
    Then the command should succeed
    And the output should contain "No recipients configured"

  Scenario: Remove a recipient
    Given recipient "jane" exists with name "Jane Doe" and email "jane@example.com"
    When I run config remove recipient "jane"
    Then the command should succeed
    And the config should not contain recipient "jane"

  Scenario: Update recipient email
    Given recipient "jane" exists with name "Jane Doe" and email "jane@example.com"
    When I run config update recipient "jane" with email "jane.new@example.com"
    Then the command should succeed
    And the config should contain recipient "jane" with name "Jane Doe" and email "jane.new@example.com"

  Scenario: Update recipient name and email
    Given recipient "jane" exists with name "Jane Doe" and email "jane@example.com"
    When I run config update recipient "jane" with name "Jane Smith" and email "jane.smith@example.com"
    Then the command should succeed
    And the config should contain recipient "jane" with name "Jane Smith" and email "jane.smith@example.com"

  # CC CRUD

  Scenario: Add a CC
    When I run config add cc with key "mary" name "Mary Jones" and email "mary@example.com"
    Then the command should succeed
    And the config should contain cc with name "Mary Jones" and email "mary@example.com"

  Scenario: Add CC with invalid email fails
    When I run config add cc with key "bad" name "Bad Email" and email "notanemail"
    Then the command should fail with "invalid email"

  Scenario: List CCs
    Given cc exists with name "Mary Jones" and email "mary@example.com"
    And cc exists with name "Bob Smith" and email "bob@example.com"
    When I run config list ccs
    Then the command should succeed
    And the output should contain "Mary Jones"
    And the output should contain "mary@example.com"
    And the output should contain "Bob Smith"

  Scenario: List CCs when none exist
    When I run config list ccs
    Then the command should succeed
    And the output should contain "No CCs configured"

  Scenario: Remove a CC
    Given cc exists with name "Mary Jones" and email "mary@example.com"
    When I run config remove cc "mary"
    Then the command should succeed
    And the config should not contain cc with name "Mary Jones"

  Scenario: Update CC email
    Given cc exists with name "Mary Jones" and email "mary@example.com"
    When I run config update cc "mary" with email "mary.new@example.com"
    Then the command should succeed
    And the config should contain cc with name "Mary Jones" and email "mary.new@example.com"

  # Key case-insensitivity

  Scenario: Keys are case-insensitive for lookup
    Given minister "Smith" exists with name "Rev. John Smith"
    When I run config remove minister "SMITH"
    Then the command should succeed
    And the config should not contain minister "smith"
