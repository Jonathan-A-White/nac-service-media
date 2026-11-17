package cmd

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	appwatch "nac-service-media/application/watch"
	domain "nac-service-media/domain/watch"
	"nac-service-media/infrastructure/mayormail"
	"nac-service-media/infrastructure/postern"
	"nac-service-media/infrastructure/sshpc"

	"github.com/spf13/cobra"
)

// WatchOptions are the flags of `watch`.
type WatchOptions struct {
	Date         string // YYYY-MM-DD, "" for today in America/New_York
	Sunday       bool   // the latest Sunday, today when it is one (for a catch-up run on Monday)
	DryRun       bool
	SkipPostern  bool
	PC           string // user@host of the church PC
	PCVideos     string
	PCRepo       string
	PosternEnv   string
	TimeoutLimit time.Duration
}

var watchOpts WatchOptions

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Sunday watch: is the service recording's alert in Postern? If not, look at the church PC",
	Long: `Runs on the Laptop, from a systemd timer on Sundays at 1 pm Eastern.

It asks Postern (settings in ~/.config/nac-service-media/postern.env) for a
service-recording alert dated the Sunday. Found: it claims the alert, logs one
line and is done. None: it looks at the church PC over key-only ssh, with
read-only commands, and sorts the day into one of

  no-recording    the PC answers and OBS made no file that day: quiet
  pc-unreachable  ssh could not reach the PC
  run-failed      a recording exists but the run errored or left no log
  mail-not-sent   the run succeeded but its log shows no mail sent
  other           anything else

Every class but no-recording sends ONE mail to the Mayor with the evidence.

  nac-service-media watch --date 2026-11-15 --dry-run   # print, send nothing`,
	Args: cobra.NoArgs,
	RunE: runWatch,
}

func init() {
	rootCmd.AddCommand(watchCmd)
	f := watchCmd.Flags()
	f.StringVar(&watchOpts.Date, "date", "", "Service date, YYYY-MM-DD (default: today in America/New_York)")
	f.BoolVar(&watchOpts.Sunday, "sunday", false, "Watch the latest Sunday in America/New_York (today when it is Sunday) instead of today: for the timer, whose Persistent=true catch-up may run on a Monday")
	f.BoolVar(&watchOpts.DryRun, "dry-run", false, "Print the mail instead of sending it, and claim nothing")
	f.BoolVar(&watchOpts.SkipPostern, "skip-postern", false, "Do not ask Postern: treat the day's alert as not found (to check the PC side alone)")
	f.StringVar(&watchOpts.PC, "pc", "whiteplainsnac@10.88.0.9", "The church PC, user@host, reached by key-only ssh")
	f.StringVar(&watchOpts.PCVideos, "pc-videos", "/mnt/d/Videos", "Where OBS records, on the PC")
	f.StringVar(&watchOpts.PCRepo, "pc-repo", "~/src/nac-service-media", "This repo's checkout on the PC (its logs/ folder holds the run logs)")
	f.StringVar(&watchOpts.PosternEnv, "postern-env", "", "Postern settings (default ~/.config/nac-service-media/postern.env)")
	f.DurationVar(&watchOpts.TimeoutLimit, "timeout", 3*time.Minute, "Give up on the whole watch after this long")
}

// serviceDate is the date to watch: --date, else today in New York, or with
// --sunday the latest Sunday there.
func serviceDate(flag string, sunday bool, now time.Time) (string, error) {
	if flag != "" {
		if sunday {
			return "", fmt.Errorf("give --date or --sunday, not both")
		}
		if _, err := time.Parse("2006-01-02", flag); err != nil {
			return "", fmt.Errorf("--date %q is not YYYY-MM-DD", flag)
		}
		return flag, nil
	}
	loc, err := time.LoadLocation(appwatch.Zone)
	if err != nil {
		return "", err
	}
	day := now.In(loc)
	if sunday {
		day = day.AddDate(0, 0, -int(day.Weekday()))
	}
	return day.Format("2006-01-02"), nil
}

func runWatch(cmd *cobra.Command, args []string) error {
	o := watchOpts
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	if o.TimeoutLimit > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.TimeoutLimit)
		defer cancel()
	}

	var alerts domain.Alerts
	switch {
	case o.SkipPostern:
		alerts = noAlertsAsked{}
	default:
		path := o.PosternEnv
		if path == "" {
			path = postern.DefaultEnvPath()
		}
		settings, err := postern.LoadEnv(path)
		if err != nil {
			// Postern cannot be asked, but the church can still be looked at.
			alerts = unavailableAlerts{err: err}
		} else {
			alerts = &postern.Client{Base: settings.URL, Key: settings.Key, HTTP: &http.Client{Timeout: 30 * time.Second}}
		}
	}
	pc := &sshpc.Runner{Target: o.PC}
	return RunWatchWithDependencies(ctx, o, alerts, pc, &mayormail.Mailer{}, cmd.OutOrStdout())
}

// RunWatchWithDependencies runs one Sunday watch with its ports given.
func RunWatchWithDependencies(ctx context.Context, o WatchOptions, alerts domain.Alerts, pc domain.PC, mailer domain.Mailer, out io.Writer) error {
	date, err := serviceDate(o.Date, o.Sunday, time.Now())
	if err != nil {
		return err
	}
	svc := &appwatch.Service{
		Alerts: alerts, PC: pc, Mailer: mailer, Out: out,
		VideosDir: o.PCVideos, RepoDir: o.PCRepo, DryRun: o.DryRun,
	}
	if svc.VideosDir == "" {
		svc.VideosDir = "/mnt/d/Videos"
	}
	if svc.RepoDir == "" {
		svc.RepoDir = "~/src/nac-service-media"
	}
	_, err = svc.Run(ctx, date)
	return err
}

// noAlertsAsked is --skip-postern.
type noAlertsAsked struct{}

func (noAlertsAsked) List(context.Context) ([]domain.Alert, error) { return nil, nil }
func (noAlertsAsked) Claim(context.Context, string) error          { return nil }

// unavailableAlerts is Postern when its settings could not be read.
type unavailableAlerts struct{ err error }

func (u unavailableAlerts) List(context.Context) ([]domain.Alert, error) { return nil, u.err }
func (u unavailableAlerts) Claim(context.Context, string) error          { return u.err }
