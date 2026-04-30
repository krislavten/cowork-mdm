//go:build windows

package paths

// Default returns the Provider for the current operating system (windows).
func Default() Provider {
	return windowsProvider{}
}
