package profile

import (
	"strings"
	"testing"
)

func TestTemplateNames_IncludesAllFive(t *testing.T) {
	names := TemplateNames()
	want := []string{"bedrock-basic", "foundry", "gateway", "mcp-only", "vertex"}
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
	if !strings.Contains(joined, "ACCOUNT") {
		t.Errorf("inferenceModels should contain ACCOUNT placeholder, got %v", arr)
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
