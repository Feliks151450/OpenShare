package repository

import (
	"gorm.io/gorm"
)

// hiddenDescendantCTE returns a CTE expression that computes all folder IDs
// descending from roots where hide_public_catalog is true.
const hiddenDescendantCTE = `
WITH RECURSIVE hidden_descendant AS (
	SELECT id FROM folders WHERE parent_id IS NULL AND COALESCE(hide_public_catalog, 0) != 0
	UNION ALL
	SELECT f.id FROM folders f INNER JOIN hidden_descendant h ON f.parent_id = h.id
)
`

// FilesNotUnderHiddenPublicCatalogRoot 限定 files：排除其所属托管根设置了 hide_public_catalog 的文件。
// 使用单次 CTE 计算所有隐藏根的后代，替代原来的逐行递归 CTE。
func FilesNotUnderHiddenPublicCatalogRoot() func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where(
			`files.folder_id IS NULL OR files.folder_id NOT IN (` +
				hiddenDescendantCTE +
				`SELECT id FROM hidden_descendant` +
				`)`,
		)
	}
}

// FoldersNotUnderHiddenPublicCatalogRoot 限定 folders：排除自身或任意上级托管根为 hide_public_catalog 的目录。
// 使用单次 CTE 计算所有隐藏根的后代。
func FoldersNotUnderHiddenPublicCatalogRoot() func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where(
			`folders.id NOT IN (` +
				hiddenDescendantCTE +
				`SELECT id FROM hidden_descendant` +
				`)`,
		)
	}
}
