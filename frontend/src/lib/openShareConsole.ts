import type { Router } from "vue-router";

import { fileEffectiveDownloadHref } from "./fileDirectUrl";
import { httpClient } from "./http/client";
import { getHomeConsoleHooks, type HomeListSortDirection, type HomeListSortMode, type HomeListViewMode } from "./homeConsoleBridge";
import {
  buildOpenSharePublicFileInfo,
  type OpenSharePublicFileInfo,
  type PublicFileDetailPayload,
} from "./openSharePublicFileInfo";
import { staticDataLoader } from "./staticDataLoader";

export type { OpenSharePublicFileInfo };

/** 收藏项类型 */
export interface FavoriteItem {
  /** 资源 ID */
  id: string;
  /** 资源类型：文件或文件夹 */
  kind: "file" | "folder";
}

/** localStorage 收藏存储 key */
const FAVORITES_STORAGE_KEY = "openShareFavorites";

declare global {
  interface Window {
    OpenShare?: OpenShareConsoleApi;
  }
}

/** 挂载在 window 上、供控制台调用的导航与首页列表 UI API（不涉及登录与管理权限）。 */
export type OpenShareConsoleApi = {
  version: string;
  runtime: "spa";
  nav: OpenShareConsoleNavSpa;
  home: OpenShareConsoleHomeSpa;
  /** 收藏管理（基于 localStorage，浏览器本地） */
  favorites: OpenShareConsoleFavorites;
  /** CDN 静态数据加载器：可配置预导出 JSON 直链，替代部分公开 API 请求 */
  staticData: typeof staticDataLoader;
};

export type ConsoleNavOpts = {
  replace?: boolean;
};

export type OpenShareConsoleNavSpa = {
  /** 当前路由摘要。自定义路径路由（如 /doc）下额外包含 resolvedFolderId 字段。 */
  getRoute(): {
    name: string;
    path: string;
    fullPath: string;
    params: Record<string, string>;
    query: Record<string, string>;
    /** 自定义路径路由下解析到的文件夹 ID（仅 public-custom-folder 路由有效） */
    resolvedFolderId?: string;
  };
  /** 跳转首页：`folder` > `root`，二者均不传则回到无前缀首页 */
  goHome(opts?: { folder?: string; root?: boolean } & ConsoleNavOpts): Promise<void>;
  /** 文件详情：`t` 为播放时间戳查询（与站内 `?t=` 一致） */
  goFile(fileID: string, opts?: { t?: string | number } & ConsoleNavOpts): Promise<void>;
  goUpload(opts?: ConsoleNavOpts): Promise<void>;
  /** 浏览器 history.back() */
  back(): void;
  /**
   * 等价于详情页「返回」：跳到文件所在目录首页；若在根文件则回到 `/`。
   * 在非详情页或未加载到详情数据时可为一次额外 GET `/public/files/:id`。
   */
  leaveFileTowardFolder(opts?: ConsoleNavOpts): Promise<boolean>;
  /** `GET /api/public/files/:id`：名称、体积、上架时间、`effectiveDownloadHref`（与站内直链优先级一致）等 */
  getFileInfo(fileID: string): Promise<OpenSharePublicFileInfo>;
  /** 根据自定义路径解析文件夹信息，未找到返回 null */
  resolveCustomPath(customPath: string): Promise<{ folder_id: string; name: string } | null>;
};

export type OpenShareConsoleHomeSpa = {
  /** 需在首页挂载完成后注册（见 Home.vue）；未打开首页时已写入 localStorage，下次进入首页生效 */
  setListView(mode: HomeListViewMode): boolean;
  setSortMode(mode: HomeListSortMode): boolean;
  setSortDirection(direction: HomeListSortDirection): boolean;
};

/** 收藏管理 API */
export type OpenShareConsoleFavorites = {
  /** 获取所有收藏项 */
  list(): FavoriteItem[];
  /** 判断某个资源是否已收藏 */
  has(id: string): boolean;
  /** 添加收藏 */
  add(id: string, kind: "file" | "folder"): boolean;
  /** 移除收藏 */
  remove(id: string): boolean;
  /** 切换收藏状态：已收藏则取消，未收藏则添加 */
  toggle(id: string, kind: "file" | "folder"): boolean;
  /** 一次性设置整个收藏列表（覆盖现有内容） */
  set(items: FavoriteItem[]): void;
  /** 清空所有收藏 */
  clear(): void;
  /** 收藏数量 */
  count(): number;
};

async function navigate(router: Router, loc: Parameters<Router["push"]>[0], opts?: ConsoleNavOpts) {
  const pending = opts?.replace ? router.replace(loc) : router.push(loc);
  await pending.catch((err: unknown) => {
    const nom = typeof err === "object" && err !== null ? (err as { name?: string }).name : "";
    if (nom === "NavigationDuplicated") {
      return;
    }
    console.warn("[OpenShare.nav] 跳转未完成：", err);
  });
}

function persistHomeListView(mode: HomeListViewMode) {
  window.localStorage.setItem("public-home-view-mode", mode);
}

function persistHomeSortMode(mode: HomeListSortMode) {
  window.localStorage.setItem("public-home-sort-mode", mode);
}

function persistHomeSortDirection(direction: HomeListSortDirection) {
  window.localStorage.setItem("public-home-sort-direction", direction);
}

/** 从 localStorage 加载收藏数据 */
function loadFavorites(): FavoriteItem[] {
  try {
    const raw = localStorage.getItem(FAVORITES_STORAGE_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed
      .map((item: unknown) => {
        if (item && typeof item === "object" && "id" in item && "kind" in item) {
          const obj = item as { id: unknown; kind: unknown };
          if (typeof obj.id === "string" && (obj.kind === "file" || obj.kind === "folder")) {
            return { id: obj.id, kind: obj.kind };
          }
        }
        return null;
      })
      .filter((item): item is FavoriteItem => item !== null);
  } catch {
    return [];
  }
}

/** 将收藏数据持久化到 localStorage */
function saveFavorites(items: FavoriteItem[]) {
  localStorage.setItem(FAVORITES_STORAGE_KEY, JSON.stringify(items));
}

export function mountOpenShareConsole(router: Router): void {
  const nav: OpenShareConsoleNavSpa = {
    getRoute() {
      const r = router.currentRoute.value;
      const qp: Record<string, string> = {};
      Object.entries(r.query).forEach(([k, v]) => {
        qp[k] = Array.isArray(v) ? String(v[0] ?? "") : String(v ?? "");
      });
      const pp: Record<string, string> = {};
      Object.entries(r.params).forEach(([k, v]) => {
        pp[k] = Array.isArray(v) ? String(v[0] ?? "") : String(v ?? "");
      });
      const result: ReturnType<OpenShareConsoleNavSpa["getRoute"]> = {
        name: String(r.name ?? ""),
        path: r.path,
        fullPath: r.fullPath,
        params: pp,
        query: qp,
      };
      // 自定义路径路由时，从 meta 读取解析后的文件夹 ID
      if (r.name === "public-custom-folder" && r.meta?.resolvedFolderId) {
        result.resolvedFolderId = String(r.meta.resolvedFolderId);
      }
      return result;
    },
    async goHome(opts = {}) {
      const replace = Boolean(opts.replace);
      const folder = String(opts.folder ?? "").trim();
      if (folder) {
        await navigate(router, { name: "public-home", query: { folder } }, { replace });
        return;
      }
      if (opts.root) {
        await navigate(router, { name: "public-home", query: { root: "1" } }, { replace });
        return;
      }
      await navigate(router, { name: "public-home" }, { replace });
    },
    async goFile(fileID, opts = {}) {
      const id = String(fileID ?? "").trim();
      if (!id) {
        throw new Error("[OpenShare.nav.goFile] 需要有效的 file id");
      }
      const query: Record<string, string> = {};
      const tRaw = opts.t;
      if (tRaw !== undefined && tRaw !== null && String(tRaw).trim() !== "") {
        query.t = String(tRaw).trim();
      }
      await navigate(
        router,
        { name: "public-file-detail", params: { fileID: id }, query },
        { replace: opts.replace },
      );
    },
    async goUpload(opts = {}) {
      await navigate(router, { path: "/upload" }, { replace: Boolean(opts.replace) });
    },
    back() {
      history.back();
    },
    async getFileInfo(fileID) {
      const id = String(fileID ?? "").trim();
      if (!id) {
        throw new Error("[OpenShare.nav.getFileInfo] 需要有效的 file id");
      }
      const payload = await httpClient.get<PublicFileDetailPayload>(`/public/files/${encodeURIComponent(id)}`);
      return buildOpenSharePublicFileInfo(
        payload,
        (fid) => `/api/public/files/${encodeURIComponent(fid)}/download`,
        fileEffectiveDownloadHref,
      );
    },
    async leaveFileTowardFolder(opts = {}) {
      const replace = Boolean(opts.replace);
      const r = router.currentRoute.value;
      if (r.name !== "public-file-detail") {
        return false;
      }
      const fid = String(r.params.fileID ?? "").trim();
      let folderID = "";
      try {
        if (fid) {
          const body = await httpClient.get<{ folder_id?: string }>(`/public/files/${encodeURIComponent(fid)}`);
          folderID = body.folder_id?.trim() ?? "";
        }
      } catch {
        /* 忽略，按无目录处理 */
      }
      if (folderID) await navigate(router, { name: "public-home", query: { folder: folderID } }, { replace });
      else await navigate(router, { name: "public-home" }, { replace });
      return true;
    },
    async resolveCustomPath(customPath) {
      const trimmed = String(customPath ?? "").trim();
      if (!trimmed) return null;
      try {
        const resp = await httpClient.get<{ folder_id: string; name: string }>(
          `/public/resolve-custom-path?path=${encodeURIComponent(trimmed)}`,
        );
        return resp;
      } catch {
        return null;
      }
    },
  };

  const home: OpenShareConsoleHomeSpa = {
    setListView(mode) {
      if (mode !== "cards" && mode !== "table") {
        console.warn("[OpenShare.home.setListView] 仅支持 cards 或 table");
        return false;
      }
      persistHomeListView(mode);
      getHomeConsoleHooks()?.setListView(mode);
      return true;
    },
    setSortMode(mode) {
      if (!["smart", "name", "download", "format", "modified"].includes(mode)) {
        console.warn("[OpenShare.home.setSortMode] mode 取值无效");
        return false;
      }
      persistHomeSortMode(mode);
      getHomeConsoleHooks()?.setListSort(mode);
      return true;
    },
    setSortDirection(direction) {
      if (direction !== "asc" && direction !== "desc") {
        console.warn("[OpenShare.home.setSortDirection] 仅支持 asc 或 desc");
        return false;
      }
      persistHomeSortDirection(direction);
      getHomeConsoleHooks()?.setListSortDirection(direction);
      return true;
    },
  };

  const favorites: OpenShareConsoleFavorites = {
    list() {
      return loadFavorites();
    },
    has(id) {
      const trimmed = String(id ?? "").trim();
      if (!trimmed) return false;
      return loadFavorites().some((item) => item.id === trimmed);
    },
    add(id, kind) {
      const trimmed = String(id ?? "").trim();
      if (!trimmed) {
        console.warn("[OpenShare.favorites.add] 需要有效的 id");
        return false;
      }
      if (kind !== "file" && kind !== "folder") {
        console.warn("[OpenShare.favorites.add] kind 必须为 file 或 folder");
        return false;
      }
      const items = loadFavorites();
      if (items.some((item) => item.id === trimmed)) {
        return true; // 已存在，视为成功
      }
      items.push({ id: trimmed, kind });
      saveFavorites(items);
      return true;
    },
    remove(id) {
      const trimmed = String(id ?? "").trim();
      if (!trimmed) return false;
      const items = loadFavorites();
      const index = items.findIndex((item) => item.id === trimmed);
      if (index < 0) return false;
      items.splice(index, 1);
      saveFavorites(items);
      return true;
    },
    toggle(id, kind) {
      const trimmed = String(id ?? "").trim();
      if (!trimmed) {
        console.warn("[OpenShare.favorites.toggle] 需要有效的 id");
        return false;
      }
      if (kind !== "file" && kind !== "folder") {
        console.warn("[OpenShare.favorites.toggle] kind 必须为 file 或 folder");
        return false;
      }
      const items = loadFavorites();
      const index = items.findIndex((item) => item.id === trimmed);
      if (index >= 0) {
        items.splice(index, 1);
      } else {
        items.push({ id: trimmed, kind });
      }
      saveFavorites(items);
      return true;
    },
    set(items) {
      if (!Array.isArray(items)) {
        console.warn("[OpenShare.favorites.set] 参数必须为数组");
        return;
      }
      const validated = items
        .filter((item) => {
          if (!item || typeof item !== "object" || !("id" in item) || !("kind" in item)) {
            return false;
          }
          const obj = item as { id: unknown; kind: unknown };
          return typeof obj.id === "string" && (obj.kind === "file" || obj.kind === "folder");
        })
        .map((item) => ({ id: (item as FavoriteItem).id, kind: (item as FavoriteItem).kind }));
      saveFavorites(validated);
    },
    clear() {
      saveFavorites([]);
    },
    count() {
      return loadFavorites().length;
    },
  };

  window.OpenShare = {
    version: "1.0",
    runtime: "spa",
    nav,
    home,
    favorites,
    staticData: staticDataLoader,
  };
}
