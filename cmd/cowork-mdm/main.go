// Command cowork-mdm is a CLI toolkit for deploying Claude Desktop in the enterprise.
package main

import (
	"os"

	"github.com/krislavten/cowork-mdm/cmd/cowork-mdm/cli"
)

// These are populated at build time via -ldflags by GoReleaser.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	os.Exit(cli.Execute(cli.BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	}))
}
