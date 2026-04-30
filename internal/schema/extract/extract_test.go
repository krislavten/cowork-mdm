//go:build extract

package main

import (
	"strings"
	"testing"
)

// fakeIndexJS mimics the shape of the minified Claude bundle well
// enough for the parser to exercise every type branch. This is NOT a
// real asar extraction — the smoke test only covers the parser.
const fakeIndexJS = `
// preamble — some other code lives here.
var x = 1;
FJ = me({
  isDesktopExtensionEnabled: nn(Hi(), {
    scopes: ["3p", "1p"],
    title: "Allow desktop extensions",
    description: "Permit users to install local desktop extensions.",
    appMin: "0.14.1",
    legacyAlias: "isDxtEnabled"
  }),
  inferenceProvider: n5(Ds(["bedrock","vertex","gateway","foundry"]), {
    scopes: ["3p", "3p-bootstrap"],
    appMin: "1.2.0",
    title: "Inference provider",
    description: "Selects the inference backend."
  }),
  coworkEgressAllowedHosts: k9(Li(MA()), {
    scopes: ["3p"],
    appMin: "1.3.0",
    title: "Allowed egress hosts",
    description: "Additional hostnames.",
    example: "[\"docs.example.com\"]"
  }),
  managedMcpServers: k3(ASt, {
    scopes: ["3p"],
    appMin: "1.2.0",
    title: "Managed MCP servers",
    description: "JSON array of MCP server configs.",
    sensitive: true,
    example: "[{\"name\":\"x\"}]"
  }),
  otlpEndpoint: k4(YsA, {
    scopes: ["3p", "3p-bootstrap"],
    appMin: "1.4.0",
    title: "OpenTelemetry collector endpoint",
    description: "Base URL of an OpenTelemetry collector."
  }),
  autoUpdaterEnforcementHours: kk(PsA.number().int(), {
    scopes: ["3p", "1p"],
    appMin: "0.14.1",
    title: "Auto-update enforcement window",
    description: "Hours before forced install."
  }),
  otlpHeaders: kz(MA(), {
    scopes: ["3p", "3p-bootstrap"],
    appMin: "1.4.0",
    title: "OpenTelemetry exporter headers",
    description: "Headers sent with every OTLP request.",
    sensitive: true
  })
});
var y = 2;
`

func TestParseSchemaLiteral(t *testing.T) {
	keys, err := parseSchemaLiteral(fakeIndexJS)
	if err != nil {
		t.Fatalf("parseSchemaLiteral: %v", err)
	}
	byName := map[string]extractedKey{}
	for _, k := range keys {
		byName[k.Name] = k
	}

	want := map[string]string{
		"isDesktopExtensionEnabled":   typeBoolean,
		"inferenceProvider":           typeEnum,
		"coworkEgressAllowedHosts":    typeStringArray,
		"managedMcpServers":           typeJSONString,
		"otlpEndpoint":                typeURL,
		"autoUpdaterEnforcementHours": typeInteger,
		"otlpHeaders":                 typeString,
	}
	for name, wantType := range want {
		k, ok := byName[name]
		if !ok {
			t.Errorf("missing extracted key %q", name)
			continue
		}
		if k.Type != wantType {
			t.Errorf("key %q: Type = %q, want %q", name, k.Type, wantType)
		}
	}

	// Spot-check metadata parsing.
	bootstrap := byName["inferenceProvider"]
	if len(bootstrap.EnumValues) != 4 {
		t.Errorf("inferenceProvider enum: got %v, want 4 values", bootstrap.EnumValues)
	}
	if bootstrap.Title != "Inference provider" {
		t.Errorf("inferenceProvider title = %q", bootstrap.Title)
	}
	wantScopes := []string{"3p", "3p-bootstrap"}
	if len(bootstrap.Scopes) != len(wantScopes) {
		t.Fatalf("scopes = %v, want %v", bootstrap.Scopes, wantScopes)
	}
	for i, s := range bootstrap.Scopes {
		if s != wantScopes[i] {
			t.Errorf("scopes[%d] = %q, want %q", i, s, wantScopes[i])
		}
	}

	mcp := byName["managedMcpServers"]
	if !mcp.Sensitive {
		t.Error("managedMcpServers Sensitive should be true")
	}
	if mcp.AppMin != "1.2.0" {
		t.Errorf("managedMcpServers appMin = %q", mcp.AppMin)
	}

	legacy := byName["isDesktopExtensionEnabled"]
	if legacy.LegacyAlias != "isDxtEnabled" {
		t.Errorf("legacyAlias = %q", legacy.LegacyAlias)
	}
}

func TestMatchBraceSkipsStrings(t *testing.T) {
	// Braces inside strings must not affect depth counting.
	src := `{ "k": "{nested}", "t": ` + "`template ${1+2} end`" + ` }trailer`
	end, err := matchBrace(src, 0)
	if err != nil {
		t.Fatalf("matchBrace: %v", err)
	}
	if src[end] != '}' {
		t.Fatalf("expected } at %d, got %q", end, src[end])
	}
	if !strings.HasPrefix(src[end+1:], "trailer") {
		t.Errorf("content after } = %q", src[end+1:])
	}
}

func TestLocateMeCallWithWhitespace(t *testing.T) {
	src := "foo();\n\nFJ   =   me (  { a: 1 }   )\n;"
	_, body, err := locateMeCall(src)
	if err != nil {
		t.Fatalf("locateMeCall: %v", err)
	}
	if !strings.Contains(body, "a: 1") {
		t.Errorf("body = %q", body)
	}
}

func TestParseSchemaLiteralMissing(t *testing.T) {
	_, err := parseSchemaLiteral("no schema here")
	if err == nil {
		t.Fatal("expected error when FJ=me({...}) is absent")
	}
}

func TestUnquoteJSString(t *testing.T) {
	cases := map[string]string{
		`hello\nworld`: "hello\nworld",
		`a\\b`:         `a\b`,
		`x\tY`:         "x\tY",
		`A`:            "A",
	}
	for in, want := range cases {
		if got := unquoteJSString(in); got != want {
			t.Errorf("unquoteJSString(%q) = %q, want %q", in, got, want)
		}
	}
}
