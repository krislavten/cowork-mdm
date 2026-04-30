package schema

import (
	"math"
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	boolKey := &Key{Name: "flag", Type: TypeBoolean, Scopes: []Scope{ScopeThirdParty}}
	intKey := &Key{Name: "n", Type: TypeInteger, Scopes: []Scope{ScopeThirdParty}}
	strKey := &Key{Name: "s", Type: TypeString, Scopes: []Scope{ScopeThirdParty}}
	saKey := &Key{Name: "arr", Type: TypeStringArray, Scopes: []Scope{ScopeThirdParty}}
	jsonKey := &Key{Name: "j", Type: TypeJSONString, Scopes: []Scope{ScopeThirdParty}}
	urlKey := &Key{Name: "u", Type: TypeURL, Scopes: []Scope{ScopeThirdParty}}
	enumKey := &Key{
		Name:       "e",
		Type:       TypeEnum,
		Scopes:     []Scope{ScopeThirdParty},
		EnumValues: []string{"bedrock", "vertex", "gateway", "foundry"},
	}
	unknownTypeKey := &Key{Name: "mystery", Type: Type("martian"), Scopes: []Scope{ScopeThirdParty}}

	cases := []struct {
		label   string
		key     *Key
		value   any
		wantErr bool
		errSubs string // substring to match when wantErr is true
	}{
		// boolean
		{label: "bool/true", key: boolKey, value: true},
		{label: "bool/false", key: boolKey, value: false},
		{label: "bool/reject-string", key: boolKey, value: "true", wantErr: true, errSubs: "expected bool"},
		{label: "bool/reject-int", key: boolKey, value: 1, wantErr: true, errSubs: "expected bool"},
		{label: "bool/reject-nil", key: boolKey, value: nil, wantErr: true, errSubs: "value is nil"},

		// integer
		{label: "int/int", key: intKey, value: 42},
		{label: "int/int64", key: intKey, value: int64(1 << 40)},
		{label: "int/int32", key: intKey, value: int32(5)},
		{label: "int/uint", key: intKey, value: uint(5)},
		{label: "int/uint64-ok", key: intKey, value: uint64(math.MaxInt64)},
		{label: "int/uint64-too-big", key: intKey, value: uint64(math.MaxUint64), wantErr: true, errSubs: "exceeds int64"},
		{label: "int/reject-float", key: intKey, value: 1.5, wantErr: true, errSubs: "expected integer"},
		{label: "int/reject-float-whole", key: intKey, value: 1.0, wantErr: true, errSubs: "expected integer"},
		{label: "int/reject-string", key: intKey, value: "42", wantErr: true, errSubs: "expected integer"},
		{label: "int/reject-bool", key: intKey, value: true, wantErr: true, errSubs: "expected integer"},

		// string
		{label: "string/ok", key: strKey, value: "hello"},
		{label: "string/empty", key: strKey, value: ""},
		{label: "string/reject-bool", key: strKey, value: false, wantErr: true, errSubs: "expected string"},
		{label: "string/reject-int", key: strKey, value: 1, wantErr: true, errSubs: "expected string"},
		{label: "string/reject-nil", key: strKey, value: nil, wantErr: true, errSubs: "value is nil"},

		// stringArray
		{label: "sa/ok", key: saKey, value: []string{"a", "b"}},
		{label: "sa/empty", key: saKey, value: []string{}},
		{label: "sa/reject-int-slice", key: saKey, value: []int{1, 2}, wantErr: true, errSubs: "expected []string"},
		{label: "sa/reject-any-slice", key: saKey, value: []any{"a"}, wantErr: true, errSubs: "expected []string"},
		{label: "sa/reject-string", key: saKey, value: "a,b", wantErr: true, errSubs: "expected []string"},

		// jsonString — v0.2 only checks that it's a string
		{label: "json/ok-string", key: jsonKey, value: `[{"name":"x"}]`},
		{label: "json/reject-struct", key: jsonKey, value: struct{}{}, wantErr: true, errSubs: "expected string"},

		// url — v0.2 only checks that it's a string
		{label: "url/ok", key: urlKey, value: "https://example.com"},
		{label: "url/empty-allowed-at-v0.2", key: urlKey, value: ""},
		{label: "url/reject-int", key: urlKey, value: 1, wantErr: true, errSubs: "expected string"},

		// enum
		{label: "enum/bedrock", key: enumKey, value: "bedrock"},
		{label: "enum/vertex", key: enumKey, value: "vertex"},
		{label: "enum/reject-unknown", key: enumKey, value: "openai", wantErr: true, errSubs: "allowed enum values"},
		{label: "enum/reject-empty", key: enumKey, value: "", wantErr: true, errSubs: "allowed enum values"},
		{label: "enum/reject-non-string", key: enumKey, value: 42, wantErr: true, errSubs: "expected string"},

		// unknown type
		{label: "unknown-type", key: unknownTypeKey, value: "whatever", wantErr: true, errSubs: "unknown type"},
	}

	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			err := tc.key.Validate(tc.value)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Validate(%v) = nil error, want error containing %q", tc.value, tc.errSubs)
				}
				if tc.errSubs != "" && !strings.Contains(err.Error(), tc.errSubs) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.errSubs)
				}
				return
			}
			if err != nil {
				t.Errorf("Validate(%v) unexpected error: %v", tc.value, err)
			}
		})
	}
}

func TestValidateNilReceiver(t *testing.T) {
	var k *Key
	err := k.Validate(true)
	if err == nil {
		t.Fatal("nil-receiver Validate returned nil error")
	}
	if !strings.Contains(err.Error(), "nil Key") {
		t.Errorf("error %q should mention nil Key", err.Error())
	}
}

// TestValidateAgainstRealSchema pins a few canonical keys from the
// embedded schema, catching accidental regressions where a key's
// declared type in schema.json diverges from what Validate accepts.
func TestValidateAgainstRealSchema(t *testing.T) {
	s := Load()

	boolKey := s.Find("isDesktopExtensionEnabled")
	if boolKey == nil {
		t.Fatal("isDesktopExtensionEnabled missing")
	}
	if err := boolKey.Validate(true); err != nil {
		t.Errorf("bool key rejected a bool: %v", err)
	}
	if err := boolKey.Validate("true"); err == nil {
		t.Error("bool key accepted a string")
	}

	enumKey := s.Find("inferenceProvider")
	if enumKey == nil {
		t.Fatal("inferenceProvider missing")
	}
	if err := enumKey.Validate("bedrock"); err != nil {
		t.Errorf("enum key rejected bedrock: %v", err)
	}
	if err := enumKey.Validate("openai"); err == nil {
		t.Error("enum key accepted 'openai'")
	}

	saKey := s.Find("coworkEgressAllowedHosts")
	if saKey == nil {
		t.Fatal("coworkEgressAllowedHosts missing")
	}
	if err := saKey.Validate([]string{"example.com"}); err != nil {
		t.Errorf("stringArray key rejected a []string: %v", err)
	}

	jsonKey := s.Find("managedMcpServers")
	if jsonKey == nil {
		t.Fatal("managedMcpServers missing")
	}
	if err := jsonKey.Validate(`[]`); err != nil {
		t.Errorf("jsonString key rejected a string: %v", err)
	}
}
