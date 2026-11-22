package fsutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"strings"
)

var utf8BOM = []byte("\xef\xbb\xbf")

// StripUTF8BOM removes an optional leading UTF-8 byte-order mark from data.
func StripUTF8BOM(data []byte) []byte {
	return bytes.TrimPrefix(data, utf8BOM)
}

// ReadJSONFile reads and decodes a JSON file at path into dst. When path is empty
// or does not exist on disk, ReadJSONFile returns (false, nil).
func ReadJSONFile(path string, dst any) (bool, error) {
	if strings.TrimSpace(path) == "" {
		return false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	if err := json.Unmarshal(StripUTF8BOM(data), dst); err != nil {
		return false, err
	}
	return true, nil
}

// EncodeJSONIndent serializes v to 2-space-indented UTF-8 JSON without HTML escaping.
func EncodeJSONIndent(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// WriteJSONAtomicIfChanged serializes v via EncodeJSONIndent and writes it to path
// atomically only when the file does not exist or its bytes differ.
func WriteJSONAtomicIfChanged(path string, v any, perm os.FileMode) (bool, string, error) {
	if strings.TrimSpace(path) == "" {
		return false, "", nil
	}
	data, err := EncodeJSONIndent(v)
	if err != nil {
		return false, "", err
	}
	return WriteFileAtomicIfChanged(path, data, perm)
}
