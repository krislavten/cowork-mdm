package profile

import (
	"errors"
	"testing"
)

func TestSetRejectsUnknownKey(t *testing.T) {
	p := New("x")
	err := p.Set("notAKey", "val")
	if err == nil {
		t.Fatal("Set on unknown key should error")
	}
	var uk *UnknownKeyError
	if !errors.As(err, &uk) {
		t.Errorf("error should be *UnknownKeyError, got %T", err)
	}
	if uk.Key != "notAKey" {
		t.Errorf("Key = %q, want %q", uk.Key, "notAKey")
	}
}

func TestSetValidatesType(t *testing.T) {
	p := New("x")
	// inferenceProvider is an enum — must be string, and within allowed set
	if err := p.Set("inferenceProvider", 42); err == nil {
		t.Error("Set(inferenceProvider, int) should error (schema says enum/string)")
	}
	if err := p.Set("inferenceProvider", "bedrock"); err != nil {
		t.Errorf("Set(inferenceProvider, bedrock) should succeed, got %v", err)
	}
	if err := p.Set("inferenceProvider", "not-a-real-provider"); err == nil {
		t.Error("Set(inferenceProvider, invalid enum) should error")
	}
}

func TestGetReturnsStoredValue(t *testing.T) {
	p := New("x")
	if err := p.Set("inferenceProvider", "bedrock"); err != nil {
		t.Fatal(err)
	}
	v, ok := p.Get("inferenceProvider")
	if !ok || v != "bedrock" {
		t.Errorf("Get = (%v, %v), want (bedrock, true)", v, ok)
	}
}

func TestKeysPreserveInsertionOrder(t *testing.T) {
	p := New("x")
	seq := []struct {
		k string
		v any
	}{
		{"inferenceProvider", "bedrock"},
		{"inferenceBedrockRegion", "us-west-2"},
		{"disableDeploymentModeChooser", true},
	}
	for _, s := range seq {
		if err := p.Set(s.k, s.v); err != nil {
			t.Fatalf("Set %s: %v", s.k, err)
		}
	}
	got := p.Keys()
	want := []string{"inferenceProvider", "inferenceBedrockRegion", "disableDeploymentModeChooser"}
	if len(got) != len(want) {
		t.Fatalf("Keys() length = %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("Keys()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSetReplacesPreservesPosition(t *testing.T) {
	p := New("x")
	_ = p.Set("inferenceProvider", "bedrock")
	_ = p.Set("inferenceBedrockRegion", "us-west-2")
	// Overwrite the first entry — it should stay at index 0.
	_ = p.Set("inferenceProvider", "vertex")
	keys := p.Keys()
	if keys[0] != "inferenceProvider" {
		t.Errorf("after overwrite, keys[0] = %q, want inferenceProvider", keys[0])
	}
	v, _ := p.Get("inferenceProvider")
	if v != "vertex" {
		t.Errorf("overwrite did not replace value: got %v", v)
	}
}

func TestDeleteShiftsIndex(t *testing.T) {
	p := New("x")
	_ = p.Set("inferenceProvider", "bedrock")
	_ = p.Set("inferenceBedrockRegion", "us-west-2")
	_ = p.Set("disableDeploymentModeChooser", true)
	p.Delete("inferenceBedrockRegion")
	if _, ok := p.Get("inferenceBedrockRegion"); ok {
		t.Error("deleted key still present")
	}
	// remaining order and indexing intact
	keys := p.Keys()
	if len(keys) != 2 || keys[0] != "inferenceProvider" || keys[1] != "disableDeploymentModeChooser" {
		t.Errorf("Keys after delete = %v", keys)
	}
	if _, ok := p.Get("disableDeploymentModeChooser"); !ok {
		t.Error("non-deleted key no longer retrievable (index corruption)")
	}
}

func TestValidateAggregatesErrors(t *testing.T) {
	p := New("x")
	_ = p.Set("inferenceProvider", "bedrock")
	// Use SetRaw to slip in a bad value past Set's front-door validation.
	p.SetRaw("inferenceBedrockRegion", 42) // should be string
	p.AttachUnknownKey("fooBar", "xyz")
	err := p.Validate()
	if err == nil {
		t.Fatal("Validate should error for bad type + unknown key")
	}
	// errors.Join yields a multi-error; check both concerns surface.
	got := err.Error()
	if !containsSubstring(got, "fooBar") {
		t.Errorf("Validate error missing unknown key: %s", got)
	}
	if !containsSubstring(got, "inferenceBedrockRegion") {
		t.Errorf("Validate error missing type-mismatch key: %s", got)
	}
}

func containsSubstring(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
