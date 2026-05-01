//go:build windows

package managed

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"golang.org/x/sys/windows/registry"

	"github.com/krislavten/cowork-mdm/internal/profile"
	"github.com/krislavten/cowork-mdm/internal/schema"
)

const windowsDefaultKey = `SOFTWARE\Policies\Claude`

func apply(p *profile.Profile, opts ApplyOptions) (*ApplyResult, error) {
	hive, root, err := resolveHive(opts.Hive)
	if err != nil {
		return nil, err
	}
	subkey := opts.TargetPath
	if subkey == "" {
		subkey = windowsDefaultKey
	}

	// Validate before writing so we fail fast on bad profiles.
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("managed.Apply: validation failed: %w", err)
	}

	res := &ApplyResult{
		TargetPath: fmt.Sprintf(`%s\%s`, hive, subkey),
		Platform:   "windows",
		DryRun:     opts.DryRun,
	}

	if opts.DryRun {
		res.Preview = renderReg(hive, subkey, p)
		return res, nil
	}

	// Two-pass apply: first compute every value conversion so we can fail
	// fast before touching the registry. Then open the key and write.
	// On any write error, roll back the values we wrote in this call.
	s := schema.Load()
	type pending struct {
		key string
		typ schema.Type
		val any
	}
	ops := make([]pending, 0, p.Len())
	for _, e := range p.Entries() {
		sk := s.Find(e.Key)
		if sk == nil {
			continue
		}
		// Validate conversion shape upfront — reject bad types before any
		// registry mutation.
		if err := validateForReg(sk.Type, e.Value); err != nil {
			return nil, fmt.Errorf("managed.Apply: %s: %w", e.Key, err)
		}
		ops = append(ops, pending{key: e.Key, typ: sk.Type, val: e.Value})
	}

	k, openedExisting, err := registry.CreateKey(root, subkey, registry.SET_VALUE|registry.CREATE_SUB_KEY|registry.QUERY_VALUE)
	if err != nil {
		if isPermissionError(err) {
			return nil, fmt.Errorf("%w: %s\\%s: %v", ErrPermission, hive, subkey, err)
		}
		return nil, fmt.Errorf("managed.Apply: open key: %w", err)
	}
	defer k.Close()

	// Snapshot any preexisting values for the keys we're about to write,
	// so a mid-sequence failure can restore them.
	snapshot := map[string]struct {
		raw any
		typ uint32
	}{}
	for _, op := range ops {
		if buf, typ, err := k.GetValue(op.key, nil); err == nil {
			snapshot[op.key] = struct {
				raw any
				typ uint32
			}{raw: nil, typ: typ}
			_ = buf
			// Best-effort stash of the actual value via a typed read.
			if v, err := readAnyValue(k, op.key); err == nil {
				snap := snapshot[op.key]
				snap.raw = v
				snapshot[op.key] = snap
			}
		}
	}

	written := make([]string, 0, len(ops))
	for _, op := range ops {
		if err := writeRegValue(k, op.key, op.typ, op.val); err != nil {
			// Roll back: restore snapshot values, delete values that we
			// newly introduced this call. The key itself we leave alone —
			// deleting a freshly-created key ahead of resync is risky.
			for _, name := range written {
				if prev, ok := snapshot[name]; ok && prev.raw != nil {
					_ = restoreRegValue(k, name, prev.typ, prev.raw)
				} else {
					_ = k.DeleteValue(name)
				}
			}
			_ = openedExisting // unused on rollback path; intentional
			return nil, fmt.Errorf("managed.Apply: write %s: %w", op.key, err)
		}
		written = append(written, op.key)
	}
	_ = openedExisting // informational; kept for future telemetry
	return res, nil
}

func validateForReg(t schema.Type, value any) error {
	switch t {
	case schema.TypeBoolean:
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("boolean expected, got %T", value)
		}
	case schema.TypeInteger:
		n, err := coerceInt64(value)
		if err != nil {
			return err
		}
		if n < 0 || n > math.MaxUint32 {
			return fmt.Errorf("integer %d out of DWORD range [0, %d]", n, math.MaxUint32)
		}
	case schema.TypeString, schema.TypeURL, schema.TypeEnum, schema.TypeJSONString:
		if _, ok := value.(string); !ok {
			// allow non-string that marshals to JSON as fallback (matches writeRegValue)
			if _, err := json.Marshal(value); err != nil {
				return fmt.Errorf("string expected, got %T", value)
			}
		}
	case schema.TypeStringArray:
		if _, err := coerceStringArrayForReg(value); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported schema type %q", t)
	}
	return nil
}

// restoreRegValue writes a captured value back to the registry using its
// original type. Best-effort — we don't surface errors here because we're
// already in a rollback path. Only the well-known value types we support.
func restoreRegValue(k registry.Key, name string, regType uint32, raw any) error {
	switch regType {
	case registry.DWORD:
		n, ok := raw.(int64)
		if !ok {
			return fmt.Errorf("restore: cannot convert %T to DWORD", raw)
		}
		return k.SetDWordValue(name, uint32(n))
	case registry.QWORD:
		n, ok := raw.(int64)
		if !ok {
			return fmt.Errorf("restore: cannot convert %T to QWORD", raw)
		}
		return k.SetQWordValue(name, uint64(n))
	case registry.SZ:
		s, _ := raw.(string)
		return k.SetStringValue(name, s)
	case registry.EXPAND_SZ:
		s, _ := raw.(string)
		return k.SetExpandStringValue(name, s)
	}
	return fmt.Errorf("restore: unsupported registry type %d", regType)
}

func resolveHive(hive string) (string, registry.Key, error) {
	switch strings.ToUpper(strings.TrimSpace(hive)) {
	case "", "HKLM":
		return "HKLM", registry.LOCAL_MACHINE, nil
	case "HKCU":
		return "HKCU", registry.CURRENT_USER, nil
	default:
		return "", 0, fmt.Errorf("managed.Apply: unsupported hive %q (want HKLM or HKCU)", hive)
	}
}

func writeRegValue(k registry.Key, name string, t schema.Type, value any) error {
	switch t {
	case schema.TypeBoolean:
		b, ok := value.(bool)
		if !ok {
			return fmt.Errorf("boolean expected, got %T", value)
		}
		v := uint32(0)
		if b {
			v = 1
		}
		return k.SetDWordValue(name, v)
	case schema.TypeInteger:
		n, err := coerceInt64(value)
		if err != nil {
			return err
		}
		if n < 0 || n > math.MaxUint32 {
			return fmt.Errorf("integer %d out of DWORD range [0, %d]", n, math.MaxUint32)
		}
		return k.SetDWordValue(name, uint32(n))
	case schema.TypeString, schema.TypeURL, schema.TypeEnum, schema.TypeJSONString:
		s, ok := value.(string)
		if !ok {
			b, err := json.Marshal(value)
			if err != nil {
				return fmt.Errorf("string expected, got %T: %w", value, err)
			}
			s = string(b)
		}
		return k.SetStringValue(name, s)
	case schema.TypeStringArray:
		arr, err := coerceStringArrayForReg(value)
		if err != nil {
			return err
		}
		b, err := json.Marshal(arr)
		if err != nil {
			return err
		}
		return k.SetStringValue(name, string(b))
	default:
		return fmt.Errorf("unsupported schema type %q", t)
	}
}

func coerceStringArrayForReg(value any) ([]string, error) {
	switch v := value.(type) {
	case []string:
		return v, nil
	case []any:
		out := make([]string, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("stringArray element %d is %T, want string", i, item)
			}
			out[i] = s
		}
		return out, nil
	default:
		return nil, fmt.Errorf("stringArray expected, got %T", value)
	}
}

func coerceInt64(value any) (int64, error) {
	switch v := value.(type) {
	case int:
		return int64(v), nil
	case int32:
		return int64(v), nil
	case int64:
		return v, nil
	case float64:
		if v != float64(int64(v)) {
			return 0, fmt.Errorf("integer expected, got non-whole float %v", v)
		}
		return int64(v), nil
	default:
		return 0, fmt.Errorf("integer expected, got %T", value)
	}
}

// renderReg returns a human-readable .reg-style preview of what Apply
// would write. Must match the conversions that writeRegValue performs so
// --dry-run output faithfully represents the actual registry operation.
func renderReg(hive, subkey string, p *profile.Profile) string {
	var b strings.Builder
	hiveFull := "HKEY_LOCAL_MACHINE"
	if hive == "HKCU" {
		hiveFull = "HKEY_CURRENT_USER"
	}
	b.WriteString("Windows Registry Editor Version 5.00\r\n\r\n")
	fmt.Fprintf(&b, "[%s\\%s]\r\n", hiveFull, subkey)
	s := schema.Load()
	for _, e := range p.Entries() {
		sk := s.Find(e.Key)
		if sk == nil {
			continue
		}
		switch sk.Type {
		case schema.TypeBoolean:
			bv, _ := e.Value.(bool)
			val := 0
			if bv {
				val = 1
			}
			fmt.Fprintf(&b, "%q=dword:%08x\r\n", e.Key, val)
		case schema.TypeInteger:
			n, _ := coerceInt64(e.Value)
			fmt.Fprintf(&b, "%q=dword:%08x\r\n", e.Key, uint32(n))
		case schema.TypeString, schema.TypeURL, schema.TypeEnum, schema.TypeJSONString:
			var str string
			if s, ok := e.Value.(string); ok {
				str = s
			} else {
				raw, _ := json.Marshal(e.Value)
				str = string(raw)
			}
			fmt.Fprintf(&b, "%q=%q\r\n", e.Key, str)
		case schema.TypeStringArray:
			arr, err := coerceStringArrayForReg(e.Value)
			if err != nil {
				fmt.Fprintf(&b, "# %s: coerce error: %s\r\n", e.Key, err)
				continue
			}
			raw, _ := json.Marshal(arr)
			fmt.Fprintf(&b, "%q=%q\r\n", e.Key, string(raw))
		default:
			fmt.Fprintf(&b, "# %s: unsupported type %q\r\n", e.Key, sk.Type)
		}
	}
	return b.String()
}

func isPermissionError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "Access is denied") ||
		strings.Contains(msg, "access denied") ||
		strings.Contains(strings.ToLower(msg), "denied")
}
