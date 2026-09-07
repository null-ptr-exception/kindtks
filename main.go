package main

import (
	_ "embed"

	"github.com/rophy/kindtks/internal/cmd"
)

//go:embed README.md
var readme string

var version = "dev"

func main() {
	cmd.SetVersion(version)
	cmd.SetProjectReadme(readme)
	cmd.Execute()
}
