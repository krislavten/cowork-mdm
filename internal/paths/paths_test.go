package paths

import (
	"os"
	"testing"
)

// TestForOS_Darwin_StaticPaths pins the literal macOS paths that other
// packages depend on. Changing any of these is a behavioural break for
// cowork-mdm's MDM payloads and should force a spec update.
func TestForOS_Darwin_StaticPaths(t *testing.T) {
	p := ForOS("darwin")

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"ManagedPrefsPlist", p.ManagedPrefsPlist(), "/Library/Managed Preferences/com.anthropic.claudefordesktop.plist"},
		{"OrgPluginsDir", p.OrgPluginsDir(), "/Library/Application Support/Claude/org-plugins"},
		{"ClaudeAppPath", p.ClaudeAppPath(), "/Applications/Claude.app"},
		{"LaunchAgentDir", p.LaunchAgentDir(), "/Library/LaunchAgents"},
		{"WindowsRegistryPath", p.WindowsRegistryPath(), ""},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("darwin %s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}

// TestForOS_Darwin_ManagedPrefsUserPlist exercises the per-user override
// rule: username parameter is required and is not read from the environment.
func TestForOS_Darwin_ManagedPrefsUserPlist(t *testing.T) {
	p := ForOS("darwin")

	cases := []struct {
		name     string
		username string
		want     string
	}{
		{"typical lowercase username", "alice", "/Library/Managed Preferences/alice/com.anthropic.claudefordesktop.plist"},
		{"username with digits", "user42", "/Library/Managed Preferences/user42/com.anthropic.claudefordesktop.plist"},
		{"username with dot", "corp.user", "/Library/Managed Preferences/corp.user/com.anthropic.claudefordesktop.plist"},
		{"empty username returns empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := p.ManagedPrefsUserPlist(tc.username); got != tc.want {
				t.Errorf("ManagedPrefsUserPlist(%q) = %q, want %q", tc.username, got, tc.want)
			}
		})
	}
}

// TestForOS_Darwin_UserSessionsDir_HomeInjection verifies the injectable
// seam for testing cross-platform. We build a darwinProvider directly
// because ForOS does not expose the injection, and ForOS's default is
// covered separately by the env-driven test below.
func TestDarwinProvider_UserSessionsDir_HomeInjection(t *testing.T) {
	cases := []struct {
		name string
		home string
		want string
	}{
		{"standard home", "/Users/alice", "/Users/alice/Library/Application Support/Claude-3p/local-agent-mode-sessions"},
		{"home with trailing slash is preserved verbatim", "/Users/bob/", "/Users/bob/Library/Application Support/Claude-3p/local-agent-mode-sessions"},
		{"home is root slash — result must stay absolute", "/", "/Library/Application Support/Claude-3p/local-agent-mode-sessions"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := darwinProvider{home: func() (string, error) { return tc.home, nil }}
			if got := p.UserSessionsDir(); got != tc.want {
				t.Errorf("UserSessionsDir() = %q, want %q", got, tc.want)
			}
		})
	}

	t.Run("empty home returns empty", func(t *testing.T) {
		p := darwinProvider{home: func() (string, error) { return "", nil }}
		if got := p.UserSessionsDir(); got != "" {
			t.Errorf("UserSessionsDir() = %q, want empty", got)
		}
	})

	t.Run("home resolver error returns empty", func(t *testing.T) {
		p := darwinProvider{home: func() (string, error) { return "/unused", os.ErrNotExist }}
		if got := p.UserSessionsDir(); got != "" {
			t.Errorf("UserSessionsDir() on resolver error = %q, want empty", got)
		}
	})
}

// TestDarwinProvider_UserSessionsDir_EnvFallback ensures the default
// resolver honours $HOME when no injection is provided. We set the env
// for the test and restore it afterwards.
func TestDarwinProvider_UserSessionsDir_EnvFallback(t *testing.T) {
	t.Setenv("HOME", "/Users/envtest")
	p := darwinProvider{} // no injection
	want := "/Users/envtest/Library/Application Support/Claude-3p/local-agent-mode-sessions"
	if got := p.UserSessionsDir(); got != want {
		t.Errorf("UserSessionsDir() = %q, want %q", got, want)
	}
}

// TestDarwinProvider_UserSessionsDir_EnvUnset verifies that simulating
// darwin from a host with no HOME env returns the documented empty
// string (and does NOT leak the host os.UserHomeDir value, which on
// Windows would be a backslash path).
func TestDarwinProvider_UserSessionsDir_EnvUnset(t *testing.T) {
	t.Setenv("HOME", "")
	p := darwinProvider{}
	if got := p.UserSessionsDir(); got != "" {
		t.Errorf("UserSessionsDir() with HOME unset = %q, want empty", got)
	}
}

// TestForOS_Windows_StaticPaths pins the literal Windows paths.
func TestForOS_Windows_StaticPaths(t *testing.T) {
	p := ForOS("windows")

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"OrgPluginsDir", p.OrgPluginsDir(), `C:\Program Files\Claude\org-plugins`},
		{"ClaudeAppPath", p.ClaudeAppPath(), `C:\Program Files\Claude\Claude.exe`},
		{"WindowsRegistryPath", p.WindowsRegistryPath(), `SOFTWARE\Policies\Claude`},
		{"ManagedPrefsPlist", p.ManagedPrefsPlist(), ""},
		{"ManagedPrefsUserPlist(non-empty)", p.ManagedPrefsUserPlist("alice"), ""},
		{"ManagedPrefsUserPlist(empty)", p.ManagedPrefsUserPlist(""), ""},
		{"LaunchAgentDir", p.LaunchAgentDir(), ""},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("windows %s = %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}

// TestWindowsProvider_UserSessionsDir_HomeInjection verifies the Windows
// home seam and the backslash joining (which must be deterministic across
// darwin/linux/windows hosts so ForOS simulations stay correct).
func TestWindowsProvider_UserSessionsDir_HomeInjection(t *testing.T) {
	cases := []struct {
		name string
		home string
		want string
	}{
		{"typical userprofile", `C:\Users\alice`, `C:\Users\alice\AppData\Roaming\Claude-3p\local-agent-mode-sessions`},
		{"non-standard drive", `D:\Profiles\bob`, `D:\Profiles\bob\AppData\Roaming\Claude-3p\local-agent-mode-sessions`},
		{"trailing backslash is trimmed", `C:\Users\alice\`, `C:\Users\alice\AppData\Roaming\Claude-3p\local-agent-mode-sessions`},
		{"trailing forward slash is trimmed", `C:\Users\alice/`, `C:\Users\alice\AppData\Roaming\Claude-3p\local-agent-mode-sessions`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := windowsProvider{home: func() (string, error) { return tc.home, nil }}
			if got := p.UserSessionsDir(); got != tc.want {
				t.Errorf("UserSessionsDir() = %q, want %q", got, tc.want)
			}
		})
	}

	t.Run("empty userprofile returns empty", func(t *testing.T) {
		p := windowsProvider{home: func() (string, error) { return "", nil }}
		if got := p.UserSessionsDir(); got != "" {
			t.Errorf("UserSessionsDir() = %q, want empty", got)
		}
	})

	t.Run("resolver error returns empty", func(t *testing.T) {
		p := windowsProvider{home: func() (string, error) { return "/unused", os.ErrNotExist }}
		if got := p.UserSessionsDir(); got != "" {
			t.Errorf("UserSessionsDir() on resolver error = %q, want empty", got)
		}
	})
}

// TestWindowsProvider_UserSessionsDir_EnvFallback ensures the default
// resolver honours %USERPROFILE%.
func TestWindowsProvider_UserSessionsDir_EnvFallback(t *testing.T) {
	t.Setenv("USERPROFILE", `C:\Users\envtest`)
	p := windowsProvider{}
	want := `C:\Users\envtest\AppData\Roaming\Claude-3p\local-agent-mode-sessions`
	if got := p.UserSessionsDir(); got != want {
		t.Errorf("UserSessionsDir() = %q, want %q", got, want)
	}
}

// TestWindowsProvider_UserSessionsDir_EnvUnset verifies that simulating
// Windows from a host with no USERPROFILE env returns the documented
// empty string (no host UserHomeDir leakage, which on darwin/linux
// would be a POSIX path).
func TestWindowsProvider_UserSessionsDir_EnvUnset(t *testing.T) {
	t.Setenv("USERPROFILE", "")
	p := windowsProvider{}
	if got := p.UserSessionsDir(); got != "" {
		t.Errorf("UserSessionsDir() with USERPROFILE unset = %q, want empty", got)
	}
}

// TestForOS_Other_AllEmpty checks that unsupported platforms return empty
// strings everywhere so callers can treat "unsupported" uniformly.
func TestForOS_Other_AllEmpty(t *testing.T) {
	for _, osName := range []string{"linux", "freebsd", "openbsd", "netbsd", "", "unknown"} {
		t.Run(osName, func(t *testing.T) {
			p := ForOS(osName)
			if got := p.ManagedPrefsPlist(); got != "" {
				t.Errorf("%s ManagedPrefsPlist = %q, want empty", osName, got)
			}
			if got := p.ManagedPrefsUserPlist("alice"); got != "" {
				t.Errorf("%s ManagedPrefsUserPlist = %q, want empty", osName, got)
			}
			if got := p.OrgPluginsDir(); got != "" {
				t.Errorf("%s OrgPluginsDir = %q, want empty", osName, got)
			}
			if got := p.ClaudeAppPath(); got != "" {
				t.Errorf("%s ClaudeAppPath = %q, want empty", osName, got)
			}
			if got := p.UserSessionsDir(); got != "" {
				t.Errorf("%s UserSessionsDir = %q, want empty", osName, got)
			}
			if got := p.WindowsRegistryPath(); got != "" {
				t.Errorf("%s WindowsRegistryPath = %q, want empty", osName, got)
			}
			if got := p.LaunchAgentDir(); got != "" {
				t.Errorf("%s LaunchAgentDir = %q, want empty", osName, got)
			}
		})
	}
}

// TestForOS_ReturnsInterface guards against accidental nil returns from
// ForOS. A nil Provider would crash callers at the first method call.
func TestForOS_ReturnsInterface(t *testing.T) {
	for _, osName := range []string{"darwin", "windows", "linux", "unknown"} {
		t.Run(osName, func(t *testing.T) {
			if p := ForOS(osName); p == nil {
				t.Fatalf("ForOS(%q) returned nil Provider", osName)
			}
		})
	}
}

// TestDefault_ReturnsProvider is a smoke test that confirms the build-tagged
// Default() dispatches and returns a non-nil Provider on whichever host
// runs the test. Per-platform value assertions live in the ForOS tests.
func TestDefault_ReturnsProvider(t *testing.T) {
	if p := Default(); p == nil {
		t.Fatal("Default() returned nil")
	}
}
