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
	if !bytes.Equal(want, got) {
		t.Errorf("bedrock-basic golden drift — diff:\nWANT:\n%s\n\nGOT:\n%s",
			string(want), string(got))
	}
}
