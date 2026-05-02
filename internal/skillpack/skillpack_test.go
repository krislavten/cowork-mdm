package skillpack

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSkill creates a skill directory at dir/name with the given frontmatter
// plus an optional extra file. Returns the skill directory path.
func writeSkill(t *testing.T, dir, name, fmName, fmDesc string, extras map[string]string) string {
	t.Helper()
	skillDir := filepath.Join(dir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	var body strings.Builder
	body.WriteString("---\n")
	if fmName != "" {
		body.WriteString("name: " + fmName + "\n")
	}
	if fmDesc != "" {
		body.WriteString("description: " + fmDesc + "\n")
	}
	body.WriteString("---\n\nBody.\n")
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	for path, content := range extras {
		full := filepath.Join(skillDir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return skillDir
}

func defaultOpts() Options {
	return Options{
		Name:        "test-skills",
		Version:     "0.1.0",
		Description: "unit test bundle",
	}
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("plugin.json is not valid JSON: %v\n%s", err, data)
	}
	return out
}

func TestPack_LayoutA(t *testing.T) {
	in := t.TempDir()
	writeSkill(t, in, "alpha", "alpha", "Alpha desc", nil)
	writeSkill(t, in, "bravo", "bravo", "Bravo desc", map[string]string{
		"references/note.md": "supporting material\n",
	})

	out := filepath.Join(t.TempDir(), "bundle")
	res, err := Pack(in, out, defaultOpts())
	if err != nil {
		t.Fatalf("Pack failed: %v", err)
	}
	if got, want := len(res.Skills), 2; got != want {
		t.Errorf("res.Skills count = %d, want %d", got, want)
	}
	if res.Skills[0].Name != "alpha" || res.Skills[1].Name != "bravo" {
		t.Errorf("skills not sorted: %+v", res.Skills)
	}
	// plugin.json
	m := readJSON(t, filepath.Join(out, ".claude-plugin", "plugin.json"))
	if m["name"] != "test-skills" {
		t.Errorf("plugin.json name = %v", m["name"])
	}
	if m["version"] != "0.1.0" {
		t.Errorf("plugin.json version = %v", m["version"])
	}
	if m["description"] != "unit test bundle" {
		t.Errorf("plugin.json description = %v", m["description"])
	}
	if _, ok := m["author"]; ok {
		t.Errorf("plugin.json should have no author block when AuthorName empty")
	}
	// README
	readme, err := os.ReadFile(filepath.Join(out, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# test-skills", "alpha", "bravo", "Alpha desc", "Bravo desc"} {
		if !strings.Contains(string(readme), want) {
			t.Errorf("README missing %q", want)
		}
	}
	// skills/ tree copied, including extras
	if _, err := os.Stat(filepath.Join(out, "skills", "bravo", "references", "note.md")); err != nil {
		t.Errorf("bravo reference file not copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "skills", "alpha", "SKILL.md")); err != nil {
		t.Errorf("alpha SKILL.md not copied: %v", err)
	}
}

func TestPack_LayoutB(t *testing.T) {
	in := t.TempDir()
	skillsRoot := filepath.Join(in, "skills")
	if err := os.MkdirAll(skillsRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, skillsRoot, "nested", "nested", "Nested desc", nil)

	out := filepath.Join(t.TempDir(), "bundle")
	res, err := Pack(in, out, defaultOpts())
	if err != nil {
		t.Fatalf("Pack failed: %v", err)
	}
	if len(res.Skills) != 1 || res.Skills[0].Name != "nested" {
		t.Errorf("expected [nested], got %+v", res.Skills)
	}
	if _, err := os.Stat(filepath.Join(out, "skills", "nested", "SKILL.md")); err != nil {
		t.Errorf("nested skill not copied: %v", err)
	}
}

func TestPack_IgnoresStraySkillsDirInLayoutA(t *testing.T) {
	in := t.TempDir()
	writeSkill(t, in, "root-skill", "root-skill", "desc", nil)
	// A stray skills/ sibling — must be ignored because Layout A already matched.
	if err := os.MkdirAll(filepath.Join(in, "skills", "hidden-skill"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(in, "skills", "hidden-skill", "SKILL.md"),
		[]byte("---\nname: hidden\ndescription: x\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(t.TempDir(), "bundle")
	res, err := Pack(in, out, defaultOpts())
	if err != nil {
		t.Fatalf("Pack failed: %v", err)
	}
	if len(res.Skills) != 1 || res.Skills[0].Name != "root-skill" {
		t.Errorf("expected [root-skill] only; Layout A should win. got %+v", res.Skills)
	}
}

func TestPack_RejectsInvalidName(t *testing.T) {
	in := t.TempDir()
	writeSkill(t, in, "any", "any", "any", nil)
	for _, bad := range []string{"", "Foo", "-foo", "foo/bar", "foo_bar", "FOO"} {
		opts := defaultOpts()
		opts.Name = bad
		out := filepath.Join(t.TempDir(), "bundle")
		_, err := Pack(in, out, opts)
		if !errors.Is(err, ErrInvalidName) {
			t.Errorf("name %q: want ErrInvalidName, got %v", bad, err)
		}
	}
}

func TestPack_RejectsMissingVersion(t *testing.T) {
	in := t.TempDir()
	writeSkill(t, in, "any", "any", "any", nil)
	opts := defaultOpts()
	opts.Version = ""
	out := filepath.Join(t.TempDir(), "bundle")
	if _, err := Pack(in, out, opts); !errors.Is(err, ErrMissingVersion) {
		t.Errorf("empty version: want ErrMissingVersion, got %v", err)
	}
}

func TestPack_RejectsMissingFrontmatter(t *testing.T) {
	in := t.TempDir()
	skillDir := filepath.Join(in, "bad")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("no frontmatter here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "bundle")
	_, err := Pack(in, out, defaultOpts())
	if !errors.Is(err, ErrInvalidSkill) {
		t.Errorf("missing frontmatter: want ErrInvalidSkill, got %v", err)
	}
}

func TestPack_RejectsMissingRequiredFields(t *testing.T) {
	cases := []struct {
		label  string
		fmName string
		fmDesc string
	}{
		{"no-name", "", "desc"},
		{"no-desc", "name", ""},
		{"neither", "", ""},
	}
	for _, c := range cases {
		t.Run(c.label, func(t *testing.T) {
			in := t.TempDir()
			writeSkill(t, in, "s", c.fmName, c.fmDesc, nil)
			out := filepath.Join(t.TempDir(), "bundle")
			_, err := Pack(in, out, defaultOpts())
			if !errors.Is(err, ErrInvalidSkill) {
				t.Errorf("want ErrInvalidSkill, got %v", err)
			}
		})
	}
}

func TestPack_RejectsNoSkills(t *testing.T) {
	in := t.TempDir()
	out := filepath.Join(t.TempDir(), "bundle")
	if _, err := Pack(in, out, defaultOpts()); !errors.Is(err, ErrNoSkills) {
		t.Errorf("empty input: want ErrNoSkills, got %v", err)
	}
}

func TestPack_RejectsMissingInput(t *testing.T) {
	out := filepath.Join(t.TempDir(), "bundle")
	_, err := Pack(filepath.Join(t.TempDir(), "nope"), out, defaultOpts())
	if !errors.Is(err, ErrInputNotFound) {
		t.Errorf("missing input: want ErrInputNotFound, got %v", err)
	}
}

func TestPack_RejectsNonEmptyOutDirWithoutForce(t *testing.T) {
	in := t.TempDir()
	writeSkill(t, in, "s", "s", "s", nil)
	out := filepath.Join(t.TempDir(), "bundle")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "pre-existing.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Pack(in, out, defaultOpts())
	if !errors.Is(err, ErrOutDirNotEmpty) {
		t.Errorf("want ErrOutDirNotEmpty, got %v", err)
	}
	// With Force: should succeed and replace contents.
	opts := defaultOpts()
	opts.Force = true
	if _, err := Pack(in, out, opts); err != nil {
		t.Fatalf("Force Pack failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "pre-existing.txt")); err == nil {
		t.Errorf("pre-existing.txt should have been removed under Force")
	}
}

// TestPack_AcceptsEmptyExistingOutDir verifies that an outDir directory
// that exists but is empty is treated like absent: Pack succeeds without
// --force and the bundle lands correctly.
func TestPack_AcceptsEmptyExistingOutDir(t *testing.T) {
	in := t.TempDir()
	writeSkill(t, in, "s", "s", "s", nil)
	out := filepath.Join(t.TempDir(), "bundle")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Pack(in, out, defaultOpts()); err != nil {
		t.Fatalf("empty existing outDir should be accepted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, ".claude-plugin", "plugin.json")); err != nil {
		t.Errorf("plugin.json missing after commit: %v", err)
	}
}

// TestPack_HiddenFilesInOutDirRequireForce documents the tightened
// safety policy: even a dotfile-only outDir requires --force because
// committing otherwise would expose a TOCTOU race where a concurrent
// write of a visible file between validation and commit could be
// destroyed by sidecar cleanup.
func TestPack_HiddenFilesInOutDirRequireForce(t *testing.T) {
	in := t.TempDir()
	writeSkill(t, in, "s", "s", "s", nil)
	out := filepath.Join(t.TempDir(), "bundle")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, ".DS_Store"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Pack(in, out, defaultOpts()); !errors.Is(err, ErrOutDirNotEmpty) {
		t.Errorf("hidden-only outDir: want ErrOutDirNotEmpty, got %v", err)
	}
	opts := defaultOpts()
	opts.Force = true
	if _, err := Pack(in, out, opts); err != nil {
		t.Errorf("hidden-only outDir with --force should succeed: %v", err)
	}
}

func TestPack_ReproducibleBytes(t *testing.T) {
	in := t.TempDir()
	writeSkill(t, in, "z", "z", "Z desc", nil)
	writeSkill(t, in, "a", "a", "A desc", nil)

	outA := filepath.Join(t.TempDir(), "a")
	outB := filepath.Join(t.TempDir(), "b")
	if _, err := Pack(in, outA, defaultOpts()); err != nil {
		t.Fatal(err)
	}
	if _, err := Pack(in, outB, defaultOpts()); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{".claude-plugin/plugin.json", "README.md"} {
		aBytes, _ := os.ReadFile(filepath.Join(outA, rel))
		bBytes, _ := os.ReadFile(filepath.Join(outB, rel))
		if string(aBytes) != string(bBytes) {
			t.Errorf("%s not reproducible:\n--- A ---\n%s\n--- B ---\n%s", rel, aBytes, bBytes)
		}
	}
}

func TestPack_AuthorBlock(t *testing.T) {
	in := t.TempDir()
	writeSkill(t, in, "s", "s", "s", nil)
	opts := defaultOpts()
	opts.AuthorName = "Your Org"
	opts.AuthorEmail = "it@example.com"
	out := filepath.Join(t.TempDir(), "bundle")
	if _, err := Pack(in, out, opts); err != nil {
		t.Fatal(err)
	}
	m := readJSON(t, filepath.Join(out, ".claude-plugin", "plugin.json"))
	author, ok := m["author"].(map[string]any)
	if !ok {
		t.Fatalf("plugin.json author block missing or wrong type: %v", m["author"])
	}
	if author["name"] != "Your Org" || author["email"] != "it@example.com" {
		t.Errorf("author block = %+v", author)
	}
}

func TestPack_AuthorEmailOmittedWhenBlank(t *testing.T) {
	in := t.TempDir()
	writeSkill(t, in, "s", "s", "s", nil)
	opts := defaultOpts()
	opts.AuthorName = "Your Org"
	out := filepath.Join(t.TempDir(), "bundle")
	if _, err := Pack(in, out, opts); err != nil {
		t.Fatal(err)
	}
	m := readJSON(t, filepath.Join(out, ".claude-plugin", "plugin.json"))
	author := m["author"].(map[string]any)
	if _, has := author["email"]; has {
		t.Errorf("email key should be absent when empty; got author=%+v", author)
	}
}

func TestPack_DefaultDescription(t *testing.T) {
	in := t.TempDir()
	writeSkill(t, in, "s", "s", "s", nil)
	opts := defaultOpts()
	opts.Description = ""
	out := filepath.Join(t.TempDir(), "bundle")
	if _, err := Pack(in, out, opts); err != nil {
		t.Fatal(err)
	}
	m := readJSON(t, filepath.Join(out, ".claude-plugin", "plugin.json"))
	if !strings.Contains(m["description"].(string), "Internal skills") {
		t.Errorf("default description missing: %v", m["description"])
	}
}

func TestPack_RejectsOutInsideInput(t *testing.T) {
	in := t.TempDir()
	writeSkill(t, in, "s", "s", "s", nil)
	out := filepath.Join(in, "nested-bundle")
	_, err := Pack(in, out, defaultOpts())
	if !errors.Is(err, ErrUnsafeOutDir) {
		t.Errorf("lexically nested outDir: want ErrUnsafeOutDir, got %v", err)
	}
}

// TestPack_RejectsOutResolvingIntoInputViaSymlink covers the case where
// the literal outDir path lies elsewhere but resolves via a symlink
// inside inputDir — this was MUST-FIX #2 from sparring.
func TestPack_RejectsOutResolvingIntoInputViaSymlink(t *testing.T) {
	in := t.TempDir()
	writeSkill(t, in, "s", "s", "s", nil)

	aliasParent := t.TempDir()
	alias := filepath.Join(aliasParent, "alias")
	if err := os.Symlink(in, alias); err != nil {
		t.Fatalf("make alias symlink: %v", err)
	}
	// out's parent resolves through the alias back into inputDir.
	out := filepath.Join(alias, "resolved-bundle")
	_, err := Pack(in, out, defaultOpts())
	if !errors.Is(err, ErrUnsafeOutDir) {
		t.Errorf("symlinked outDir: want ErrUnsafeOutDir, got %v", err)
	}
}

// TestPack_RefusesPreexistingSymlinkOutDir covers MUST-FIX #1: a symlink
// at outDir without --force must be rejected, not written through.
func TestPack_RefusesPreexistingSymlinkOutDir(t *testing.T) {
	in := t.TempDir()
	writeSkill(t, in, "s", "s", "s", nil)

	realTarget := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.MkdirAll(realTarget, 0o755); err != nil {
		t.Fatal(err)
	}
	outParent := t.TempDir()
	out := filepath.Join(outParent, "bundle")
	if err := os.Symlink(realTarget, out); err != nil {
		t.Fatalf("seed symlink outDir: %v", err)
	}

	_, err := Pack(in, out, defaultOpts())
	if !errors.Is(err, ErrUnsafeOutDir) {
		t.Errorf("symlink outDir without --force: want ErrUnsafeOutDir, got %v", err)
	}
	// The external target must remain untouched.
	entries, _ := os.ReadDir(realTarget)
	if len(entries) != 0 {
		t.Errorf("Pack leaked files into the symlink target: %+v", entries)
	}
}

// TestPack_ForceReplacesSymlinkOutDir pairs with the previous test —
// with --force we replace the symlink itself, and never write through it.
func TestPack_ForceReplacesSymlinkOutDir(t *testing.T) {
	in := t.TempDir()
	writeSkill(t, in, "s", "s", "s", nil)

	realTarget := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.MkdirAll(realTarget, 0o755); err != nil {
		t.Fatal(err)
	}
	outParent := t.TempDir()
	out := filepath.Join(outParent, "bundle")
	if err := os.Symlink(realTarget, out); err != nil {
		t.Fatalf("seed symlink outDir: %v", err)
	}

	opts := defaultOpts()
	opts.Force = true
	if _, err := Pack(in, out, opts); err != nil {
		t.Fatalf("Pack with Force on symlink outDir: %v", err)
	}

	info, err := os.Lstat(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("after Force, outDir should be a real directory, not a symlink")
	}
	entries, _ := os.ReadDir(realTarget)
	if len(entries) != 0 {
		t.Errorf("Pack leaked files into the symlink target even with Force: %+v", entries)
	}
}

// TestPack_AtomicOnSkillCopyFailure covers MUST-FIX #3: if a skill copy
// fails mid-way, no bundle files should remain at outDir.
func TestPack_AtomicOnSkillCopyFailure(t *testing.T) {
	in := t.TempDir()
	writeSkill(t, in, "good", "good", "good", nil)
	// "bad" skill has a dangling symlink inside, which makes copySkillTree
	// fail when it tries to stat the target.
	writeSkill(t, in, "bad", "bad", "bad", nil)
	if err := os.Symlink(
		filepath.Join(t.TempDir(), "does-not-exist"),
		filepath.Join(in, "bad", "broken.md"),
	); err != nil {
		t.Fatal(err)
	}

	outParent := t.TempDir()
	out := filepath.Join(outParent, "bundle")
	if _, err := Pack(in, out, defaultOpts()); err == nil {
		t.Fatal("expected Pack to fail on dangling symlink in skill")
	}

	if _, err := os.Lstat(out); err == nil {
		t.Errorf("bundle at %s should not exist after failed Pack", out)
	}
	// The parent directory should also be clean of stage temp dirs.
	entries, _ := os.ReadDir(outParent)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".skillpack-stage-") {
			t.Errorf("staged temp dir leaked into parent: %s", e.Name())
		}
	}
}

// TestPack_ForceHandlesBrokenSymlinkOutDir covers the round-2 MUST-FIX:
// a dangling symlink at outDir used to make resolveOutCanonical fail
// before the Force-aware policy could run. Now a broken symlink is
// treated as non-canonicalizable, and Force replaces it.
func TestPack_ForceHandlesBrokenSymlinkOutDir(t *testing.T) {
	in := t.TempDir()
	writeSkill(t, in, "s", "s", "s", nil)

	outParent := t.TempDir()
	out := filepath.Join(outParent, "bundle")
	// Broken symlink — target does not exist.
	if err := os.Symlink(filepath.Join(t.TempDir(), "nowhere"), out); err != nil {
		t.Fatal(err)
	}

	opts := defaultOpts()
	opts.Force = true
	if _, err := Pack(in, out, opts); err != nil {
		t.Fatalf("Pack with Force on broken-symlink outDir: %v", err)
	}
	info, err := os.Lstat(out)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("after Force, outDir should be a real directory, not a symlink")
	}
}

// TestPack_RefusesBrokenSymlinkOutDirWithoutForce: the mirror case. A
// broken symlink at outDir without --force must error via policy, not
// via a canonicalization failure.
func TestPack_RefusesBrokenSymlinkOutDirWithoutForce(t *testing.T) {
	in := t.TempDir()
	writeSkill(t, in, "s", "s", "s", nil)
	outParent := t.TempDir()
	out := filepath.Join(outParent, "bundle")
	if err := os.Symlink(filepath.Join(t.TempDir(), "nowhere"), out); err != nil {
		t.Fatal(err)
	}
	_, err := Pack(in, out, defaultOpts())
	if !errors.Is(err, ErrUnsafeOutDir) {
		t.Errorf("broken-symlink outDir without Force: want ErrUnsafeOutDir, got %v", err)
	}
}

// TestPack_DetectsSymlinkCycleInSkillTree covers the round-2 SHOULD-FIX:
// a symlink loop inside a skill must not cause unbounded recursion.
func TestPack_DetectsSymlinkCycleInSkillTree(t *testing.T) {
	in := t.TempDir()
	writeSkill(t, in, "s", "s", "s", nil)
	// Cycle: s/docs -> . (points back at the skill directory itself).
	if err := os.Symlink(filepath.Join(in, "s"), filepath.Join(in, "s", "docs")); err != nil {
		t.Fatal(err)
	}

	outParent := t.TempDir()
	out := filepath.Join(outParent, "bundle")
	_, err := Pack(in, out, defaultOpts())
	if err == nil {
		t.Error("Pack should fail on symlink cycle")
	} else if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error should mention cycle: %v", err)
	}
	// And no partial bundle should remain.
	if _, statErr := os.Lstat(out); statErr == nil {
		t.Errorf("outDir should not exist after cycle failure")
	}
}

// TestCommitBundle_AbsentPathAbortsOnRacedContent unit-tests the commit
// primitive directly, proving the round-5 TOCTOU property: when policy
// captured outDirAbsent at validation, commitBundle uses plain Rename,
// which fails if a racer populated dest before the commit — racer
// content is preserved byte-for-byte.
func TestCommitBundle_AbsentPathAbortsOnRacedContent(t *testing.T) {
	parent := t.TempDir()
	dest := filepath.Join(parent, "bundle")

	// Stage a faux-bundle dir.
	staged, err := os.MkdirTemp(parent, "stage-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staged, "marker"), []byte("staged"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Racer: populated dest with content AFTER the "validation" moment
	// (we simulate by creating it here). Validation would have seen
	// absent; commit runs with outDirAbsent policy.
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "raced.txt"), []byte("racer"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := commitBundle(staged, dest, outDirAbsent); err == nil {
		t.Fatal("commitBundle should fail when racer populated dest")
	}
	// Racer content must still be intact.
	got, err := os.ReadFile(filepath.Join(dest, "raced.txt"))
	if err != nil {
		t.Fatalf("racer content lost: %v", err)
	}
	if string(got) != "racer" {
		t.Errorf("racer content = %q", got)
	}
	// And the staged dir should still exist (Pack's caller is
	// responsible for cleanup; commitBundle doesn't cleanup on failure).
	if _, err := os.Stat(filepath.Join(staged, "marker")); err != nil {
		t.Errorf("staged cleanup should be caller's job: %v", err)
	}
}

// TestCommitBundle_EmptyPathAbortsOnRacedContent is the same property
// for the outDirEmpty branch (existing-empty-dir). A racer populates
// dest between validation and commit; os.Remove refuses to delete a
// non-empty dir, commitBundle aborts, racer content preserved.
func TestCommitBundle_EmptyPathAbortsOnRacedContent(t *testing.T) {
	parent := t.TempDir()
	dest := filepath.Join(parent, "bundle")

	// Validation's view: dest exists and is empty.
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	// Racer populates dest after validation.
	if err := os.WriteFile(filepath.Join(dest, "raced.txt"), []byte("racer"), 0o644); err != nil {
		t.Fatal(err)
	}

	staged, err := os.MkdirTemp(parent, "stage-")
	if err != nil {
		t.Fatal(err)
	}

	if err := commitBundle(staged, dest, outDirEmpty); err == nil {
		t.Fatal("commitBundle(outDirEmpty) should fail when racer populated dest")
	}
	got, err := os.ReadFile(filepath.Join(dest, "raced.txt"))
	if err != nil {
		t.Fatalf("racer content lost: %v", err)
	}
	if string(got) != "racer" {
		t.Errorf("racer content = %q", got)
	}
}

// TestCommitBundle_EmptyPathAbortsWhenDirReplacedByFile covers the
// round-6 MUST-FIX: a racer replaces the empty dir with a regular
// file between validation and commit. os.Remove would happily delete
// the file; syscall.Rmdir refuses (ENOTDIR), so Pack aborts and the
// racer's file survives.
func TestCommitBundle_EmptyPathAbortsWhenDirReplacedByFile(t *testing.T) {
	parent := t.TempDir()
	dest := filepath.Join(parent, "bundle")

	// Validation's view: dest existed as empty dir. Now racer
	// atomically replaces with a regular file.
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dest); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("racer-file"), 0o644); err != nil {
		t.Fatal(err)
	}

	staged, err := os.MkdirTemp(parent, "stage-")
	if err != nil {
		t.Fatal(err)
	}

	if err := commitBundle(staged, dest, outDirEmpty); err == nil {
		t.Fatal("commitBundle(outDirEmpty) must abort when dest was replaced by a file")
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("racer file lost: %v", err)
	}
	if string(got) != "racer-file" {
		t.Errorf("racer file = %q", got)
	}
}

// TestPack_RejectsPopulatedOutDirWithoutForce covers the initial
// validation layer: if outDir is already populated (visible files)
// without --force, Pack rejects via ErrOutDirNotEmpty and the
// content survives unchanged.
func TestPack_RejectsPopulatedOutDirWithoutForce(t *testing.T) {
	in := t.TempDir()
	writeSkill(t, in, "s", "s", "s", nil)
	outParent := t.TempDir()
	out := filepath.Join(outParent, "bundle")
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "raced.txt"), []byte("racer"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Pack(in, out, defaultOpts()); !errors.Is(err, ErrOutDirNotEmpty) {
		t.Errorf("populated outDir: want ErrOutDirNotEmpty, got %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(out, "raced.txt"))
	if string(got) != "racer" {
		t.Errorf("racer content = %q", got)
	}
}

// TestPack_AllowsSiblingSymlinksToSameTarget ensures the cycle guard
// doesn't false-positive on two symlinks pointing at the same external
// directory (an ancestor-chain, not a global-visited, design).
func TestPack_AllowsSiblingSymlinksToSameTarget(t *testing.T) {
	in := t.TempDir()
	writeSkill(t, in, "s", "s", "s", nil)

	shared := filepath.Join(t.TempDir(), "shared")
	if err := os.MkdirAll(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(shared, "x.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(shared, filepath.Join(in, "s", "docs-a")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(shared, filepath.Join(in, "s", "docs-b")); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(t.TempDir(), "bundle")
	if _, err := Pack(in, out, defaultOpts()); err != nil {
		t.Fatalf("Pack should accept sibling symlinks to the same target: %v", err)
	}
	// Both copies should be present and populated.
	for _, name := range []string{"docs-a", "docs-b"} {
		got, err := os.ReadFile(filepath.Join(out, "skills", "s", name, "x.md"))
		if err != nil {
			t.Errorf("skills/s/%s/x.md missing: %v", name, err)
		} else if string(got) != "hi" {
			t.Errorf("skills/s/%s/x.md = %q", name, got)
		}
	}
}

// TestPack_CopiesSymlinkedDirectoryContent covers MUST-FIX #4: symlinked
// subdirectories inside a skill must have their full contents copied,
// not be silently flattened to empty directories.
func TestPack_CopiesSymlinkedDirectoryContent(t *testing.T) {
	in := t.TempDir()
	writeSkill(t, in, "s", "s", "s", nil)

	external := filepath.Join(t.TempDir(), "ext-docs")
	if err := os.MkdirAll(external, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(external, "a.md"), []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(external, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(external, "nested", "b.md"), []byte("B"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(in, "s", "docs")); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(t.TempDir(), "bundle")
	if _, err := Pack(in, out, defaultOpts()); err != nil {
		t.Fatalf("Pack: %v", err)
	}

	// Both files, at the right depths.
	if got, err := os.ReadFile(filepath.Join(out, "skills", "s", "docs", "a.md")); err != nil {
		t.Errorf("docs/a.md missing: %v", err)
	} else if string(got) != "A" {
		t.Errorf("docs/a.md content = %q", got)
	}
	if got, err := os.ReadFile(filepath.Join(out, "skills", "s", "docs", "nested", "b.md")); err != nil {
		t.Errorf("docs/nested/b.md missing: %v", err)
	} else if string(got) != "B" {
		t.Errorf("docs/nested/b.md content = %q", got)
	}
	// And none of it should be symlinks.
	info, err := os.Lstat(filepath.Join(out, "skills", "s", "docs"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("bundle docs/ should be a real directory, not a symlink")
	}
}

// TestJSONString_EscapeTable covers SHOULD-FIX #2: JSON correctness is a
// stated review priority, so exercise the escape logic directly.
func TestJSONString_EscapeTable(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{``, `""`},
		{`plain`, `"plain"`},
		{`quote "x"`, `"quote \"x\""`},
		{`back\slash`, `"back\\slash"`},
		{"line\nbreak", `"line\nbreak"`},
		{"cr\rlf", `"cr\rlf"`},
		{"tab\there", `"tab\there"`},
		{"bell\x07", `"bell\u0007"`},
		{"bs\bws", `"bs\bws"`},
		{"ff\f!", `"ff\f!"`},
		{"unit\x1fsep", `"unit\u001fsep"`},
		{"UTF-8 中文 — ok", `"UTF-8 中文 — ok"`},
		{"emoji 😀", `"emoji 😀"`},
	}
	for _, c := range cases {
		if got := jsonString(c.in); got != c.want {
			t.Errorf("jsonString(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestPack_ResultBundleDirIsAbsolute covers SHOULD-FIX #1: spec says
// Result.BundleDir must be absolute.
func TestPack_ResultBundleDirIsAbsolute(t *testing.T) {
	in := t.TempDir()
	writeSkill(t, in, "s", "s", "s", nil)

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWD) })
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	relOut := "bundle"
	res, err := Pack(in, relOut, defaultOpts())
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(res.BundleDir) {
		t.Errorf("Result.BundleDir = %q, want absolute path", res.BundleDir)
	}
}

func TestPack_FollowsSymlinkedFiles(t *testing.T) {
	in := t.TempDir()
	external := t.TempDir()
	if err := os.WriteFile(filepath.Join(external, "ref.md"), []byte("external ref"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, in, "s", "s", "s", nil)
	if err := os.Symlink(filepath.Join(external, "ref.md"), filepath.Join(in, "s", "ref.md")); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "bundle")
	if _, err := Pack(in, out, defaultOpts()); err != nil {
		t.Fatalf("Pack with symlink failed: %v", err)
	}
	info, err := os.Lstat(filepath.Join(out, "skills", "s", "ref.md"))
	if err != nil {
		t.Fatalf("ref.md not in bundle: %v", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Error("bundle should contain a regular file, not a symlink")
	}
	data, _ := os.ReadFile(filepath.Join(out, "skills", "s", "ref.md"))
	if string(data) != "external ref" {
		t.Errorf("ref.md content = %q", data)
	}
}
