package cmd

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func decode(t *testing.T, s string) any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestValueLines(t *testing.T) {
	doc := decode(t, `{"gateway":{"extraHosts":["*.example.net","*.example.com"],"port":8080,"tls":false,"name":"gw","none":null},"empty":[]}`)
	cases := []struct {
		path  string
		lines []string
		found bool
	}{
		{"gateway.extraHosts", []string{"*.example.net", "*.example.com"}, true},
		{"gateway.port", []string{"8080"}, true},
		{"gateway.tls", []string{"false"}, true},
		{"gateway.name", []string{"gw"}, true},
		{"gateway.none", nil, false},
		{"gateway.missing", nil, false},
		{"gateway.name.deeper", nil, false},
		{"empty", []string{}, true},
	}
	for _, c := range cases {
		lines, found, err := valueLines(doc, c.path)
		if err != nil {
			t.Errorf("%s: unexpected error %v", c.path, err)
			continue
		}
		if found != c.found || (c.found && !reflect.DeepEqual(lines, c.lines)) {
			t.Errorf("%s: got %v %v, want %v %v", c.path, lines, found, c.lines, c.found)
		}
	}
}

func TestValueCmd_SilencesUsage(t *testing.T) {
	if !valueCmd.SilenceUsage {
		t.Error("valueCmd should set SilenceUsage so a script error doesn't dump cobra usage")
	}
}

func TestValueLines_Errors(t *testing.T) {
	doc := decode(t, `{"gateway":{"extraHosts":["a"]},"objs":[{"a":1}]}`)
	for _, path := range []string{"gateway", "objs", ""} {
		if _, _, err := valueLines(doc, path); err == nil {
			t.Errorf("%q: expected error", path)
		}
	}
}
