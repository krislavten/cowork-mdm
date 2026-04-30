//go:build darwin

package paths

// Default returns the Provider for the current operating system (darwin).
func Default() Provider {
	return darwinProvider{}
}
