//go:build darwin

package managed

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/krislavten/cowork-mdm/internal/profile"
)

func buildValidProfile(t *testing.T) *profile.Profile {
	t.Helper()
	p := profile.New("apply-test")
	mustSet := func(k string, v any) {
		if err := p.Set(k, v); err != nil {
			t.Fatalf("Set %s: %v", k, err)
		}
	}
	mustSet("inferenceProvider", "bedrock")
	mustSet("inferenceBedrockRegion", "us-west-2")
	mustSet("disableDeploymentModeChooser", true)
	return p
}

func TestApply_DryRun_EmitsPreviewNoSideEffects(t *testing.T) {
	target := filepath.Join(t.TempDir(), "claude.plist")
	p := buildValidProfile(t)
	res, err := Apply(p, ApplyOptions{TargetPath: target, DryRun: true})
	if err != nil {
		t.Fatalf("Apply dry-run: %v", err)
	}
	if !res.DryRun {
		t.Error("result DryRun flag should be true")
	}
	if res.Platform != "darwin" {
		t.Errorf("Platform = %q", res.Platform)
	}
	if res.BytesWritten != 0 {
		t.Error("BytesWritten should be 0 on DryRun")
	}
	if !strings.Contains(res.Preview, "<key>inferenceProvider</key>") {
		t.Errorf("Preview missing expected content: %s", res.Preview[:200])
	}
	if _, err := os.Stat(target); err == nil {
		t.Error("dry-run should not create file")
	}
}

func TestApply_WritesPlistAtomic(t *testing.T) {
	target := filepath.Join(t.TempDir(), "claude.plist")
	p := buildValidProfile(t)
	res, err := Apply(p, ApplyOptions{TargetPath: target})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if res.BytesWritten == 0 {
		t.Error("BytesWritten should be nonzero")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(data), "<key>inferenceProvider</key>") {
		t.Errorf("written file missing content: %s", string(data))
	}
	// No lingering temp file in the directory.
	entries, _ := os.ReadDir(filepath.Dir(target))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".cowork-mdm.") {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
}

func TestApply_RejectsInvalidProfile(t *testing.T) {
	target := filepath.Join(t.TempDir(), "claude.plist")
	p := profile.New("bad")
	p.AttachUnknownKey("doesNotExist", "nope")
	_, err := Apply(p, ApplyOptions{TargetPath: target})
	if err == nil {
		t.Fatal("Apply on profile with unknown key should fail")
	}
	if _, statErr := os.Stat(target); statErr == nil {
		t.Error("file should not have been created")
	}
}

func TestApply_NoWarningForCustomTarget(t *testing.T) {
	target := filepath.Join(t.TempDir(), "claude.plist")
	p := buildValidProfile(t)
	res, err := Apply(p, ApplyOptions{TargetPath: target, DryRun: true})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(res.Warnings) != 0 {
		t.Errorf("custom target should not produce direct-write warning, got %v", res.Warnings)
	}
}

func TestStatus_MissingFileReportsNotPresent(t *testing.T) {
	target := filepath.Join(t.TempDir(), "missing.plist")
	rep, err := Status(StatusOptions{SourcePath: target})
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if rep.Present {
		t.Error("Present should be false when file is absent")
	}
	if rep.Profile != nil {
		t.Error("Profile should be nil when file is absent")
	}
}

func TestStatus_ReadsWrittenProfile(t *testing.T) {
	target := filepath.Join(t.TempDir(), "claude.plist")
	p := buildValidProfile(t)
	if _, err := Apply(p, ApplyOptions{TargetPath: target}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	rep, err := Status(StatusOptions{SourcePath: target})
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !rep.Present {
		t.Fatal("Status should report Present=true after Apply")
	}
	if rep.Profile == nil {
		t.Fatal("Profile should be non-nil")
	}
	if v, _ := rep.Profile.Get("inferenceProvider"); v != "bedrock" {
		t.Errorf("inferenceProvider = %v, want bedrock", v)
	}
	if len(rep.UnknownKeys) != 0 {
		t.Errorf("unexpected unknown keys: %v", rep.UnknownKeys)
	}
}

func TestApply_PermissionMappedFromWriteError(t *testing.T) {
	// Point Apply at a file inside a directory we can't write to. Then
	// the atomic write fails with EACCES and Apply must wrap it as
	// ErrPermission.
	if os.Geteuid() == 0 {
		t.Skip("running as root; permission test skipped")
	}
	readonlyDir := t.TempDir()
	if err := os.Chmod(readonlyDir, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(readonlyDir, 0o755) })
	target := filepath.Join(readonlyDir, "claude.plist")
	p := buildValidProfile(t)
	_, err := Apply(p, ApplyOptions{TargetPath: target})
	if err == nil {
		t.Fatal("Apply to read-only dir should fail")
	}
	if !errors.Is(err, ErrPermission) {
		t.Errorf("expected ErrPermission, got %v", err)
	}
}
