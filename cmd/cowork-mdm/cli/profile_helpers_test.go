package cli

import (
	"strings"
	"testing"
)

func TestFormatStatusValue_RedactsSensitive(t *testing.T) {
	// inferenceGatewayApiKey is schema-marked Sensitive.
	got := formatStatusValue("inferenceGatewayApiKey", "super-secret-token", false)
	if strings.Contains(got, "super-secret-token") {
		t.Errorf("sensitive value leaked: %q", got)
	}
	if !strings.Contains(got, "<redacted>") {
		t.Errorf("expected <redacted> marker, got %q", got)
	}
	if !strings.Contains(got, "--unmasked") {
		t.Errorf("expected hint about --unmasked flag, got %q", got)
	}
}

func TestFormatStatusValue_UnmaskedShowsRaw(t *testing.T) {
	got := formatStatusValue("inferenceGatewayApiKey", "super-secret-token", true)
	if got != "super-secret-token" {
		t.Errorf("unmasked should show raw value, got %q", got)
	}
}

func TestFormatStatusValue_NonSensitivePassesThrough(t *testing.T) {
	// inferenceProvider is NOT Sensitive in schema.
	got := formatStatusValue("inferenceProvider", "bedrock", false)
	if got != "bedrock" {
		t.Errorf("non-sensitive value should pass through, got %q", got)
	}
}

func TestFormatStatusValue_UnknownKeyPassesThrough(t *testing.T) {
	// Unknown key: schema.Find returns nil → pass through. Transparent
	// debugging beats false safety.
	got := formatStatusValue("madeUpKey", "some-value", false)
	if got != "some-value" {
		t.Errorf("unknown key should pass through, got %q", got)
	}
}

func TestFormatStatusValue_SensitiveWithComplexValue(t *testing.T) {
	// managedMcpServers is Sensitive and its value is a complex jsonString.
	// Redaction must still trigger even for non-scalar values.
	mcp := `[{"name":"x","url":"https://mcp.internal/?Authorization=abc123"}]`
	got := formatStatusValue("managedMcpServers", mcp, false)
	if strings.Contains(got, "Authorization=abc123") {
		t.Errorf("embedded secret leaked despite sensitive marker: %q", got)
	}
	if !strings.Contains(got, "<redacted>") {
		t.Errorf("expected <redacted>, got %q", got)
	}
}
