package mayormail

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSendMayor_MwMailSendWithSubjectAndBody(t *testing.T) {
	var name string
	var args, env []string
	m := &Mailer{Exec: func(ctx context.Context, e []string, n string, a ...string) (string, error) {
		name, args, env = n, a, e
		return "sent msg-1 to mayor", nil
	}, Env: []string{"HOME=/h"}}
	if err := m.SendMayor(context.Background(), "Church Sunday watch 2026-11-15: pc-unreachable", "body\nlines"); err != nil {
		t.Fatal(err)
	}
	if name != "mw" || strings.Join(args, "|") != "mail|send|mayor|-s|Church Sunday watch 2026-11-15: pc-unreachable|-m|body\nlines" {
		t.Errorf("%s %q", name, args)
	}
	if !contains(env, "MW_SEAT="+DefaultSeat) {
		t.Errorf("MW_SEAT must default to %s when unset: %v", DefaultSeat, env)
	}
}

func TestSendMayor_KeepsASeatAlreadySet(t *testing.T) {
	var env []string
	m := &Mailer{Env: []string{"MW_SEAT=builder"}, Exec: func(ctx context.Context, e []string, n string, a ...string) (string, error) {
		env = e
		return "", nil
	}}
	if err := m.SendMayor(context.Background(), "s", "b"); err != nil {
		t.Fatal(err)
	}
	if contains(env, "MW_SEAT="+DefaultSeat) || !contains(env, "MW_SEAT=builder") {
		t.Errorf("%v", env)
	}
}

func TestSendMayor_FailureCarriesMwsWords(t *testing.T) {
	m := &Mailer{Exec: func(ctx context.Context, e []string, n string, a ...string) (string, error) {
		return "mw: no such seat", errors.New("exit status 1")
	}}
	err := m.SendMayor(context.Background(), "s", "b")
	if err == nil || !strings.Contains(err.Error(), "no such seat") {
		t.Fatalf("%v", err)
	}
}

func contains(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}
