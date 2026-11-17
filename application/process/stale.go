package process

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"nac-service-media/domain/notification"
)

// StaleAfterDays is how old, in calendar days, the newest recording may be
// before a run that finds it already processed tells the recipient. Services
// are weekly, so a recording 7 days old or older means a Sunday went by
// without a new file; one up to 6 days old is just the last service.
const StaleAfterDays = 6

// AgeInDays is the number of calendar days from the service date to today.
func AgeInDays(serviceDate, today time.Time) int {
	y, m, d := today.Date()
	todayDate := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	sy, sm, sd := serviceDate.Date()
	service := time.Date(sy, sm, sd, 0, 0, 0, 0, time.UTC)
	return int(todayDate.Sub(service).Hours() / 24)
}

// StaleRecording is what a run knew when it found that the newest recording is
// already processed and too old: no new recording has been made.
type StaleRecording struct {
	VideoPath     string    // the newest file in the source folder
	ServiceDate   time.Time // its date, from the filename
	Folder        string    // the folder searched
	LastProcessed time.Time // the date of the last service on Drive
	When          time.Time // when the run looked
}

// Subject is "No new service recording since <date>".
func (s StaleRecording) Subject() string {
	return fmt.Sprintf("No new service recording since %s", s.ServiceDate.Format("2006-01-02"))
}

// Body says what the run found and where to look first.
func (s StaleRecording) Body() string {
	var b strings.Builder
	b.WriteString("No new service recording was found, so nothing was processed. The run did not fail: the newest recording is already on Drive.\n\n")
	fmt.Fprintf(&b, "Newest recording: %s (%s, %d days old)\n", filepath.Base(s.VideoPath), s.ServiceDate.Format("2006-01-02"), AgeInDays(s.ServiceDate, s.When))
	fmt.Fprintf(&b, "Folder searched: %s\n", s.Folder)
	fmt.Fprintf(&b, "Last service on Drive: %s\n", s.LastProcessed.Format("2006-01-02"))
	fmt.Fprintf(&b, "When: %s\n\n", s.When.Format("2006-01-02 15:04 MST"))
	b.WriteString("Most likely OBS made no new file: check that OBS was opened and recorded on the church PC, and look at its logs in %APPDATA%\\obs-studio\\logs. If the PC was off, there is no run log for that day either.\n")
	return b.String()
}

// SendStaleEmail sends the one email that says no new recording has come.
func SendStaleEmail(mailer FailureMailer, to []notification.Recipient, s StaleRecording) error {
	return mailer.SendPlain(to, s.Subject(), s.Body())
}
