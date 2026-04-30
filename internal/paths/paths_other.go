//go:build !darwin && !windows

package paths

// Default returns the Provider for the current operating system. On
// unsupported platforms (linux, BSDs, etc.) every path is an empty string,
// letting callers treat the absence uniformly.
func Default() Provider {
	return otherProvider{}
}
