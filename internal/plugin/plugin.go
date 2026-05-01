// Package plugin inspects and manages individual plugin entries under
// Claude Desktop's org-plugins/ directory.
//
// It reads marketplace state produced by internal/marketplace/ and the
// per-user Claude-3p sessions to report each plugin's status (source,
// dangling, enabled-in-session). It also provides destructive operations
// (unlink, prune) that act only on symlinks, never on real directories.
package plugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// Source classifies where a plugin came from.
type Source string

const (
	SourceSymlinkUnknown Source = "symlink-to-unknown"
	SourceLocalDirectory Source = "local-directory"
	// SourceMarketplace is the string prefix; full value is "marketplace:<repo-name>".
	SourceMarketplace Source = "marketplace"
)

// Plugin describes one discoverable plugin entry.
type Plugin struct {
	Name       string
	Source     string // e.g. "marketplace:claude-plugins-official" or "local-directory"
	TargetPath string
	IsSymlink  bool
	Dangling   bool
	Manifest   *Manifest
}

// Manifest mirrors .claude-plugin/plugin.json.
type Manifest struct {
	Name        string       `json:"name"`
	Version     string       `json:"version"`
	Description string       `json:"description"`
	Author      ManifestAuth `json:"author,omitempty"`
}

// ManifestAuth is the author block of a plugin manifest.
type ManifestAuth struct {
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

// SessionEnabled is one session's enabled state for a plugin.
type SessionEnabled struct {
	SessionPath string
	Enabled     *bool // nil = not set; pointer to true/false = explicit
	Installed   bool
}

// EnabledState summarizes one plugin's configuration across all sessions.
type EnabledState struct {
	Plugin      string
	BySession   []SessionEnabled
	AllEnabled  bool
	AnyDisabled bool
}

// Inspector reads org-plugins state without mutating.
type Inspector struct {
	orgPluginsDir string
	sessionsDir   string
}

// NewInspector constructs an Inspector bound to the given directories.
// sessionsDir may be empty on platforms where it's unsupported (linux).
func NewInspector(orgPluginsDir, sessionsDir string) *Inspector {
	return &Inspector{orgPluginsDir: orgPluginsDir, sessionsDir: sessionsDir}
}

// List returns every top-level entry under orgPluginsDir.
func (i *Inspector) List() ([]Plugin, error) {
	entries, err := os.ReadDir(i.orgPluginsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("plugin.List: %w", err)
	}
	out := []Plugin{}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		path := filepath.Join(i.orgPluginsDir, e.Name())
		info, err := os.Lstat(path)
		if err != nil {
			continue
		}
		p := Plugin{Name: e.Name()}
		if info.Mode()&os.ModeSymlink != 0 {
			p.IsSymlink = true
			target, err := os.Readlink(path)
			if err != nil {
				continue
			}
			if !filepath.IsAbs(target) {
				target = filepath.Clean(filepath.Join(i.orgPluginsDir, target))
			}
			p.TargetPath = target
			if _, err := os.Stat(target); err != nil {
				p.Dangling = true
			}
			p.Source = classifySymlink(i.orgPluginsDir, target)
			if !p.Dangling {
				p.Manifest = readManifest(target)
			}
		} else if info.IsDir() {
			p.TargetPath = path
			p.Source = string(SourceLocalDirectory)
			p.Manifest = readManifest(path)
		} else {
			// skip non-dir non-symlink files (stray .DS_Store etc.)
			continue
		}
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Get returns one plugin by name.
func (i *Inspector) Get(name string) (*Plugin, error) {
	all, err := i.List()
	if err != nil {
		return nil, err
	}
	for k := range all {
		if all[k].Name == name {
			return &all[k], nil
		}
	}
	return nil, ErrPluginNotFound
}

// ErrPluginNotFound is returned by Get when the named plugin isn't present.
var ErrPluginNotFound = errors.New("plugin: not found")

// EnabledStates enumerates per-session enablement. Returns empty slice on
// platforms where sessionsDir isn't available.
func (i *Inspector) EnabledStates() ([]EnabledState, error) {
	if i.sessionsDir == "" {
		return nil, nil
	}
	matches, err := filepath.Glob(filepath.Join(i.sessionsDir, "*", "*"))
	if err != nil {
		return nil, err
	}
	plugins, err := i.List()
	if err != nil {
		return nil, err
	}

	// Pre-load per-session state (settings + installed).
	type sessionState struct {
		path      string
		enabled   map[string]bool     // key → explicit bool (absent means nil)
		present   map[string]struct{} // set of keys in enabledPlugins
		installed map[string]bool
	}
	sessions := []sessionState{}
	for _, dir := range matches {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			continue
		}
		st := sessionState{
			path:      dir,
			enabled:   map[string]bool{},
			present:   map[string]struct{}{},
			installed: map[string]bool{},
		}
		// cowork_settings.json
		if data, err := os.ReadFile(filepath.Join(dir, "cowork_settings.json")); err == nil {
			var settings struct {
				EnabledPlugins map[string]bool `json:"enabledPlugins"`
			}
			if err := json.Unmarshal(data, &settings); err == nil {
				for k, v := range settings.EnabledPlugins {
					st.enabled[k] = v
					st.present[k] = struct{}{}
				}
			}
		}
		// cowork_plugins/installed_plugins.json
		if data, err := os.ReadFile(filepath.Join(dir, "cowork_plugins", "installed_plugins.json")); err == nil {
			var installed struct {
				Plugins map[string]any `json:"plugins"`
			}
			if err := json.Unmarshal(data, &installed); err == nil {
				for k := range installed.Plugins {
					st.installed[k] = true
				}
			}
		}
		sessions = append(sessions, st)
	}

	states := make([]EnabledState, 0, len(plugins))
	for _, p := range plugins {
		key := p.Name + "@org-provisioned"
		state := EnabledState{Plugin: p.Name, AllEnabled: true}
		hadAny := false
		for _, s := range sessions {
			se := SessionEnabled{SessionPath: s.path}
			if _, present := s.present[key]; present {
				v := s.enabled[key]
				se.Enabled = &v
				if v {
					hadAny = true
				} else {
					state.AnyDisabled = true
					state.AllEnabled = false
				}
			} else {
				state.AllEnabled = false
			}
			se.Installed = s.installed[key]
			state.BySession = append(state.BySession, se)
		}
		if !hadAny {
			state.AllEnabled = false
		}
		states = append(states, state)
	}
	return states, nil
}

// Mutator allows destructive operations. Only touches symlinks — never
// real directories (in case the user placed content manually).
type Mutator struct {
	orgPluginsDir string
}

// NewMutator returns a Mutator. On platforms without symlink support v0.2
// still compiles; runtime ops are guarded.
func NewMutator(orgPluginsDir string) *Mutator {
	return &Mutator{orgPluginsDir: orgPluginsDir}
}

// ErrInvalidName is returned when a user-supplied plugin name contains path
// separators or '..', which could otherwise escape org-plugins.
var ErrInvalidName = errors.New("plugin: name must be a bare filename, no separators or '..'")

func validName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: empty name", ErrInvalidName)
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("%w: %q contains path separator", ErrInvalidName, name)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("%w: %q", ErrInvalidName, name)
	}
	return nil
}

// Unlink removes a top-level symlink. Returns ErrPluginNotFound if the entry
// doesn't exist. Returns an error if the entry is a real directory — use
// the shell for those to preserve intent.
func (m *Mutator) Unlink(name string) error {
	if runtime.GOOS != "darwin" {
		return ErrUnsupportedPlatform
	}
	if err := validName(name); err != nil {
		return err
	}
	path := filepath.Join(m.orgPluginsDir, name)
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrPluginNotFound
		}
		return err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("plugin.Unlink: %s is a real directory; remove manually if intended", name)
	}
	return os.Remove(path)
}

// Prune removes every dangling top-level symlink. Returns the list of
// removed names.
func (m *Mutator) Prune() ([]string, error) {
	if runtime.GOOS != "darwin" {
		return nil, ErrUnsupportedPlatform
	}
	entries, err := os.ReadDir(m.orgPluginsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("plugin.Prune: %w", err)
	}
	var removed []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		path := filepath.Join(m.orgPluginsDir, e.Name())
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		target, err := os.Readlink(path)
		if err != nil {
			continue
		}
		if !filepath.IsAbs(target) {
			target = filepath.Clean(filepath.Join(m.orgPluginsDir, target))
		}
		// Only prune when the target DEFINITELY doesn't exist. Other stat
		// errors (EACCES, EIO) are NOT the same as "target missing" — deleting
		// on those would destroy user data hidden behind a permission boundary.
		if _, err := os.Stat(target); err != nil && errors.Is(err, os.ErrNotExist) {
			if err := os.Remove(path); err == nil {
				removed = append(removed, e.Name())
			}
		}
	}
	sort.Strings(removed)
	return removed, nil
}

// ErrUnsupportedPlatform is returned for symlink operations on non-macOS.
var ErrUnsupportedPlatform = errors.New("plugin: symlink management only on macOS in v0.2")

// --- helpers ---

func readManifest(dir string) *Manifest {
	data, err := os.ReadFile(filepath.Join(dir, ".claude-plugin", "plugin.json"))
	if err != nil {
		return nil
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return &m
}

// classifySymlink returns either "marketplace:<repo-basename>" or
// "symlink-to-unknown" based on whether the target is inside orgPluginsDir.
func classifySymlink(orgPluginsDir, target string) string {
	if !strings.HasPrefix(target, orgPluginsDir+string(filepath.Separator)) {
		return string(SourceSymlinkUnknown)
	}
	rel := strings.TrimPrefix(target, orgPluginsDir+string(filepath.Separator))
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) == 0 {
		return string(SourceSymlinkUnknown)
	}
	return fmt.Sprintf("marketplace:%s", parts[0])
}
