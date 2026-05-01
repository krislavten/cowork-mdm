package marketplace

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// setupFakeMarketplace creates a real git repo on disk that mimics the
// marketplace convention (plugins/foo/.claude-plugin/plugin.json). Returns
// the file:// URL suitable for passing to Add().
func setupFakeMarketplace(t *testing.T, name string) string {
	t.Helper()
	// Skip if git isn't on PATH — go-git clone needs a URL we can reach.
	// file:// clones work via go-git's pure-Go transport, no external git needed.
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(filepath.Join(dir, "plugins", "alpha", ".claude-plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "plugins", "beta", ".claude-plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "external_plugins", "gamma", ".claude-plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Plugin manifests (minimal valid)
	for _, name := range []string{"alpha", "beta"} {
		manifestPath := filepath.Join(dir, "plugins", name, ".claude-plugin", "plugin.json")
		if err := os.WriteFile(manifestPath, []byte(`{"name":"`+name+`","version":"0.0.1"}`), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "external_plugins", "gamma", ".claude-plugin", "plugin.json"),
		[]byte(`{"name":"gamma","version":"0.0.1"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// init + commit via shell git (tests require it; go-git's commit API is
	// more verbose than a quick subprocess)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available — integration test requires it to set up fixtures")
	}
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
			"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, string(out))
		}
	}
	runGit("init", "--initial-branch=main", "-q")
	runGit("add", ".")
	runGit("commit", "-m", "initial", "-q")

	return toFileURL(dir)
}

// toFileURL converts a native filesystem path into a file:// URL that
// go-git's transport layer accepts. On Windows, the native path uses
// backslashes and a drive letter, which net/url can't parse as-is —
// we need "file:///C:/Users/..." with forward slashes.
func toFileURL(dir string) string {
	if runtime.GOOS == "windows" {
		return "file:///" + strings.ReplaceAll(dir, "\\", "/")
	}
	return "file://" + dir
}

func TestAdd_ClonesRepoAndDiscoversPlugins(t *testing.T) {
	url := setupFakeMarketplace(t, "fake-mp")
	root := t.TempDir()
	m := NewManager(root)

	repo, err := m.Add(context.Background(), url, AddOptions{})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if repo.Name != "fake-mp" {
		t.Errorf("Name = %q, want fake-mp", repo.Name)
	}
	if len(repo.Plugins) != 3 {
		t.Errorf("expected 3 plugins, got %v", repo.Plugins)
	}
}

func TestAdd_RejectsExisting(t *testing.T) {
	url := setupFakeMarketplace(t, "fake-mp")
	root := t.TempDir()
	m := NewManager(root)
	if _, err := m.Add(context.Background(), url, AddOptions{}); err != nil {
		t.Fatal(err)
	}
	_, err := m.Add(context.Background(), url, AddOptions{})
	if err == nil {
		t.Error("second Add of same URL should fail")
	}
}

func TestAdd_CustomName(t *testing.T) {
	url := setupFakeMarketplace(t, "fake-mp")
	root := t.TempDir()
	m := NewManager(root)
	repo, err := m.Add(context.Background(), url, AddOptions{Name: "my-custom"})
	if err != nil {
		t.Fatal(err)
	}
	if repo.Name != "my-custom" {
		t.Errorf("Name = %q, want my-custom", repo.Name)
	}
	if _, err := os.Stat(filepath.Join(root, "my-custom", ".git")); err != nil {
		t.Errorf(".git missing at custom name: %v", err)
	}
}

func TestList_ReturnsClonedReposOnly(t *testing.T) {
	url := setupFakeMarketplace(t, "fake-mp")
	root := t.TempDir()
	m := NewManager(root)
	_, err := m.Add(context.Background(), url, AddOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// Create a non-git directory that must NOT show up as a repo.
	_ = os.MkdirAll(filepath.Join(root, "not-a-repo"), 0o755)
	repos, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0].Name != "fake-mp" {
		t.Errorf("List = %+v, want [fake-mp]", repos)
	}
}

func TestList_MissingRoot(t *testing.T) {
	m := NewManager(filepath.Join(t.TempDir(), "does-not-exist"))
	repos, err := m.List()
	if err != nil {
		t.Errorf("List on missing root should not error, got %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("List on missing root = %v, want empty", repos)
	}
}

func TestGet_UnknownRepoReturnsErr(t *testing.T) {
	m := NewManager(t.TempDir())
	_, err := m.Get("nope")
	if err == nil {
		t.Error("Get on unknown repo should error")
	}
}

func TestLinkAll_CreatesSymlinks(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skipf("LinkAll is darwin-only in v0.2, got %s", runtime.GOOS)
	}
	url := setupFakeMarketplace(t, "fake-mp")
	root := t.TempDir()
	m := NewManager(root)
	if _, err := m.Add(context.Background(), url, AddOptions{}); err != nil {
		t.Fatal(err)
	}
	report, err := m.LinkAll()
	if err != nil {
		t.Fatalf("LinkAll: %v", err)
	}
	if len(report.Created) != 3 {
		t.Errorf("Created = %v, want 3 entries", report.Created)
	}
	for _, name := range []string{"alpha", "beta", "gamma"} {
		link := filepath.Join(root, name)
		info, err := os.Lstat(link)
		if err != nil {
			t.Errorf("%s: Lstat failed: %v", name, err)
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Errorf("%s: not a symlink", name)
		}
	}
}

func TestLinkAll_PreservesRealDirectory(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skipf("LinkAll is darwin-only in v0.2, got %s", runtime.GOOS)
	}
	url := setupFakeMarketplace(t, "fake-mp")
	root := t.TempDir()
	m := NewManager(root)
	if _, err := m.Add(context.Background(), url, AddOptions{}); err != nil {
		t.Fatal(err)
	}
	// Create a real dir at the top level with same name as one of the plugins.
	if err := os.Mkdir(filepath.Join(root, "alpha"), 0o755); err != nil {
		t.Fatal(err)
	}
	sentinelPath := filepath.Join(root, "alpha", "sentinel.txt")
	if err := os.WriteFile(sentinelPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, err := m.LinkAll()
	if err != nil {
		t.Fatal(err)
	}
	var alphaConflict bool
	for _, c := range report.Conflicts {
		if c.Name == "alpha" {
			alphaConflict = true
		}
	}
	if !alphaConflict {
		t.Errorf("expected conflict for 'alpha' real directory, got report %+v", report)
	}
	// Sentinel must survive.
	if _, err := os.Stat(sentinelPath); err != nil {
		t.Errorf("real-dir content was clobbered: %v", err)
	}
}

func TestLinkAll_UpdatesMovedSymlink(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skipf("LinkAll is darwin-only in v0.2")
	}
	url := setupFakeMarketplace(t, "fake-mp")
	root := t.TempDir()
	m := NewManager(root)
	if _, err := m.Add(context.Background(), url, AddOptions{}); err != nil {
		t.Fatal(err)
	}
	// Plant an existing symlink pointing somewhere bogus.
	link := filepath.Join(root, "alpha")
	if err := os.Symlink("/nonexistent/old/alpha", link); err != nil {
		t.Fatal(err)
	}
	_, err := m.LinkAll()
	if err != nil {
		t.Fatal(err)
	}
	current, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	if current == "/nonexistent/old/alpha" {
		t.Errorf("LinkAll did not update stale symlink; still points at %q", current)
	}
}

func TestRemove_RemovesRepoAndSymlinks(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skipf("symlink test is darwin-only")
	}
	url := setupFakeMarketplace(t, "fake-mp")
	root := t.TempDir()
	m := NewManager(root)
	if _, err := m.Add(context.Background(), url, AddOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.LinkAll(); err != nil {
		t.Fatal(err)
	}
	// All three symlinks exist
	for _, name := range []string{"alpha", "beta", "gamma"} {
		if _, err := os.Lstat(filepath.Join(root, name)); err != nil {
			t.Fatalf("symlink %s missing pre-remove: %v", name, err)
		}
	}
	if err := m.Remove("fake-mp"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	// Now: clone gone + symlinks gone
	if _, err := os.Stat(filepath.Join(root, "fake-mp")); err == nil {
		t.Error("repo directory should be removed")
	}
	for _, name := range []string{"alpha", "beta", "gamma"} {
		if _, err := os.Lstat(filepath.Join(root, name)); err == nil {
			t.Errorf("symlink %s should have been cleaned up", name)
		}
	}
}

func TestBasenameFromURL(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"https://github.com/foo/bar", "bar"},
		{"https://github.com/foo/bar.git", "bar"},
		{"https://github.com/foo/bar/", "bar"},
		{"git@github.com:foo/bar.git", "bar"},
		{"/tmp/local/repo", "repo"},
	}
	for _, c := range cases {
		if got := basenameFromURL(c.in); got != c.want {
			t.Errorf("basenameFromURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
