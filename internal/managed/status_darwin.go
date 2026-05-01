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
	p, decoded, err := profile.DecodePlist(data)
	if err != nil {
		rep.ParseError = err.Error()
		return rep, nil
	}
	rep.Profile = p
	rep.UnknownKeys = decoded.UnknownKeys
	_ = decoded.Warnings // currently unused but kept for future surfacing
	return rep, nil
}
