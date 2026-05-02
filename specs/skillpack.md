# Spec: `internal/skillpack/`

## Intent

Package a local collection of skills into a Claude Desktop plugin bundle,
so an IT admin can drop the output under `org-plugins/` (via MDM) and have
Cowork load every skill at launch.

Context: Cowork only mounts **plugin bundles** under `org-plugins/` — the
scanner reads each subdirectory's `.claude-plugin/plugin.json` and skips
anything without a manifest. A bare `skills/foo/SKILL.md` in the mount
folder is silently ignored (verified experimentally against Claude.app
`1.5354.0`). To deliver skills via MDM, they must be wrapped in a minimal
plugin bundle. `skill pack` automates that wrapping.

## Public interface

```go
package skillpack

// Options controls the bundle emitted by Pack.
type Options struct {
    // Name is the plugin bundle name (required).
    // Must match ^[a-z0-9][a-z0-9-]*$ — Anthropic's org plugin naming rule.
    Name string

    // Version is the bundle version (required; default "0.1.0" set by caller).
    // Free-form string, written verbatim into plugin.json.
    Version string

    // Description goes into plugin.json. Optional. If empty, Pack writes a
    // generic placeholder ("Internal skills packaged by cowork-mdm skill pack").
    Description string

    // AuthorName and AuthorEmail populate plugin.json's author block.
    // Both optional. If AuthorName is empty the author block is omitted.
    AuthorName  string
    AuthorEmail string

    // Force allows OutDir to already exist. When true, OutDir is wiped and
    // recreated. When false (default), Pack returns ErrOutDirNotEmpty if
    // OutDir exists and is not empty.
    Force bool
}

// Result summarizes what Pack produced.
type Result struct {
    // BundleDir is the absolute path of the generated bundle (equal to
    // OutDir passed to Pack, cleaned).
    BundleDir string

    // Skills lists the skills that were packaged, ordered alphabetically
    // by skill name. Each entry has the parsed name/description from the
    // SKILL.md frontmatter.
    Skills []PackedSkill
}

// PackedSkill describes one skill included in the bundle.
type PackedSkill struct {
    // Name is the skill's directory name under skills/ in the output bundle.
    // Copied verbatim from the input directory name.
    Name string

    // FrontmatterName is the value of the `name:` field in SKILL.md
    // frontmatter. May differ from Name (directory) — Pack does not
    // rewrite SKILL.md.
    FrontmatterName string

    // Description is the value of the `description:` field in SKILL.md
    // frontmatter. Empty if absent.
    Description string
}

// Pack reads inputDir, detects its layout, validates each skill, and writes
// a plugin bundle to outDir. Pack is idempotent: given the same input it
// produces byte-identical output (ordered writes, stable JSON key order).
//
// inputDir and outDir are filesystem paths. outDir may be absolute or
// relative; Pack cleans both before use.
//
// Pack returns an error if any validation fails. No bundle is written on
// error — Pack validates fully before emitting.
func Pack(inputDir, outDir string, opts Options) (*Result, error)

// Sentinel errors. All wrapped with fmt.Errorf("%w: …") to preserve detail.
var (
    // ErrInvalidName is returned when opts.Name does not match the
    // ^[a-z0-9][a-z0-9-]*$ naming rule.
    ErrInvalidName = errors.New("skillpack: invalid bundle name")

    // ErrMissingVersion is returned when opts.Version is empty.
    ErrMissingVersion = errors.New("skillpack: version is required")

    // ErrInputNotFound is returned when inputDir does not exist or is not
    // a directory.
    ErrInputNotFound = errors.New("skillpack: input directory not found")

    // ErrNoSkills is returned when the detected input layout contains zero
    // valid skill subdirectories.
    ErrNoSkills = errors.New("skillpack: no skills found in input")

    // ErrInvalidSkill is returned when a candidate skill directory is
    // missing SKILL.md or its frontmatter lacks required fields.
    ErrInvalidSkill = errors.New("skillpack: invalid skill")

    // ErrOutDirNotEmpty is returned when outDir exists and is non-empty
    // without Options.Force.
    ErrOutDirNotEmpty = errors.New("skillpack: output directory not empty")

    // ErrUnsafeOutDir is returned when outDir resolves (through symlinks)
    // inside inputDir, or is a symlink / non-directory file that Pack
    // refuses to overwrite without Force.
    ErrUnsafeOutDir = errors.New("skillpack: unsafe output directory")
)
```

## Input layout detection

Two layouts are accepted. Detection is a single rule, applied after
`inputDir` is confirmed to be a directory:

1. **Layout A — skills at the root.** If `inputDir` directly contains one
   or more subdirectories with a `SKILL.md` file, treat those subdirs
   as the skill set. Any `skills/` subdirectory at this level is ignored
   in Layout A.
2. **Layout B — skills nested under `skills/`.** If Layout A finds zero
   skills AND `inputDir/skills/` exists AND contains subdirectories with
   `SKILL.md`, use those.
3. If neither produces any skill, return `ErrNoSkills`.

Rationale: Layout A matches how users keep a flat skill library
(`company-skills/foo/SKILL.md`); Layout B matches when users already have
a bundle-shaped source tree. Rule 1 wins so a mixed tree (`./foo/SKILL.md`
plus a stray `./skills/`) is unambiguous.

## Validation

For each skill directory:

- `SKILL.md` must exist and be readable.
- The file must begin with YAML frontmatter (`---\n…\n---\n`).
- Frontmatter must contain non-empty `name:` and `description:` fields.
  Both are required by Claude Desktop; missing either causes the skill
  to load with placeholder metadata, which is a silent footgun worth
  rejecting at pack time.
- The directory name (not the frontmatter name) is the identifier used
  on disk; Pack does not enforce a match between directory name and
  frontmatter `name` — the two legitimately differ in existing bundles.

Bundle-level:

- `opts.Name` must match `^[a-z0-9][a-z0-9-]*$`.
- `opts.Version` must be non-empty.

Output dir:

- If `outDir` exists and contains any entries — including hidden files
  like `.DS_Store` — Pack returns `ErrOutDirNotEmpty` unless
  `opts.Force`. A truly empty directory at `outDir` is accepted
  without `--force`.
- If `outDir` is a symlink or a non-directory file, Pack returns
  `ErrUnsafeOutDir` unless `opts.Force`.
- The canonical path (through symlinks) of `outDir` must not be equal
  to or inside the canonical path of `inputDir`; otherwise
  `ErrUnsafeOutDir`.
- Pack never writes to arbitrary locations outside `outDir` — content
  is staged in a sibling temp directory and moved into place only
  after every skill has been copied successfully, so a failed Pack
  cannot leave a half-built bundle behind.
- Commit is driven by the `outDirPolicy` captured at validation time
  (one of `outDirAbsent`, `outDirEmpty`, or `outDirForceReplace`), not
  by a fresh `Lstat` at commit time. This closes the TOCTOU where a
  racer creates content at `outDir` between validation and commit:
    - `outDirAbsent` → plain `os.Rename(staged, outDir)`; Rename fails
      if a racer created any content at outDir, Pack aborts.
    - `outDirEmpty` → `syscall.Rmdir(outDir)` (directory-only; fails
      ENOTDIR if outDir was swapped for a regular file, ENOTEMPTY if
      populated) followed by `os.Rename(staged, outDir)`.
    - `outDirForceReplace` → move existing `outDir` to a sidecar name
      via `os.Rename`, install staged at `outDir`, then `RemoveAll`
      the sidecar. The sidecar is a path Pack picks, so `RemoveAll`
      only wipes state Pack owns.
  None of these branches call `RemoveAll` against the user-facing
  `outDir` path.

## Output layout

```
outDir/
├── .claude-plugin/
│   └── plugin.json
├── README.md
└── skills/
    ├── <skill-a>/SKILL.md
    ├── <skill-a>/<anything else copied verbatim>
    └── <skill-b>/...
```

### `plugin.json`

Emitted with stable key order for reproducibility:

```json
{
  "name": "<opts.Name>",
  "version": "<opts.Version>",
  "description": "<opts.Description or default>",
  "author": {
    "name": "<opts.AuthorName>",
    "email": "<opts.AuthorEmail>"
  }
}
```

- `author` block omitted entirely when `AuthorName` is empty.
- `author.email` omitted when empty (but `name` present).
- Two-space indentation, trailing newline.

### `README.md`

Auto-generated. Lists every packed skill with its frontmatter description.
Intent: when an IT admin `ls`s the bundle they can see what's inside
without reading every SKILL.md. Content:

```markdown
# <opts.Name>

<opts.Description>

Generated by `cowork-mdm skill pack`. Do not edit by hand — regenerate
when the source skills change.

## Skills

- **<skill-a>** — <frontmatter description>
- **<skill-b>** — <frontmatter description>
```

### `skills/` tree

Each skill directory is copied recursively from the input. File modes
are preserved (rwx bits); timestamps are not preserved (so repeated
packs produce identical output). Symlinks inside a skill are resolved
to their targets and copied as files — Pack never writes a symlink to
the output, so MDM transport stays portable.

## Non-goals

- No zip/tar output. `org-plugins/` mounts directories, not archives.
- No git init / push / tag. Pack produces a directory; the caller can
  commit it, push it, or ship it via MDM however they want.
- No `commands/` / `agents/` / `hooks/` support. `skill pack` is
  skill-only by design — users with other plugin components should hand-
  author the bundle.
- No SKILL.md content linting beyond frontmatter required fields. Future
  `cowork-mdm skill lint` may do deeper checks.
- No Windows-specific path handling beyond what `filepath` gives us.
  `org-plugins/` is mounted the same way on both platforms.

## Testing

`internal/skillpack/skillpack_test.go`:

- `TestPack_LayoutA` — input has skills at root, verifies bundle
  structure, plugin.json content, README content, and Result metadata.
- `TestPack_LayoutB` — input has `skills/` subdir, same assertions.
- `TestPack_IgnoresStraySkillsInLayoutA` — mixed tree picks Layout A.
- `TestPack_RejectsInvalidName` — names like `Foo`, `-foo`, `foo/bar`,
  empty → ErrInvalidName.
- `TestPack_RejectsMissingFrontmatter` — SKILL.md without frontmatter →
  ErrInvalidSkill.
- `TestPack_RejectsMissingRequiredFields` — frontmatter without `name`
  or `description` → ErrInvalidSkill.
- `TestPack_RejectsNoSkills` — empty input or input with no SKILL.md
  anywhere → ErrNoSkills.
- `TestPack_RejectsNonEmptyOutDir` — populated outDir without Force →
  ErrOutDirNotEmpty; with Force → overwrites.
- `TestPack_ReproducibleBytes` — packing the same input twice to
  different outDirs produces byte-identical `plugin.json` and
  `README.md`.
- `TestPack_OmitsAuthorWhenEmpty` — no AuthorName → no `author` key in
  plugin.json.

Fixtures use `t.TempDir()`. No golden files — assertions check specific
JSON fields and markdown line content, not full-file snapshots, to keep
tests resilient to cosmetic changes.

## CLI surface

Exposed as `cowork-mdm skill pack`:

```
cowork-mdm skill pack INPUT_DIR \
  --name NAME \
  --out OUT_DIR \
  [--version 0.1.0] \
  [--description "..."] \
  [--author NAME] \
  [--author-email EMAIL] \
  [--force] \
  [--json]
```

`--name` and `--out` are required. Text output is a one-liner per skill
followed by a summary; `--json` emits the `Result` struct verbatim.
Exits 1 on any Pack error.
