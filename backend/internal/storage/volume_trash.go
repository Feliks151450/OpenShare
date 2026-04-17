package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// VolumeTrashDirectory returns the path to the trash folder on the same volume /
// mount point as absolutePath, i.e. <volumeRoot>/<trashLeaf> (e.g. /SSD1/trash).
// The directory is created if missing.
func VolumeTrashDirectory(absolutePath string, trashLeaf string) (string, error) {
	root, err := volumeRootForPath(absolutePath)
	if err != nil {
		return "", err
	}
	leaf := strings.TrimSpace(trashLeaf)
	if leaf == "" {
		leaf = "trash"
	}
	leaf = filepath.Clean(leaf)
	if leaf == "." || filepath.IsAbs(leaf) || filepath.Base(leaf) != leaf || strings.Contains(leaf, "..") {
		return "", fmt.Errorf("invalid trash leaf name")
	}
	out := filepath.Join(root, leaf)
	if err := os.MkdirAll(out, 0o755); err != nil {
		return "", fmt.Errorf("create volume trash directory: %w", err)
	}
	return out, nil
}
