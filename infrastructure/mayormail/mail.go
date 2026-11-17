// Package mayormail sends the Sunday watch's evidence to the Mayor with
// `mw mail send mayor`.
package mayormail

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	domain "nac-service-media/domain/watch"
)

// DefaultSeat signs the mail when $MW_SEAT is unset (mw refuses to sign as no one).
const DefaultSeat = "nac-sunday-watch"

// ExecFunc runs a program with an environment and returns its combined output.
type ExecFunc func(ctx context.Context, env []string, name string, args ...string) (string, error)

// Mailer is the watch's domain.Mailer.
type Mailer struct {
	Exec ExecFunc // nil means a real process
	Env  []string // nil means the process's environment
}

var _ domain.Mailer = (*Mailer)(nil)

// SendMayor files one message to the Mayor's mailbox.
func (m *Mailer) SendMayor(ctx context.Context, subject, body string) error {
	env := m.Env
	if env == nil {
		env = os.Environ()
	}
	seat := false
	for _, e := range env {
		if strings.HasPrefix(e, "MW_SEAT=") && len(e) > len("MW_SEAT=") {
			seat = true
		}
	}
	if !seat {
		env = append(append([]string{}, env...), "MW_SEAT="+DefaultSeat)
	}
	exe := m.Exec
	if exe == nil {
		exe = realExec
	}
	out, err := exe(ctx, env, "mw", "mail", "send", "mayor", "-s", subject, "-m", body)
	if err != nil {
		return fmt.Errorf("mw mail send mayor: %w: %s", err, strings.TrimSpace(out))
	}
	return nil
}

func realExec(ctx context.Context, env []string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}
