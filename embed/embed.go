package embed

import (
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
)

//go:embed agents skills memory
var FS embed.FS

// Extract copies embedded files to the target directory.
// It only writes files that do not already exist on disk,
// so user modifications are preserved.
func Extract(targetDir string) error {
	return fs.WalkDir(FS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		targetPath := filepath.Join(targetDir, path)

		if d.IsDir() {
			if err := os.Mkdir(targetPath, 0o700); err != nil {
				if !os.IsExist(err) {
					return fmt.Errorf("create dir %s: %w", targetPath, err)
				}
			}

			return nil
		}

		// Skip if file already exists
		if _, err := os.Stat(targetPath); err == nil {
			return nil
		}

		// Read from embed
		data, err := FS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read embedded file %s: %w", path, err)
		}

		// Write to disk
		if err := os.WriteFile(targetPath, data, 0o600); err != nil {
			return fmt.Errorf("write %s: %w", targetPath, err)
		}

		slog.Debug("extracted embedded file", "path", targetPath)
		return nil
	})
}
