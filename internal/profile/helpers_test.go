package profile

import "os"

func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o600)
}
