//go:build !darwin && !windows

package doctor

import "context"

func defaultChecks() []Check {
	return []Check{
		unsupportedCheck{},
	}
}

type unsupportedCheck struct{}

func (unsupportedCheck) Run(_ context.Context) Result {
	return Result{
		ID:      "platform",
		Name:    "Platform support",
		Status:  StatusSkipped,
		Message: "doctor only supports macOS and Windows in v0.2",
	}
}
