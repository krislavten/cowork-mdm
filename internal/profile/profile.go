// Package profile represents a Claude Desktop MDM configuration as a typed,
// format-agnostic Go value. Encoders transform it into .mobileconfig / plist /
// .reg / Jamf / Intune formats; decoders parse those back. All encoders and
// decoders share the same Profile type so round-tripping is safe.
//
// Values are validated against the embedded schema on Set() — unknown keys
// and type mismatches surface immediately rather than at encode time.
package profile

import (
	"errors"
	"fmt"

	"github.com/krislavten/cowork-mdm/internal/schema"
)

// Profile is an in-memory MDM configuration. Insertion order is preserved so
// encoded output is deterministic.
type Profile struct {
	// Name is used as the PayloadDisplayName in mobileconfig output and as a
	// filename hint when callers export to disk.
	Name string

	entries     []Entry
	index       map[string]int // key → entries index
	unknownKeys []UnknownKey   // keys decoded from input that aren't in the schema
}

// Entry preserves insertion order.
type Entry struct {
	Key   string
	Value any
}

// UnknownKey is a key decoded from an input file that isn't in the current
// schema. Decoders attach these to the returned Profile rather than dropping
// them, so `profile validate` and `profile status` can surface drift.
type UnknownKey struct {
	Key      string
	RawValue string // stringified representation for reporting only
}

// DecodeReport accompanies the Profile returned by decoders.
type DecodeReport struct {
	UnknownKeys []UnknownKey
	Warnings    []string
}

// UnknownKeyError is returned by Validate when a profile contains a key the
// schema doesn't recognize. Callers can errors.As into this type to
// distinguish from type-mismatch errors.
type UnknownKeyError struct {
	Key string
}

func (e *UnknownKeyError) Error() string { return fmt.Sprintf("profile: unknown key %q", e.Key) }

// New returns an empty Profile with the given display name.
func New(name string) *Profile {
	return &Profile{
		Name:  name,
		index: make(map[string]int),
	}
}

// Set stores value under key. Validates against the schema. Replaces any
// existing entry (preserving its insertion position).
//
// Returns *UnknownKeyError if key is unknown, or a descriptive error if the
// value doesn't match the key's declared type.
func (p *Profile) Set(key string, value any) error {
	k := schema.Load().Find(key)
	if k == nil {
		return &UnknownKeyError{Key: key}
	}
	if err := k.Validate(value); err != nil {
		return fmt.Errorf("profile: %s: %w", key, err)
	}
	if idx, ok := p.index[key]; ok {
		p.entries[idx].Value = value
		return nil
	}
	p.entries = append(p.entries, Entry{Key: key, Value: value})
	p.index[key] = len(p.entries) - 1
	return nil
}

// SetRaw stores a value without schema validation. For internal use by
// decoders that have already classified a value. Prefer Set for user input.
func (p *Profile) SetRaw(key string, value any) {
	if idx, ok := p.index[key]; ok {
		p.entries[idx].Value = value
		return
	}
	p.entries = append(p.entries, Entry{Key: key, Value: value})
	p.index[key] = len(p.entries) - 1
}

// Get returns the value stored under key and whether it exists.
func (p *Profile) Get(key string) (any, bool) {
	idx, ok := p.index[key]
	if !ok {
		return nil, false
	}
	return p.entries[idx].Value, true
}

// Keys returns keys in insertion order.
func (p *Profile) Keys() []string {
	out := make([]string, len(p.entries))
	for i, e := range p.entries {
		out[i] = e.Key
	}
	return out
}

// Entries returns entries in insertion order. The returned slice is a copy
// so callers can't mutate profile state through it.
func (p *Profile) Entries() []Entry {
	out := make([]Entry, len(p.entries))
	copy(out, p.entries)
	return out
}

// Delete removes a key. No-op if absent. Preserves order for remaining
// entries. Also clears any matching UnknownKey side-channel record so
// Validate doesn't double-report a removed entry.
func (p *Profile) Delete(key string) {
	if idx, ok := p.index[key]; ok {
		p.entries = append(p.entries[:idx], p.entries[idx+1:]...)
		delete(p.index, key)
		for i := idx; i < len(p.entries); i++ {
			p.index[p.entries[i].Key] = i
		}
	}
	// Prune matching unknown-key records.
	filtered := p.unknownKeys[:0]
	for _, uk := range p.unknownKeys {
		if uk.Key != key {
			filtered = append(filtered, uk)
		}
	}
	p.unknownKeys = filtered
}

// Len returns the number of entries.
func (p *Profile) Len() int {
	return len(p.entries)
}

// Validate re-validates every entry against the current schema. Aggregates
// errors via errors.Join so callers can see all problems at once. Unknown
// keys are reported once even if they appear both as a regular entry (from
// decoder's SetRaw round-trip preservation) and in the side-channel
// unknownKeys slice.
func (p *Profile) Validate() error {
	s := schema.Load()
	var errs []error
	reported := make(map[string]bool)
	for _, e := range p.entries {
		k := s.Find(e.Key)
		if k == nil {
			if !reported[e.Key] {
				errs = append(errs, &UnknownKeyError{Key: e.Key})
				reported[e.Key] = true
			}
			continue
		}
		if err := k.Validate(e.Value); err != nil {
			errs = append(errs, fmt.Errorf("profile: %s: %w", e.Key, err))
		}
	}
	for _, uk := range p.unknownKeys {
		if !reported[uk.Key] {
			errs = append(errs, &UnknownKeyError{Key: uk.Key})
			reported[uk.Key] = true
		}
	}
	return errors.Join(errs...)
}

// AttachUnknownKey records a key that was present in decoded input but isn't
// in the current schema. Used by decoders only.
func (p *Profile) AttachUnknownKey(key, raw string) {
	p.unknownKeys = append(p.unknownKeys, UnknownKey{Key: key, RawValue: raw})
}

// UnknownKeys returns keys decoded from input but not present in the schema.
func (p *Profile) UnknownKeys() []UnknownKey {
	out := make([]UnknownKey, len(p.unknownKeys))
	copy(out, p.unknownKeys)
	return out
}
