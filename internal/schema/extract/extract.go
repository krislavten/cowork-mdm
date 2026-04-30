//go:build extract

// Command extract regenerates internal/schema/schema.json from a
// Claude Desktop release. Guarded by the `extract` build tag so it's
// never linked into the production binary.
//
// Typical usage:
//
//	go run -tags extract ./internal/schema/extract \
//	    --from /Applications/Claude.app \
//	    --out  internal/schema/schema.json
//
// The tool:
//  1. Extracts the app.asar (shells out to `npx @electron/asar extract`
//     unless --asar-tool is overridden).
//  2. Locates `.vite/build/index.js` inside the extracted tree.
//  3. Finds the minified schema literal of the form `FJ=me({...})` and
//     brace-matches the outer `()`.
//  4. Parses the block into a []Key list using regex scanners.
//  5. Emits a pretty-printed JSON document.
//
// The extractor is fragile by design — it's a one-time-per-release
// maintenance task, not runtime logic. Produced output only needs to
// be "a reasonable schema"; downstream (schema.go + tests) checks
// shape and key count. Review the diff before committing.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Type / Scope names must track internal/schema/schema.go. Re-declared
// here as plain strings so the extractor stays a self-contained main
// and doesn't import the schema package (the schema package embeds
// this tool's output — importing back would be a circular mess).
const (
	typeString      = "string"
	typeBoolean     = "boolean"
	typeInteger     = "integer"
	typeStringArray = "stringArray"
	typeJSONString  = "jsonString"
	typeURL         = "url"
	typeEnum        = "enum"
)

type extractedKey struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Scopes      []string `json:"scopes"`
	AppMin      string   `json:"appMin,omitempty"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Example     any      `json:"example,omitempty"`
	LegacyAlias string   `json:"legacyAlias,omitempty"`
	Sensitive   bool     `json:"sensitive,omitempty"`
	Provider    string   `json:"provider,omitempty"`
	Category    string   `json:"category,omitempty"`
	EnumValues  []string `json:"enumValues,omitempty"`
}

type extractedSchema struct {
	Version                 string         `json:"version"`
	ExtractedFromAppVersion string         `json:"extractedFromAppVersion"`
	Keys                    []extractedKey `json:"keys"`
}

func main() {
	var (
		fromPath   = flag.String("from", "", "path to Claude.app (or any dir/file that resolves to app.asar)")
		outPath    = flag.String("out", "-", "output file path, or '-' for stdout")
		asarTool   = flag.String("asar-tool", "npx @electron/asar", "command used to unpack app.asar (--from override)")
		appVersion = flag.String("app-version", "", "override the extractedFromAppVersion field (else read from Info.plist / package.json if possible)")
		keepTmp    = flag.Bool("keep-tmp", false, "keep the extracted asar tmpdir for inspection")
	)
	flag.Parse()

	if *fromPath == "" {
		fmt.Fprintln(os.Stderr, "error: --from is required")
		flag.Usage()
		os.Exit(2)
	}

	schema, err := extractFromApp(*fromPath, *asarTool, *appVersion, *keepTmp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "extract failed: %v\n", err)
		os.Exit(1)
	}

	if err := writeJSON(*outPath, schema); err != nil {
		fmt.Fprintf(os.Stderr, "write failed: %v\n", err)
		os.Exit(1)
	}
}

func writeJSON(path string, s *extractedSchema) error {
	buf, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	buf = append(buf, '\n')
	if path == "-" {
		_, err := os.Stdout.Write(buf)
		return err
	}
	return os.WriteFile(path, buf, 0o644)
}

// extractFromApp is the top-level driver. It is separated from main()
// so the smoke test can drive individual sub-steps.
func extractFromApp(appPath, asarTool, appVersion string, keepTmp bool) (*extractedSchema, error) {
	asarFile, err := locateAsar(appPath)
	if err != nil {
		return nil, err
	}

	tmpDir, err := os.MkdirTemp("", "cowork-mdm-extract-")
	if err != nil {
		return nil, fmt.Errorf("mkdir temp: %w", err)
	}
	if !keepTmp {
		defer os.RemoveAll(tmpDir)
	} else {
		fmt.Fprintf(os.Stderr, "keeping extracted asar at %s\n", tmpDir)
	}

	if err := runAsarExtract(asarTool, asarFile, tmpDir); err != nil {
		return nil, err
	}

	indexJS, err := os.ReadFile(filepath.Join(tmpDir, ".vite", "build", "index.js"))
	if err != nil {
		return nil, fmt.Errorf("read .vite/build/index.js: %w", err)
	}

	keys, err := parseSchemaLiteral(string(indexJS))
	if err != nil {
		return nil, err
	}

	if appVersion == "" {
		appVersion = detectAppVersion(appPath, tmpDir)
	}

	return &extractedSchema{
		Version:                 "1",
		ExtractedFromAppVersion: appVersion,
		Keys:                    keys,
	}, nil
}

func locateAsar(appPath string) (string, error) {
	info, err := os.Stat(appPath)
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", appPath, err)
	}
	if !info.IsDir() {
		// Assume user passed the .asar directly.
		return appPath, nil
	}
	// Try the macOS bundle layout first, then generic Electron layouts.
	candidates := []string{
		filepath.Join(appPath, "Contents", "Resources", "app.asar"),
		filepath.Join(appPath, "resources", "app.asar"),
		filepath.Join(appPath, "app.asar"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("could not find app.asar under %s (looked in %v)", appPath, candidates)
}

func runAsarExtract(toolCmd, asar, dst string) error {
	parts := strings.Fields(toolCmd)
	if len(parts) == 0 {
		return errors.New("--asar-tool is empty")
	}
	args := append(parts[1:], "extract", asar, dst)
	cmd := exec.Command(parts[0], args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s extract: %w", toolCmd, err)
	}
	return nil
}

func detectAppVersion(appPath, tmpDir string) string {
	// package.json inside the asar is authoritative.
	if data, err := os.ReadFile(filepath.Join(tmpDir, "package.json")); err == nil {
		var pkg struct {
			Version string `json:"version"`
		}
		if json.Unmarshal(data, &pkg) == nil && pkg.Version != "" {
			return pkg.Version
		}
	}
	// Fall back to macOS Info.plist CFBundleShortVersionString grep.
	plist := filepath.Join(appPath, "Contents", "Info.plist")
	if data, err := os.ReadFile(plist); err == nil {
		re := regexp.MustCompile(`<key>CFBundleShortVersionString</key>\s*<string>([^<]+)</string>`)
		if m := re.FindSubmatch(data); len(m) == 2 {
			return string(m[1])
		}
	}
	return "unknown"
}

// parseSchemaLiteral locates and decodes the `FJ=me({...})` block.
// Exported for the smoke test.
func parseSchemaLiteral(src string) ([]extractedKey, error) {
	start, body, err := locateMeCall(src)
	if err != nil {
		return nil, err
	}
	_ = start

	keys, err := parseKeyBlock(body)
	if err != nil {
		return nil, fmt.Errorf("parse key block: %w", err)
	}
	if len(keys) == 0 {
		return nil, errors.New("schema literal yielded 0 keys — likely a parser mismatch")
	}
	sort.SliceStable(keys, func(i, j int) bool { return keys[i].Name < keys[j].Name })
	return keys, nil
}

// locateMeCall finds `FJ=me({...})` and returns the argument substring
// between the outer `{` and matching `}` (i.e. the key-set literal).
func locateMeCall(src string) (int, string, error) {
	// Tolerate whitespace between tokens: `FJ = me ({`.
	re := regexp.MustCompile(`FJ\s*=\s*me\s*\(\s*\{`)
	loc := re.FindStringIndex(src)
	if loc == nil {
		return 0, "", errors.New("could not find `FJ=me({` in index.js")
	}
	// loc[1] points just past the '{'. Brace-match from there.
	end, err := matchBrace(src, loc[1]-1)
	if err != nil {
		return 0, "", fmt.Errorf("brace-match from FJ=me({: %w", err)
	}
	return loc[0], src[loc[1]:end], nil
}

// matchBrace assumes src[open] == '{' and returns the index of its
// matching '}'. String literals (single, double, backtick) are
// skipped.
func matchBrace(src string, open int) (int, error) {
	if open >= len(src) || src[open] != '{' {
		return 0, fmt.Errorf("matchBrace: src[%d] != '{'", open)
	}
	depth := 1
	i := open + 1
	for i < len(src) {
		c := src[i]
		switch c {
		case '\\':
			i += 2 // skip escape even outside strings — harmless
			continue
		case '\'', '"', '`':
			j, err := skipString(src, i)
			if err != nil {
				return 0, err
			}
			i = j
			continue
		case '/':
			// Skip // line comments and /* block comments — the JS
			// bundle is minified but this is cheap safety.
			if i+1 < len(src) && src[i+1] == '/' {
				for i < len(src) && src[i] != '\n' {
					i++
				}
				continue
			}
			if i+1 < len(src) && src[i+1] == '*' {
				end := strings.Index(src[i+2:], "*/")
				if end < 0 {
					return 0, errors.New("unterminated /* */ comment")
				}
				i = i + 2 + end + 2
				continue
			}
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i, nil
			}
		}
		i++
	}
	return 0, errors.New("unterminated { block")
}

// skipString advances past a string literal starting at src[i]. The
// opening quote is src[i]; returns the index just past the closing
// quote.
func skipString(src string, i int) (int, error) {
	quote := src[i]
	j := i + 1
	for j < len(src) {
		c := src[j]
		if c == '\\' {
			j += 2
			continue
		}
		if c == quote {
			return j + 1, nil
		}
		// Backtick template literals may contain ${...} expressions;
		// brace-match inside.
		if quote == '`' && c == '$' && j+1 < len(src) && src[j+1] == '{' {
			end, err := matchBrace(src, j+1)
			if err != nil {
				return 0, fmt.Errorf("template-literal ${...}: %w", err)
			}
			j = end + 1
			continue
		}
		j++
	}
	return 0, fmt.Errorf("unterminated %c-quoted string", quote)
}

// parseKeyBlock walks the body of `FJ=me({ ... })`, locating each
// top-level `<name>:<suffix?>(<typeExpr>,{...})` entry.
//
// The actual Claude bundle uses minifier-chosen suffixes like
// `:nn(...)` or `:n5(...)`; we accept any short identifier between the
// colon and the opening `(`.
func parseKeyBlock(body string) ([]extractedKey, error) {
	var keys []extractedKey
	i := 0
	for i < len(body) {
		// Skip whitespace / commas between entries.
		for i < len(body) && (body[i] == ' ' || body[i] == '\n' || body[i] == '\r' || body[i] == '\t' || body[i] == ',') {
			i++
		}
		if i >= len(body) {
			break
		}

		// Read identifier (possibly quoted).
		name, next, ok := readPropertyName(body, i)
		if !ok {
			// Not at an entry start — skip this char and retry.
			i++
			continue
		}
		i = next

		// Expect ':'.
		i = skipSpaces(body, i)
		if i >= len(body) || body[i] != ':' {
			// Not a property; skip a char and keep scanning. (The me()
			// block is flat, but forgiving here protects against the
			// occasional comment or stray token.)
			continue
		}
		i++

		// Expect <suffix-ident>?  '('.
		i = skipSpaces(body, i)
		suffixStart := i
		for i < len(body) && isIdent(body[i]) {
			i++
		}
		_ = body[suffixStart:i] // suffix is ignored; kept for debugging
		i = skipSpaces(body, i)

		if i >= len(body) || body[i] != '(' {
			continue
		}
		// Brace-match the call's ().
		end, err := matchParen(body, i)
		if err != nil {
			return nil, fmt.Errorf("key %q: %w", name, err)
		}
		args := body[i+1 : end]
		i = end + 1

		key, err := parseKeyArgs(name, args)
		if err != nil {
			// Rather than fail the whole extraction, skip malformed
			// entries with a stderr note. This makes the tool
			// resilient to future Claude minification changes.
			fmt.Fprintf(os.Stderr, "warn: key %q: %v\n", name, err)
			continue
		}
		keys = append(keys, key)
	}
	return keys, nil
}

func readPropertyName(src string, i int) (string, int, bool) {
	if i >= len(src) {
		return "", i, false
	}
	// Quoted property name.
	if src[i] == '"' || src[i] == '\'' {
		end, err := skipString(src, i)
		if err != nil {
			return "", i, false
		}
		// src[i+1:end-1] is the raw content.
		return src[i+1 : end-1], end, true
	}
	if !isIdentStart(src[i]) {
		return "", i, false
	}
	j := i
	for j < len(src) && isIdent(src[j]) {
		j++
	}
	return src[i:j], j, true
}

func isIdentStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' || c == '$'
}

func isIdent(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

func skipSpaces(src string, i int) int {
	for i < len(src) && (src[i] == ' ' || src[i] == '\t' || src[i] == '\n' || src[i] == '\r') {
		i++
	}
	return i
}

// matchParen behaves like matchBrace but for '(' / ')'.
func matchParen(src string, open int) (int, error) {
	if open >= len(src) || src[open] != '(' {
		return 0, fmt.Errorf("matchParen: src[%d] != '('", open)
	}
	depth := 1
	i := open + 1
	for i < len(src) {
		c := src[i]
		switch c {
		case '\\':
			i += 2
			continue
		case '\'', '"', '`':
			j, err := skipString(src, i)
			if err != nil {
				return 0, err
			}
			i = j
			continue
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i, nil
			}
		case '{':
			// Descend to keep string-in-object parsing consistent.
			j, err := matchBrace(src, i)
			if err != nil {
				return 0, err
			}
			i = j + 1
			continue
		}
		i++
	}
	return 0, errors.New("unterminated ( block")
}

// parseKeyArgs handles the `<typeExpr>, {<metadata>}` payload. The
// metadata object carries title/description/scopes/etc.
func parseKeyArgs(name, args string) (extractedKey, error) {
	// Find the metadata object — the last '{' at the top level.
	depth := 0
	metaStart := -1
	for i := 0; i < len(args); i++ {
		c := args[i]
		switch c {
		case '\\':
			i++
			continue
		case '\'', '"', '`':
			j, err := skipString(args, i)
			if err != nil {
				return extractedKey{}, err
			}
			i = j - 1
			continue
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case '{':
			if depth == 0 {
				metaStart = i
			} else {
				depth++
			}
		case '}':
			if depth > 0 {
				depth--
			}
		}
	}

	var typeExpr, metaBody string
	if metaStart >= 0 {
		typeExpr = strings.TrimSpace(trimTrailingComma(args[:metaStart]))
		metaEnd, err := matchBrace(args, metaStart)
		if err != nil {
			return extractedKey{}, fmt.Errorf("metadata brace: %w", err)
		}
		metaBody = args[metaStart+1 : metaEnd]
	} else {
		typeExpr = strings.TrimSpace(args)
	}

	k := extractedKey{Name: name}
	t, enumVals, err := classifyType(typeExpr)
	if err != nil {
		return extractedKey{}, fmt.Errorf("classify type %q: %w", typeExpr, err)
	}
	k.Type = t
	k.EnumValues = enumVals

	if metaBody != "" {
		parseMetadata(&k, metaBody)
	}
	return k, nil
}

func trimTrailingComma(s string) string {
	s = strings.TrimRight(s, " \t\n\r")
	s = strings.TrimRight(s, ",")
	return strings.TrimRight(s, " \t\n\r")
}

// classifyType maps Claude's schema-builder expressions to our Type
// constants. This is the spec's "extraction algorithm step 5" mapping.
func classifyType(expr string) (string, []string, error) {
	e := strings.TrimSpace(expr)

	// Enum: Ds([...])
	if strings.HasPrefix(e, "Ds(") {
		open := strings.Index(e, "[")
		if open < 0 {
			return "", nil, errors.New("Ds(...) without [ literal")
		}
		close := findMatching(e, open, '[', ']')
		if close < 0 {
			return "", nil, errors.New("unterminated [ in Ds")
		}
		vals := parseStringLiteralArray(e[open+1 : close])
		return typeEnum, vals, nil
	}

	// Li(...) → stringArray (the element expression is conventionally MA()).
	if strings.HasPrefix(e, "Li(") {
		return typeStringArray, nil, nil
	}

	// Bare identifiers (possibly with trailing `.something()`).
	head := leadingIdent(e)
	switch head {
	case "Hi":
		return typeBoolean, nil, nil
	case "MA":
		return typeString, nil, nil
	case "YsA", "r_":
		return typeURL, nil, nil
	case "ASt", "Vee":
		return typeJSONString, nil, nil
	case "PsA":
		// PsA.number().int() — integer. PsA.string() shouldn't
		// appear in practice; default to string if so.
		if strings.Contains(e, ".number(") {
			return typeInteger, nil, nil
		}
		return typeString, nil, nil
	}
	return "", nil, fmt.Errorf("unrecognized type expression %q", e)
}

func leadingIdent(s string) string {
	i := 0
	for i < len(s) && isIdent(s[i]) {
		i++
	}
	return s[:i]
}

func findMatching(s string, open int, openCh, closeCh byte) int {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
			continue
		case '\'', '"', '`':
			j, err := skipString(s, i)
			if err != nil {
				return -1
			}
			i = j - 1
			continue
		}
		if s[i] == openCh {
			depth++
		} else if s[i] == closeCh {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// parseStringLiteralArray returns the string values from `"a","b",'c'`.
func parseStringLiteralArray(body string) []string {
	var out []string
	i := 0
	for i < len(body) {
		c := body[i]
		if c == '"' || c == '\'' || c == '`' {
			end, err := skipString(body, i)
			if err != nil {
				return out
			}
			raw := body[i+1 : end-1]
			out = append(out, unquoteJSString(raw))
			i = end
			continue
		}
		i++
	}
	return out
}

// parseMetadata populates the {scopes,title,description,...} block.
// Minified JS sometimes uses unquoted keys; we scan by top-level
// identifiers.
func parseMetadata(k *extractedKey, body string) {
	entries := splitTopLevelEntries(body)
	for _, e := range entries {
		colon := indexColonTopLevel(e)
		if colon < 0 {
			continue
		}
		rawKey := strings.TrimSpace(e[:colon])
		rawVal := strings.TrimSpace(e[colon+1:])
		// Key may be quoted.
		if len(rawKey) >= 2 && (rawKey[0] == '"' || rawKey[0] == '\'') {
			rawKey = rawKey[1 : len(rawKey)-1]
		}

		switch rawKey {
		case "title":
			k.Title = stringValue(rawVal)
		case "description":
			k.Description = stringValue(rawVal)
		case "appMin":
			k.AppMin = stringValue(rawVal)
		case "legacyAlias":
			k.LegacyAlias = stringValue(rawVal)
		case "sensitive":
			k.Sensitive = boolValue(rawVal)
		case "provider":
			k.Provider = stringValue(rawVal)
		case "category":
			k.Category = stringValue(rawVal)
		case "example":
			k.Example = stringValue(rawVal)
		case "scopes":
			k.Scopes = parseStringLiteralArray(trimArrayBrackets(rawVal))
		}
	}
}

func splitTopLevelEntries(body string) []string {
	var out []string
	depth := 0
	start := 0
	for i := 0; i < len(body); i++ {
		c := body[i]
		switch c {
		case '\\':
			i++
			continue
		case '\'', '"', '`':
			j, err := skipString(body, i)
			if err != nil {
				return out
			}
			i = j - 1
			continue
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, body[start:i])
				start = i + 1
			}
		}
	}
	if start < len(body) {
		out = append(out, body[start:])
	}
	return out
}

func indexColonTopLevel(s string) int {
	depth := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\':
			i++
			continue
		case '\'', '"', '`':
			j, err := skipString(s, i)
			if err != nil {
				return -1
			}
			i = j - 1
			continue
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ':':
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func stringValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) < 2 {
		return ""
	}
	q := raw[0]
	if q != '"' && q != '\'' && q != '`' {
		return ""
	}
	// The value might be a concat: "foo " + "bar". Handle the common
	// case by finding the first string token only — good enough for
	// the minified code which rarely concatenates here.
	end, err := skipString(raw, 0)
	if err != nil {
		return ""
	}
	return unquoteJSString(raw[1 : end-1])
}

func boolValue(raw string) bool {
	return strings.TrimSpace(raw) == "true" || strings.TrimSpace(raw) == "!0"
}

func trimArrayBrackets(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '[' && s[len(s)-1] == ']' {
		return s[1 : len(s)-1]
	}
	return s
}

// unquoteJSString decodes the most common JS escape sequences inside a
// string literal body (without its delimiter quotes). It is NOT a
// complete JS tokenizer — it handles \n \t \r \\ \" \' \` \uXXXX and
// leaves unknown escapes alone. Template-literal ${...} expressions
// are preserved as-is.
func unquoteJSString(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	i := 0
	for i < len(s) {
		c := s[i]
		if c != '\\' || i+1 >= len(s) {
			b.WriteByte(c)
			i++
			continue
		}
		nxt := s[i+1]
		switch nxt {
		case 'n':
			b.WriteByte('\n')
			i += 2
		case 't':
			b.WriteByte('\t')
			i += 2
		case 'r':
			b.WriteByte('\r')
			i += 2
		case '\\', '"', '\'', '`', '/':
			b.WriteByte(nxt)
			i += 2
		case 'u':
			if i+6 <= len(s) {
				if n, err := strconv.ParseUint(s[i+2:i+6], 16, 32); err == nil {
					b.WriteRune(rune(n))
					i += 6
					continue
				}
			}
			b.WriteByte(c)
			i++
		default:
			b.WriteByte(nxt)
			i += 2
		}
	}
	return b.String()
}
