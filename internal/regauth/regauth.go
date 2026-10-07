// Package regauth checks registry credentials the way `docker login` does:
// GET /v2/, then answer its Basic or Bearer challenge with the credentials.
package regauth

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Endpoint is the registry API base URL (scheme://host[:port]) and whether to
// skip TLS verification, as containerd will connect to it.
type Endpoint struct {
	URL        string
	SkipVerify bool
}

type Status int

const (
	OK       Status = iota // credentials accepted
	Rejected               // registry answered 401 or 403
	NoAuth                 // registry needs no login
)

type Result struct {
	Status Status
	Detail string // HTTP status of a rejection
}

const timeout = 10 * time.Second

// Check returns OK, Rejected or NoAuth; an error means the credentials could
// not be checked (network, TLS, or an unexpected answer).
func Check(ctx context.Context, ep Endpoint, username, password string, scopes []string) (Result, error) {
	client := newClient(ep.SkipVerify)
	resp, err := get(ctx, client, ep.URL+"/v2/", "", "")
	if err != nil {
		return Result{}, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return Result{Status: NoAuth}, nil
	case http.StatusUnauthorized:
	default:
		return Result{}, fmt.Errorf("unexpected %s from %s/v2/", resp.Status, ep.URL)
	}

	challenge := resp.Header.Get("WWW-Authenticate")
	scheme, params := parseChallenge(challenge)
	var target string
	switch strings.ToLower(scheme) {
	case "basic":
		target = ep.URL + "/v2/"
	case "bearer":
		u, err := url.Parse(params["realm"])
		if err != nil || params["realm"] == "" {
			return Result{}, fmt.Errorf("unusable login challenge %q", challenge)
		}
		if strings.HasPrefix(ep.URL, "https://") && strings.EqualFold(u.Scheme, "http") {
			return Result{}, fmt.Errorf("refusing to send credentials to plain-HTTP token realm %s from an HTTPS registry", params["realm"])
		}
		q := u.Query()
		if s := params["service"]; s != "" {
			q.Set("service", s)
		}
		for _, s := range scopes {
			q.Add("scope", s)
		}
		u.RawQuery = q.Encode()
		target = u.String()
	default:
		return Result{}, fmt.Errorf("unsupported login challenge %q", challenge)
	}

	resp, err = get(ctx, client, target, username, password)
	if err != nil {
		return Result{}, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return Result{Status: OK}, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return Result{Status: Rejected, Detail: resp.Status}, nil
	}
	return Result{}, fmt.Errorf("unexpected %s from %s", resp.Status, target)
}

func newClient(skipVerify bool) *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if skipVerify {
		// Matches containerd's skip_verify for this host.
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: tr,
		// Never follow redirects: Go would forward Authorization to a
		// same-host redirect on another scheme or port.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

func get(ctx context.Context, client *http.Client, target, username, password string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, err
	}
	if username != "" || password != "" {
		req.SetBasicAuth(username, password)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	return resp, nil
}

// parseChallenge splits `Scheme k1="v1",k2=v2` into the scheme and its
// parameters; quoted values may contain commas.
func parseChallenge(h string) (string, map[string]string) {
	scheme, rest, _ := strings.Cut(strings.TrimSpace(h), " ")
	params := map[string]string{}
	for rest = strings.TrimSpace(rest); rest != ""; {
		key, after, ok := strings.Cut(rest, "=")
		if !ok {
			break
		}
		key = strings.ToLower(strings.TrimSpace(key))
		var val string
		if strings.HasPrefix(after, `"`) {
			end := strings.Index(after[1:], `"`)
			if end < 0 {
				val, rest = after[1:], ""
			} else {
				val, rest = after[1:1+end], after[2+end:]
			}
		} else {
			val, rest, _ = strings.Cut(after, ",")
		}
		params[key] = val
		rest = strings.TrimLeft(strings.TrimSpace(rest), ", ")
	}
	return scheme, params
}
