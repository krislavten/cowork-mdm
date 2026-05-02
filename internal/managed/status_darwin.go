//go:build darwin

package managed

import (
	"errors"
	"fmt"
	"os"

	"github.com/krislavten/cowork-mdm/internal/paths"
	"github.com/krislavten/cowork-mdm/internal/profile"
)

func status(opts StatusOptions) (*StatusReport, error) {
	target := opts.SourcePath
	if target == "" {
		target = paths.Default().ManagedPrefsPlist()
	}
	if target == "" {
		return nil, ErrUnsupportedPlatform
	}
	rep := &StatusReport{
		Platform:   "darwin",
		TargetPath: target,
	}
	data, err := os.ReadFile(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			rep.Present = false
			return rep, nil
		}
		return nil, fmt.Errorf("managed.Status: read %s: %w", target, err)
	}
	rep.Present = true
	// Auto-detect format: `status --source` can point at a .plist
	// (default managed-prefs shape) or a .mobileconfig (Custom Settings
	// payload from the CLI's `profile new` default output). Without
	// detection the wrapper plist's outer keys leak into the report.
	var (
		p       *profile.Profile
		decoded profile.DecodeReport
		decErr  error
	)
	switch profile.Detect(data) {
	case "mobileconfig":
		p, decoded, decErr = profile.DecodeMobileConfig(data)
	default:
		p, decoded, decErr = profile.DecodePlist(data)
	}
	if decErr != nil {
		rep.ParseError = decErr.Error()
		return rep, nil
	}
	rep.Profile = p
	rep.UnknownKeys = decoded.UnknownKeys
	_ = decoded.Warnings // currently unused but kept for future surfacing
	return rep, nil
}
