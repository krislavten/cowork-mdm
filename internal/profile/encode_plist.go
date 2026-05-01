package profile

import (
	"bytes"
	"fmt"
)

// EncodePlist emits the bare com.anthropic.claudefordesktop.plist body with
// no Configuration wrapper. This is the format that actually lives at
//
//	/Library/Managed Preferences/com.anthropic.claudefordesktop.plist
//
// It's the same per-key type mapping as EncodeMobileConfig, just without
// the Apple Configuration Profile envelope. Use this when writing directly
// to the managed-prefs path (i.e. for `profile apply`), not for MDM delivery.
func EncodePlist(p *Profile) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(plistHeader)
	buf.WriteString("<plist version=\"1.0\">\n")
	buf.WriteString("<dict>\n")
	if err := writeProfileEntries(&buf, p, "\t"); err != nil {
		return nil, err
	}
	buf.WriteString("</dict>\n")
	buf.WriteString("</plist>\n")
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("profile validation failed: %w", err)
	}
	return buf.Bytes(), nil
}
