//go:build windows

package doctor

import (
	"context"
	"fmt"
	"os"

	"github.com/krislavten/cowork-mdm/internal/managed"
	"github.com/krislavten/cowork-mdm/internal/paths"
)

func defaultChecks() []Check {
	return []Check{
		appInstalledWindows{},
		registryExistsCheck{},
		registryParseCheck{},
		gitCheck{},
	}
}

type appInstalledWindows struct{}

func (appInstalledWindows) Run(_ context.Context) Result {
	path := paths.Default().ClaudeAppPath()
	if _, err := os.Stat(path); err != nil {
		return Result{
			ID:      "app.installed",
			Name:    "Claude Desktop installed",
			Status:  StatusError,
			Message: fmt.Sprintf("%s not found", path),
		}
	}
	return Result{ID: "app.installed", Name: "Claude Desktop installed", Status: StatusOK, Message: path}
}

type registryExistsCheck struct{}

func (registryExistsCheck) Run(_ context.Context) Result {
	rep, err := managed.Status(managed.StatusOptions{})
	if err != nil {
		return Result{ID: "registry.exists", Name: "Managed registry key present", Status: StatusError, Message: err.Error()}
	}
	if !rep.Present {
		return Result{
			ID:      "registry.exists",
			Name:    "Managed registry key present",
			Status:  StatusWarning,
			Message: "no managed key at " + rep.TargetPath,
			Detail:  "Run `cowork-mdm profile apply` or deploy via Group Policy / Intune.",
		}
	}
	return Result{ID: "registry.exists", Name: "Managed registry key present", Status: StatusOK, Message: rep.TargetPath}
}

type registryParseCheck struct{}

func (registryParseCheck) Run(_ context.Context) Result {
	rep, err := managed.Status(managed.StatusOptions{})
	if err != nil {
		return Result{ID: "registry.schema", Name: "Registry values match schema", Status: StatusError, Message: err.Error()}
	}
	if !rep.Present {
		return Result{ID: "registry.schema", Name: "Registry values match schema", Status: StatusSkipped}
	}
	if rep.ParseError != "" {
		return Result{
			ID:      "registry.schema",
			Name:    "Registry values match schema",
			Status:  StatusError,
			Message: "parse error",
			Detail:  rep.ParseError,
		}
	}
	if len(rep.UnknownKeys) > 0 {
		return Result{
			ID:      "registry.schema",
			Name:    "Registry values match schema",
			Status:  StatusWarning,
			Message: fmt.Sprintf("%d unknown key(s)", len(rep.UnknownKeys)),
		}
	}
	return Result{ID: "registry.schema", Name: "Registry values match schema", Status: StatusOK, Message: fmt.Sprintf("%d keys", rep.Profile.Len())}
}
