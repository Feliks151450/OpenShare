package repository

import (
	"gorm.io/gorm"
)

// FilesNotUnderHiddenPublicCatalogRoot 限定 files：排除其所属托管根设置了 hide_public_catalog 的文件；
// folder_id 为空的文件仍保留（与首页根列表行为一致）。
func FilesNotUnderHiddenPublicCatalogRoot() func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where(
			`(files.folder_id IS NULL OR NOT EXISTS (
WITH RECURSIVE ancestor AS (
  SELECT id, parent_id, hide_public_catalog FROM folders WHERE id = files.folder_id
  UNION ALL
  SELECT f.id, f.parent_id, f.hide_public_catalog
  FROM folders f
  INNER JOIN ancestor a ON f.id = a.parent_id
)
SELECT 1 FROM ancestor WHERE parent_id IS NULL AND COALESCE(hide_public_catalog, 0) != 0
))`,
		)
	}
}

// FoldersNotUnderHiddenPublicCatalogRoot 限定 folders：排除自身或任意上级托管根为 hide_public_catalog 的目录（含被隐藏的根）。
func FoldersNotUnderHiddenPublicCatalogRoot() func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where(
			`NOT EXISTS (
WITH RECURSIVE ancestor AS (
  SELECT id, parent_id, hide_public_catalog FROM folders WHERE id = folders.id
  UNION ALL
  SELECT f.id, f.parent_id, f.hide_public_catalog
  FROM folders f
  INNER JOIN ancestor a ON f.id = a.parent_id
)
SELECT 1 FROM ancestor WHERE parent_id IS NULL AND COALESCE(hide_public_catalog, 0) != 0
)`,
		)
	}
}
