package profile

import (
	"os"
	"path/filepath"
	"testing"
)

// Not a real test — writes the golden file when run with -run GoldenGen.
// Usage: go test -run TestGoldenGen ./internal/profile/...
// Commit the resulting file; delete this file after the golden is stable,
// or keep it to make regeneration a one-liner.
func TestGoldenGen(t *testing.T) {
	if os.Getenv("GEN_GOLDEN") != "1" {
		t.Skip("set GEN_GOLDEN=1 to (re)generate testdata/bedrock-basic.golden.mobileconfig")
	}
	p, err := LoadTemplate("bedrock-basic")
	if err != nil {
		t.Fatalf("LoadTemplate: %v", err)
	}
	out, err := EncodeMobileConfig(p, MobileConfigOpts{
		ConfigurationPayloadUUID: "00000000-0000-4000-8000-000000000000",
		PayloadUUID:              "11111111-1111-4111-8111-111111111111",
		PayloadIdentifier:        "com.example.cowork-mdm.bedrock-basic",
	})
	if err != nil {
		t.Fatalf("EncodeMobileConfig: %v", err)
	}
	path := filepath.Join("testdata", "bedrock-basic.golden.mobileconfig")
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Logf("wrote %s (%d bytes)", path, len(out))
}
