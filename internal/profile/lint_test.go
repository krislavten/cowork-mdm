package profile

import (
	"strings"
	"testing"
)

func TestLintPlaceholders_EnterpriseCNDirty(t *testing.T) {
	// enterprise-cn-full ships with REPLACE_* residuals by design —
	// it's a scaffold, not a deployable profile. Lint must flag them.
	p, err := LoadTemplate("enterprise-cn-full")
	if err != nil {
		t.Fatalf("LoadTemplate: %v", err)
	}
	findings := LintPlaceholders(p)
	if len(findings) == 0 {
		t.Fatal("expected REPLACE_* findings in enterprise-cn-full scaffold, got none")
	}
	// Sanity: the critical LLM-auth key must be flagged.
	var apiKeyFlagged bool
	for _, f := range findings {
		if f.Key == "inferenceGatewayApiKey" && f.Match == "REPLACE_WITH_YOUR_API_KEY" {
			apiKeyFlagged = true
		}
	}
	if !apiKeyFlagged {
		t.Errorf("inferenceGatewayApiKey REPLACE_WITH_YOUR_API_KEY was not flagged; findings=%v", findings)
	}
}

func TestLintPlaceholders_BedrockBasicClean(t *testing.T) {
	// bedrock-basic uses {{ACCOUNT}} / {{OPUS_PROFILE_ID}} style
	// mustache-delimited placeholders that are NOT the REPLACE_*
	// convention. Lint must not flag them — that's the scope boundary.
	// The {{…}} delimiters are admin-visible tokens safe for sed /
	// envsubst substitution; they're documented, not residual.
	p, err := LoadTemplate("bedrock-basic")
	if err != nil {
		t.Fatalf("LoadTemplate: %v", err)
	}
	findings := LintPlaceholders(p)
	if len(findings) != 0 {
		t.Errorf("bedrock-basic should have zero REPLACE_* findings (its variables use {{ACCOUNT}} / {{PROFILE_ID}} style), got %d: %v",
			len(findings), findings)
	}
}

func TestLintPlaceholders_MatchesWithDigits(t *testing.T) {
	// Regression guard for the regex — \bREPLACE_[A-Z0-9_]+\b must
	// match trailing digits (REPLACE_WITH_MODEL_ID_1 appears in
	// enterprise-cn-full's inferenceModels list).
	p, err := LoadTemplate("enterprise-cn-full")
	if err != nil {
		t.Fatalf("LoadTemplate: %v", err)
	}
	findings := LintPlaceholders(p)
	var sawDigit bool
	for _, f := range findings {
		if strings.HasSuffix(f.Match, "_1") || strings.HasSuffix(f.Match, "_2") {
			sawDigit = true
		}
	}
	if !sawDigit {
		t.Errorf("expected at least one digit-suffixed match (REPLACE_WITH_MODEL_ID_1/_2); got %v", findings)
	}
}

func TestLintPlaceholders_CleanAfterFilling(t *testing.T) {
	// If a user fills in all placeholders, lint returns zero findings.
	p, err := LoadTemplate("enterprise-cn-full")
	if err != nil {
		t.Fatalf("LoadTemplate: %v", err)
	}
	for _, k := range []string{
		"inferenceGatewayBaseUrl",
		"inferenceGatewayApiKey",
	} {
		if err := p.Set(k, "https://actual-value.example.com"); err != nil {
			t.Fatalf("Set %s: %v", k, err)
		}
	}
	// Models + MCP + egress still contain placeholders, so findings
	// should still be non-zero. But the two we filled above must not
	// appear.
	findings := LintPlaceholders(p)
	for _, f := range findings {
		if f.Key == "inferenceGatewayBaseUrl" || f.Key == "inferenceGatewayApiKey" {
			t.Errorf("filled key %s still flagged: %+v", f.Key, f)
		}
	}
}

func TestLintPlaceholders_JsonStringNested(t *testing.T) {
	// managedMcpServers is a jsonString — the REPLACE_ME match is
	// inside a JSON-formatted string. Confirm lint sees it.
	p, err := LoadTemplate("enterprise-cn-full")
	if err != nil {
		t.Fatalf("LoadTemplate: %v", err)
	}
	findings := LintPlaceholders(p)
	var sawMCP bool
	for _, f := range findings {
		if f.Key == "managedMcpServers" && strings.HasPrefix(f.Match, "REPLACE_") {
			sawMCP = true
		}
	}
	if !sawMCP {
		t.Errorf("expected managedMcpServers to surface REPLACE_ME findings; got %v", findings)
	}
}

func TestReadTemplateSource_EnterpriseCN(t *testing.T) {
	b, err := ReadTemplateSource("enterprise-cn-full")
	if err != nil {
		t.Fatalf("ReadTemplateSource: %v", err)
	}
	if !strings.HasPrefix(string(b), "name: enterprise-cn-full") {
		t.Errorf("expected YAML to start with 'name: enterprise-cn-full', got %q", string(b[:50]))
	}
	if !strings.Contains(string(b), "REPLACE_WITH_YOUR_API_KEY") {
		t.Errorf("expected placeholder in source; got none")
	}
}

func TestReadTemplateSource_Unknown(t *testing.T) {
	_, err := ReadTemplateSource("does-not-exist")
	if err == nil {
		t.Fatal("expected error for unknown template")
	}
	if !strings.Contains(err.Error(), "bedrock-basic") {
		t.Errorf("error should list available templates, got: %s", err)
	}
}

func TestFormatFindings(t *testing.T) {
	empty := FormatFindings(nil)
	if !strings.Contains(empty, "no placeholder") {
		t.Errorf("empty findings format unclear: %q", empty)
	}
	two := FormatFindings([]PlaceholderFinding{
		{Key: "inferenceGatewayApiKey", Match: "REPLACE_WITH_YOUR_API_KEY"},
		{Key: "managedMcpServers", Match: "REPLACE_ME"},
	})
	if !strings.Contains(two, "2 placeholder") {
		t.Errorf("expected count in output, got %q", two)
	}
}
