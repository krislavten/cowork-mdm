package profile

import "encoding/json"

// jsonUnmarshalImpl delegates to the standard library. Split out so tests
// can override the `jsonUnmarshal` variable without touching the whole
// decoder body.
func jsonUnmarshalImpl(s string, v any) error {
	return json.Unmarshal([]byte(s), v)
}
