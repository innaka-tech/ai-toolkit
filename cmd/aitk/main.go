// Command aitk is the continuity and quality layer for AI coding agents.
package main

import (
	"os"

	"github.com/innaka-tech/ai-toolkit/internal/cli"
)

func main() { os.Exit(cli.Execute(os.Args[1:], os.Stdout, os.Stderr)) }
