/**
 * 收藏功能 composable
 * 使用 localStorage 存储收藏的文件和文件夹 ID
 * 提供响应式的收藏状态管理和操作方法
 */

import { computed, ref, watch } from "vue";

/** 收藏项类型 */
export interface FavoriteItem {
  /** 资源 ID */
  id: string;
  /** 资源类型：文件或文件夹 */
  kind: "file" | "folder";
}

const STORAGE_KEY = "openShareFavorites";

/** 全局共享的收藏列表响应式状态 */
const favoriteItems = ref<FavoriteItem[]>(loadFromStorage());

/** 导出响应式状态供控制台 API 使用 */
export { favoriteItems };

/** 从 localStorage 加载收藏数据 */
function loadFromStorage(): FavoriteItem[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    // 兼容旧格式（纯 ID 数组）和新格式
    return parsed
      .map((item: unknown) => {
        if (typeof item === "string") {
          // 旧格式：纯 ID 字符串，无法确定类型，默认跳过
          return null;
        }
        if (item && typeof item === "object" && "id" in item && "kind" in item) {
          const obj = item as { id: unknown; kind: unknown };
          if (typeof obj.id === "string" && (obj.kind === "file" || obj.kind === "folder")) {
            return { id: obj.id, kind: obj.kind };
          }
        }
        return null;
      })
      .filter((item: unknown): item is FavoriteItem => item !== null);
  } catch {
    return [];
  }
}

/** 将收藏数据持久化到 localStorage */
function saveToStorage(items: FavoriteItem[]) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(items));
}

/** 监听数据变化自动持久化 */
watch(favoriteItems, (items) => {
  saveToStorage(items);
}, { deep: true });

/** 收藏 ID 集合（用于快速查找） */
const favoriteIdSet = computed(() => new Set(favoriteItems.value.map((item) => item.id)));

/**
 * 收藏功能 composable
 * 提供收藏状态查询、切换和管理方法
 */
export function useFavorites() {
  /** 判断某个资源是否已收藏 */
  function isFavorited(id: string): boolean {
    return favoriteIdSet.value.has(id);
  }

  /** 切换收藏状态：已收藏则取消，未收藏则添加 */
  function toggleFavorite(id: string, kind: "file" | "folder") {
    const index = favoriteItems.value.findIndex((item) => item.id === id);
    if (index >= 0) {
      favoriteItems.value.splice(index, 1);
    } else {
      favoriteItems.value.push({ id, kind });
    }
  }

  /** 取消收藏 */
  function removeFavorite(id: string) {
    const index = favoriteItems.value.findIndex((item) => item.id === id);
    if (index >= 0) {
      favoriteItems.value.splice(index, 1);
    }
  }

  /** 获取所有收藏项 */
  function getFavoriteItems(): FavoriteItem[] {
    return [...favoriteItems.value];
  }

  /** 获取收藏数量 */
  const favoriteCount = computed(() => favoriteItems.value.length);

  return {
    favoriteItems,
    favoriteCount,
    isFavorited,
    toggleFavorite,
    removeFavorite,
    getFavoriteItems,
  };
}
