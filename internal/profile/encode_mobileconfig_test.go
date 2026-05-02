package profile

import (
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

func TestPayloadIdentifier_Precedence(t *testing.T) {
	p, err := LoadTemplate("bedrock-basic")
	if err != nil {
		t.Fatalf("LoadTemplate: %v", err)
	}

	t.Run("default prefix when nothing set", func(t *testing.T) {
		t.Setenv(EnvPayloadIdentifierPrefix, "")
		out, err := EncodeMobileConfig(p, MobileConfigOpts{})
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		want := DefaultPayloadIdentifierPrefix + "." + slugify(p.Name)
		if !strings.Contains(string(out), want) {
			t.Errorf("default PayloadIdentifier %q not in output", want)
		}
	})

	t.Run("env var overrides default", func(t *testing.T) {
		t.Setenv(EnvPayloadIdentifierPrefix, "com.env.acme")
		out, err := EncodeMobileConfig(p, MobileConfigOpts{})
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		want := "com.env.acme." + slugify(p.Name)
		if !strings.Contains(string(out), want) {
			t.Errorf("env-var PayloadIdentifier %q not in output", want)
		}
	})

	t.Run("opts prefix overrides env var", func(t *testing.T) {
		t.Setenv(EnvPayloadIdentifierPrefix, "com.env.acme")
		out, err := EncodeMobileConfig(p, MobileConfigOpts{
			PayloadIdentifierPrefix: "com.flag.beta",
		})
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		want := "com.flag.beta." + slugify(p.Name)
		if !strings.Contains(string(out), want) {
			t.Errorf("opts PayloadIdentifierPrefix %q not in output", want)
		}
		if strings.Contains(string(out), "com.env.acme") {
			t.Errorf("opts prefix should have overridden env var but env still present")
		}
	})

	t.Run("full identifier wins over prefix sources", func(t *testing.T) {
		t.Setenv(EnvPayloadIdentifierPrefix, "com.env.acme")
		out, err := EncodeMobileConfig(p, MobileConfigOpts{
			PayloadIdentifier:       "com.explicit.full.id",
			PayloadIdentifierPrefix: "com.flag.beta",
		})
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		if !strings.Contains(string(out), "com.explicit.full.id") {
			t.Errorf("explicit PayloadIdentifier not in output")
		}
		if strings.Contains(string(out), "com.flag.beta") || strings.Contains(string(out), "com.env.acme") {
			t.Errorf("explicit identifier should be sole value; prefix/env leaked through")
		}
	})

	t.Run("no legacy com.yuanli in default output", func(t *testing.T) {
		t.Setenv(EnvPayloadIdentifierPrefix, "")
		out, err := EncodeMobileConfig(p, MobileConfigOpts{})
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		if strings.Contains(string(out), "com.yuanli") {
			t.Errorf("output still contains legacy 'com.yuanli' prefix — default should now be %q", DefaultPayloadIdentifierPrefix)
		}
	})
}

// buildBedrockProfile returns a profile resembling the handcrafted plists
// we used to write for real-world deployments — provider + region + profile
// + aws dir + model list + a minimal MCP server with tool policy.
func buildBedrockProfile(t *testing.T) *Profile {
	t.Helper()
	p := New("cowork-mdm-test")
	set := func(k string, v any) {
		if err := p.Set(k, v); err != nil {
			t.Fatalf("Set %s: %v", k, err)
		}
	}
	set("inferenceProvider", "bedrock")
	set("disableDeploymentModeChooser", true)
	set("inferenceBedrockRegion", "us-west-2")
	set("inferenceBedrockProfile", "default")
	set("inferenceBedrockAwsDir", "/Users/testuser/.aws")
	set("coworkEgressAllowedHosts", []string{"*"})
	set("inferenceModels", []string{
		"arn:aws:bedrock:us-west-2:111111111111:application-inference-profile/AAAAAAAAAAAA",
		"arn:aws:bedrock:us-west-2:111111111111:application-inference-profile/BBBBBBBBBBBB[1m]",
	})
	set("managedMcpServers", `[{"name":"example-mcp","url":"https://mcp.example.com/stream","transport":"http","toolPolicy":{"exampleTool":"allow"}}]`)
	return p
}

func TestEncodeMobileConfig_StructuralInvariants(t *testing.T) {
	p := buildBedrockProfile(t)
	out, err := EncodeMobileConfig(p, MobileConfigOpts{
		ConfigurationPayloadUUID: "00000000-0000-4000-8000-000000000000",
		PayloadUUID:              "11111111-1111-4111-8111-111111111111",
	})
	if err != nil {
		t.Fatalf("EncodeMobileConfig: %v", err)
	}
	s := string(out)

	// mobileconfig envelope
	for _, want := range []string{
		"<!DOCTYPE plist", "<plist version=\"1.0\">",
		"<key>PayloadContent</key>", "<key>PayloadType</key>",
		"<string>Configuration</string>",
		"<string>com.anthropic.claudefordesktop</string>",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing expected fragment: %s", want)
		}
	}

	// Per-key type mapping invariants
	if !strings.Contains(s, "<key>inferenceProvider</key>\n\t\t\t<string>bedrock</string>") {
		t.Errorf("enum value not emitted as <string>: see\n%s", s)
	}
	if !strings.Contains(s, "<key>disableDeploymentModeChooser</key>\n\t\t\t<true/>") {
		t.Errorf("boolean true not emitted as <true/>")
	}
	// stringArray becomes JSON-in-string — must NOT be a <array> element.
	if strings.Contains(s, "<key>coworkEgressAllowedHosts</key>\n\t\t\t<array>") {
		t.Errorf("stringArray should be encoded as <string>, not <array>")
	}
	if !strings.Contains(s, `<key>coworkEgressAllowedHosts</key>`) {
		t.Errorf("egress key missing")
	}
	if !strings.Contains(s, `[&quot;*&quot;]`) {
		t.Errorf(`egress JSON quote escaping wrong — expected &quot;*&quot; in XML, got:\n%s`, s)
	}
	// jsonString (managedMcpServers) preserves its raw JSON verbatim
	if !strings.Contains(s, "example-mcp") {
		t.Errorf("managedMcpServers content missing")
	}

	// The outer payload UUID we requested
	if !strings.Contains(s, "00000000-0000-4000-8000-000000000000") {
		t.Errorf("requested outer UUID not honored")
	}

	// PayloadContent array has exactly one inner dict (the claude one)
	if strings.Count(s, "<key>PayloadType</key>") != 2 {
		t.Errorf("expected PayloadType to appear twice (inner+outer), got %d",
			strings.Count(s, "<key>PayloadType</key>"))
	}
}

// TestEncodeMobileConfig_PlutilLint confirms the output is accepted by
// macOS's plutil -lint. Runs only on darwin where plutil is present.
func TestEncodeMobileConfig_PlutilLint(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skipf("plutil only on darwin, got %s", runtime.GOOS)
	}
	if _, err := exec.LookPath("plutil"); err != nil {
		t.Skip("plutil not in PATH")
	}
	p := buildBedrockProfile(t)
	out, err := EncodeMobileConfig(p, MobileConfigOpts{})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	tmp := t.TempDir() + "/test.mobileconfig"
	if err := writeFile(tmp, out); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	cmd := exec.Command("plutil", "-lint", tmp)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("plutil -lint failed: %v\noutput: %s\nfile:\n%s",
			err, string(output), string(out))
	}
}

func TestEncodePlist_StructuralInvariants(t *testing.T) {
	p := buildBedrockProfile(t)
	out, err := EncodePlist(p)
	if err != nil {
		t.Fatalf("EncodePlist: %v", err)
	}
	s := string(out)

	// plist body only, no mobileconfig wrapper
	if strings.Contains(s, "PayloadContent") || strings.Contains(s, "PayloadType") {
		t.Errorf("EncodePlist must not emit Apple payload wrapper keys")
	}
	// but it must still have every MDM key
	for _, want := range []string{
		"<key>inferenceProvider</key>",
		"<key>inferenceBedrockRegion</key>",
		"<key>inferenceModels</key>",
		"<key>managedMcpServers</key>",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s", want)
		}
	}
}

func TestEncodePlist_PlutilLint(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skipf("plutil only on darwin, got %s", runtime.GOOS)
	}
	if _, err := exec.LookPath("plutil"); err != nil {
		t.Skip("plutil not in PATH")
	}
	p := buildBedrockProfile(t)
	out, err := EncodePlist(p)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	tmp := t.TempDir() + "/test.plist"
	if err := writeFile(tmp, out); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	cmd := exec.Command("plutil", "-lint", tmp)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("plutil -lint failed: %v\noutput: %s\nfile:\n%s",
			err, string(output), string(out))
	}
}

func TestEncodeMobileConfig_GeneratesDifferentUUIDsEachCall(t *testing.T) {
	p := buildBedrockProfile(t)
	a, err := EncodeMobileConfig(p, MobileConfigOpts{})
	if err != nil {
		t.Fatalf("encode a: %v", err)
	}
	b, err := EncodeMobileConfig(p, MobileConfigOpts{})
	if err != nil {
		t.Fatalf("encode b: %v", err)
	}
	// Deliberately: outputs should differ only in PayloadUUID fields.
	// Simpler assertion: not byte-identical.
	if string(a) == string(b) {
		t.Error("two EncodeMobileConfig calls with default opts produced byte-identical output; expected UUIDs to differ")
	}
}
