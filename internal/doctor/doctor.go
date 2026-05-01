// Package doctor enumerates environment checks that report whether
// Claude Desktop deployment is healthy on this host. Each check is a pure
// function producing a Result; the Runner orchestrates them.
package doctor

import (
	"context"
	"encoding/json"
	"runtime"
)

// Status classifies a check result.
type Status string

const (
	StatusOK      Status = "ok"
	StatusWarning Status = "warning"
	StatusError   Status = "error"
	StatusSkipped Status = "skipped"
)

// Result is one check's outcome.
type Result struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Status       Status `json:"status"`
	Message      string `json:"message,omitempty"`
	Detail       string `json:"detail,omitempty"`
	FixAvailable bool   `json:"fixAvailable"`
}

// Check is the interface every check implements.
type Check interface {
	Run(ctx context.Context) Result
}

// Fixable checks can attempt an automatic remediation.
type Fixable interface {
	Check
	Fix(ctx context.Context) error
}

// Runner orchestrates a set of checks.
type Runner struct {
	Checks []Check
}

// DefaultRunner returns a Runner with all checks appropriate to the host OS.
func DefaultRunner() *Runner {
	return &Runner{Checks: defaultChecks()}
}

// Run executes every check in order. Never stops on failure.
func (r *Runner) Run(ctx context.Context) []Result {
	out := make([]Result, 0, len(r.Checks))
	for _, c := range r.Checks {
		out = append(out, c.Run(ctx))
	}
	return out
}

// RunAndFix executes every check; for any non-OK Fixable, calls Fix and
// re-runs that check. Returns the final (post-fix) results.
func (r *Runner) RunAndFix(ctx context.Context) []Result {
	out := make([]Result, 0, len(r.Checks))
	for _, c := range r.Checks {
		res := c.Run(ctx)
		if res.Status != StatusOK && res.Status != StatusSkipped {
			if fix, ok := c.(Fixable); ok && res.FixAvailable {
				if err := fix.Fix(ctx); err == nil {
					res = c.Run(ctx)
				} else {
					res.Detail = appendDetail(res.Detail, "auto-fix failed: "+err.Error())
				}
			}
		}
		out = append(out, res)
	}
	return out
}

// JSON marshals results as a JSON array with a summary object attached.
func JSON(results []Result) ([]byte, error) {
	summary := map[string]int{"total": len(results)}
	for _, r := range results {
		summary[string(r.Status)]++
	}
	payload := map[string]any{
		"platform": runtime.GOOS,
		"summary":  summary,
		"results":  results,
	}
	return json.MarshalIndent(payload, "", "  ")
}

// Exit returns an appropriate CLI exit code.
//
//	0 — all OK or Warning only
//	1 — any Error
//	2 — internal doctor failure (not used currently)
func Exit(results []Result) int {
	for _, r := range results {
		if r.Status == StatusError {
			return 1
		}
	}
	return 0
}

func appendDetail(existing, extra string) string {
	if existing == "" {
		return extra
	}
	return existing + "\n" + extra
}
