package config

import (
	"os"
	"path/filepath"
)

func writeRaw(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}

func writeMkdirFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o755)
}
