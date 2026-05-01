//go:build windows

package managed

import (
	"encoding/json"
	"errors"
	"fmt"

	"golang.org/x/sys/windows/registry"

	"github.com/krislavten/cowork-mdm/internal/profile"
	"github.com/krislavten/cowork-mdm/internal/schema"
)

func status(opts StatusOptions) (*StatusReport, error) {
	hive, root, err := resolveHive(opts.Hive)
	if err != nil {
		return nil, err
	}
	subkey := opts.SourcePath
	if subkey == "" {
		subkey = windowsDefaultKey
	}
	rep := &StatusReport{
		Platform:   "windows",
		TargetPath: fmt.Sprintf(`%s\%s`, hive, subkey),
	}
	k, err := registry.OpenKey(root, subkey, registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return rep, nil
		}
		return nil, fmt.Errorf("managed.Status: open %s\\%s: %w", hive, subkey, err)
	}
	defer k.Close()
	rep.Present = true

	names, err := k.ReadValueNames(-1)
	if err != nil {
		rep.ParseError = err.Error()
		return rep, nil
	}
	p := profile.New("")
	unknowns := []profile.UnknownKey{}
	s := schema.Load()
	for _, name := range names {
		sk := s.Find(name)
		if sk == nil {
			raw, _ := readAnyValue(k, name)
			p.AttachUnknownKey(name, fmt.Sprint(raw))
			unknowns = append(unknowns, profile.UnknownKey{Key: name, RawValue: fmt.Sprint(raw)})
			p.SetRaw(name, raw)
			continue
		}
		v, err := readTypedValue(k, name, sk.Type)
		if err != nil {
			// Capture as string so Validate can flag a type mismatch.
			raw, _ := readAnyValue(k, name)
			p.SetRaw(name, raw)
			continue
		}
		p.SetRaw(name, v)
	}
	rep.Profile = p
	rep.UnknownKeys = unknowns
	return rep, nil
}

// readAnyValue returns the best-effort Go value for a registry value of
// unknown type. Uses the type from QueryValue.
func readAnyValue(k registry.Key, name string) (any, error) {
	_, typ, err := k.GetValue(name, nil)
	if err != nil {
		return nil, err
	}
	switch typ {
	case registry.DWORD, registry.QWORD:
		n, _, err := k.GetIntegerValue(name)
		if err != nil {
			return nil, err
		}
		return int64(n), nil
	case registry.SZ, registry.EXPAND_SZ:
		s, _, err := k.GetStringValue(name)
		return s, err
	default:
		s, _, err := k.GetStringValue(name)
		return s, err
	}
}

// readTypedValue coerces a registry value into the expected Go type per
// the schema's declared type.
func readTypedValue(k registry.Key, name string, t schema.Type) (any, error) {
	switch t {
	case schema.TypeBoolean:
		n, _, err := k.GetIntegerValue(name)
		if err != nil {
			return nil, err
		}
		return n != 0, nil
	case schema.TypeInteger:
		n, _, err := k.GetIntegerValue(name)
		if err != nil {
			return nil, err
		}
		return int64(n), nil
	case schema.TypeString, schema.TypeURL, schema.TypeEnum, schema.TypeJSONString:
		s, _, err := k.GetStringValue(name)
		return s, err
	case schema.TypeStringArray:
		s, _, err := k.GetStringValue(name)
		if err != nil {
			return nil, err
		}
		var arr []string
		if err := json.Unmarshal([]byte(s), &arr); err != nil {
			return nil, fmt.Errorf("stringArray: %w", err)
		}
		return arr, nil
	default:
		return nil, fmt.Errorf("unsupported schema type %q", t)
	}
}
