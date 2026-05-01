//go:build darwin || windows

package doctor

import (
	"context"
	"os/exec"
)

// gitCheck verifies that `git` is available for troubleshooting marketplace
// operations. Not strictly required (the marketplace package uses go-git
// internally), but helpful for debugging.
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
