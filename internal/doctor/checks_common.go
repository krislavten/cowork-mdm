package doctor

import (
	"context"
	"os/exec"
)

// appPresentCheck is a placeholder cross-platform check. The real logic is
// platform-specific and lives in checks_darwin.go / checks_windows.go. On
// linux we emit a single "unsupported platform" skipped result so the
// user gets a clear signal.
type appPresentCheck struct{}

func (appPresentCheck) Run(_ context.Context) Result {
	return Result{
		ID:     "app.installed",
		Name:   "Claude Desktop installed",
		Status: StatusSkipped,
	}
}

// gitCheck — verify that `git` is available, for troubleshooting marketplace
// operations (our marketplace package uses go-git so this is a user-facing
// convenience, not a strict requirement).
type gitCheck struct{}

func (gitCheck) Run(_ context.Context) Result {
	if _, err := exec.LookPath("git"); err != nil {
		return Result{
			ID:      "git.available",
			Name:    "git binary on PATH (optional)",
			Status:  StatusWarning,
			Message: "git not found on PATH",
			Detail:  "cowork-mdm uses go-git internally so this is not fatal, but git on PATH helps debug marketplace issues.",
		}
	}
	return Result{
		ID:     "git.available",
		Name:   "git binary on PATH (optional)",
		Status: StatusOK,
	}
}
