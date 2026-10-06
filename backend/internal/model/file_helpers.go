package model

import (
	"path/filepath"
	"strings"
)

func NormalizeManagedFileName(raw string) (string, string, bool) {
	name := filepath.Base(strings.TrimSpace(raw))
	switch name {
	case "", ".", "..":
		return "", "", false
	}

	extension := strings.ToLower(strings.TrimSpace(filepath.Ext(name)))
	return name, extension, true
}

// NormalizeManagedFolderName 校验托管目录名：必须是单段路径名，拒绝空值、
// "."/".."、路径分隔符（含 Windows 的 `\`）与控制字符，防止通过目录名把
// SourcePath 拼到托管根之外。
func NormalizeManagedFolderName(raw string) (string, bool) {
	name := strings.TrimSpace(raw)
	if name != filepath.Base(name) || strings.Contains(name, "\\") {
		return "", false
	}
	switch name {
	case "", ".", "..":
		return "", false
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return "", false
		}
	}
	return name, true
}

func ManagedFileTitle(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}

	title := strings.TrimSuffix(name, filepath.Ext(name))
	if strings.TrimSpace(title) == "" {
		return name
	}
	return title
}

func BuildManagedFilePath(folderSourcePath *string, fileName string) string {
	fileName = filepath.Base(strings.TrimSpace(fileName))
	if fileName == "" || folderSourcePath == nil || strings.TrimSpace(*folderSourcePath) == "" {
		return ""
	}
	return filepath.Join(strings.TrimSpace(*folderSourcePath), fileName)
}
