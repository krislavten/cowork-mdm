#!/usr/bin/env bash
# verify.sh <task-id> — gate before committing any task work.
# Agents invoke this before committing. CI also runs this for merged PRs.
# Emit non-zero on failure. Quiet on success except final ✓.
set -eu

TASK="${1:-}"
if [ -z "$TASK" ]; then
    echo "usage: $0 <task-id>" >&2
    exit 2
fi

cd "$(git rev-parse --show-toplevel)"

FAIL=0
section() {
    echo ""
    echo "=== $1 ==="
}

run() {
    # Prints command, runs it, marks FAIL on non-zero.
    local label="$1"
    shift
    echo "• $label"
    if ! "$@"; then
        echo "  ✗ $label FAILED"
        FAIL=1
    fi
}

# ----- global gates (every task must pass these) -----
section "Global: format"
run "gofmt" bash -c '[ -z "$(gofmt -l .)" ] || { gofmt -l . ; exit 1; }'

section "Global: vet"
run "go vet ./..." go vet ./...

section "Global: tidy"
run "go mod tidy -diff" go mod tidy -diff

section "Global: build (native)"
run "go build ./..." go build ./...

section "Global: build (darwin arm64)"
run "GOOS=darwin GOARCH=arm64 go build ./..." bash -c 'GOOS=darwin GOARCH=arm64 go build ./...'

section "Global: build (darwin amd64)"
run "GOOS=darwin GOARCH=amd64 go build ./..." bash -c 'GOOS=darwin GOARCH=amd64 go build ./...'

section "Global: build (windows amd64)"
run "GOOS=windows GOARCH=amd64 go build ./..." bash -c 'GOOS=windows GOARCH=amd64 go build ./...'

section "Global: build (linux amd64)"
run "GOOS=linux GOARCH=amd64 go build ./..." bash -c 'GOOS=linux GOARCH=amd64 go build ./...'

# ----- forbidden-file checks -----
section "Forbidden file check"
# Agents must not modify protected files. We check three slices so staged /
# uncommitted edits are caught before commit (not just after push):
#   1. committed diff vs origin/main (HEAD vs origin/main)
#   2. staged changes (index vs HEAD)
#   3. working tree changes (worktree vs index)
# Coordinator branches (plan/*, chore/*, coord/*) are allowed through — that's
# the sanctioned path for spec / plan / verify.sh edits.
BRANCH=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "")
PROTECTED_RE='^(\.claude/plans/.+|specs/.+|AGENTS\.md|docs/execution/(verify\.sh|TASKS\.md)|internal/schema/schema\.json)$'
TOUCHED_FILE=$(mktemp)
{
    git diff --name-only "origin/main...HEAD" 2>/dev/null || true
    git diff --cached --name-only 2>/dev/null || true
    git diff --name-only 2>/dev/null || true
} | sort -u | grep -E "$PROTECTED_RE" > "$TOUCHED_FILE" || true

if [ -s "$TOUCHED_FILE" ]; then
    if ! echo "$BRANCH" | grep -qE '^(plan/|chore/|coord/|release/)'; then
        echo "  ✗ task branch '$BRANCH' modifies protected files:"
        sed 's/^/    /' "$TOUCHED_FILE"
        FAIL=1
    fi
fi
rm -f "$TOUCHED_FILE"

# ----- per-task targeted checks -----
section "Task: $TASK"
case "$TASK" in
    task-01)
        run "schema unit tests" go test ./internal/schema/...
        run "schema.json has >=51 keys (test assertion)" go test -run TestSchemaHasAllKeys ./internal/schema/...
        run "extractor build tag compiles" bash -c '
            if [ -d internal/schema/extract ]; then
                go build -tags extract ./internal/schema/extract/...
            else
                echo "    (extract package not yet created; skipping)"
            fi
        '
        ;;
    task-02)
        run "paths unit tests" go test ./internal/paths/...
        ;;
    task-03)
        run "profile unit tests" go test ./internal/profile/...
        if [ "$(uname)" = "Darwin" ]; then
            run "plutil -lint on golden file" plutil -lint internal/profile/testdata/bedrock-basic.golden.mobileconfig
        fi
        ;;
    task-04|task-05|task-06)
        run "profile unit tests" go test ./internal/profile/...
        ;;
    task-07)
        run "marketplace unit tests" go test ./internal/marketplace/...
        ;;
    task-08)
        run "plugin unit tests" go test ./internal/plugin/...
        ;;
    task-09)
        run "doctor unit tests" go test ./internal/doctor/...
        ;;
    task-apply)
        run "managed unit tests" go test ./internal/managed/...
        ;;
    task-10)
        run "ui unit tests" go test ./internal/ui/...
        ;;
    task-11)
        run "cmd wiring tests" go test ./cmd/...
        run "binary produces --version" bash -c '
            go build -o /tmp/cowork-mdm-verify ./cmd/cowork-mdm &&
            /tmp/cowork-mdm-verify --version | grep -qi "cowork-mdm"
        '
        ;;
    task-ci|task-release|task-docs)
        echo "  (no additional runtime checks — infra task, inspected in PR review)"
        ;;
    *)
        echo "  ✗ unknown task id: $TASK"
        FAIL=1
        ;;
esac

# ----- full suite gate (skippable per-task but enforced by CI) -----
section "Full suite"
run "go test ./... -race -count=1" go test ./... -race -count=1

if [ "$FAIL" -ne 0 ]; then
    echo ""
    echo "❌ verify.sh $TASK FAILED"
    exit 1
fi
echo ""
echo "✓ verify.sh $TASK PASSED"
