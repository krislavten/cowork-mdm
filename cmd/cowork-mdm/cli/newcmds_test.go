package cli

import (
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
