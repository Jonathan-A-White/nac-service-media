package sshpc

import (
	"context"
	"errors"
	"strings"
	"testing"

	domain "nac-service-media/domain/watch"
)

func fake(stdout, stderr string, code int, err error, got *[]string) ExecFunc {
	return func(ctx context.Context, name string, args ...string) (string, string, int, error) {
		*got = append([]string{name}, args...)
		return stdout, stderr, code, err
	}
}

func TestRun_KeyOnlyBatchSshWithTheScriptAsOneArgument(t *testing.T) {
	var got []string
	r := &Runner{Target: "whiteplainsnac@10.88.0.9", Exec: fake("out\n", "", 0, nil, &got)}
	out, err := r.Run(context.Background(), "echo hi; echo there")
	if err != nil || out != "out\n" {
		t.Fatalf("%q %v", out, err)
	}
	joined := strings.Join(got, " ")
	for _, want := range []string{"ssh", "-o BatchMode=yes", "-o ConnectTimeout=", "whiteplainsnac@10.88.0.9"} {
		if !strings.Contains(joined, want) {
			t.Errorf("args lack %q: %v", want, got)
		}
	}
	if got[len(got)-1] != "echo hi; echo there" {
		t.Errorf("the script must be the last argument, whole: %v", got)
	}
}

func TestRun_Exit255IsUnreachableWithSshsWords(t *testing.T) {
	var got []string
	r := &Runner{Target: "u@h", Exec: fake("", "ssh: connect to host 10.88.0.9 port 22: Connection timed out\n", 255, errors.New("exit status 255"), &got)}
	_, err := r.Run(context.Background(), "x")
	var un *domain.Unreachable
	if !errors.As(err, &un) || !strings.Contains(un.Detail, "Connection timed out") {
		t.Fatalf("err = %v", err)
	}
}

func TestRun_AContextDeadlineIsUnreachable(t *testing.T) {
	var got []string
	r := &Runner{Target: "u@h", Exec: fake("", "", -1, context.DeadlineExceeded, &got)}
	_, err := r.Run(context.Background(), "x")
	var un *domain.Unreachable
	if !errors.As(err, &un) {
		t.Fatalf("err = %v", err)
	}
}

func TestRun_OtherFailureIsNotUnreachable(t *testing.T) {
	var got []string
	r := &Runner{Target: "u@h", Exec: fake("partial", "sh: boom", 2, errors.New("exit status 2"), &got)}
	out, err := r.Run(context.Background(), "x")
	var un *domain.Unreachable
	if err == nil || errors.As(err, &un) || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v", err)
	}
	if out != "partial" {
		t.Errorf("out = %q", out)
	}
}

func TestRun_CannotStartSshIsAnError(t *testing.T) {
	var got []string
	r := &Runner{Target: "u@h", Exec: fake("", "", -1, errors.New(`exec: "ssh": executable file not found in $PATH`), &got)}
	_, err := r.Run(context.Background(), "x")
	var un *domain.Unreachable
	if err == nil || errors.As(err, &un) {
		t.Fatalf("err = %v", err)
	}
}
