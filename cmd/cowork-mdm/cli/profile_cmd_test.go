package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProfileTemplates_Lists(t *testing.T) {
	out, _, err := runCmd(t, "profile", "templates")
	if err != nil {
		t.Fatalf("profile templates: %v", err)
	}
	for _, want := range []string{"bedrock-basic", "gateway", "vertex", "foundry", "mcp-only"} {
		if !strings.Contains(out, want) {
			t.Errorf("template list missing %q: %s", want, out)
		}
	}
}

func TestProfileNew_FromTemplate_Mobileconfig(t *testing.T) {
	out, _, err := runCmd(t, "profile", "new", "--template", "bedrock-basic")
	if err != nil {
		t.Fatalf("profile new: %v", err)
	}
	for _, want := range []string{
		"<!DOCTYPE plist",
		"com.anthropic.claudefordesktop",
		"<key>inferenceProvider</key>",
		"<string>bedrock</string>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestProfileNew_FormatPlist(t *testing.T) {
	out, _, err := runCmd(t, "profile", "new", "--template", "bedrock-basic", "--format", "plist")
	if err != nil {
		t.Fatalf("profile new --format plist: %v", err)
	}
	// Plain plist, NO Apple Configuration wrapper.
	if strings.Contains(out, "PayloadContent") {
		t.Errorf("--format plist should not include PayloadContent envelope")
	}
	if !strings.Contains(out, "<key>inferenceProvider</key>") {
		t.Errorf("plist output missing provider key")
	}
}

func TestProfileNew_SetOverride(t *testing.T) {
	// Override inferenceBedrockRegion via --set, make sure it appears in output.
	out, _, err := runCmd(t, "profile", "new",
		"--template", "bedrock-basic",
		"--set", "inferenceBedrockRegion=eu-west-1",
	)
	if err != nil {
		t.Fatalf("profile new --set: %v", err)
	}
	if !strings.Contains(out, "<string>eu-west-1</string>") {
		t.Errorf("override not reflected: %s", firstN(out, 400))
	}
}

func TestProfileNew_SetBadKey(t *testing.T) {
	_, stderr, err := runCmd(t, "profile", "new",
		"--template", "bedrock-basic",
		"--set", "notARealKey=x",
	)
	if err == nil {
		t.Fatal("--set on unknown key should fail")
	}
	if !strings.Contains(err.Error()+stderr, "unknown key") {
		t.Errorf("error should mention unknown key: err=%v stderr=%s", err, stderr)
	}
}

func TestProfileNew_RequiresTemplateOrFrom(t *testing.T) {
	_, _, err := runCmd(t, "profile", "new")
	if err == nil {
		t.Error("profile new without --template or --from should fail")
	}
}

func TestProfileNew_FromYAML(t *testing.T) {
	dir := t.TempDir()
	yaml := `name: custom
values:
  inferenceProvider: gateway
  inferenceGatewayBaseUrl: https://gw.example.internal
`
	path := filepath.Join(dir, "custom.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _, err := runCmd(t, "profile", "new", "--from", path)
	if err != nil {
		t.Fatalf("profile new --from: %v", err)
	}
	if !strings.Contains(out, "<string>gateway</string>") {
		t.Errorf("--from YAML output missing provider")
	}
}

func TestProfileValidate_GeneratedProfileIsValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "p.mobileconfig")
	// Generate first
	out, _, err := runCmd(t, "profile", "new", "--template", "bedrock-basic", "--out", path)
	if err != nil {
		t.Fatalf("profile new: %v", err)
	}
	_ = out
	// Validate
	vOut, _, err := runCmd(t, "profile", "validate", path)
	if err != nil {
		t.Fatalf("profile validate: %v\noutput=%s", err, vOut)
	}
	if !strings.Contains(vOut, "OK") {
		t.Errorf("validate output should say OK, got: %s", vOut)
	}
}

func TestProfileValidate_FlagsGarbage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.mobileconfig")
	if err := os.WriteFile(path, []byte("<html>not a plist</html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := runCmd(t, "profile", "validate", path)
	if err == nil {
		t.Error("validate on garbage input should fail")
	}
}

func TestProfileStatus_RedactsSensitiveByDefault(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skipf("status SourcePath darwin-only for this test, got %s", runtime.GOOS)
	}
	// Build a fixture profile with a known-sensitive key populated.
	dir := t.TempDir()
	yaml := `name: fixture
values:
  inferenceProvider: gateway
  inferenceGatewayBaseUrl: https://gw.example.internal
  inferenceGatewayApiKey: super-secret-token-must-not-leak
`
	yamlPath := filepath.Join(dir, "fixture.yaml")
	if err := os.WriteFile(yamlPath, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	profilePath := filepath.Join(dir, "fixture.plist")
	if _, _, err := runCmd(t, "profile", "new", "--from", yamlPath, "--format", "plist", "--out", profilePath); err != nil {
		t.Fatal(err)
	}

	// Default: sensitive values must be redacted in human output.
	out, _, err := runCmd(t, "profile", "status", "--source", profilePath)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if strings.Contains(out, "super-secret-token-must-not-leak") {
		t.Errorf("sensitive value leaked in default output:\n%s", out)
	}
	if !strings.Contains(out, "<redacted>") {
		t.Errorf("expected <redacted> marker in output:\n%s", out)
	}
	// Non-sensitive key should pass through.
	if !strings.Contains(out, "gw.example.internal") {
		t.Errorf("non-sensitive value was unexpectedly redacted")
	}
}

func TestProfileStatus_UnmaskedFlag(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skipf("status SourcePath darwin-only for this test, got %s", runtime.GOOS)
	}
	dir := t.TempDir()
	yaml := `name: fixture
values:
  inferenceProvider: gateway
  inferenceGatewayBaseUrl: https://gw.example.internal
  inferenceGatewayApiKey: super-secret-token
`
	yamlPath := filepath.Join(dir, "fixture.yaml")
	if err := os.WriteFile(yamlPath, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	profilePath := filepath.Join(dir, "fixture.plist")
	if _, _, err := runCmd(t, "profile", "new", "--from", yamlPath, "--format", "plist", "--out", profilePath); err != nil {
		t.Fatal(err)
	}
	out, _, err := runCmd(t, "profile", "status", "--source", profilePath, "--unmasked")
	if err != nil {
		t.Fatalf("status --unmasked: %v", err)
	}
	if !strings.Contains(out, "super-secret-token") {
		t.Errorf("expected --unmasked to show raw value, output=%s", out)
	}
}

func TestProfileStatus_JSONKeepsRawValues(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skipf("status SourcePath darwin-only for this test, got %s", runtime.GOOS)
	}
	dir := t.TempDir()
	yaml := `name: fixture
values:
  inferenceProvider: gateway
  inferenceGatewayApiKey: super-secret-token
`
	yamlPath := filepath.Join(dir, "fixture.yaml")
	if err := os.WriteFile(yamlPath, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	profilePath := filepath.Join(dir, "fixture.plist")
	if _, _, err := runCmd(t, "profile", "new", "--from", yamlPath, "--format", "plist", "--out", profilePath); err != nil {
		t.Fatal(err)
	}
	out, _, err := runCmd(t, "profile", "status", "--source", profilePath, "--json")
	if err != nil {
		t.Fatalf("status --json: %v", err)
	}
	if !strings.Contains(out, "super-secret-token") {
		t.Errorf("JSON mode should return raw values for machine consumers; got %s", out)
	}
}

func TestProfileStatus_SourceMobileconfig(t *testing.T) {
	// Regression guard: --source should auto-detect .mobileconfig wrapper
	// and decode the inner Claude payload, not leak outer PayloadContent.
	if runtime.GOOS != "darwin" {
		t.Skipf("status SourcePath darwin-only for this test, got %s", runtime.GOOS)
	}
	dir := t.TempDir()
	yaml := `name: fixture
values:
  inferenceProvider: gateway
  inferenceGatewayBaseUrl: https://gw.example.internal
  inferenceGatewayApiKey: super-secret-token
`
	yamlPath := filepath.Join(dir, "fixture.yaml")
	if err := os.WriteFile(yamlPath, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	profilePath := filepath.Join(dir, "fixture.mobileconfig")
	if _, _, err := runCmd(t, "profile", "new", "--from", yamlPath, "--out", profilePath); err != nil {
		t.Fatal(err)
	}
	out, _, err := runCmd(t, "profile", "status", "--source", profilePath)
	if err != nil {
		t.Fatalf("status --source mobileconfig: %v", err)
	}
	// Must surface the inner Claude payload, not the outer envelope keys.
	if !strings.Contains(out, "inferenceProvider") {
		t.Errorf("status should decode inner Claude payload; got %s", out)
	}
	if strings.Contains(out, "PayloadContent") {
		t.Errorf("status leaked mobileconfig outer-wrapper keys; got %s", out)
	}
	// Sensitive value still redacted.
	if strings.Contains(out, "super-secret-token") {
		t.Errorf("sensitive value leaked: %s", out)
	}
}

func TestProfileNew_PayloadIdentifierPrefix(t *testing.T) {
	// Explicit flag wins over env var and default.
	t.Setenv("COWORK_MDM_PAYLOAD_ID_PREFIX", "com.env.example")
	out, _, err := runCmd(t, "profile", "new",
		"--template", "bedrock-basic",
		"--payload-identifier-prefix", "com.acme.it",
	)
	if err != nil {
		t.Fatalf("profile new: %v", err)
	}
	if !strings.Contains(out, "com.acme.it.bedrock-basic") {
		t.Errorf("flag-provided prefix not in output")
	}
	if strings.Contains(out, "com.env.example") {
		t.Errorf("env var should have been overridden by flag")
	}
	if strings.Contains(out, "com.yuanli") {
		t.Errorf("legacy com.yuanli prefix leaked into output")
	}
}

func TestProfileNew_PayloadIdentifierEnvFallback(t *testing.T) {
	// No flag → env var takes effect.
	t.Setenv("COWORK_MDM_PAYLOAD_ID_PREFIX", "com.env.example")
	out, _, err := runCmd(t, "profile", "new", "--template", "bedrock-basic")
	if err != nil {
		t.Fatalf("profile new: %v", err)
	}
	if !strings.Contains(out, "com.env.example.bedrock-basic") {
		t.Errorf("env-var prefix not in output")
	}
}

func TestProfileShowTemplate_Stdout(t *testing.T) {
	out, _, err := runCmd(t, "profile", "show-template", "enterprise-cn-full")
	if err != nil {
		t.Fatalf("show-template: %v", err)
	}
	if !strings.HasPrefix(out, "name: enterprise-cn-full") {
		t.Errorf("expected YAML to start with 'name: enterprise-cn-full', got %q", firstN(out, 60))
	}
	if !strings.Contains(out, "REPLACE_WITH_YOUR_API_KEY") {
		t.Errorf("dumped YAML missing placeholder")
	}
}

func TestProfileShowTemplate_OutFile(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "overrides.yaml")
	_, _, err := runCmd(t, "profile", "show-template", "enterprise-cn-full", "--out", outPath)
	if err != nil {
		t.Fatalf("show-template --out: %v", err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "name: enterprise-cn-full") {
		t.Errorf("file content wrong prefix: %q", firstN(string(data), 60))
	}
}

func TestProfileShowTemplate_UnknownName(t *testing.T) {
	_, _, err := runCmd(t, "profile", "show-template", "does-not-exist")
	if err == nil {
		t.Error("unknown template name should fail")
	}
	// Error should list available templates.
	if !strings.Contains(err.Error(), "bedrock-basic") {
		t.Errorf("error should list available templates, got: %v", err)
	}
}

func TestProfileLint_DirtyProfileFails(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "dirty.mobileconfig")
	if _, _, err := runCmd(t, "profile", "new", "--template", "enterprise-cn-full", "--out", p); err != nil {
		t.Fatal(err)
	}
	out, _, err := runCmd(t, "profile", "lint", p)
	if err == nil {
		t.Error("lint on dirty profile should fail")
	}
	if !strings.Contains(out, "REPLACE_WITH_YOUR_API_KEY") {
		t.Errorf("lint output should name the placeholder, got: %q", firstN(out, 200))
	}
	if !strings.Contains(out, "placeholder") {
		t.Errorf("lint output should mention placeholder, got: %q", firstN(out, 200))
	}
}

func TestProfileLint_CleanProfilePasses(t *testing.T) {
	// Build a profile from bedrock-basic which uses ACCOUNT / PROFILE_ID
	// variable names (not the REPLACE_* convention) — lint must not flag.
	dir := t.TempDir()
	p := filepath.Join(dir, "clean.mobileconfig")
	if _, _, err := runCmd(t, "profile", "new", "--template", "bedrock-basic", "--out", p); err != nil {
		t.Fatal(err)
	}
	out, _, err := runCmd(t, "profile", "lint", p)
	if err != nil {
		t.Errorf("lint on bedrock-basic should pass, got err=%v out=%s", err, out)
	}
	if !strings.Contains(out, "no placeholder") {
		t.Errorf("lint output should confirm clean, got: %q", out)
	}
}

func TestProfileLint_JSON(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "dirty.mobileconfig")
	if _, _, err := runCmd(t, "profile", "new", "--template", "enterprise-cn-full", "--out", p); err != nil {
		t.Fatal(err)
	}
	out, _, err := runCmd(t, "profile", "lint", "--json", p)
	if err == nil {
		t.Error("lint on dirty profile should fail even with --json")
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Errorf("--json output should start with {, got: %q", firstN(out, 80))
	}
	if !strings.Contains(out, `"findings"`) || !strings.Contains(out, `"match"`) {
		t.Errorf("--json output missing findings structure: %q", firstN(out, 200))
	}
}

func TestProfileApply_DryRun_NoSideEffects(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skipf("apply tests run only on darwin, got %s", runtime.GOOS)
	}
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "p.mobileconfig")
	if _, _, err := runCmd(t, "profile", "new", "--template", "bedrock-basic", "--out", profilePath); err != nil {
		t.Fatal(err)
	}
	out, _, err := runCmd(t, "profile", "apply", profilePath, "--dry-run")
	if err != nil {
		t.Fatalf("apply --dry-run: %v", err)
	}
	if !strings.Contains(out, "dry-run") {
		t.Errorf("dry-run output missing marker: %s", out)
	}
	if !strings.Contains(out, "<key>inferenceProvider</key>") {
		t.Errorf("dry-run preview missing content")
	}
}
