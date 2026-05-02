// Package skillpack turns a directory of skills into a Claude Desktop
// plugin bundle suitable for delivery via org-plugins/.
//
// Claude Desktop only loads plugin bundles (directories with
// .claude-plugin/plugin.json) from org-plugins/. A bare skill directory
// is silently ignored. skillpack.Pack wraps a skills source tree in the
// minimal plugin shell so IT admins can ship skill libraries through the
// same MDM channel used for regular plugins.
package skillpack

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
)

// Options controls the bundle emitted by Pack. See specs/skillpack.md.
type Options struct {
	Name        string
	Version     string
	Description string
	AuthorName  string
	AuthorEmail string
	Force       bool
}

// Result summarizes what Pack produced.
type Result struct {
	BundleDir string
	Skills    []PackedSkill
}

// PackedSkill describes one skill included in the bundle.
type PackedSkill struct {
	Name            string
	FrontmatterName string
	Description     string
}

// Sentinel errors — see spec.
var (
	ErrInvalidName    = errors.New("skillpack: invalid bundle name")
	ErrMissingVersion = errors.New("skillpack: version is required")
	ErrInputNotFound  = errors.New("skillpack: input directory not found")
	ErrNoSkills       = errors.New("skillpack: no skills found in input")
	ErrInvalidSkill   = errors.New("skillpack: invalid skill")
	ErrOutDirNotEmpty = errors.New("skillpack: output directory not empty")
	ErrUnsafeOutDir   = errors.New("skillpack: unsafe output directory")
)

var bundleNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// defaultDescription is used when Options.Description is empty.
const defaultDescription = "Internal skills packaged by cowork-mdm skill pack"

// Pack reads inputDir, detects the layout, validates every skill, stages
// the bundle in a sibling temp directory, then atomically renames it into
// place. Pack never partially overwrites outDir — callers can retry after
// any error and be confident the previous bundle (if any) is intact or
// fully replaced.
func Pack(inputDir, outDir string, opts Options) (*Result, error) {
	if !bundleNameRE.MatchString(opts.Name) {
		return nil, fmt.Errorf("%w: %q (must match ^[a-z0-9][a-z0-9-]*$)", ErrInvalidName, opts.Name)
	}
	if strings.TrimSpace(opts.Version) == "" {
		return nil, ErrMissingVersion
	}

	inputDir = filepath.Clean(inputDir)
	outDir = filepath.Clean(outDir)

	info, err := os.Stat(inputDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrInputNotFound, inputDir)
		}
		return nil, fmt.Errorf("skillpack: stat input: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: %s is not a directory", ErrInputNotFound, inputDir)
	}

	absIn, err := resolveCanonical(inputDir)
	if err != nil {
		return nil, fmt.Errorf("skillpack: resolve input: %w", err)
	}
	// absOutLiteral: the literal outDir path, made absolute but NOT
	// symlink-resolved. This is what we Lstat, RemoveAll, and Rename to —
	// so we never write *through* a symlink that happens to sit at outDir.
	absOutLiteral, err := filepath.Abs(outDir)
	if err != nil {
		return nil, fmt.Errorf("skillpack: resolve output: %w", err)
	}
	// absOutResolved: same path with every symlink in the existing prefix
	// resolved, so the inside-input check catches cases where outDir (or
	// an ancestor) symlinks back into inputDir.
	absOutResolved, err := resolveOutCanonical(absOutLiteral)
	if err != nil {
		return nil, fmt.Errorf("skillpack: resolve output: %w", err)
	}
	if isSameOrInside(absOutResolved, absIn) {
		return nil, fmt.Errorf("%w: output %s resolves inside input %s", ErrUnsafeOutDir, absOutResolved, absIn)
	}

	skillsRoot, skills, err := detectSkills(inputDir)
	if err != nil {
		return nil, err
	}
	if len(skills) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrNoSkills, inputDir)
	}

	packed := make([]PackedSkill, 0, len(skills))
	for _, s := range skills {
		fm, err := parseSkillFrontmatter(filepath.Join(skillsRoot, s, "SKILL.md"))
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %v", ErrInvalidSkill, s, err)
		}
		packed = append(packed, PackedSkill{
			Name:            s,
			FrontmatterName: fm.name,
			Description:     fm.description,
		})
	}
	sort.Slice(packed, func(i, j int) bool { return packed[i].Name < packed[j].Name })

	// Make sure outDir's parent exists and the existing state is acceptable
	// BEFORE staging, so we fail fast without building a temp bundle we'd
	// have to delete. Policy runs against the literal path — we refuse to
	// treat a symlink at outDir as "our directory to overwrite." The
	// policy value is carried forward to commitBundle so a racer who
	// creates content at outDir mid-Pack cannot hijack commit branch
	// selection.
	policy, err := checkOutDirPolicy(absOutLiteral, opts.Force)
	if err != nil {
		return nil, err
	}

	// Stage the new bundle into a sibling temp dir of the literal outDir.
	// os.Rename between two entries in the same directory is atomic on the
	// same filesystem.
	parent := filepath.Dir(absOutLiteral)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, fmt.Errorf("skillpack: create output parent: %w", err)
	}
	staged, err := os.MkdirTemp(parent, ".skillpack-stage-")
	if err != nil {
		return nil, fmt.Errorf("skillpack: mkdir stage: %w", err)
	}
	// If anything below fails, scrub the staged tree so we never leave
	// half-written state behind.
	cleanupStaged := func() { _ = os.RemoveAll(staged) }

	if err := os.MkdirAll(filepath.Join(staged, ".claude-plugin"), 0o755); err != nil {
		cleanupStaged()
		return nil, fmt.Errorf("skillpack: create bundle dir: %w", err)
	}
	if err := writeManifest(filepath.Join(staged, ".claude-plugin", "plugin.json"), opts); err != nil {
		cleanupStaged()
		return nil, err
	}
	if err := writeReadme(filepath.Join(staged, "README.md"), opts, packed); err != nil {
		cleanupStaged()
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(staged, "skills"), 0o755); err != nil {
		cleanupStaged()
		return nil, fmt.Errorf("skillpack: create skills dir: %w", err)
	}
	for _, s := range packed {
		src := filepath.Join(skillsRoot, s.Name)
		dst := filepath.Join(staged, "skills", s.Name)
		if err := copySkillTree(src, dst); err != nil {
			cleanupStaged()
			return nil, fmt.Errorf("skillpack: copy %s: %w", s.Name, err)
		}
	}

	// Commit. The branch is decided by the policy captured at
	// validation time, not by a fresh Lstat at commit time, so a racer
	// that creates content at outDir mid-Pack cannot trick us into
	// treating their content as an existing bundle to sweep aside.
	if err := commitBundle(staged, absOutLiteral, policy); err != nil {
		cleanupStaged()
		return nil, err
	}

	return &Result{BundleDir: absOutLiteral, Skills: packed}, nil
}

// commitBundle moves the staged bundle into place atomically. The
// branch is driven by the policy captured at validation time, not by
// a fresh Lstat — that way a racer who creates content at dest while
// we stage cannot coerce us into a destructive path. None of the
// branches issue RemoveAll against dest.
//
//   - outDirAbsent: plain os.Rename. If a racer creates any content
//     at dest between validation and commit, Rename fails (EEXIST on
//     macOS, ENOTEMPTY on Linux) and Pack aborts; racer content
//     preserved.
//   - outDirEmpty: syscall.Rmdir(dest) followed by os.Rename. Rmdir
//     is directory-only on every supported platform: it fails with
//     ENOTDIR if dest was swapped for a regular file, and ENOTEMPTY
//     if dest was populated. os.Remove would happily delete a regular
//     file, which is why Rmdir is used here.
//   - outDirForceReplace: move existing dest to sidecar via os.Rename
//     (atomic), install staged at dest, then RemoveAll the sidecar.
//     Sidecar is at a path we constructed so RemoveAll only wipes
//     state we own.
func commitBundle(staged, dest string, policy outDirPolicy) error {
	switch policy {
	case outDirAbsent:
		if err := os.Rename(staged, dest); err != nil {
			return fmt.Errorf("skillpack: install bundle: %w", err)
		}
		return nil
	case outDirEmpty:
		// syscall.Rmdir is directory-only on all supported platforms:
		// fails with ENOTDIR if the path is now a regular file, and
		// ENOTEMPTY if the path is now a populated dir. Either way,
		// a racer that swapped dest between validation and commit
		// causes this to fail and Pack aborts. os.Remove would happily
		// delete a regular file, which is the hole Codex flagged.
		if err := syscall.Rmdir(dest); err != nil {
			return fmt.Errorf("skillpack: install bundle: %s changed during commit: %w",
				dest, err)
		}
		if err := os.Rename(staged, dest); err != nil {
			return fmt.Errorf("skillpack: install bundle: %w", err)
		}
		return nil
	case outDirForceReplace:
		parent := filepath.Dir(dest)
		base := filepath.Base(dest)
		sidecar, err := uniquePath(parent, ".skillpack-old-"+base+"-")
		if err != nil {
			return fmt.Errorf("skillpack: pick sidecar: %w", err)
		}
		if err := os.Rename(dest, sidecar); err != nil {
			return fmt.Errorf("skillpack: move existing bundle aside: %w", err)
		}
		if err := os.Rename(staged, dest); err != nil {
			if rbErr := os.Rename(sidecar, dest); rbErr != nil {
				return fmt.Errorf("skillpack: install bundle: %w; rollback also failed: %v", err, rbErr)
			}
			return fmt.Errorf("skillpack: install bundle: %w", err)
		}
		if err := os.RemoveAll(sidecar); err != nil {
			return fmt.Errorf("skillpack: bundle installed but failed to remove old copy at %s: %w", sidecar, err)
		}
		return nil
	default:
		return fmt.Errorf("skillpack: internal: unknown outDirPolicy %d", policy)
	}
}

// uniquePath returns a path in dir with the given prefix that is
// guaranteed not to collide with an existing entry. Uses MkdirTemp's
// atomicity guarantee, then removes the empty dir it created so the
// caller can use the path as a rename target.
func uniquePath(dir, prefix string) (string, error) {
	tmp, err := os.MkdirTemp(dir, prefix)
	if err != nil {
		return "", err
	}
	// Remove the placeholder so os.Rename can target the path.
	if err := os.Remove(tmp); err != nil {
		return "", err
	}
	return tmp, nil
}

// detectSkills resolves the skills root and returns the ordered list of
// skill directory names. See spec for layout rules.
func detectSkills(inputDir string) (root string, skills []string, err error) {
	rootSkills, err := listSkillDirs(inputDir)
	if err != nil {
		return "", nil, fmt.Errorf("skillpack: read input: %w", err)
	}
	if len(rootSkills) > 0 {
		return inputDir, rootSkills, nil
	}
	nested := filepath.Join(inputDir, "skills")
	if info, serr := os.Stat(nested); serr == nil && info.IsDir() {
		nestedSkills, err := listSkillDirs(nested)
		if err != nil {
			return "", nil, fmt.Errorf("skillpack: read skills/: %w", err)
		}
		if len(nestedSkills) > 0 {
			return nested, nestedSkills, nil
		}
	}
	return inputDir, nil, nil
}

// listSkillDirs returns subdirectory names of dir that contain a SKILL.md.
// Follows symlinks at the top level so a linked skill dir still works,
// but leaves deeper traversal to copySkillTree.
func listSkillDirs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.Name() == "skills" {
			// Never treat the nested skills/ dir itself as a skill.
			continue
		}
		// os.Stat follows symlinks so a symlinked skill directory is
		// accepted at the top level.
		info, err := os.Stat(filepath.Join(dir, e.Name()))
		if err != nil || !info.IsDir() {
			continue
		}
		skillFile := filepath.Join(dir, e.Name(), "SKILL.md")
		if info, err := os.Stat(skillFile); err == nil && !info.IsDir() {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

type skillFrontmatter struct {
	name        string
	description string
}

var frontmatterRE = regexp.MustCompile(`(?s)\A---\s*\n(.*?)\n---\s*\n`)

// parseSkillFrontmatter loads SKILL.md and extracts name + description.
// Only supports a tiny YAML subset (key: value on single lines). That's
// what Claude Desktop's own parser handles, so matching scope is correct.
func parseSkillFrontmatter(path string) (skillFrontmatter, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return skillFrontmatter{}, err
	}
	m := frontmatterRE.FindSubmatch(data)
	if m == nil {
		return skillFrontmatter{}, fmt.Errorf("SKILL.md missing YAML frontmatter")
	}
	fm := skillFrontmatter{}
	for _, line := range strings.Split(string(m[1]), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		switch key {
		case "name":
			fm.name = value
		case "description":
			fm.description = value
		}
	}
	if fm.name == "" {
		return fm, fmt.Errorf("SKILL.md frontmatter missing 'name'")
	}
	if fm.description == "" {
		return fm, fmt.Errorf("SKILL.md frontmatter missing 'description'")
	}
	return fm, nil
}

// resolveCanonical returns an absolute path with all symlinks resolved.
// Used for the input directory, which must already exist.
func resolveCanonical(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return resolved, nil
}

// resolveOutCanonical resolves as much of an absolute path as currently
// exists, then appends the still-missing tail. This catches an outDir
// whose eventual physical location would land inside inputDir, even when
// the outDir itself doesn't exist yet.
//
// A broken symlink along the path is treated as if the entry didn't
// exist — we walk past it and continue upward until EvalSymlinks
// succeeds, or we hit the filesystem root. This matches the "best
// effort, never fail the caller" contract the Pack guard needs.
func resolveOutCanonical(abs string) (string, error) {
	prefix := abs
	var tail []string
	for {
		// Walk up until we find an entry we can resolve. An entry that
		// exists (Lstat) but whose EvalSymlinks fails (broken symlink,
		// permission denied on the target) is treated as "not usable for
		// canonicalisation" — we append it to the tail and try the parent.
		if info, err := os.Lstat(prefix); err == nil {
			if info.Mode()&os.ModeSymlink == 0 {
				// Real file/dir — EvalSymlinks will succeed.
				break
			}
			// It's a symlink. Try to resolve; if it works, break.
			if _, err := filepath.EvalSymlinks(prefix); err == nil {
				break
			}
			// Broken symlink — treat as not present and fall through.
		}
		parent := filepath.Dir(prefix)
		if parent == prefix {
			// Hit filesystem root without finding a resolvable ancestor.
			return abs, nil
		}
		tail = append([]string{filepath.Base(prefix)}, tail...)
		prefix = parent
	}
	resolved, err := filepath.EvalSymlinks(prefix)
	if err != nil {
		return "", err
	}
	if len(tail) == 0 {
		return resolved, nil
	}
	return filepath.Join(append([]string{resolved}, tail...)...), nil
}

// isSameOrInside reports whether a is equal to b or lives under b.
// Both arguments must already be absolute and symlink-resolved.
func isSameOrInside(a, b string) bool {
	if a == b {
		return true
	}
	sep := string(filepath.Separator)
	return strings.HasPrefix(a+sep, b+sep)
}

// checkOutDirPolicy verifies outDir is either absent, empty-ish, or
// force is set. It never mutates the filesystem.
//
// Returns (policy, err). policy describes how commitBundle should
// handle the destination — see outDirPolicy docs. Threading this bit
// through the Pack body keeps a racer from hijacking commit behaviour
// after the initial check.
type outDirPolicy int

const (
	// outDirAbsent: dest did not exist at validation time. Commit uses
	// a plain os.Rename. If a racer creates any content at dest between
	// validation and commit, Rename fails (EEXIST on macOS, ENOTEMPTY
	// on Linux) and Pack aborts — racer content is preserved.
	outDirAbsent outDirPolicy = iota
	// outDirEmpty: dest existed as a truly empty directory (no visible
	// entries and no hidden entries). Commit does syscall.Rmdir(dest)
	// followed by os.Rename. Rmdir is directory-only: if a racer
	// swapped dest for a regular file or populated the dir, Rmdir
	// fails and Pack aborts without deleting their content.
	outDirEmpty
	// outDirForceReplace: dest existed with content and --force was
	// supplied (or is a symlink / non-directory file with --force, or
	// a dotfile-only dir with --force). Commit uses the sidecar path:
	// move existing aside, install staged, RemoveAll the sidecar.
	// RemoveAll targets a path Pack picks, not the user-facing dest.
	outDirForceReplace
)

func checkOutDirPolicy(outDir string, force bool) (outDirPolicy, error) {
	info, err := os.Lstat(outDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return outDirAbsent, nil
		}
		return 0, fmt.Errorf("skillpack: inspect output: %w", err)
	}
	// Refuse to treat a symlink at outDir as "our own directory to
	// overwrite" — we won't write through it. Force required to
	// explicitly replace.
	if info.Mode()&os.ModeSymlink != 0 {
		if !force {
			return 0, fmt.Errorf("%w: %s is a symlink (re-run with --force to replace)",
				ErrUnsafeOutDir, outDir)
		}
		return outDirForceReplace, nil
	}
	if !info.IsDir() {
		if !force {
			return 0, fmt.Errorf("%w: %s exists and is not a directory (re-run with --force to replace)",
				ErrUnsafeOutDir, outDir)
		}
		return outDirForceReplace, nil
	}
	entries, err := os.ReadDir(outDir)
	if err != nil {
		return 0, fmt.Errorf("skillpack: inspect output: %w", err)
	}
	nonHidden := 0
	hidden := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			hidden++
		} else {
			nonHidden++
		}
	}
	if nonHidden > 0 {
		if !force {
			return 0, fmt.Errorf("%w: %s", ErrOutDirNotEmpty, outDir)
		}
		return outDirForceReplace, nil
	}
	// Visible content absent. If hidden entries are present (e.g. a
	// stray .DS_Store), require --force: without it the sidecar path
	// would destroy a racer's visible content that appears between
	// validation and commit, and the plain-rename path can't overwrite
	// the non-empty dir on macOS. --force is the user's explicit
	// acknowledgement that this dir is theirs to replace.
	if hidden > 0 {
		if !force {
			return 0, fmt.Errorf("%w: %s (contains hidden entries; re-run with --force to replace)",
				ErrOutDirNotEmpty, outDir)
		}
		return outDirForceReplace, nil
	}
	return outDirEmpty, nil
}

// writeManifest emits plugin.json with stable key order and two-space
// indentation. We hand-render instead of using encoding/json to keep the
// output byte-identical across Go minor versions and to control key
// ordering.
func writeManifest(path string, opts Options) error {
	desc := opts.Description
	if desc == "" {
		desc = defaultDescription
	}
	var b bytes.Buffer
	b.WriteString("{\n")
	fmt.Fprintf(&b, "  %s: %s,\n", jsonKey("name"), jsonString(opts.Name))
	fmt.Fprintf(&b, "  %s: %s,\n", jsonKey("version"), jsonString(opts.Version))
	if opts.AuthorName != "" {
		fmt.Fprintf(&b, "  %s: %s,\n", jsonKey("description"), jsonString(desc))
		b.WriteString("  \"author\": {\n")
		fmt.Fprintf(&b, "    %s: %s", jsonKey("name"), jsonString(opts.AuthorName))
		if opts.AuthorEmail != "" {
			b.WriteString(",\n")
			fmt.Fprintf(&b, "    %s: %s\n", jsonKey("email"), jsonString(opts.AuthorEmail))
		} else {
			b.WriteString("\n")
		}
		b.WriteString("  }\n")
	} else {
		fmt.Fprintf(&b, "  %s: %s\n", jsonKey("description"), jsonString(desc))
	}
	b.WriteString("}\n")
	return os.WriteFile(path, b.Bytes(), 0o644)
}

func jsonKey(k string) string { return `"` + k + `"` }

// jsonString escapes per RFC 8259 for the subset of code points that can
// appear in plugin.json fields: Unicode scalars get written as UTF-8,
// control chars and the two mandatory escapes (" and \) are escaped.
// Non-BMP runes (>= 0x10000) are emitted directly as UTF-8 rather than
// surrogate pairs — JSON parsers accept both; UTF-8 keeps the output
// readable.
func jsonString(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case r == '"':
			b.WriteString(`\"`)
		case r == '\b':
			b.WriteString(`\b`)
		case r == '\f':
			b.WriteString(`\f`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func writeReadme(path string, opts Options, skills []PackedSkill) error {
	desc := opts.Description
	if desc == "" {
		desc = defaultDescription
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "# %s\n\n", opts.Name)
	fmt.Fprintf(&b, "%s\n\n", desc)
	b.WriteString("Generated by `cowork-mdm skill pack`. Do not edit by hand — regenerate\n")
	b.WriteString("when the source skills change.\n\n")
	b.WriteString("## Skills\n\n")
	for _, s := range skills {
		fmt.Fprintf(&b, "- **%s** — %s\n", s.Name, s.Description)
	}
	return os.WriteFile(path, b.Bytes(), 0o644)
}

// copySkillTree recursively copies src → dst, resolving symlinks to
// their targets so the emitted bundle has no external link dependencies.
// We don't use filepath.WalkDir because it refuses to recurse into
// symlinked directories, which would silently drop content.
func copySkillTree(src, dst string) error {
	return copySkillTreeGuarded(src, dst, nil)
}

// copySkillTreeGuarded is copySkillTree with an ancestor-chain cycle
// guard. ancestors is the stack of canonical directory paths currently
// open on the call stack. A cycle exists only when a descendant points
// back at one of those ancestors (not when two siblings happen to link
// to the same external directory — both of those should be copied).
func copySkillTreeGuarded(src, dst string, ancestors []string) error {
	info, err := os.Stat(src) // follows symlinks
	if err != nil {
		return err
	}
	if info.IsDir() {
		// Canonicalise for cycle detection. If EvalSymlinks fails, fall
		// back to the lexical path — we'd rather do one extra traversal
		// than bail with an ambiguous error.
		canonical := src
		if resolved, err := filepath.EvalSymlinks(src); err == nil {
			canonical = resolved
		}
		for _, a := range ancestors {
			if a == canonical {
				return fmt.Errorf("skillpack: symlink cycle detected at %s", src)
			}
		}
		ancestors = append(ancestors, canonical)
		if err := os.MkdirAll(dst, 0o755); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := copySkillTreeGuarded(
				filepath.Join(src, e.Name()),
				filepath.Join(dst, e.Name()),
				ancestors,
			); err != nil {
				return err
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("skillpack: unsupported file type at %s", src)
	}
	return copyFile(src, dst, info.Mode().Perm())
}

func copyFile(src, dst string, perm fs.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
