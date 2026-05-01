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
