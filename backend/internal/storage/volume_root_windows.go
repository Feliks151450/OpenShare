//go:build windows

package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func volumeRootForPath(absPath string) (string, error) {
	absPath = filepath.Clean(absPath)
	if !filepath.IsAbs(absPath) && !strings.HasPrefix(absPath, `\\`) {
		return "", fmt.Errorf("path must be absolute: %s", absPath)
	}
	if r, err := filepath.EvalSymlinks(absPath); err == nil {
		absPath = filepath.Clean(r)
	}

	vol := filepath.VolumeName(absPath)
	if vol == "" {
		return "", fmt.Errorf("could not resolve volume for %s", absPath)
	}
	if strings.HasPrefix(vol, `\\`) {
		return filepath.Clean(vol + `\`), nil
	}
	if len(vol) >= 2 && vol[1] == ':' {
		return vol + string(os.PathSeparator), nil
	}
	return "", fmt.Errorf("unsupported volume name %q", vol)
}
