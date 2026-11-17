// Package watch holds what the Sunday watch asks of the world: Postern's
// alerts, the church PC and the Mayor's mailbox.
package watch

import (
	"context"
	"time"
)

// Alert is one alert of Postern's family, as much of it as the watch reads.
type Alert struct {
	ID   string
	Kind string
	When time.Time
}

// KindServiceRecording is the alert kind Postern gives the church's
// "Recording of Service" mail (Postern docs/alerts-mail.md).
const KindServiceRecording = "service-recording"

// Alerts is Postern's alerts routes for the nac-service-media family.
type Alerts interface {
	List(ctx context.Context) ([]Alert, error)
	Claim(ctx context.Context, id string) error
}

// PC runs one read-only shell script on the church PC and returns what it
// printed. An ssh that could not connect returns *Unreachable.
type PC interface {
	Run(ctx context.Context, script string) (string, error)
}

// Mailer mails the Mayor.
type Mailer interface {
	SendMayor(ctx context.Context, subject, body string) error
}

// Unreachable is the error of a PC that ssh could not reach: no route, a
// refused or timed-out connection, a refused key. Detail is ssh's own words.
type Unreachable struct{ Detail string }

func (e *Unreachable) Error() string { return "church PC unreachable: " + e.Detail }
