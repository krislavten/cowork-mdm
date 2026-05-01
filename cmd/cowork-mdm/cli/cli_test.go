package cli

import (
	"bytes"
	"strings"
	"testing"
)

func runCmd(t *testing.T, args ...string) (stdout, stderr string, exitErr error) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	root := NewRootCommand(BuildInfo{Version: "test", Commit: "x", Date: "y"}, &outBuf, &errBuf)
	root.SetArgs(args)
	exitErr = root.Execute()
	return outBuf.String(), errBuf.String(), exitErr
}

func TestSchemaList_ContainsKnownKeys(t *testing.T) {
	out, _, err := runCmd(t, "schema", "list")
	if err != nil {
		t.Fatalf("schema list failed: %v", err)
	}
	for _, want := range []string{
		"inferenceProvider", "managedMcpServers", "coworkEgressAllowedHosts",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("schema list output missing %q", want)
		}
	}
	if !strings.Contains(out, "keys total") {
		t.Error("schema list missing summary line")
	}
}

func TestSchemaList_JSON(t *testing.T) {
	out, _, err := runCmd(t, "schema", "list", "--json")
	if err != nil {
		t.Fatalf("schema list --json failed: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "[") {
		t.Errorf("expected JSON array, got: %q", firstN(out, 80))
	}
}

func TestSchemaShow_KnownKey(t *testing.T) {
	out, _, err := runCmd(t, "schema", "show", "inferenceProvider")
	if err != nil {
		t.Fatalf("schema show failed: %v", err)
	}
	for _, want := range []string{"Name:", "inferenceProvider", "bedrock"} {
		if !strings.Contains(out, want) {
			t.Errorf("schema show missing %q", want)
		}
	}
}

func TestSchemaShow_UnknownKey(t *testing.T) {
	_, errOut, err := runCmd(t, "schema", "show", "fooBar")
	if err == nil {
		t.Error("schema show on unknown key should fail")
	}
	if !strings.Contains(errOut, "unknown key") {
		t.Errorf("stderr should mention unknown key, got: %q", errOut)
	}
}

func TestPathsShow_Darwin(t *testing.T) {
	out, _, err := runCmd(t, "paths", "show", "--os", "darwin")
	if err != nil {
		t.Fatalf("paths show failed: %v", err)
	}
	for _, want := range []string{
		"/Library/Managed Preferences",
		"/Library/Application Support/Claude/org-plugins",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("paths show --os darwin missing %q", want)
		}
	}
}

func TestPathsShow_Windows(t *testing.T) {
	out, _, err := runCmd(t, "paths", "show", "--os", "windows")
	if err != nil {
		t.Fatalf("paths show failed: %v", err)
	}
	if !strings.Contains(out, `C:\Program Files\Claude\org-plugins`) {
		t.Errorf("paths show --os windows missing OrgPluginsDir")
	}
	if !strings.Contains(out, `SOFTWARE\Policies\Claude`) {
		t.Errorf("paths show --os windows missing WindowsRegistryPath")
	}
}

func TestPathsShow_JSON(t *testing.T) {
	out, _, err := runCmd(t, "paths", "show", "--os", "darwin", "--json")
	if err != nil {
		t.Fatalf("paths show --json failed: %v", err)
	}
	if !strings.Contains(out, `"OrgPluginsDir"`) {
		t.Errorf("JSON output missing OrgPluginsDir key, got: %q", firstN(out, 120))
	}
}

func TestVersionFlag(t *testing.T) {
	out, _, err := runCmd(t, "--version")
	if err != nil {
		t.Fatalf("--version failed: %v", err)
	}
	if !strings.Contains(out, "test") {
		t.Errorf("--version output should include Version=test, got: %q", out)
	}
}

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
