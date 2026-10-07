package cmd

import "testing"

func TestExpandEnv(t *testing.T) {
	env := map[string]string{"A": "x", "B": "y"}
	get := func(k string) string { return env[k] }
	cases := []struct{ name, in, want string }{
		{"braces", "v=${A}!", "v=x!"},
		{"bare", "v=$A!", "v=x!"},
		{"bare greedy name", "$AB", ""},
		{"unset", "a${NOPE}b$NOPE.", "ab."},
		{"positional", "cost $1", "cost $1"},
		{"double dollar", "$$", "$$"},
		{"braced digit", "${1}", "${1}"},
		{"trailing dollar", "price$", "price$"},
		{"no variables", "plain: text\n", "plain: text\n"},
		{"multiple", "${A}-$B", "x-y"},
	}
	for _, c := range cases {
		if got := expandEnv(c.in, get); got != c.want {
			t.Errorf("%s: expandEnv(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}
