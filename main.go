package main

import (
	_ "embed"
	"strings"

	"github.com/rophy/kindtks/internal/cmd"
)

//go:embed README.md
var readme string

//go:embed VERSION
var versionFile string

func buildVersion() string {
	return strings.TrimSpace(versionFile)
}

func main() {
	cmd.SetVersion(buildVersion())
	cmd.SetProjectReadme(readme)
	cmd.Execute()
}
