package cli

import (
	"encoding/json"
	"strconv"

	"github.com/krislavten/cowork-mdm/internal/schema"
)

type schemaKey struct {
	Type string
}

// schemaLoadKey returns a minimal key descriptor for the --set coercer.
// Returns nil for unknown keys.
func schemaLoadKey(name string) *schemaKey {
	k := schema.Load().Find(name)
	if k == nil {
		return nil
	}
	return &schemaKey{Type: string(k.Type)}
}

func parseInt64(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

func jsonDecode(s string, v any) error {
	return json.Unmarshal([]byte(s), v)
}
