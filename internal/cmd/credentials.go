package cmd

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/rophy/kindtks/internal/config"
	"github.com/rophy/kindtks/internal/regauth"
)

const maxAttempts = 3

// terminal is where credentials are asked for; tests fake it.
type terminal interface {
	ReadLine(prompt string) (string, error)
	ReadPassword(prompt string) (string, error)
}

// checkFunc checks credentials against a registry; an error means they could
// not be checked.
type checkFunc func(registry, username, password string) (regauth.Result, error)

// credentialFlow checks the credentials written in the config, then asks for
// the ones marked `auth: prompt`. term is nil without a terminal; check is
// nil with --skip-auth-check.
type credentialFlow struct {
	cfg   *config.Config
	term  terminal
	out   io.Writer
	check checkFunc
}

type credentials struct {
	registry, username, password string
}

func (f *credentialFlow) run() error {
	for _, name := range f.cfg.AuthRegistries() {
		auth := f.cfg.Registries[name].Auth
		if !auth.Prompt && (auth.Username == "" || auth.Password == "") {
			return fmt.Errorf("registries.%s.auth has an empty username or password", name)
		}
	}
	if f.check != nil {
		for _, name := range f.cfg.AuthRegistries() {
			auth := f.cfg.Registries[name].Auth
			if auth.Prompt {
				continue
			}
			res, err := f.check(name, auth.Username, auth.Password)
			if err != nil {
				fmt.Fprintf(f.out, "warning: could not check the credentials for %s: %v; continuing (use --skip-auth-check to skip checking)\n", name, err)
				continue
			}
			switch res.Status {
			case regauth.Rejected:
				return fmt.Errorf("the credentials for %s in the config file were rejected by the registry (%s)", name, res.Detail)
			case regauth.NoAuth:
				fmt.Fprintf(f.out, "%s does not require a login; credentials not checked\n", name)
			}
		}
	}

	var last *credentials
	for _, name := range f.cfg.PromptRegistries() {
		if f.term == nil {
			return fmt.Errorf(`registries.%s.auth is "prompt", but there is no terminal to ask on`, name)
		}
		fmt.Fprintf(f.out, "Credentials for %s\n", name)
		c, err := f.ask(name, last)
		if err != nil {
			return err
		}
		auth := f.cfg.Registries[name].Auth
		auth.Username, auth.Password, auth.Prompt = c.username, c.password, false
		last = c
	}
	return nil
}

// ask offers the previous registry's credentials, then asks for new ones,
// until the registry accepts them or maxAttempts entries were made.
func (f *credentialFlow) ask(name string, last *credentials) (*credentials, error) {
	if last != nil {
		yes, err := f.confirm(fmt.Sprintf("  Use the same as %s (%s)? [Y/n] ", last.registry, last.username), true)
		if err != nil {
			return nil, inputErr(name, err)
		}
		if yes {
			ok, err := f.verify(name, last.username, last.password)
			if err != nil {
				return nil, err
			}
			if ok {
				return &credentials{name, last.username, last.password}, nil
			}
		}
	}
	for attempt := 0; attempt < maxAttempts; attempt++ {
		user, err := f.term.ReadLine("  Username for " + name + ": ")
		if err != nil {
			return nil, inputErr(name, err)
		}
		pass, err := f.term.ReadPassword("  Password for " + name + ": ")
		if err != nil {
			return nil, inputErr(name, err)
		}
		if user == "" || pass == "" {
			fmt.Fprintln(f.out, "  Username and password are required.")
			continue
		}
		ok, err := f.verify(name, user, pass)
		if err != nil {
			return nil, err
		}
		if ok {
			return &credentials{name, user, pass}, nil
		}
	}
	return nil, fmt.Errorf("the credentials for %s were rejected %d times", name, maxAttempts)
}

// verify reports whether to keep the credentials: true when accepted, when
// the registry needs no login, or when the user continues without a check.
func (f *credentialFlow) verify(name, user, pass string) (bool, error) {
	if f.check == nil {
		return true, nil
	}
	res, err := f.check(name, user, pass)
	if err != nil {
		fmt.Fprintf(f.out, "  Could not check the credentials: %v\n", err)
		yes, cerr := f.confirm("  Continue without checking? [y/N] ", false)
		if cerr != nil {
			return false, inputErr(name, cerr)
		}
		if !yes {
			return false, fmt.Errorf("could not check the credentials for %s", name)
		}
		return true, nil
	}
	switch res.Status {
	case regauth.OK:
		fmt.Fprintln(f.out, "  Credentials accepted.")
		return true, nil
	case regauth.NoAuth:
		fmt.Fprintf(f.out, "%s does not require a login; credentials not checked\n", name)
		return true, nil
	}
	fmt.Fprintf(f.out, "  Rejected by %s (%s).\n", name, res.Detail)
	return false, nil
}

func (f *credentialFlow) confirm(prompt string, def bool) (bool, error) {
	answer, err := f.term.ReadLine(prompt)
	if err != nil {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	}
	return def, nil
}

// inputErr names the registry being asked about when reading input fails.
func inputErr(name string, err error) error {
	if errors.Is(err, io.EOF) {
		return fmt.Errorf("input closed while asking for credentials for %s", name)
	}
	return fmt.Errorf("reading credentials for %s: %w", name, err)
}
