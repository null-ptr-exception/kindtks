package regauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func basicHandler(user, pass string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); ok && u == user && p == pass {
			return
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
		w.WriteHeader(http.StatusUnauthorized)
	}
}

// bearerServer challenges /v2/ with a token realm on itself and records the
// scopes and service the token request carried.
func bearerServer(t *testing.T, user, pass string, scopes *[]string, service *string) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="`+srv.URL+`/token",service="test-registry"`)
		w.WriteHeader(http.StatusUnauthorized)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		*scopes = r.URL.Query()["scope"]
		*service = r.URL.Query().Get("service")
		if u, p, ok := r.BasicAuth(); !ok || u != user || p != pass {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write([]byte(`{"token":"t"}`))
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func check(t *testing.T, ep Endpoint, user, pass string, scopes []string) (Result, error) {
	t.Helper()
	return Check(context.Background(), ep, user, pass, scopes)
}

func TestCheck_Basic(t *testing.T) {
	srv := httptest.NewServer(basicHandler("u", "p"))
	defer srv.Close()
	if r, err := check(t, Endpoint{URL: srv.URL}, "u", "p", nil); err != nil || r.Status != OK {
		t.Errorf("right: %+v, %v", r, err)
	}
	r, err := check(t, Endpoint{URL: srv.URL}, "u", "wrong", nil)
	if err != nil || r.Status != Rejected || r.Detail != "401 Unauthorized" {
		t.Errorf("wrong: %+v, %v", r, err)
	}
}

func TestCheck_Bearer(t *testing.T) {
	var scopes []string
	var service string
	srv := bearerServer(t, "u", "p", &scopes, &service)
	want := []string{"repository:a:pull", "repository:b:pull"}
	if r, err := check(t, Endpoint{URL: srv.URL}, "u", "p", want); err != nil || r.Status != OK {
		t.Errorf("right: %+v, %v", r, err)
	}
	if !slices.Equal(scopes, want) || service != "test-registry" {
		t.Errorf("token request: scopes %v, service %q", scopes, service)
	}
	if r, err := check(t, Endpoint{URL: srv.URL}, "u", "wrong", want); err != nil || r.Status != Rejected {
		t.Errorf("wrong: %+v, %v", r, err)
	}
}

func TestCheck_NoAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	if r, err := check(t, Endpoint{URL: srv.URL}, "u", "p", nil); err != nil || r.Status != NoAuth {
		t.Errorf("got %+v, %v", r, err)
	}
}

func TestCheck_CouldNotCheck(t *testing.T) {
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failing.Close()
	if _, err := check(t, Endpoint{URL: failing.URL}, "u", "p", nil); err == nil {
		t.Error("500: want error")
	}

	closed := httptest.NewServer(basicHandler("u", "p"))
	closed.Close()
	if _, err := check(t, Endpoint{URL: closed.URL}, "u", "p", nil); err == nil {
		t.Error("connection refused: want error")
	}

	plain := httptest.NewServer(basicHandler("u", "p"))
	defer plain.Close()
	if _, err := check(t, Endpoint{URL: "https://" + strings.TrimPrefix(plain.URL, "http://")}, "u", "p", nil); err == nil {
		t.Error("TLS to plain HTTP: want error")
	}
}

func TestCheck_SkipVerify(t *testing.T) {
	srv := httptest.NewTLSServer(basicHandler("u", "p"))
	defer srv.Close()
	if _, err := check(t, Endpoint{URL: srv.URL}, "u", "p", nil); err == nil {
		t.Error("self-signed without skip: want error")
	}
	if r, err := check(t, Endpoint{URL: srv.URL, SkipVerify: true}, "u", "p", nil); err != nil || r.Status != OK {
		t.Errorf("skip verify: %+v, %v", r, err)
	}
}

func TestParseChallenge_QuotedCommas(t *testing.T) {
	scheme, params := parseChallenge(`Bearer realm="https://auth.example.com/token",service="registry.example.com",scope="repository:a:pull,push"`)
	if scheme != "Bearer" || params["realm"] != "https://auth.example.com/token" ||
		params["service"] != "registry.example.com" || params["scope"] != "repository:a:pull,push" {
		t.Errorf("got %q %v", scheme, params)
	}
}
