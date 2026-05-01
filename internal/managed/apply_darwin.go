//go:build darwin

package managed

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	"github.com/krislavten/cowork-mdm/internal/paths"
	"github.com/krislavten/cowork-mdm/internal/profile"
)

const directWriteWarning = "Direct write to /Library/Managed Preferences/ bypasses macOS's MDM pipeline (managedappconfigd). The file may be clobbered when the system resyncs managed profiles. For production deployments, push this as a .mobileconfig through your MDM channel (Jamf / Kandji / Intune)."

func apply(p *profile.Profile, opts ApplyOptions) (*ApplyResult, error) {
	target := opts.TargetPath
	if target == "" {
		target = paths.Default().ManagedPrefsPlist()
	}
	if target == "" {
		return nil, ErrUnsupportedPlatform
	}

	// Encode first so we fail fast on invalid profiles.
	data, err := profile.EncodePlist(p)
	if err != nil {
		return nil, fmt.Errorf("managed.Apply: encode failed: %w", err)
	}

	res := &ApplyResult{
		TargetPath:   target,
		Platform:     "darwin",
		DryRun:       opts.DryRun,
		BytesWritten: 0,
	}

	// Warn whenever we'd write to the system-wide managed-prefs path.
	// Tests that pass a custom TargetPath skip this warning.
	systemPath := paths.Default().ManagedPrefsPlist()
	if target == systemPath {
		res.Warnings = append(res.Warnings, directWriteWarning)
	}

	if opts.DryRun {
		res.Preview = string(data)
		return res, nil
	}

	// Rely on the actual filesystem write to detect permission problems
	// rather than prejudging via os.Geteuid(). An admin using `sudo -u`
	// or a specially-permissive path should succeed; only the real write
	// result determines permission. writeAtomic maps EACCES/EPERM to
	// ErrPermission so callers can errors.Is check it.
	if err := writeAtomic(target, data); err != nil {
		if os.IsPermission(err) {
			return nil, fmt.Errorf("%w: %s: %v", ErrPermission, target, err)
		}
		return nil, fmt.Errorf("managed.Apply: write failed: %w", err)
	}
	res.BytesWritten = len(data)
	return res, nil
}

// writeAtomic writes data to path via a temp file + rename so a crash mid-
// write can never leave a half-written plist behind that Claude.app might
// read.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".cowork-mdm.*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		// In case rename fails, clean the leftover tmp.
		if _, statErr := os.Stat(tmpName); statErr == nil {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// Match the typical plist perms (root-owned, world-readable).
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// currentUsername is resolved from OS facilities; used by ManagedPrefsUserPlist.
// Unused in apply() but retained for parity when we add user-scope support.
var _ = func() string {
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return ""
}
