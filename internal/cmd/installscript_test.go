package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func runInstallScript(t *testing.T, id string) (string, error) {
	t.Helper()
	t.Setenv("KINDTKS_BUILD_ID", id)
	var out bytes.Buffer
	installScriptCmd.SetOut(&out)
	err := installScriptCmd.RunE(installScriptCmd, nil)
	return out.String(), err
}

func TestInstallScript_FillsBuildID(t *testing.T) {
	out, err := runInstallScript(t, "0.6.0-abc1234-20261008T000000Z")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `BUILD_ID="0.6.0-abc1234-20261008T000000Z"`) || strings.Contains(out, "__KINDTKS_BUILD_ID__") {
		t.Errorf("build ID not filled in:\n%s", out)
	}
	if !strings.HasPrefix(out, "#!/usr/bin/env bash\n") {
		t.Errorf("not a bash script")
	}
}

func TestInstallScript_RejectsMissingOrUnsafeID(t *testing.T) {
	for _, id := range []string{"", "dev", `x"; rm -rf ~; "`} {
		if _, err := runInstallScript(t, id); err == nil {
			t.Errorf("build ID %q: want error", id)
		}
	}
}
