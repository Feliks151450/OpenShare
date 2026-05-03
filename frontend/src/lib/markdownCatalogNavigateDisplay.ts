import type { RouteLocationRaw } from "vue-router";

import { HttpError, httpClient } from "./http/client";
import { renderSimpleMarkdown } from "./markdown";

/** 文件夹 Markdown 跳转确认框中的展示文案 */
export type MarkdownCatalogConfirmPresentation = {
  /** 简短状态：加载中 | 就绪 */
  loading: boolean;
  /** 目标类型：具体文件夹 / 根列表 / 无 folder 的首页 */
  variant: "folder" | "root" | "home";
  /** 文件夹名，或根/首页说明标题 */
  headline: string;
  /** 根/首页说明、加载中提示、或文件夹拉取失败的说明文案 */
  detail?: string;
  /** 非空表示链向某公开文件夹（用于展示异步拉取的信息） */
  folderId?: string;
  /** 备注（trim 后非空才展示） */
  remark?: string;
  /** 已格式化的「x 个文件 · 总大小」 */
  filesSummary?: string;
  /** 简介 Markdown 渲染后的 HTML（仅用于 `markdown-content` v-html） */
  descriptionHtml?: string;
  /** 成功拉到 GET /public/folders/:id 时为 true；false 或未设表示仅展示 headline/detail（如拉取失败） */
  folderDetailLoaded?: boolean;
  /** 面包屑名称链，不包含「路径：」前缀 */
  folderPathTrail?: string;
  /** 无面包屑时展示的文件夹 ID 说明行 */
  folderIdLine?: string;
};

function formatFolderTotalSizeBytes(size: number): string {
  if (!Number.isFinite(size) || size < 0) {
    return "—";
  }
  if (size < 1024) {
    return `${size} B`;
  }
  if (size < 1024 * 1024) {
    return `${(size / 1024).toFixed(2)} KB`;
  }
  if (size < 1024 * 1024 * 1024) {
    return `${(size / (1024 * 1024)).toFixed(2)} MB`;
  }
  return `${(size / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

type PublicFolderDetailJson = {
  name?: string;
  description?: string;
  remark?: string;
  file_count?: number;
  total_size?: number;
  breadcrumbs?: Array<{ id: string; name: string }>;
};

function folderIdFromCatalogRoute(route: RouteLocationRaw): string | null {
  if (typeof route !== "object" || route === null || !("query" in route) || route.query == null) {
    return null;
  }
  const raw = typeof route.query.folder === "string" ? route.query.folder.trim() : "";
  return raw || null;
}

function routeHasCatalogRoot(route: RouteLocationRaw): boolean {
  if (typeof route !== "object" || route === null || !("query" in route) || route.query == null) {
    return false;
  }
  const raw = typeof route.query.root === "string" ? route.query.root : "";
  return raw === "1" || raw === "true";
}

/** 初始展示（打开弹窗瞬间，避免空白） */
export function markdownCatalogNavigateInitialPresentation(route: RouteLocationRaw): MarkdownCatalogConfirmPresentation {
  if (routeHasCatalogRoot(route)) {
    return {
      loading: false,
      variant: "root",
      headline: "根目录视图（首页根列表）",
      detail: "将按本站规则切换到根列表，相当于 ?root=1。",
    };
  }
  const fid = folderIdFromCatalogRoute(route);
  if (fid) {
    return {
      loading: true,
      variant: "folder",
      folderId: fid,
      headline: "正在加载文件夹信息…",
      detail: `文件夹 ID：${fid}`,
    };
  }
  return {
    loading: false,
    variant: "home",
    headline: "公开资料首页",
    detail: "将切换到顶层资料浏览（无特定文件夹筛选）。",
  };
}

/** 异步补全文件夹显示名与路径（根/首页不写 loading） */
export async function hydrateMarkdownCatalogNavigatePresentation(
  route: RouteLocationRaw,
): Promise<MarkdownCatalogConfirmPresentation> {
  if (routeHasCatalogRoot(route)) {
    return markdownCatalogNavigateInitialPresentation(route);
  }
  const fid = folderIdFromCatalogRoute(route);
  if (!fid) {
    return markdownCatalogNavigateInitialPresentation(route);
  }

  try {
    const detail = await httpClient.get<PublicFolderDetailJson>(`/public/folders/${encodeURIComponent(fid)}`);
    const name = (detail.name ?? "").trim() || `（未命名 · ${fid.slice(0, 8)}…）`;
    const crumbs = detail.breadcrumbs ?? [];
    const pathJoined =
      crumbs.length > 0
        ? crumbs
            .map((c) => (c.name ?? "").trim())
            .filter(Boolean)
            .join(" / ")
            .trim()
        : "";
    const remark = (detail.remark ?? "").trim();
    const descRaw = (detail.description ?? "").trim();
    const fileCount = Number(detail.file_count ?? 0);
    const totalSize = Number(detail.total_size ?? 0);

    return {
      loading: false,
      variant: "folder",
      folderId: fid,
      folderDetailLoaded: true,
      headline: name,
      folderPathTrail: pathJoined || undefined,
      folderIdLine: pathJoined ? undefined : `文件夹 ID：${fid}`,
      remark: remark || undefined,
      filesSummary: `${Number.isFinite(fileCount) ? fileCount : 0} 个文件 · ${formatFolderTotalSizeBytes(totalSize)}`,
      descriptionHtml: descRaw ? renderSimpleMarkdown(descRaw) : undefined,
    };
  } catch (err: unknown) {
    let hint = "无法获取该文件夹的公开详情。";
    if (err instanceof HttpError && err.status === 404) {
      hint = "该文件夹不存在或未公开。";
    }
    return {
      loading: false,
      variant: "folder",
      folderId: fid,
      folderDetailLoaded: false,
      headline: `文件夹（${fid.slice(0, 8)}${fid.length > 8 ? "…" : ""}）`,
      detail: `${hint} 仍可尝试「前往」，若无权或不存在将在列表页提示。`,
    };
  }
}
