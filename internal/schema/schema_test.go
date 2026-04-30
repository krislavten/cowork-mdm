package schema

import (
	"testing"
)

// TestSchemaHasAllKeys is the authoritative size gate used by
// docs/execution/verify.sh task-01. Raising the minimum is fine;
// dropping it silently is not.
func TestSchemaHasAllKeys(t *testing.T) {
	s := Load()
	if s == nil {
		t.Fatal("Load() returned nil")
	}
	const minKeys = 51
	if got := len(s.Keys); got < minKeys {
		t.Fatalf("schema has %d keys, want >= %d", got, minKeys)
	}
	if s.Version == "" {
		t.Error("Schema.Version is empty")
	}
	if s.ExtractedFromAppVersion == "" {
		t.Error("Schema.ExtractedFromAppVersion is empty")
	}
}

func TestLoadPropagatesAppVersion(t *testing.T) {
	s := Load()
	for i := range s.Keys {
		k := &s.Keys[i]
		if k.ExtractedFromApp != s.ExtractedFromAppVersion {
			t.Errorf(
				"key %q: ExtractedFromApp=%q, want %q",
				k.Name, k.ExtractedFromApp, s.ExtractedFromAppVersion,
			)
		}
	}
}

func TestFindReturnsKnownKey(t *testing.T) {
	s := Load()

	cases := []struct {
		name         string
		wantType     Type
		wantSensitve bool
		wantScope    Scope
	}{
		{name: "inferenceProvider", wantType: TypeEnum, wantScope: ScopeThirdParty},
		{name: "managedMcpServers", wantType: TypeJSONString, wantSensitve: true, wantScope: ScopeThirdParty},
		{name: "coworkEgressAllowedHosts", wantType: TypeStringArray, wantScope: ScopeThirdParty},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			k := s.Find(tc.name)
			if k == nil {
				t.Fatalf("Find(%q) = nil; want a key", tc.name)
			}
			if k.Name != tc.name {
				t.Errorf("Name = %q, want %q", k.Name, tc.name)
			}
			if k.Type != tc.wantType {
				t.Errorf("Type = %q, want %q", k.Type, tc.wantType)
			}
			if k.Sensitive != tc.wantSensitve {
				t.Errorf("Sensitive = %v, want %v", k.Sensitive, tc.wantSensitve)
			}
			found := false
			for _, s := range k.Scopes {
				if s == tc.wantScope {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("scope %q not present in %v", tc.wantScope, k.Scopes)
			}
		})
	}
}

func TestInferenceProviderEnumValues(t *testing.T) {
	s := Load()
	k := s.Find("inferenceProvider")
	if k == nil {
		t.Fatal("inferenceProvider missing")
	}
	want := map[string]bool{"bedrock": true, "vertex": true, "gateway": true, "foundry": true}
	if len(k.EnumValues) == 0 {
		t.Fatal("inferenceProvider has no enumValues")
	}
	for _, v := range k.EnumValues {
		if !want[v] {
			t.Errorf("unexpected enum value %q", v)
		}
		delete(want, v)
	}
	if len(want) != 0 {
		t.Errorf("missing expected enum values: %v", want)
	}
}

func TestFindUnknownKeyReturnsNil(t *testing.T) {
	s := Load()
	if k := s.Find("thisKeyDoesNotExist"); k != nil {
		t.Errorf("Find(unknown) = %+v, want nil", k)
	}
	if k := s.Find(""); k != nil {
		t.Errorf("Find(\"\") = %+v, want nil", k)
	}
}

func TestLoadIsSingleton(t *testing.T) {
	a := Load()
	b := Load()
	if a != b {
		t.Errorf("Load() returned different pointers: %p vs %p", a, b)
	}
}

func TestFindOnNilReceiverIsSafe(t *testing.T) {
	var s *Schema
	if k := s.Find("inferenceProvider"); k != nil {
		t.Errorf("nil-receiver Find() = %+v, want nil", k)
	}
}

func TestDefaultFieldPreserved(t *testing.T) {
	// The schema.json records per-key defaults (e.g. isDesktopExtensionEnabled
	// defaults to true, disableAutoUpdates defaults to false). Downstream
	// consumers rely on these to distinguish "unset" from "set to the default"
	// when rendering diffs or status reports. The JSON field must round-trip
	// into Key.Default rather than being silently dropped.
	s := Load()
	k := s.Find("isDesktopExtensionEnabled")
	if k == nil {
		t.Fatal("Find(\"isDesktopExtensionEnabled\") returned nil")
	}
	if k.Default == nil {
		t.Errorf("isDesktopExtensionEnabled.Default = nil; schema.json declares a default — field was dropped during Unmarshal")
	}
	// Spot-check one more: disableAutoUpdates defaults to false in schema.json.
	if k := s.Find("disableAutoUpdates"); k != nil && k.Default == nil {
		t.Errorf("disableAutoUpdates.Default = nil; schema.json declares a default")
	}
}

func TestAllKeysHaveSupportedType(t *testing.T) {
	s := Load()
	supported := map[Type]bool{
		TypeString: true, TypeBoolean: true, TypeInteger: true,
		TypeStringArray: true, TypeJSONString: true, TypeURL: true,
		TypeEnum: true,
	}
	seen := make(map[string]bool, len(s.Keys))
	for i := range s.Keys {
		k := &s.Keys[i]
		if !supported[k.Type] {
			t.Errorf("key %q has unsupported type %q", k.Name, k.Type)
		}
		if k.Name == "" {
			t.Errorf("key at index %d has empty Name", i)
		}
		if seen[k.Name] {
			t.Errorf("duplicate key Name %q", k.Name)
		}
		seen[k.Name] = true
		// Note: some keys in the current schema.json carry no `scopes`
		// (e.g. bootstrap* which is inherently 3p-bootstrap and the
		// bedrock service tier which is bedrock-only). We do not
		// enforce a non-empty Scopes list here — but any scope that
		// IS present must be one of the three known values.
		for _, sc := range k.Scopes {
			if sc != ScopeThirdParty && sc != ScopeFirstParty && sc != ScopeThirdPartyBootstrap {
				t.Errorf("key %q has unknown scope %q", k.Name, sc)
			}
		}
		if k.Type == TypeEnum && len(k.EnumValues) == 0 {
			t.Errorf("key %q is enum but has no EnumValues", k.Name)
		}
	}
}

func TestFindOnCustomSchemaDoesNotTouchCache(t *testing.T) {
	custom := &Schema{
		Version:                 "test",
		ExtractedFromAppVersion: "0.0.0",
		Keys: []Key{
			{Name: "alpha", Type: TypeBoolean, Scopes: []Scope{ScopeThirdParty}},
			{Name: "beta", Type: TypeString, Scopes: []Scope{ScopeThirdParty}},
		},
	}
	if custom.Find("alpha") == nil {
		t.Error("Find(alpha) on custom schema returned nil")
	}
	if custom.Find("missing") != nil {
		t.Error("Find(missing) on custom schema returned non-nil")
	}
	// The embedded cache must not be affected.
	if Load().Find("alpha") != nil {
		t.Error("custom-schema lookup leaked into embedded cache")
	}
}
