package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"golang.org/x/term"
)

// ttyTerminal asks on /dev/tty, so prompts work when stdin or stdout is
// redirected.
type ttyTerminal struct {
	f *os.File
}

func openTerminal() (*ttyTerminal, error) {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	return &ttyTerminal{f: f}, nil
}

func (t *ttyTerminal) Close() error { return t.f.Close() }

// ReadLine reads one byte at a time so nothing typed ahead is buffered away
// from a following ReadPassword.
func (t *ttyTerminal) ReadLine(prompt string) (string, error) {
	fmt.Fprint(t.f, prompt)
	var b strings.Builder
	buf := make([]byte, 1)
	for {
		n, err := t.f.Read(buf)
		if n == 1 {
			if buf[0] == '\n' {
				break
			}
			b.WriteByte(buf[0])
		}
		if err != nil {
			if b.Len() > 0 {
				break
			}
			return "", err
		}
	}
	return strings.TrimSpace(b.String()), nil
}

func (t *ttyTerminal) ReadPassword(prompt string) (string, error) {
	fmt.Fprint(t.f, prompt)
	fd := int(t.f.Fd())
	// Ctrl-C would otherwise kill the process with echo still off.
	if state, err := term.GetState(fd); err == nil {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(sig)
		go func() {
			if _, ok := <-sig; ok {
				term.Restore(fd, state)
				fmt.Fprintln(t.f)
				os.Exit(130)
			}
		}()
	}
	p, err := term.ReadPassword(fd)
	fmt.Fprintln(t.f)
	// Only the line ending is not part of the password.
	return strings.TrimRight(string(p), "\r\n"), err
}
