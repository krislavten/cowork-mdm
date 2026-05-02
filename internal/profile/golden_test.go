package profile

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// TestEncodeMobileConfig_GoldenBedrockBasic pins the byte-exact output of
// EncodeMobileConfig for the bedrock-basic template with fixed UUIDs.
// This catches any accidental drift in XML formatting or key ordering.
//
// Regenerate via: GEN_GOLDEN=1 go test -run TestGoldenGen ./internal/profile/...
func TestEncodeMobileConfig_GoldenBedrockBasic(t *testing.T) {
	path := filepath.Join("testdata", "bedrock-basic.golden.mobileconfig")
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v (run GEN_GOLDEN=1 go test -run TestGoldenGen ... to create it)", err)
	}
	p, err := LoadTemplate("bedrock-basic")
	if err != nil {
		t.Fatalf("LoadTemplate: %v", err)
	}
	got, err := EncodeMobileConfig(p, MobileConfigOpts{
		ConfigurationPayloadUUID: "00000000-0000-4000-8000-000000000000",
		PayloadUUID:              "11111111-1111-4111-8111-111111111111",
		PayloadIdentifier:        "com.example.cowork-mdm.bedrock-basic",
	})
	if err != nil {
		t.Fatalf("EncodeMobileConfig: %v", err)
	}
	// Normalize line endings: on Windows runners, git may check out the
	// golden with CRLF even though we ship it as LF, and our encoder
	// always emits LF. Compare LF-normalized so CI stays green without
	// relying on the runner's core.autocrlf config.
	want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
	got = bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n"))
	if !bytes.Equal(want, got) {
		t.Errorf("bedrock-basic golden drift — diff:\nWANT:\n%s\n\nGOT:\n%s",
			string(want), string(got))
	}
}

// TestEncodeMobileConfig_GoldenGatewayDeepseek pins the byte-exact output
// for the gateway-deepseek template. One gateway vendor is enough to lock
// the encoder shape; per-vendor baseURL / auth scheme drift is caught by
// TestLoadTemplate_GatewayVendors at the Profile level.
//
// Regenerate via: GEN_GOLDEN=1 go test -run TestGoldenGen ./internal/profile/...
func TestEncodeMobileConfig_GoldenGatewayDeepseek(t *testing.T) {
	path := filepath.Join("testdata", "gateway-deepseek.golden.mobileconfig")
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v (run GEN_GOLDEN=1 go test -run TestGoldenGen ... to create it)", err)
	}
	p, err := LoadTemplate("gateway-deepseek")
	if err != nil {
		t.Fatalf("LoadTemplate: %v", err)
	}
	got, err := EncodeMobileConfig(p, MobileConfigOpts{
		ConfigurationPayloadUUID: "00000000-0000-4000-8000-000000000000",
		PayloadUUID:              "11111111-1111-4111-8111-111111111111",
		PayloadIdentifier:        "com.example.cowork-mdm.gateway-deepseek",
	})
	if err != nil {
		t.Fatalf("EncodeMobileConfig: %v", err)
	}
	want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
	got = bytes.ReplaceAll(got, []byte("\r\n"), []byte("\n"))
	if !bytes.Equal(want, got) {
		t.Errorf("gateway-deepseek golden drift — diff:\nWANT:\n%s\n\nGOT:\n%s",
			string(want), string(got))
	}
}
