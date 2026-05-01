// Package marketplace manages git-backed plugin marketplace repositories
// that sit under Claude Desktop's org-plugins/ directory.
//
// A marketplace repo follows Anthropic's convention: top-level plugins/<name>/
// and optionally external_plugins/<name>/ with each subdirectory containing
// .claude-plugin/plugin.json. This package clones such repos into
//
//	<org-plugins>/<repo-basename>/
//
// and then manages symlinks at <org-plugins>/<plugin-name> so Claude Desktop
// sees every plugin at the top level of org-plugins.
//
// Platform: macOS only for v0.2. Windows org-plugins requires junctions +
// admin + Developer Mode and is deferred.
package marketplace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// Repo describes one cloned marketplace.
type Repo struct {
	Name       string
	URL        string
	Path       string
	Plugins    []string
	CurrentRef string
	LastPull   time.Time
}

// AddOptions configures Add.
type AddOptions struct {
	// Name overrides the default basename derived from the URL.
	Name string
	// Depth sets git clone depth.
	//   Depth <  0 (or unset, since 0 is the zero value) → default 1 (shallow).
	//   Depth == 0 → full clone (unlimited history).
	//   Depth >  0 → explicit shallow depth.
	//
	// Because 0 is Go's zero value, AddOptions{} gives you shallow (1).
	// To request a full clone, set Depth to a sentinel: pass DepthFull.
	Depth int
}

// DepthFull is the explicit "full history" sentinel for AddOptions.Depth.
// Using a negative value avoids collision with Go's zero-value default.
const DepthFull = -1

// UpdateResult is one entry returned from UpdateAll.
type UpdateResult struct {
	Name    string
	Updated bool
	FromRef string
	ToRef   string
	Err     error
}

// Conflict describes a top-level entry that LinkAll refused to touch.
type Conflict struct {
	Name   string
	Reason string
}

// LinkReport summarizes a LinkAll pass.
type LinkReport struct {
	Created   []string
	Updated   []string
	Unchanged []string
	Removed   []string
	Conflicts []Conflict
}

// Manager owns the set of marketplaces under a given org-plugins root.
type Manager struct {
	rootDir string
}

// NewManager returns a Manager bound to rootDir. rootDir should be an
// absolute path; typically paths.Default().OrgPluginsDir() on macOS.
func NewManager(rootDir string) *Manager {
	return &Manager{rootDir: rootDir}
}

// Sentinel errors.
var (
	ErrRepoNotFound        = errors.New("marketplace: repo not found")
	ErrUnsupportedPlatform = errors.New("marketplace: symlink management only supported on macOS in v0.2")
	ErrInvalidName         = errors.New("marketplace: name contains path separators or '..'")
)

// validRepoName rejects names containing path separators, '..', or starting
// with '.'. These could escape the rootDir via filepath.Join tricks.
func validRepoName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: empty name", ErrInvalidName)
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("%w: %q contains path separator", ErrInvalidName, name)
	}
	if name == "." || name == ".." || strings.HasPrefix(name, ".") {
		return fmt.Errorf("%w: %q", ErrInvalidName, name)
	}
	return nil
}

// Add clones url into rootDir/<basename>. Returns the created Repo.
// Errors if the destination basename already exists.
func (m *Manager) Add(ctx context.Context, url string, opts AddOptions) (*Repo, error) {
	if m.rootDir == "" {
		return nil, fmt.Errorf("marketplace.Add: root dir is empty (unsupported platform?)")
	}
	name := opts.Name
	if name == "" {
		name = basenameFromURL(url)
	}
	if name == "" {
		return nil, fmt.Errorf("marketplace.Add: cannot derive repo name from URL %q", url)
	}
	if err := validRepoName(name); err != nil {
		return nil, fmt.Errorf("marketplace.Add: %w", err)
	}
	dest := filepath.Join(m.rootDir, name)

	if _, err := os.Stat(dest); err == nil {
		return nil, fmt.Errorf("marketplace.Add: %s already exists", dest)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("marketplace.Add: stat %s: %w", dest, err)
	}

	if err := os.MkdirAll(m.rootDir, 0o755); err != nil {
		return nil, fmt.Errorf("marketplace.Add: mkdir rootDir: %w", err)
	}

	// depth: 0 means default shallow, DepthFull (-1) means full, positive
	// means explicit shallow depth. go-git's CloneOptions treats 0 as
	// unlimited, which isn't what we want as the default — so we normalize.
	cloneDepth := opts.Depth
	switch {
	case cloneDepth == 0:
		cloneDepth = 1 // default shallow
	case cloneDepth == DepthFull:
		cloneDepth = 0 // full clone for go-git
	case cloneDepth < 0:
		cloneDepth = 1 // treat unknown negatives as default shallow
	}

	_, err := git.PlainCloneContext(ctx, dest, false, &git.CloneOptions{
		URL:   url,
		Depth: cloneDepth,
	})
	if err != nil {
		// Clean up partial clone on failure.
		_ = os.RemoveAll(dest)
		return nil, fmt.Errorf("marketplace.Add: clone %s: %w", url, err)
	}

	return m.repoInfo(name)
}

// List returns all known marketplace repos (directories under rootDir
// that contain a .git/ subdir).
func (m *Manager) List() ([]Repo, error) {
	entries, err := os.ReadDir(m.rootDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("marketplace.List: read %s: %w", m.rootDir, err)
	}
	var repos []Repo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if _, err := os.Stat(filepath.Join(m.rootDir, e.Name(), ".git")); err != nil {
			continue
		}
		r, err := m.repoInfo(e.Name())
		if err != nil {
			continue
		}
		repos = append(repos, *r)
	}
	sort.Slice(repos, func(i, j int) bool { return repos[i].Name < repos[j].Name })
	return repos, nil
}

// Get returns a single repo by basename.
func (m *Manager) Get(name string) (*Repo, error) {
	return m.repoInfo(name)
}

// Update fast-forwards a marketplace.
func (m *Manager) Update(ctx context.Context, name string) error {
	dir := filepath.Join(m.rootDir, name)
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return fmt.Errorf("%w: %s", ErrRepoNotFound, name)
	}
	r, err := git.PlainOpen(dir)
	if err != nil {
		return fmt.Errorf("marketplace.Update: open %s: %w", dir, err)
	}
	wt, err := r.Worktree()
	if err != nil {
		return fmt.Errorf("marketplace.Update: worktree: %w", err)
	}
	err = wt.PullContext(ctx, &git.PullOptions{
		RemoteName: "origin",
		Force:      false,
	})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return fmt.Errorf("marketplace.Update: pull: %w", err)
	}
	return nil
}

// UpdateAll iterates over List() and updates each. Returns per-repo results;
// never returns an error itself.
func (m *Manager) UpdateAll(ctx context.Context) []UpdateResult {
	repos, err := m.List()
	if err != nil {
		return []UpdateResult{{Err: err}}
	}
	out := make([]UpdateResult, 0, len(repos))
	for _, repo := range repos {
		res := UpdateResult{Name: repo.Name, FromRef: repo.CurrentRef}
		if err := m.Update(ctx, repo.Name); err != nil {
			res.Err = err
			out = append(out, res)
			continue
		}
		if after, err := m.repoInfo(repo.Name); err == nil {
			res.ToRef = after.CurrentRef
			res.Updated = res.ToRef != res.FromRef
		}
		out = append(out, res)
	}
	return out
}

// Remove deletes the marketplace clone and any top-level symlinks pointing
// into it. Strict safety: refuses anything that isn't a recognized
// marketplace repo (must have .git/).
func (m *Manager) Remove(name string) error {
	if err := validRepoName(name); err != nil {
		return fmt.Errorf("marketplace.Remove: %w", err)
	}
	dir := filepath.Join(m.rootDir, name)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("%w: %s", ErrRepoNotFound, name)
	}
	// Refuse to remove a directory that isn't a marketplace clone. The
	// presence of .git/ is our signal — aligns with List()'s discovery.
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return fmt.Errorf("marketplace.Remove: %s is not a marketplace (no .git/): refusing to delete", name)
	}
	// Unlink any top-level symlinks pointing into this repo first.
	// Errors during cleanup are aggregated and returned BEFORE the repo
	// removal so callers can see partial failure.
	entries, err := os.ReadDir(m.rootDir)
	if err != nil {
		return fmt.Errorf("marketplace.Remove: read root: %w", err)
	}
	var cleanupErrs []error
	for _, e := range entries {
		p := filepath.Join(m.rootDir, e.Name())
		info, err := os.Lstat(p)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		target, err := os.Readlink(p)
		if err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("readlink %s: %w", p, err))
			continue
		}
		if !filepath.IsAbs(target) {
			target = filepath.Clean(filepath.Join(m.rootDir, target))
		}
		// Containment check via filepath.Rel to avoid HasPrefix edge cases.
		rel, err := filepath.Rel(dir, target)
		if err != nil || strings.HasPrefix(rel, "..") {
			continue
		}
		if err := os.Remove(p); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("unlink %s: %w", p, err))
		}
	}
	if len(cleanupErrs) > 0 {
		return fmt.Errorf("marketplace.Remove: %d cleanup error(s): %w", len(cleanupErrs), errors.Join(cleanupErrs...))
	}
	return os.RemoveAll(dir)
}

// LinkAll refreshes top-level symlinks so every discovered plugin is
// reachable from <rootDir>/<plugin-name>.
func (m *Manager) LinkAll() (*LinkReport, error) {
	if runtime.GOOS != "darwin" {
		return nil, ErrUnsupportedPlatform
	}
	report := &LinkReport{}
	desired := map[string]string{} // plugin-name → absolute target dir

	repos, err := m.List()
	if err != nil {
		return nil, err
	}
	for _, repo := range repos {
		for _, sub := range []string{"plugins", "external_plugins"} {
			dir := filepath.Join(repo.Path, sub)
			items, err := os.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, item := range items {
				if !item.IsDir() || strings.HasPrefix(item.Name(), ".") {
					continue
				}
				manifest := filepath.Join(dir, item.Name(), ".claude-plugin", "plugin.json")
				if _, err := os.Stat(manifest); err != nil {
					continue
				}
				// plugins/ wins over external_plugins/ on same-repo collision.
				if _, seen := desired[item.Name()]; seen && sub == "external_plugins" {
					continue
				}
				desired[item.Name()] = filepath.Join(dir, item.Name())
			}
		}
	}

	for name, target := range desired {
		topLevel := filepath.Join(m.rootDir, name)
		info, err := os.Lstat(topLevel)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				continue
			}
			// doesn't exist → create
			if err := os.Symlink(target, topLevel); err != nil {
				report.Conflicts = append(report.Conflicts, Conflict{Name: name, Reason: fmt.Sprintf("create: %s", err)})
				continue
			}
			report.Created = append(report.Created, name)
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			// real directory — don't clobber user-placed content
			report.Conflicts = append(report.Conflicts, Conflict{
				Name:   name,
				Reason: "top-level is a real directory, not overwriting",
			})
			continue
		}
		current, _ := os.Readlink(topLevel)
		if current == target {
			report.Unchanged = append(report.Unchanged, name)
			continue
		}
		if err := os.Remove(topLevel); err != nil {
			report.Conflicts = append(report.Conflicts, Conflict{Name: name, Reason: fmt.Sprintf("replace: %s", err)})
			continue
		}
		if err := os.Symlink(target, topLevel); err != nil {
			report.Conflicts = append(report.Conflicts, Conflict{Name: name, Reason: fmt.Sprintf("symlink: %s", err)})
			continue
		}
		report.Updated = append(report.Updated, name)
	}

	// Sort result slices for deterministic CLI output.
	sortStrings(report.Created, report.Updated, report.Unchanged, report.Removed)
	return report, nil
}

// --- helpers ---

func (m *Manager) repoInfo(name string) (*Repo, error) {
	dir := filepath.Join(m.rootDir, name)
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrRepoNotFound, name)
	}
	r, err := git.PlainOpen(dir)
	if err != nil {
		return nil, err
	}
	var url string
	if remote, err := r.Remote("origin"); err == nil {
		if urls := remote.Config().URLs; len(urls) > 0 {
			url = urls[0]
		}
	}
	var shortRef string
	if head, err := r.Head(); err == nil {
		shortRef = shortenHash(head.Hash())
	}
	repo := &Repo{
		Name:       name,
		URL:        url,
		Path:       dir,
		CurrentRef: shortRef,
	}

	// plugin discovery
	seen := map[string]bool{}
	for _, sub := range []string{"plugins", "external_plugins"} {
		items, err := os.ReadDir(filepath.Join(dir, sub))
		if err != nil {
			continue
		}
		for _, item := range items {
			if !item.IsDir() || strings.HasPrefix(item.Name(), ".") {
				continue
			}
			manifest := filepath.Join(dir, sub, item.Name(), ".claude-plugin", "plugin.json")
			if _, err := os.Stat(manifest); err != nil {
				continue
			}
			if !seen[item.Name()] {
				seen[item.Name()] = true
				repo.Plugins = append(repo.Plugins, item.Name())
			}
		}
	}
	sort.Strings(repo.Plugins)

	// LastPull from .git/FETCH_HEAD mtime (zero if missing)
	if info, err := os.Stat(filepath.Join(dir, ".git", "FETCH_HEAD")); err == nil {
		repo.LastPull = info.ModTime()
	}
	return repo, nil
}

func shortenHash(h plumbing.Hash) string {
	s := h.String()
	if len(s) >= 7 {
		return s[:7]
	}
	return s
}

func basenameFromURL(url string) string {
	url = strings.TrimSuffix(url, ".git")
	url = strings.TrimRight(url, "/\\")
	// Use the rightmost separator — git URLs use '/', but Windows file://
	// URLs can carry '\' from native path strings.
	if i := strings.LastIndexAny(url, "/\\"); i >= 0 {
		return url[i+1:]
	}
	return ""
}

func sortStrings(slices ...[]string) {
	for _, s := range slices {
		sort.Strings(s)
	}
}
