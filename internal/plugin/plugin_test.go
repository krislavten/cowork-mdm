package plugin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
)

// buildFakeOrgPlugins sets up an org-plugins dir with a representative mix:
//   - real-dir/           — real directory with manifest
//   - linked/             — symlink → mp/plugins/linked
//   - dangling/           — symlink with no target
//   - mp/                 — fake marketplace subtree
func buildFakeOrgPlugins(t *testing.T) (root string, marketplaceDir string) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skipf("symlink-heavy fixtures run only on darwin, got %s", runtime.GOOS)
	}
	root = t.TempDir()
	// Real directory
	if err := os.MkdirAll(filepath.Join(root, "real-dir", ".claude-plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "real-dir", ".claude-plugin", "plugin.json"),
		[]byte(`{"name":"real-dir","version":"1.0.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Marketplace subtree with a manifested plugin
	mpDir := filepath.Join(root, "mp")
	if err := os.MkdirAll(filepath.Join(mpDir, "plugins", "linked-plugin", ".claude-plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mpDir, "plugins", "linked-plugin", ".claude-plugin", "plugin.json"),
		[]byte(`{"name":"linked-plugin","version":"0.2.0","description":"linked one"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// Fake .git so List() would treat mp as a marketplace repo
	// (Inspector.List doesn't require it though — it treats mp as a real dir.)
	if err := os.MkdirAll(filepath.Join(mpDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Symlink pointing into marketplace
	if err := os.Symlink(
		filepath.Join(mpDir, "plugins", "linked-plugin"),
		filepath.Join(root, "linked"),
	); err != nil {
		t.Fatal(err)
	}
	// Dangling symlink
	if err := os.Symlink(
		filepath.Join(root, "does-not-exist"),
		filepath.Join(root, "dangling"),
	); err != nil {
		t.Fatal(err)
	}
	return root, mpDir
}

func TestList_ClassifiesEntries(t *testing.T) {
	root, _ := buildFakeOrgPlugins(t)
	ins := NewInspector(root, "")
	got, err := ins.List()
	if err != nil {
		t.Fatal(err)
	}
	// We expect 4 entries: dangling, linked, mp (real dir), real-dir
	byName := map[string]Plugin{}
	for _, p := range got {
		byName[p.Name] = p
	}
	if len(byName) != 4 {
		t.Errorf("got %d plugins, want 4: %+v", len(byName), got)
	}

	real := byName["real-dir"]
	if real.IsSymlink || real.Source != string(SourceLocalDirectory) {
		t.Errorf("real-dir classification wrong: %+v", real)
	}
	if real.Manifest == nil || real.Manifest.Version != "1.0.0" {
		t.Errorf("real-dir manifest missing: %+v", real.Manifest)
	}

	linked := byName["linked"]
	if !linked.IsSymlink || linked.Dangling {
		t.Errorf("linked classification wrong: %+v", linked)
	}
	if linked.Source != "marketplace:mp" {
		t.Errorf("linked Source = %q, want marketplace:mp", linked.Source)
	}
	if linked.Manifest == nil || linked.Manifest.Name != "linked-plugin" {
		t.Errorf("linked manifest = %+v", linked.Manifest)
	}

	dangling := byName["dangling"]
	if !dangling.IsSymlink || !dangling.Dangling {
		t.Errorf("dangling classification wrong: %+v", dangling)
	}

	mp := byName["mp"]
	if mp.Source != string(SourceLocalDirectory) {
		t.Errorf("mp Source = %q, want local-directory (Inspector treats mp as real dir by design)", mp.Source)
	}
}

func TestGet_UnknownReturnsErr(t *testing.T) {
	root, _ := buildFakeOrgPlugins(t)
	ins := NewInspector(root, "")
	if _, err := ins.Get("nope"); err != ErrPluginNotFound {
		t.Errorf("want ErrPluginNotFound, got %v", err)
	}
}

func TestPrune_RemovesOnlyDangling(t *testing.T) {
	root, _ := buildFakeOrgPlugins(t)
	mut := NewMutator(root)
	removed, err := mut.Prune()
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != "dangling" {
		t.Errorf("removed = %v, want [dangling]", removed)
	}
	// real-dir + linked + mp must still exist
	for _, name := range []string{"real-dir", "linked", "mp"} {
		if _, err := os.Lstat(filepath.Join(root, name)); err != nil {
			t.Errorf("%s was removed but shouldn't be: %v", name, err)
		}
	}
}

func TestUnlink_RemovesSymlinkOnly(t *testing.T) {
	root, _ := buildFakeOrgPlugins(t)
	mut := NewMutator(root)

	// linked → symlink, should be removed
	if err := mut.Unlink("linked"); err != nil {
		t.Fatalf("Unlink linked: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "linked")); err == nil {
		t.Error("linked should be gone after Unlink")
	}

	// real-dir → real dir, should error out
	if err := mut.Unlink("real-dir"); err == nil {
		t.Error("Unlink on real directory should error")
	}
	if _, err := os.Stat(filepath.Join(root, "real-dir")); err != nil {
		t.Error("real-dir content should survive")
	}

	// unknown → ErrPluginNotFound
	if err := mut.Unlink("nope"); err != ErrPluginNotFound {
		t.Errorf("unknown Unlink = %v, want ErrPluginNotFound", err)
	}
}

func TestEnabledStates_MergesSessions(t *testing.T) {
	root, _ := buildFakeOrgPlugins(t)
	// Build fake Claude-3p sessions dir with two sessions
	sessionsDir := t.TempDir()
	mkSession := func(uuid1, uuid2 string, enabled map[string]bool, installed []string) {
		dir := filepath.Join(sessionsDir, uuid1, uuid2)
		if err := os.MkdirAll(filepath.Join(dir, "cowork_plugins"), 0o755); err != nil {
			t.Fatal(err)
		}
		settings := map[string]any{"enabledPlugins": enabled}
		b, _ := json.Marshal(settings)
		if err := os.WriteFile(filepath.Join(dir, "cowork_settings.json"), b, 0o644); err != nil {
			t.Fatal(err)
		}
		plugins := map[string]any{}
		for _, name := range installed {
			plugins[name] = []any{}
		}
		b, _ = json.Marshal(map[string]any{"version": 2, "plugins": plugins})
		if err := os.WriteFile(filepath.Join(dir, "cowork_plugins", "installed_plugins.json"), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Session A: linked-plugin enabled
	mkSession("aaaa", "bbbb", map[string]bool{"linked@org-provisioned": true}, []string{"linked@org-provisioned"})
	// Session B: linked-plugin disabled
	mkSession("cccc", "dddd", map[string]bool{"linked@org-provisioned": false}, []string{"linked@org-provisioned"})

	ins := NewInspector(root, sessionsDir)
	states, err := ins.EnabledStates()
	if err != nil {
		t.Fatal(err)
	}
	byPlugin := map[string]EnabledState{}
	for _, s := range states {
		byPlugin[s.Plugin] = s
	}
	linked := byPlugin["linked"]
	if !linked.AnyDisabled {
		t.Error("linked should report AnyDisabled=true (session B has it off)")
	}
	if linked.AllEnabled {
		t.Error("linked should NOT report AllEnabled=true (split states)")
	}
	if len(linked.BySession) != 2 {
		t.Errorf("BySession count = %d, want 2", len(linked.BySession))
	}
}

// Make sorting deterministic in the enabled-states test, independent of
// filepath.Glob ordering.
func TestEnabledStates_SortOrder(t *testing.T) {
	root, _ := buildFakeOrgPlugins(t)
	sessionsDir := t.TempDir()
	// Zero sessions → empty result, non-nil.
	ins := NewInspector(root, sessionsDir)
	states, err := ins.EnabledStates()
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, len(states))
	for i, s := range states {
		names[i] = s.Plugin
	}
	want := append([]string(nil), names...)
	sort.Strings(want)
	for i := range names {
		if names[i] != want[i] {
			t.Errorf("EnabledStates not sorted: %v", names)
			break
		}
	}
}
