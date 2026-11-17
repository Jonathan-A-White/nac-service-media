// Package sshpc runs the Sunday watch's read-only script on the church PC
// over key-only ssh (the Laptop's WireGuard reaches it).
package sshpc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	domain "nac-service-media/domain/watch"
)

// ExecFunc runs a program and returns its output and exit code. A program
// that could not start returns code -1 and an error.
type ExecFunc func(ctx context.Context, name string, args ...string) (stdout, stderr string, code int, err error)

// Runner is the watch's domain.PC.
type Runner struct {
	Target string   // user@host
	Exec   ExecFunc // nil means a real process
}

var _ domain.PC = (*Runner)(nil)

// Run runs script on the PC. ssh's own failures (exit 255: no route, timeout,
// refused key) and a run that outlives its context are *domain.Unreachable.
func (r *Runner) Run(ctx context.Context, script string) (string, error) {
	exe := r.Exec
	if exe == nil {
		exe = realExec
	}
	stdout, stderr, code, err := exe(ctx, "ssh",
		"-o", "BatchMode=yes", "-o", "ConnectTimeout=15", "-o", "ServerAliveInterval=15",
		r.Target, script)
	switch {
	case err == nil && code == 0:
		return stdout, nil
	case errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil:
		return stdout, &domain.Unreachable{Detail: "ssh did not answer in time: " + firstNonEmpty(oneLine(stderr), "no output")}
	case code == 255:
		return stdout, &domain.Unreachable{Detail: firstNonEmpty(oneLine(stderr), "ssh exited 255 with no message")}
	case code == -1:
		return stdout, fmt.Errorf("could not run ssh: %w", err)
	}
	return stdout, fmt.Errorf("ssh to %s exited %d: %s", r.Target, code, firstNonEmpty(oneLine(stderr), err.Error()))
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func realExec(ctx context.Context, name string, args ...string) (string, string, int, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if err == nil {
		return out.String(), errb.String(), 0, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return out.String(), errb.String(), ee.ExitCode(), err
	}
	return out.String(), errb.String(), -1, err
}
