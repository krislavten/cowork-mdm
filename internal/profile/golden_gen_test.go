package profile

import (
	"os"
	"path/filepath"
	"testing"
)

// Not a real test — writes the golden files when run with -run GoldenGen.
// Usage: GEN_GOLDEN=1 go test -run TestGoldenGen ./internal/profile/...
// Commit the resulting files; regenerate whenever the template or encoder
// shape changes intentionally.
func TestGoldenGen(t *testing.T) {
	if os.Getenv("GEN_GOLDEN") != "1" {
		t.Skip("set GEN_GOLDEN=1 to (re)generate testdata/*.golden.mobileconfig")
	}
	cases := []struct {
		template  string
		file      string
		payloadID string
	}{
		{"bedrock-basic", "bedrock-basic.golden.mobileconfig", "com.example.cowork-mdm.bedrock-basic"},
		{"gateway-deepseek", "gateway-deepseek.golden.mobileconfig", "com.example.cowork-mdm.gateway-deepseek"},
	}
	for _, tc := range cases {
		p, err := LoadTemplate(tc.template)
		if err != nil {
			t.Fatalf("LoadTemplate %s: %v", tc.template, err)
		}
		out, err := EncodeMobileConfig(p, MobileConfigOpts{
			ConfigurationPayloadUUID: "00000000-0000-4000-8000-000000000000",
			PayloadUUID:              "11111111-1111-4111-8111-111111111111",
			PayloadIdentifier:        tc.payloadID,
		})
		if err != nil {
			t.Fatalf("EncodeMobileConfig %s: %v", tc.template, err)
		}
		path := filepath.Join("testdata", tc.file)
		if err := os.WriteFile(path, out, 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", path, err)
		}
		t.Logf("wrote %s (%d bytes)", path, len(out))
	}
}
