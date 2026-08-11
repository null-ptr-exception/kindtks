package main

import (
	_ "embed"

	"github.com/rophy/kindtks/internal/cmd"
)

//go:embed README.md
var readme string

func main() {
	cmd.SetProjectReadme(readme)
	cmd.Execute()
}
