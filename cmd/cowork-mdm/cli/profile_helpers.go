package cli

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/krislavten/cowork-mdm/internal/schema"
)

// formatStatusValue renders one profile value for the human-readable
// `profile status` output, applying redaction policy: keys whose schema
// entry has Sensitive=true are masked unless the caller passes
// unmasked=true. Unknown keys (nil schema lookup) always pass through —
// we can't judge sensitivity and transparent debugging beats false
// safety. JSON output bypasses this entirely (machine consumers apply
// their own filtering policy).
func formatStatusValue(key string, value any, unmasked bool) string {
	if !unmasked {
		if k := schema.Load().Find(key); k != nil && k.Sensitive {
			return "<redacted> (pass --unmasked to reveal)"
		}
	}
	return fmt.Sprintf("%v", value)
}

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
