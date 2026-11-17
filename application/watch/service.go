// Package watch is the Sunday and Wednesday watch: when no service-recording
// alert for a service day is in Postern, look at the church PC (read-only) and sort the day.
package watch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	domain "nac-service-media/domain/watch" // the watch runs from a timer: do not depend on the host's zoneinfo
)

// Class is how the watch sorted a service day.
type Class string

const (
	// Found: Postern holds the day's service-recording alert; nothing to do.
	Found Class = "found"
	// NoRecording: the PC answers and OBS made no file that day. Quiet.
	NoRecording Class = "no-recording"
	// PCUnreachable: ssh could not reach the church PC.
	PCUnreachable Class = "pc-unreachable"
	// RunFailed: a recording exists but the run errored or left no log.
	RunFailed Class = "run-failed"
	// MailNotSent: the run succeeded but its log shows no mail sent.
	MailNotSent Class = "mail-not-sent"
	// Other: anything else the watch cannot sort.
	Other Class = "other"
)

// Quiet reports whether the class needs nobody told.
func (c Class) Quiet() bool { return c == Found || c == NoRecording }

// The ports live in domain/watch; these names keep this package's own API short.
type (
	Alert       = domain.Alert
	Alerts      = domain.Alerts
	PC          = domain.PC
	Mailer      = domain.Mailer
	Unreachable = domain.Unreachable
)

// KindServiceRecording is the alert kind of the church's recording mail.
const KindServiceRecording = domain.KindServiceRecording

// Result is what a run decided.
type Result struct {
	Class   Class
	Subject string // the Mayor's mail, empty for a quiet class
	Body    string
}

// Service is one watch of one service day.
type Service struct {
	Alerts Alerts
	PC     PC
	Mailer Mailer
	Out    io.Writer

	// VideosDir and RepoDir are paths on the church PC; RepoDir may start "~/".
	VideosDir string
	RepoDir   string
	// DryRun prints the mail instead of sending it, and claims nothing.
	DryRun bool
}

// Zone is where a service date is read: the church's.
const Zone = "America/New_York"

var dateRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// Run watches one service date (YYYY-MM-DD, Eastern).
func (s *Service) Run(ctx context.Context, date string) (Result, error) {
	if !dateRE.MatchString(date) {
		return Result{}, fmt.Errorf("date %q is not YYYY-MM-DD", date)
	}
	if _, err := time.Parse("2006-01-02", date); err != nil {
		return Result{}, fmt.Errorf("date %q: %w", date, err)
	}
	loc, err := time.LoadLocation(Zone)
	if err != nil {
		return Result{}, err
	}

	var postern string // Postern's error, kept for the evidence
	alerts, aerr := s.Alerts.List(ctx)
	if aerr != nil {
		postern = aerr.Error()
	}
	for _, a := range alerts {
		if a.Kind == KindServiceRecording && a.When.In(loc).Format("2006-01-02") == date {
			if s.DryRun {
				fmt.Fprintf(s.Out, "Church %s watch %s: found service-recording alert %s (dry run: not claimed)\n", dayName(date), date, a.ID)
				return Result{Class: Found}, nil
			}
			if err := s.Alerts.Claim(ctx, a.ID); err != nil {
				return Result{}, fmt.Errorf("claiming alert %s: %w", a.ID, err)
			}
			fmt.Fprintf(s.Out, "Church %s watch %s: found service-recording alert %s, claimed\n", dayName(date), date, a.ID)
			return Result{Class: Found}, nil
		}
	}

	res := s.sort(ctx, date, postern)
	if res.Class.Quiet() {
		note := ""
		if postern != "" {
			note = " (Postern could not be asked: " + oneLine(postern) + ")"
		}
		fmt.Fprintf(s.Out, "Church %s watch %s: %s%s\n", dayName(date), date, res.Class, note)
		return res, nil
	}
	if s.DryRun {
		fmt.Fprintf(s.Out, "Church %s watch %s: %s (dry run: mail to mayor not sent)\n--- subject: %s\n%s\n", dayName(date), date, res.Class, res.Subject, res.Body)
		return res, nil
	}
	if err := s.Mailer.SendMayor(ctx, res.Subject, res.Body); err != nil {
		return res, fmt.Errorf("class %s but the mail to the Mayor failed: %w", res.Class, err)
	}
	fmt.Fprintf(s.Out, "Church %s watch %s: %s, mailed the Mayor\n", dayName(date), date, res.Class)
	return res, nil
}

// sort looks at the church PC and sorts the day. postern is Postern's own
// error when it could not be asked, "" otherwise.
func (s *Service) sort(ctx context.Context, date, postern string) Result {
	out, err := s.PC.Run(ctx, RemoteScript(date, s.VideosDir, s.RepoDir))
	var ev []string
	add := func(format string, args ...any) { ev = append(ev, fmt.Sprintf(format, args...)) }
	if postern != "" {
		add("Postern could not be asked for the day's alert, so its absence is not confirmed: %s", postern)
	}
	finish := func(c Class, why string) Result {
		subject := fmt.Sprintf("Church %s watch %s: %s", dayName(date), date, c)
		body := fmt.Sprintf("No service-recording alert for %s is in Postern at the watch's look.\nClass: %s: %s\n\n%s\n",
			date, c, why, strings.Join(ev, "\n"))
		return Result{Class: c, Subject: subject, Body: body}
	}

	if err != nil {
		var un *Unreachable
		if errors.As(err, &un) {
			add("ssh error: %s", un.Detail)
			return finish(PCUnreachable, "ssh could not reach the church PC")
		}
		add("ssh error: %s", err.Error())
		return finish(Other, "the ssh command failed in a way that is not 'unreachable'")
	}

	rep, perr := parseReport(out)
	if perr != nil {
		add("pcReport from the PC: %s\n%s", perr.Error(), out)
		return finish(Other, "the PC's report could not be read")
	}
	add("recordings dated %s in %s: %s", date, s.VideosDir, rep.recordingsText())
	add("run logs dated %s: %s", date, rep.logsText())
	if len(rep.markers) > 0 {
		add("lines of the logs that say how the runs went:\n%s", strings.Join(rep.markers, "\n"))
	}
	if len(rep.tail) > 0 {
		add("tail of the newest run log (%s):\n%s", rep.newestLog(), strings.Join(rep.tail, "\n"))
	}

	n := rep.newest()
	switch {
	case len(rep.recordings) == 0:
		switch {
		case len(rep.logs) == 0:
			return finish(NoRecording, "the PC answers, no OBS recording that day, and no run log")
		case n.nothingToDo || (n.noNew && n.exit != nil):
			return finish(NoRecording, "the PC answers, no OBS recording that day, and the run said nothing to do")
		}
		return finish(Other, "no OBS recording that day, but the run's log says something other than 'nothing to do'")
	case len(rep.logs) == 0:
		return finish(RunFailed, "a recording exists but there is no run log for the day (the task never started, or it failed before logging)")
	case n.exit == nil:
		return finish(Other, "a recording exists and the newest run log has no 'Exit code' line: the run did not finish (still running, killed, or the PC went to sleep)")
	case *n.exit != 0 || n.failure:
		return finish(RunFailed, fmt.Sprintf("a recording exists and the run failed (exit code %d)", *n.exit))
	case !rep.anySent():
		return finish(MailNotSent, "a recording exists and the run succeeded, but no log of the day shows a mail sent")
	}
	return finish(Other, "a recording exists, the run succeeded and its log shows the mail was sent, yet Postern holds no alert: look at the Gmail forward and the alerts-mail receiver")
}

// dayName is the weekday of a YYYY-MM-DD date ("Sunday", "Wednesday"), as the
// watch's log lines and mail subjects name it.
func dayName(date string) string {
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return "service"
	}
	return t.Weekday().String()
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// --- the remote script and its pcReport ---

// RemoteScript is the one read-only shell script the watch runs on the PC:
// it lists the day's recordings and run logs, greps the lines that say how
// each run ended, and prints the tail of the newest log. It writes nothing.
func RemoteScript(date, videosDir, repoDir string) string {
	v, r := shellPath(videosDir), shellPath(repoDir)
	return strings.Join([]string{
		"d='" + date + "'; v=" + v + "; r=" + r,
		`echo '@@recordings'`,
		`for f in "$v"/"$d "*; do [ -e "$f" ] && echo "$(stat -c %s "$f") $f"; done`,
		`echo '@@logs'`,
		`for f in "$r"/logs/scheduled-run-"$d"_*.log; do [ -e "$f" ] && echo "$f"; done`,
		`echo '@@markers'`,
		`grep -H -n -E 'Exit code:|SUCCESS:|FAILURE|Sent to:|nothing to do|No new service recording since' "$r"/logs/scheduled-run-"$d"_*.log 2>/dev/null`,
		`echo '@@tail'`,
		`last=$(ls -1 "$r"/logs/scheduled-run-"$d"_*.log 2>/dev/null | tail -n 1)`,
		`[ -n "$last" ] && tail -n 30 "$last"`,
		`echo '@@end'`,
	}, "\n")
}

// shellPath quotes a path for sh; a leading "~/" becomes "$HOME/".
func shellPath(p string) string {
	q := func(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
	if strings.HasPrefix(p, "~/") {
		esc := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "$", `\$`, "`", "\\`").Replace(strings.TrimPrefix(p, "~/"))
		return `"$HOME/` + esc + `"`
	}
	return q(p)
}

type runInfo struct {
	exit        *int
	failure     bool
	sent        bool
	nothingToDo bool
	noNew       bool
}

type pcReport struct {
	recordings []string // "<size> <path>"
	logs       []string // sorted by name, so by time
	runs       map[string]*runInfo
	markers    []string // the lines of the logs that say how a run went
	tail       []string
}

var exitRE = regexp.MustCompile(`Exit code:\s*(-?\d+)`)

func parseReport(out string) (*pcReport, error) {
	rep := &pcReport{runs: map[string]*runInfo{}}
	section := ""
	ended := false
	for _, line := range strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, "@@") {
			section = strings.TrimPrefix(line, "@@")
			if section == "end" {
				ended = true
			}
			continue
		}
		if line == "" {
			continue
		}
		switch section {
		case "recordings":
			rep.recordings = append(rep.recordings, line)
		case "logs":
			rep.logs = append(rep.logs, line)
		case "markers":
			// grep -H -n: <path>:<line no>:<text>
			rep.markers = append(rep.markers, line)
			parts := strings.SplitN(line, ":", 3)
			if len(parts) < 3 {
				continue
			}
			ri := rep.runs[parts[0]]
			if ri == nil {
				ri = &runInfo{}
				rep.runs[parts[0]] = ri
			}
			text := parts[2]
			if m := exitRE.FindStringSubmatch(text); m != nil {
				if code, err := strconv.Atoi(m[1]); err == nil {
					ri.exit = &code
				}
			}
			switch {
			case strings.Contains(text, "FAILURE"):
				ri.failure = true
			case strings.Contains(text, "Sent to:"):
				ri.sent = true
			case strings.Contains(text, "nothing to do"):
				ri.nothingToDo = true
			case strings.Contains(text, "No new service recording since"):
				ri.noNew = true
			}
		case "tail":
			rep.tail = append(rep.tail, line)
		}
	}
	if !ended {
		return nil, errors.New("the report has no @@end: the script was cut short")
	}
	sort.Strings(rep.logs)
	return rep, nil
}

func (r *pcReport) newestLog() string {
	if len(r.logs) == 0 {
		return ""
	}
	return r.logs[len(r.logs)-1]
}

func (r *pcReport) newest() runInfo {
	if ri := r.runs[r.newestLog()]; ri != nil {
		return *ri
	}
	return runInfo{}
}

func (r *pcReport) anySent() bool {
	for _, ri := range r.runs {
		if ri.sent {
			return true
		}
	}
	return false
}

func (r *pcReport) recordingsText() string {
	if len(r.recordings) == 0 {
		return "none"
	}
	var parts []string
	for _, l := range r.recordings {
		size, path, _ := strings.Cut(l, " ")
		parts = append(parts, fmt.Sprintf("%s (%s bytes)", path, size))
	}
	return strings.Join(parts, "; ")
}

func (r *pcReport) logsText() string {
	if len(r.logs) == 0 {
		return "none (no run log for the day)"
	}
	return strings.Join(r.logs, "; ")
}
