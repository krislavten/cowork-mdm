//go:build !darwin && !windows

package managed

import "github.com/krislavten/cowork-mdm/internal/profile"

func apply(_ *profile.Profile, _ ApplyOptions) (*ApplyResult, error) {
	return nil, ErrUnsupportedPlatform
}

func status(_ StatusOptions) (*StatusReport, error) {
	return nil, ErrUnsupportedPlatform
}
