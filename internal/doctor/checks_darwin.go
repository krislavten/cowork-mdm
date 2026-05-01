//go:build darwin

package doctor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/krislavten/cowork-mdm/internal/managed"
	"github.com/krislavten/cowork-mdm/internal/marketplace"
	"github.com/krislavten/cowork-mdm/internal/paths"
	"github.com/krislavten/cowork-mdm/internal/plugin"
)

func defaultChecks() []Check {
	return []Check{
		appInstalledDarwin{},
		plistExistsCheck{},
		plistParseCheck{},
		orgPluginsExistCheck{},
		orgPluginsSymlinksCheck{},
		orgPluginsDanglingCheck{},
		marketplaceReposCheck{},
		userSessionsCheck{},
		gitCheck{},
	}
}

// --- app installed ---

type appInstalledDarwin struct{}

func (appInstalledDarwin) Run(_ context.Context) Result {
	path := paths.Default().ClaudeAppPath()
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Result{
				ID:      "app.installed",
				Name:    "Claude Desktop installed",
				Status:  StatusError,
				Message: fmt.Sprintf("%s not found", path),
				Detail:  "Download Claude Desktop from https://claude.com/download and re-run doctor.",
			}
		}
		return Result{ID: "app.installed", Name: "Claude Desktop installed", Status: StatusError, Message: err.Error()}
	}
	return Result{ID: "app.installed", Name: "Claude Desktop installed", Status: StatusOK, Message: path}
}

// --- plist exists ---

type plistExistsCheck struct{}

func (plistExistsCheck) Run(_ context.Context) Result {
	path := paths.Default().ManagedPrefsPlist()
	if _, err := os.Stat(path); err != nil {
		return Result{
			ID:      "plist.exists",
			Name:    "Managed plist present",
			Status:  StatusWarning,
			Message: "no managed plist installed at " + path,
			Detail:  "This is fine if you're not deploying Claude Desktop via MDM. Otherwise, run `cowork-mdm profile apply`.",
		}
	}
	return Result{ID: "plist.exists", Name: "Managed plist present", Status: StatusOK, Message: path}
}

// --- plist parse + schema ---

type plistParseCheck struct{}

func (plistParseCheck) Run(_ context.Context) Result {
	rep, err := managed.Status(managed.StatusOptions{})
	if err != nil {
		return Result{ID: "plist.schema", Name: "Managed plist parses + matches schema", Status: StatusError, Message: err.Error()}
	}
	if !rep.Present {
		return Result{ID: "plist.schema", Name: "Managed plist parses + matches schema", Status: StatusSkipped, Message: "no plist installed"}
	}
	if rep.ParseError != "" {
		return Result{
			ID:      "plist.schema",
			Name:    "Managed plist parses + matches schema",
			Status:  StatusError,
			Message: "plist parse failed",
			Detail:  rep.ParseError,
		}
	}
	// Run BOTH checks regardless of unknown keys so schema violations
	// don't stay hidden behind a soft-warning for unknowns.
	var details []string
	var status Status = StatusOK
	if len(rep.UnknownKeys) > 0 {
		names := make([]string, len(rep.UnknownKeys))
		for i, k := range rep.UnknownKeys {
			names[i] = k.Key
		}
		details = append(details, fmt.Sprintf("unknown keys (not in schema): %s", joinStrings(names)))
		// Unknown keys are an error — they indicate the plist was generated
		// for a different cowork-mdm release OR someone hand-edited it.
		// Either way the user needs to know with error-level urgency.
		status = StatusError
	}
	if err := rep.Profile.Validate(); err != nil {
		details = append(details, "schema violations: "+err.Error())
		status = StatusError
	}
	if status == StatusOK {
		return Result{
			ID:      "plist.schema",
			Name:    "Managed plist parses + matches schema",
			Status:  StatusOK,
			Message: fmt.Sprintf("%d keys valid", rep.Profile.Len()),
		}
	}
	return Result{
		ID:      "plist.schema",
		Name:    "Managed plist parses + matches schema",
		Status:  status,
		Message: "plist does not match schema",
		Detail:  strings.Join(details, "\n"),
	}
}

// --- org-plugins dir ---

type orgPluginsExistCheck struct{}

func (c orgPluginsExistCheck) Run(_ context.Context) Result {
	path := paths.Default().OrgPluginsDir()
	if _, err := os.Stat(path); err != nil {
		return Result{
			ID:           "orgplugins.exists",
			Name:         "org-plugins/ directory exists",
			Status:       StatusWarning,
			Message:      path + " not present",
			Detail:       "No plugins have been installed via cowork-mdm marketplace. Run `cowork-mdm marketplace add <git-url>` to install some.",
			FixAvailable: true,
		}
	}
	return Result{ID: "orgplugins.exists", Name: "org-plugins/ directory exists", Status: StatusOK, Message: path}
}

func (c orgPluginsExistCheck) Fix(_ context.Context) error {
	return os.MkdirAll(paths.Default().OrgPluginsDir(), 0o755)
}

// --- symlinks check ---

type orgPluginsSymlinksCheck struct{}

func (orgPluginsSymlinksCheck) Run(_ context.Context) Result {
	root := paths.Default().OrgPluginsDir()
	ins := plugin.NewInspector(root, paths.Default().UserSessionsDir())
	plugins, err := ins.List()
	if err != nil {
		return Result{ID: "orgplugins.symlinks", Name: "Top-level org-plugins entries are valid", Status: StatusError, Message: err.Error()}
	}
	var bad []string
	for _, p := range plugins {
		if p.IsSymlink && p.Dangling {
			continue // separate check
		}
		if p.IsSymlink && p.Source == string(plugin.SourceSymlinkUnknown) {
			bad = append(bad, p.Name+" → "+p.TargetPath)
		}
	}
	if len(bad) > 0 {
		return Result{
			ID:      "orgplugins.symlinks",
			Name:    "Top-level org-plugins entries are valid",
			Status:  StatusWarning,
			Message: fmt.Sprintf("%d symlink(s) point outside org-plugins/", len(bad)),
			Detail:  joinStrings(bad),
		}
	}
	return Result{
		ID:      "orgplugins.symlinks",
		Name:    "Top-level org-plugins entries are valid",
		Status:  StatusOK,
		Message: fmt.Sprintf("%d entries", len(plugins)),
	}
}

// --- dangling symlinks ---

type orgPluginsDanglingCheck struct{}

func (c orgPluginsDanglingCheck) Run(_ context.Context) Result {
	root := paths.Default().OrgPluginsDir()
	ins := plugin.NewInspector(root, "")
	plugins, err := ins.List()
	if err != nil {
		return Result{ID: "orgplugins.dangling", Name: "No dangling symlinks", Status: StatusError, Message: err.Error()}
	}
	var dangling []string
	for _, p := range plugins {
		if p.Dangling {
			dangling = append(dangling, p.Name)
		}
	}
	if len(dangling) > 0 {
		return Result{
			ID:           "orgplugins.dangling",
			Name:         "No dangling symlinks",
			Status:       StatusWarning,
			Message:      fmt.Sprintf("%d dangling symlink(s): %s", len(dangling), joinStrings(dangling)),
			Detail:       "Fix with: cowork-mdm plugin prune",
			FixAvailable: true,
		}
	}
	return Result{ID: "orgplugins.dangling", Name: "No dangling symlinks", Status: StatusOK}
}

func (c orgPluginsDanglingCheck) Fix(_ context.Context) error {
	mut := plugin.NewMutator(paths.Default().OrgPluginsDir())
	_, err := mut.Prune()
	return err
}

// --- user sessions ---

type userSessionsCheck struct{}

func (userSessionsCheck) Run(_ context.Context) Result {
	sessionsDir := paths.Default().UserSessionsDir()
	if sessionsDir == "" {
		return Result{ID: "user.sessions", Name: "Claude-3p sessions discoverable", Status: StatusSkipped}
	}
	if _, err := os.Stat(sessionsDir); err != nil {
		return Result{
			ID:      "user.sessions",
			Name:    "Claude-3p sessions discoverable",
			Status:  StatusWarning,
			Message: "no sessions dir (user hasn't launched Claude Desktop in 3p mode yet)",
			Detail:  sessionsDir,
		}
	}
	matches, _ := filepath.Glob(filepath.Join(sessionsDir, "*", "*"))
	return Result{
		ID:      "user.sessions",
		Name:    "Claude-3p sessions discoverable",
		Status:  StatusOK,
		Message: fmt.Sprintf("%d session(s)", len(matches)),
	}
}

// (marketplace.repos-operable check would need a Manager instance — we
// compose one from paths. Kept simple; go-git errors surface via List.)
type marketplaceReposCheck struct{}

func (marketplaceReposCheck) Run(_ context.Context) Result {
	root := paths.Default().OrgPluginsDir()
	m := marketplace.NewManager(root)
	repos, err := m.List()
	if err != nil {
		return Result{ID: "marketplace.repos-operable", Name: "Marketplace repos operable", Status: StatusError, Message: err.Error()}
	}
	if len(repos) == 0 {
		return Result{ID: "marketplace.repos-operable", Name: "Marketplace repos operable", Status: StatusSkipped, Message: "no marketplaces installed"}
	}
	return Result{
		ID:      "marketplace.repos-operable",
		Name:    "Marketplace repos operable",
		Status:  StatusOK,
		Message: fmt.Sprintf("%d marketplace(s)", len(repos)),
	}
}

func joinStrings(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += ", "
		}
		out += s
	}
	return out
}
