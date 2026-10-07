package cmd

import (
	"fmt"
	"os"
	"strings"

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
	p, err := term.ReadPassword(int(t.f.Fd()))
	fmt.Fprintln(t.f)
	return strings.TrimSpace(string(p)), err
}
