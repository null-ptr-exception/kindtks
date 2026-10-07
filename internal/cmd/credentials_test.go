package cmd

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/rophy/kindtks/internal/config"
	"github.com/rophy/kindtks/internal/regauth"
)

type fakeTerm struct {
	answers []string
	prompts []string
}

func (f *fakeTerm) next(p string) (string, error) {
	f.prompts = append(f.prompts, p)
	if len(f.answers) == 0 {
		return "", io.EOF
	}
	a := f.answers[0]
	f.answers = f.answers[1:]
	return a, nil
}

func (f *fakeTerm) ReadLine(p string) (string, error)     { return f.next(p) }
func (f *fakeTerm) ReadPassword(p string) (string, error) { return f.next(p) }

// fakeCheck accepts the password "right" (strict.example.com accepts only
// "right-strict"), says open.example.com needs no login, and cannot reach
// down.example.com.
func fakeCheck(calls *[]string) checkFunc {
	return func(reg, user, pass string) (regauth.Result, error) {
		*calls = append(*calls, reg+":"+user+":"+pass)
		switch {
		case reg == "down.example.com":
			return regauth.Result{}, errors.New("dial tcp: connection refused")
		case reg == "open.example.com":
			return regauth.Result{Status: regauth.NoAuth}, nil
		case reg == "strict.example.com" && pass == "right-strict", reg != "strict.example.com" && pass == "right":
			return regauth.Result{Status: regauth.OK}, nil
		}
		return regauth.Result{Status: regauth.Rejected, Detail: "401 Unauthorized"}, nil
	}
}

func cfgWith(regs map[string]*config.RegistryAuth) *config.Config {
	cfg := &config.Config{Registries: map[string]*config.Registry{}}
	for name, auth := range regs {
		cfg.Registries[name] = &config.Registry{Auth: auth}
	}
	return cfg
}

func prompt() *config.RegistryAuth { return &config.RegistryAuth{Prompt: true} }

func runFlow(cfg *config.Config, term terminal, check checkFunc) (string, error) {
	var out bytes.Buffer
	f := &credentialFlow{cfg: cfg, out: &out, check: check}
	if term != nil {
		f.term = term
	}
	err := f.run()
	return out.String(), err
}

func wantAuth(t *testing.T, cfg *config.Config, reg, user, pass string) {
	t.Helper()
	a := cfg.Registries[reg].Auth
	if a.Prompt || a.Username != user || a.Password != pass {
		t.Errorf("%s auth = %+v, want %s/%s", reg, a, user, pass)
	}
}

func TestCredentials_PromptAccepted(t *testing.T) {
	var calls []string
	cfg := cfgWith(map[string]*config.RegistryAuth{"a.example.com": prompt()})
	term := &fakeTerm{answers: []string{"svc", "right"}}
	if _, err := runFlow(cfg, term, fakeCheck(&calls)); err != nil {
		t.Fatal(err)
	}
	wantAuth(t, cfg, "a.example.com", "svc", "right")
	if len(calls) != 1 {
		t.Errorf("checks: %v", calls)
	}
}

func TestCredentials_RejectedThenAccepted(t *testing.T) {
	var calls []string
	cfg := cfgWith(map[string]*config.RegistryAuth{"a.example.com": prompt()})
	term := &fakeTerm{answers: []string{"svc", "wrong", "svc", "right"}}
	out, err := runFlow(cfg, term, fakeCheck(&calls))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Rejected by a.example.com (401 Unauthorized)") {
		t.Errorf("output:\n%s", out)
	}
	wantAuth(t, cfg, "a.example.com", "svc", "right")
}

func TestCredentials_ThreeRejections(t *testing.T) {
	var calls []string
	cfg := cfgWith(map[string]*config.RegistryAuth{"a.example.com": prompt()})
	term := &fakeTerm{answers: []string{"svc", "w1", "svc", "w2", "svc", "w3"}}
	_, err := runFlow(cfg, term, fakeCheck(&calls))
	if err == nil || !strings.Contains(err.Error(), "rejected 3 times") {
		t.Fatalf("got %v", err)
	}
}

func TestCredentials_ReuseAccepted(t *testing.T) {
	var calls []string
	cfg := cfgWith(map[string]*config.RegistryAuth{"a.example.com": prompt(), "b.example.com": prompt()})
	term := &fakeTerm{answers: []string{"svc", "right", ""}}
	if _, err := runFlow(cfg, term, fakeCheck(&calls)); err != nil {
		t.Fatal(err)
	}
	wantAuth(t, cfg, "b.example.com", "svc", "right")
	if !strings.Contains(strings.Join(term.prompts, "\n"), "Use the same as a.example.com (svc)? [Y/n]") {
		t.Errorf("prompts: %q", term.prompts)
	}
}

func TestCredentials_ReuseDeclined(t *testing.T) {
	var calls []string
	cfg := cfgWith(map[string]*config.RegistryAuth{"a.example.com": prompt(), "b.example.com": prompt()})
	term := &fakeTerm{answers: []string{"svc", "right", "n", "other", "right"}}
	if _, err := runFlow(cfg, term, fakeCheck(&calls)); err != nil {
		t.Fatal(err)
	}
	wantAuth(t, cfg, "b.example.com", "other", "right")
}

func TestCredentials_ReusedRejectedAsksAgain(t *testing.T) {
	var calls []string
	cfg := cfgWith(map[string]*config.RegistryAuth{"a.example.com": prompt(), "strict.example.com": prompt()})
	term := &fakeTerm{answers: []string{"svc", "right", "y", "svc", "right-strict"}}
	out, err := runFlow(cfg, term, fakeCheck(&calls))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Rejected by strict.example.com") {
		t.Errorf("output:\n%s", out)
	}
	wantAuth(t, cfg, "strict.example.com", "svc", "right-strict")
}

func TestCredentials_EmptyInputCountsAsAttempt(t *testing.T) {
	var calls []string
	cfg := cfgWith(map[string]*config.RegistryAuth{"a.example.com": prompt()})
	term := &fakeTerm{answers: []string{"", "", "svc", "right"}}
	out, err := runFlow(cfg, term, fakeCheck(&calls))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "required") {
		t.Errorf("output:\n%s", out)
	}
	wantAuth(t, cfg, "a.example.com", "svc", "right")
}

func TestCredentials_CouldNotCheck(t *testing.T) {
	var calls []string
	cfg := cfgWith(map[string]*config.RegistryAuth{"down.example.com": prompt()})
	out, err := runFlow(cfg, &fakeTerm{answers: []string{"svc", "pw", "y"}}, fakeCheck(&calls))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "connection refused") {
		t.Errorf("output:\n%s", out)
	}
	wantAuth(t, cfg, "down.example.com", "svc", "pw")

	cfg = cfgWith(map[string]*config.RegistryAuth{"down.example.com": prompt()})
	if _, err := runFlow(cfg, &fakeTerm{answers: []string{"svc", "pw", ""}}, fakeCheck(&calls)); err == nil ||
		!strings.Contains(err.Error(), "could not check") {
		t.Errorf("default no: got %v", err)
	}
}

func TestCredentials_NoTerminal(t *testing.T) {
	var calls []string
	cfg := cfgWith(map[string]*config.RegistryAuth{"a.example.com": prompt()})
	_, err := runFlow(cfg, nil, fakeCheck(&calls))
	if err == nil || err.Error() != `registries.a.example.com.auth is "prompt", but there is no terminal to ask on` {
		t.Fatalf("got %v", err)
	}
}

func TestCredentials_NoAuthNote(t *testing.T) {
	var calls []string
	cfg := cfgWith(map[string]*config.RegistryAuth{"open.example.com": prompt()})
	out, err := runFlow(cfg, &fakeTerm{answers: []string{"svc", "pw"}}, fakeCheck(&calls))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "open.example.com does not require a login; credentials not checked") {
		t.Errorf("output:\n%s", out)
	}
}

func TestCredentials_SkipCheck(t *testing.T) {
	cfg := cfgWith(map[string]*config.RegistryAuth{
		"a.example.com": prompt(),
		"b.example.com": {Username: "u", Password: "anything"},
	})
	if _, err := runFlow(cfg, &fakeTerm{answers: []string{"svc", "anything"}}, nil); err != nil {
		t.Fatal(err)
	}
	wantAuth(t, cfg, "a.example.com", "svc", "anything")
}

func TestCredentials_FileCredentials(t *testing.T) {
	var calls []string
	cfg := cfgWith(map[string]*config.RegistryAuth{
		"a.example.com": {Username: "u", Password: "wrong"},
		"b.example.com": prompt(),
	})
	term := &fakeTerm{answers: []string{"svc", "right"}}
	_, err := runFlow(cfg, term, fakeCheck(&calls))
	if err == nil || !strings.Contains(err.Error(), "rejected") || !strings.Contains(err.Error(), "a.example.com") {
		t.Fatalf("got %v", err)
	}
	if len(term.prompts) != 0 {
		t.Errorf("prompted before failing on file credentials: %q", term.prompts)
	}

	cfg = cfgWith(map[string]*config.RegistryAuth{"down.example.com": {Username: "u", Password: "p"}})
	if _, err := runFlow(cfg, nil, fakeCheck(&calls)); err == nil || !strings.Contains(err.Error(), "--skip-auth-check") {
		t.Errorf("could not check: got %v", err)
	}

	cfg = cfgWith(map[string]*config.RegistryAuth{"a.example.com": {Username: "u", Password: "right"}})
	if _, err := runFlow(cfg, nil, fakeCheck(&calls)); err != nil {
		t.Errorf("accepted: %v", err)
	}
}
