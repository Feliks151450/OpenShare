//go:build !windows

package storage

import (
	"fmt"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func volumeRootForPath(absPath string) (string, error) {
	absPath = filepath.Clean(absPath)
	if !filepath.IsAbs(absPath) {
		return "", fmt.Errorf("path must be absolute: %s", absPath)
	}
	if r, err := filepath.EvalSymlinks(absPath); err == nil {
		absPath = filepath.Clean(r)
	}

	cur := absPath
	for {
		parent := filepath.Dir(cur)
		if parent == cur {
			return cur, nil
		}
		var cst, pst unix.Stat_t
		if err := unix.Lstat(cur, &cst); err != nil {
			return "", fmt.Errorf("stat %s: %w", cur, err)
		}
		if err := unix.Lstat(parent, &pst); err != nil {
			return "", fmt.Errorf("stat %s: %w", parent, err)
		}
		if cst.Dev != pst.Dev {
			return cur, nil
		}
		cur = parent
	}
}
