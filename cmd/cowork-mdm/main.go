// Command cowork-mdm is a CLI toolkit for deploying Claude Desktop in the enterprise.
//
// This is a pre-release stub; the command surface lands in milestone M3 (see
// docs/execution/TASKS.md). The stub exists so that `go build ./...` and the
// release pipeline have a compilable entry point from day 1.
package main

import "fmt"

// These are populated at build time via -ldflags by GoReleaser.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	fmt.Printf("cowork-mdm %s (commit %s, built %s)\n", version, commit, date)
	fmt.Println("(pre-release — full command surface arrives in v0.2 milestone M3)")
}
