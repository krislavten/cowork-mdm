//go:build !darwin && !windows

package managed

import (
	"errors"
	"testing"

	"github.com/krislavten/cowork-mdm/internal/profile"
)

func TestApply_LinuxReturnsUnsupported(t *testing.T) {
	_, err := Apply(profile.New("x"), ApplyOptions{})
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Errorf("want ErrUnsupportedPlatform, got %v", err)
	}
}

func TestStatus_LinuxReturnsUnsupported(t *testing.T) {
	_, err := Status(StatusOptions{})
	if !errors.Is(err, ErrUnsupportedPlatform) {
		t.Errorf("want ErrUnsupportedPlatform, got %v", err)
	}
}
