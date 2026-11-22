package engine

import "github.com/lock14/openrct2-cli/internal/fsutil"

// Receipt records metadata for the currently installed user-space OpenRCT2 engine build.
type Receipt struct {
	Channel     string `json:"channel"`
	Tag         string `json:"tag"`
	SHA256      string `json:"sha256,omitempty"`
	BinaryPath  string `json:"binary_path"`
	InstalledAt string `json:"installed_at"`
}

// LoadReceipt reads the engine installation receipt from path if present.
func LoadReceipt(path string) (*Receipt, error) {
	var r Receipt
	ok, err := fsutil.ReadJSONFile(path, &r)
	if err != nil || !ok {
		return nil, err
	}
	return &r, nil
}

// SaveReceipt writes r atomically to path.
func SaveReceipt(path string, r Receipt) error {
	_, _, err := fsutil.WriteJSONAtomicIfChanged(path, r, 0o644)
	return err
}
