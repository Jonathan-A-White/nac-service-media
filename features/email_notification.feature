Feature: Email Notification
  As a user
  I want to send recording links via email
  So that recipients can access the service recording

  Background:
    Given the email config has from name "White Plains" and address "whiteplainsnac@gmail.com"
    And valid Gmail credentials

  Scenario: Send email to single recipient
    Given I have uploaded files with URLs:
      | type  | url                                           |
      | audio | https://drive.google.com/file/d/abc/view      |
      | video | https://drive.google.com/file/d/xyz/view      |
    And the service date is "2025-12-28"
    And the minister was "Pr. Smith"
    And I have a recipient "jonathan" with name "Jonathan White" and email "jonathan@example.com"
    When I send notification to "jonathan"
    Then an email should be sent
    And the email should be sent to "Jonathan White <jonathan@example.com>"
    And the subject should be "White Plains: Recording of Service on 12/28/2025"
    And the body should contain "Dear Jonathan,"
    And the body should contain the audio URL
    And the body should contain the video URL

  Scenario: Send email to two recipients
    Given I have uploaded files with URLs:
      | type  | url                                           |
      | audio | https://drive.google.com/file/d/abc/view      |
      | video | https://drive.google.com/file/d/xyz/view      |
    And the service date is "2025-12-28"
    And the minister was "Pr. Henkel"
    And I have a recipient "jonathan" with name "Jonathan White" and email "jonathan@example.com"
    And I have a recipient "jane" with name "Jane Doe" and email "jane@example.com"
    When I send notification to "jonathan,jane"
    Then an email should be sent
    And the body should contain "Dear Jonathan & Jane,"

  Scenario: Send email to three or more recipients
    Given I have uploaded files with URLs:
      | type  | url                                           |
      | audio | https://drive.google.com/file/d/abc/view      |
      | video | https://drive.google.com/file/d/xyz/view      |
    And the service date is "2025-12-28"
    And the minister was "Pr. Henkel"
    And I have a recipient "jonathan" with name "Jonathan White" and email "jonathan@example.com"
    And I have a recipient "jane" with name "Jane Doe" and email "jane@example.com"
    And I have a recipient "alice" with name "Alice Smith" and email "alice@example.com"
    When I send notification to "jonathan,jane,alice"
    Then an email should be sent
    And the body should contain "Hey Everyone!"

  Scenario: Lookup recipient by first name
    Given I have a recipient "jonathan" with name "Jonathan White" and email "jonathan@example.com"
    When I lookup recipient "Jonathan"
    Then I should find "Jonathan White <jonathan@example.com>"

  Scenario: Lookup recipient by last name
    Given I have a recipient "jonathan" with name "Jonathan White" and email "jonathan@example.com"
    When I lookup recipient "White"
    Then I should find "Jonathan White <jonathan@example.com>"

  Scenario: Unknown recipient
    When I lookup recipient "unknown"
    Then I should receive an error about unknown recipient

  Scenario: CC default recipients
    Given I have a default CC "Admin <admin@example.com>"
    And I have a recipient "jonathan" with name "Jonathan White" and email "jonathan@example.com"
    And I have uploaded files with URLs:
      | type  | url                                           |
      | audio | https://drive.google.com/file/d/abc/view      |
    And the service date is "2025-12-28"
    And the minister was "Pr. Smith"
    When I send notification to "jonathan"
    Then an email should be sent
    And the email should CC "Admin <admin@example.com>"

  Scenario: Email contains HTML links
    Given I have uploaded files with URLs:
      | type  | url                                           |
      | audio | https://drive.google.com/file/d/abc/view      |
      | video | https://drive.google.com/file/d/xyz/view      |
    And the service date is "2025-12-28"
    And the minister was "Pr. Smith"
    And I have a recipient "jonathan" with name "Jonathan White" and email "jonathan@example.com"
    When I send notification to "jonathan"
    Then an email should be sent
    And the HTML body should contain clickable audio link
    And the HTML body should contain clickable video link
