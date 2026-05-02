package profile

import (
	"regexp"
	"strings"
	"testing"
)

func TestTemplateNames_IncludesAll(t *testing.T) {
	names := TemplateNames()
	want := []string{
		"bedrock-basic",
		"enterprise-cn-full",
		"foundry",
		"gateway",
		"gateway-deepseek",
		"gateway-glm",
		"gateway-minimax",
		"mcp-only",
		"vertex",
	}
	if len(names) != len(want) {
		t.Fatalf("TemplateNames() = %v, want %v", names, want)
	}
	for i, n := range want {
		if names[i] != n {
			t.Errorf("TemplateNames()[%d] = %q, want %q", i, names[i], n)
		}
	}
}

func TestLoadTemplate_UnknownErrors(t *testing.T) {
	_, err := LoadTemplate("does-not-exist")
	if err == nil {
		t.Fatal("LoadTemplate on unknown name should error")
	}
	// error should list available templates to help the user
	if !strings.Contains(err.Error(), "bedrock-basic") {
		t.Errorf("error should list available templates, got: %s", err)
	}
}

func TestLoadTemplate_BedrockBasic(t *testing.T) {
	p, err := LoadTemplate("bedrock-basic")
	if err != nil {
		t.Fatalf("LoadTemplate bedrock-basic: %v", err)
	}
	if p.Name != "bedrock-basic" {
		t.Errorf("Name = %q", p.Name)
	}
	if v, _ := p.Get("inferenceProvider"); v != "bedrock" {
		t.Errorf("inferenceProvider = %v", v)
	}
	if v, _ := p.Get("inferenceBedrockRegion"); v != "us-west-2" {
		t.Errorf("inferenceBedrockRegion = %v", v)
	}
	// Placeholder ARN is an obvious marker — don't let templates leak real ARNs.
	v, _ := p.Get("inferenceModels")
	arr, ok := v.([]string)
	if !ok {
		t.Fatalf("inferenceModels should be []string after template load, got %T: %v", v, v)
	}
	joined := strings.Join(arr, " ")
	if !strings.Contains(joined, "{{ACCOUNT}}") {
		t.Errorf("inferenceModels should contain {{ACCOUNT}} placeholder, got %v", arr)
	}
}

func TestLoadTemplate_AllTemplatesValidate(t *testing.T) {
	for _, name := range TemplateNames() {
		t.Run(name, func(t *testing.T) {
			p, err := LoadTemplate(name)
			if err != nil {
				t.Fatalf("LoadTemplate %s: %v", name, err)
			}
			// All keys set via LoadTemplate pass through Profile.Set which
			// validates against the schema, so reaching here is implicit
			// schema compliance. Additional: Validate() should succeed
			// even for placeholder values, because placeholders are strings
			// of the correct shape.
			if err := p.Validate(); err != nil {
				t.Errorf("template %s has validation errors: %v", name, err)
			}
		})
	}
}

func TestLoadTemplate_MCPOnlyDoesNotLeakRealData(t *testing.T) {
	// Regression guard: mcp-only template is a documented skeleton; it
	// must NOT contain any real MCP URL with embedded token-looking
	// query parameters (Authorization=..., token=..., etc.).
	p, err := LoadTemplate("mcp-only")
	if err != nil {
		t.Fatal(err)
	}
	v, _ := p.Get("managedMcpServers")
	s := v.(string)
	for _, tok := range []string{"Authorization=", "access_token=", "@bigmodel", "@anthropic"} {
		if strings.Contains(s, tok) {
			t.Errorf("mcp-only template leaks data containing %q — check templates/mcp-only.yaml", tok)
		}
	}
	if !strings.Contains(s, "REPLACE_ME") {
		t.Errorf("mcp-only template should contain REPLACE_ME placeholder, got %s", s)
	}
}

func TestLoadTemplateFile_FromRawYAML(t *testing.T) {
	raw := []byte(`
name: custom
description: test
values:
  inferenceProvider: vertex
  inferenceVertexProjectId: my-project
  inferenceVertexRegion: us-east5
  inferenceModels: '["claude-opus-4"]'
`)
	p, err := LoadTemplateFile(raw)
	if err != nil {
		t.Fatalf("LoadTemplateFile: %v", err)
	}
	if v, _ := p.Get("inferenceProvider"); v != "vertex" {
		t.Errorf("inferenceProvider = %v", v)
	}
	if v, _ := p.Get("inferenceVertexProjectId"); v != "my-project" {
		t.Errorf("projectId = %v", v)
	}
}

func TestLoadTemplateFile_MissingName(t *testing.T) {
	raw := []byte(`description: bad
values:
  inferenceProvider: bedrock
`)
	_, err := LoadTemplateFile(raw)
	if err == nil || !strings.Contains(err.Error(), "name") {
		t.Errorf("expected name-missing error, got %v", err)
	}
}

func TestLoadTemplate_GatewayVendors(t *testing.T) {
	cases := []struct {
		name       string
		baseURL    string
		authScheme string
	}{
		{"gateway-deepseek", "https://api.deepseek.com/anthropic", "x-api-key"},
		{"gateway-glm", "https://open.bigmodel.cn/api/anthropic", "bearer"},
		{"gateway-minimax", "https://api.minimaxi.com/anthropic", "x-api-key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := LoadTemplate(tc.name)
			if err != nil {
				t.Fatalf("LoadTemplate %s: %v", tc.name, err)
			}
			if v, _ := p.Get("inferenceProvider"); v != "gateway" {
				t.Errorf("inferenceProvider = %v, want gateway", v)
			}
			if v, _ := p.Get("inferenceGatewayBaseUrl"); v != tc.baseURL {
				t.Errorf("inferenceGatewayBaseUrl = %v, want %q", v, tc.baseURL)
			}
			if v, _ := p.Get("inferenceGatewayAuthScheme"); v != tc.authScheme {
				t.Errorf("inferenceGatewayAuthScheme = %v, want %q", v, tc.authScheme)
			}
			// API key must be the placeholder — regression guard against
			// accidentally committing a real vendor key to a shipped template.
			v, _ := p.Get("inferenceGatewayApiKey")
			key, _ := v.(string)
			if !strings.Contains(key, "REPLACE_WITH_YOUR_API_KEY") {
				t.Errorf("inferenceGatewayApiKey = %q, want REPLACE_WITH_YOUR_API_KEY placeholder", key)
			}
		})
	}
}

func TestLoadTemplate_NoLeakedKeysInGatewayTemplates(t *testing.T) {
	// Regression guard: the three CN-vendor gateway templates must never
	// carry a real key. Scan the raw embedded YAML for key-shaped patterns.
	// Each regex targets a concrete vendor-key prefix + enough entropy to
	// be unmistakably a real secret, not a prose mention (e.g. "sk-" alone
	// would false-positive on words like "sk-SNAPSHOT" in a comment).
	leakPatterns := []*regexp.Regexp{
		regexp.MustCompile(`sk-[A-Za-z0-9_\-]{20,}`),         // OpenAI / DeepSeek / Anthropic test keys
		regexp.MustCompile(`Bearer\s+ey[A-Za-z0-9_\-]{20,}`), // JWT literal
		regexp.MustCompile(`xai-[A-Za-z0-9]{20,}`),           // xAI
		regexp.MustCompile(`gsk_[A-Za-z0-9]{20,}`),           // Groq
	}
	for _, name := range []string{"gateway-deepseek", "gateway-glm", "gateway-minimax", "enterprise-cn-full"} {
		t.Run(name, func(t *testing.T) {
			raw, err := templateFS.ReadFile("templates/" + name + ".yaml")
			if err != nil {
				t.Fatalf("read template: %v", err)
			}
			text := string(raw)
			if !strings.Contains(text, "REPLACE_WITH_YOUR_API_KEY") {
				t.Errorf("%s must contain REPLACE_WITH_YOUR_API_KEY placeholder", name)
			}
			for _, re := range leakPatterns {
				if m := re.FindString(text); m != "" {
					t.Errorf("%s contains leaked-key match %q — review template before shipping", name, m)
				}
			}
		})
	}
}

func TestLoadTemplate_EnterpriseCN(t *testing.T) {
	p, err := LoadTemplate("enterprise-cn-full")
	if err != nil {
		t.Fatalf("LoadTemplate enterprise-cn-full: %v", err)
	}
	if v, _ := p.Get("inferenceProvider"); v != "gateway" {
		t.Errorf("inferenceProvider = %v, want gateway", v)
	}
	if v, _ := p.Get("autoUpdaterEnforcementHours"); v != int64(168) {
		t.Errorf("autoUpdaterEnforcementHours = %v (%T), want int64(168)", v, v)
	}
	if v, _ := p.Get("disableNonessentialTelemetry"); v != true {
		t.Errorf("disableNonessentialTelemetry = %v, want true", v)
	}
	if v, _ := p.Get("disableDeploymentModeChooser"); v != true {
		t.Errorf("disableDeploymentModeChooser = %v, want true", v)
	}
	mcp, _ := p.Get("managedMcpServers")
	if s, ok := mcp.(string); !ok || !strings.Contains(s, "REPLACE_ME") {
		t.Errorf("managedMcpServers should contain REPLACE_ME placeholder, got %v", mcp)
	}
	egress, _ := p.Get("coworkEgressAllowedHosts")
	arr, ok := egress.([]string)
	if !ok {
		t.Fatalf("coworkEgressAllowedHosts should be []string, got %T", egress)
	}
	if len(arr) == 0 || !strings.Contains(arr[0], "REPLACE_WITH_YOUR_INTERNAL_DOMAIN") {
		t.Errorf("coworkEgressAllowedHosts should contain internal-domain placeholder, got %v", arr)
	}
}
