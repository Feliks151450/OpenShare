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
      if (!["name", "download", "format", "modified"].includes(mode)) {
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

  window.OpenShare = {
    version: "1.0",
    runtime: "spa",
    nav,
    home,
    staticData: staticDataLoader,
  };
}
