package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMarketplaceHelp_Mentions(t *testing.T) {
	out, _, err := runCmd(t, "marketplace", "--help")
	if err != nil {
		t.Fatalf("marketplace --help: %v", err)
	}
	for _, want := range []string{"add", "list", "update", "remove", "link"} {
		if !strings.Contains(out, want) {
			t.Errorf("marketplace help missing %q", want)
		}
	}
}

func TestPluginHelp_Mentions(t *testing.T) {
	out, _, err := runCmd(t, "plugin", "--help")
	if err != nil {
		t.Fatalf("plugin --help: %v", err)
	}
	for _, want := range []string{"list", "show", "unlink", "prune"} {
		if !strings.Contains(out, want) {
			t.Errorf("plugin help missing %q", want)
		}
	}
}

func TestDoctorHelp_Mentions(t *testing.T) {
	out, _, err := runCmd(t, "doctor", "--help")
	if err != nil {
		t.Fatalf("doctor --help: %v", err)
	}
	for _, want := range []string{"--fix", "--json"} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor help missing %q", want)
		}
	}
}

func TestMarketplaceList_OnHostEmpty(t *testing.T) {
	// On a fresh CI runner the default org-plugins/ dir likely doesn't
	// exist; marketplace list should gracefully produce the "no
	// marketplaces installed" message rather than erroring.
	out, _, err := runCmd(t, "marketplace", "list")
	if err != nil {
		t.Fatalf("marketplace list: %v", err)
	}
	// Either "no marketplaces installed" or a table header — both are fine.
	if !strings.Contains(out, "no marketplaces") && !strings.Contains(out, "NAME") {
		t.Errorf("unexpected marketplace list output: %s", out)
	}
}

func TestPluginList_OnHostEmpty(t *testing.T) {
	out, _, err := runCmd(t, "plugin", "list")
	if err != nil {
		t.Fatalf("plugin list: %v", err)
	}
	if !strings.Contains(out, "no plugins") && !strings.Contains(out, "NAME") {
		t.Errorf("unexpected plugin list output: %s", out)
	}
}

func TestSkillHelp_Mentions(t *testing.T) {
	out, _, err := runCmd(t, "skill", "--help")
	if err != nil {
		t.Fatalf("skill --help: %v", err)
	}
	if !strings.Contains(out, "pack") {
		t.Errorf("skill help missing 'pack' subcommand: %s", out)
	}
}

func TestSkillPackHelp_RequiredFlags(t *testing.T) {
	out, _, err := runCmd(t, "skill", "pack", "--help")
	if err != nil {
		t.Fatalf("skill pack --help: %v", err)
	}
	for _, want := range []string{"--name", "--out", "--version", "--force"} {
		if !strings.Contains(out, want) {
			t.Errorf("skill pack help missing flag %q", want)
		}
	}
}

func TestSkillPack_HappyPath(t *testing.T) {
	in := t.TempDir()
	skillDir := filepath.Join(in, "hello")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"),
		[]byte("---\nname: hello\ndescription: says hi\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "bundle")

	stdout, _, err := runCmd(t, "skill", "pack", in,
		"--name", "hello-skills",
		"--out", out,
		"--json",
	)
	if err != nil {
		t.Fatalf("skill pack failed: %v", err)
	}
	var res struct {
		BundleDir string `json:"BundleDir"`
		Skills    []struct {
			Name string `json:"Name"`
		} `json:"Skills"`
	}
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("skill pack --json output not JSON: %v\n%s", err, stdout)
	}
	if len(res.Skills) != 1 || res.Skills[0].Name != "hello" {
		t.Errorf("unexpected skills: %+v", res.Skills)
	}
	if _, err := os.Stat(filepath.Join(out, ".claude-plugin", "plugin.json")); err != nil {
		t.Errorf("plugin.json not produced: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "skills", "hello", "SKILL.md")); err != nil {
		t.Errorf("skill not copied into bundle: %v", err)
	}
}

func TestSkillPack_RejectsInvalidName(t *testing.T) {
	in := t.TempDir()
	if err := os.MkdirAll(filepath.Join(in, "s"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(in, "s", "SKILL.md"),
		[]byte("---\nname: s\ndescription: d\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "bundle")
	_, _, err := runCmd(t, "skill", "pack", in, "--name", "BadName", "--out", out)
	if err == nil {
		t.Error("skill pack with invalid name should fail")
	}
}
