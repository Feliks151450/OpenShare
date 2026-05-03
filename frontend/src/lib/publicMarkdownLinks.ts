import type { RouteLocationRaw, Router } from "vue-router";

/** 与默认 Tailwind `xl` 断点（1280px）一致，用于与首页 `xl:` 侧栏抽屉等布局对齐 */
export function isViewportTailwindXlMin(): boolean {
  if (typeof window === "undefined") {
    return false;
  }
  return window.matchMedia("(min-width: 1280px)").matches;
}

/** 与同页 document 算不算「同源」Markdown 站内导航（含 file://） */
function isSameDocumentOriginForMarkdownNav(resolved: URL): boolean {
  if (typeof window === "undefined") {
    return false;
  }
  if (window.location.protocol === "file:" && resolved.protocol === "file:") {
    return true;
  }
  return resolved.origin === window.location.origin;
}

/**
 * 将 Markdown 中超链接转为 Vue Router 目标（若为站内公开路由）。
 *
 * 支持：
 * - 相对或绝对：`./files/<id>`、`/files/<id>`、`../?folder=<id>`（由浏览器 URL 解析结果决定）
 * - 首页：`/`、`/?folder=<id>`、`/?root=1`（不支持其它查询参数）
 * - 上传页：`/upload`
 * - 文件详情：`/files/<id>?t=` 可选
 *
 * http(s) 外链、mailto、tel、锚点占位等返回 null，走浏览器默认行为。
 *
 * @param options.resolutionBaseHref 解析相对路径时使用的基准绝对 URL。
 *        例如在首页仍以 `/` 挂载时预览文件详情抽屉，站内 `./other-id` 需相对 `/files/当前id`；
 *        若不传则用 `window.location.href`。
 */
export function markdownHrefToVueRoute(
  hrefRaw: string,
  options?: { resolutionBaseHref?: string },
): RouteLocationRaw | null {
  if (typeof window === "undefined") {
    return null;
  }
  const trimmed = hrefRaw.trim();
  if (!trimmed) {
    return null;
  }
  if (/^(mailto:|tel:|javascript:)/i.test(trimmed)) {
    return null;
  }
  if (trimmed.startsWith("#") && !trimmed.slice(1).includes("/")) {
    return null;
  }

  const baseForResolve =
    (options?.resolutionBaseHref ?? "").trim() || window.location.href;

  let u: URL;
  try {
    u = new URL(trimmed, baseForResolve);
  } catch {
    return null;
  }

  if (!isSameDocumentOriginForMarkdownNav(u)) {
    return null;
  }

  const pathMatch = u.pathname.match(/^\/files\/([^/]+)\/?$/);
  if (pathMatch) {
    const fileID = decodeURIComponent(pathMatch[1] ?? "").trim();
    if (!fileID) {
      return null;
    }
    const query: Record<string, string> = {};
    const t = u.searchParams.get("t");
    if (t != null && t !== "") {
      query.t = t;
    }
    const out: RouteLocationRaw = { name: "public-file-detail", params: { fileID } };
    if (Object.keys(query).length > 0) {
      Object.assign(out, { query });
    }
    return out;
  }

  if (u.pathname === "/upload" || u.pathname === "/upload/") {
    return { name: "public-upload" };
  }

  const isHomePath = u.pathname === "/" || u.pathname === "";
  if (!isHomePath) {
    return null;
  }

  for (const k of u.searchParams.keys()) {
    if (k !== "folder" && k !== "root") {
      return null;
    }
  }

  const folderRaw = u.searchParams.get("folder");
  const folder = folderRaw != null ? folderRaw.trim() : "";
  const rootRaw = u.searchParams.get("root");
  const rootOn = rootRaw === "1" || rootRaw === "true";

  const query: Record<string, string> = {};
  if (folder !== "") {
    query.folder = folder;
  }
  if (rootOn) {
    query.root = "1";
  }

  if (Object.keys(query).length > 0) {
    return { name: "public-home", query };
  }
  return { name: "public-home" };
}

/** 站内 Markdown 导航解析结果是否为公开资料首页（目录/根目录视图） */
export function markdownRouteIsCatalogHome(route: RouteLocationRaw): boolean {
  if (typeof route !== "object" || route === null) {
    return false;
  }
  if ("name" in route && route.name === "public-home") {
    return true;
  }
  return false;
}

/** 若为 `public-file-detail`，返回解码后的文件 id（站内 Markdown / 预览抽屉等）。 */
export function markdownRoutePublicFileDetailId(route: RouteLocationRaw): string | null {
  if (typeof route !== "object" || route === null || !("name" in route)) {
    return null;
  }
  if (route.name !== "public-file-detail") {
    return null;
  }
  const rawId = route.params?.fileID;
  const part =
    rawId === undefined || rawId === null ? "" : Array.isArray(rawId) ? String(rawId[0] ?? "") : String(rawId);
  const id = decodeURIComponent(part).trim();
  return id || null;
}

/** 站内 Markdown 导航可选项（相对路径基准、劫持 push）。 */
export type MarkdownRouterNavOptions = {
  resolutionBaseHref?: string;
  /** 返回 true 表示已由调用方处理，跳过 `router.push` */
  interceptPush?: (route: RouteLocationRaw) => boolean;
  /**
   * Markdown 链解析为资料目录（`public-home`）时：先由页面弹窗确认，`true` 再 `router.push`。
   */
  confirmBeforeMarkdownCatalogNavigate?: (route: RouteLocationRaw) => Promise<boolean>;
};

/** 捕获阶段委托：Markdown 正文中的站内链使用 Router 跳转，避免整页刷新。 */
export function onMarkdownLinkClickCapture(
  ev: MouseEvent,
  router: Pick<Router, "push">,
  markdownOptions?: MarkdownRouterNavOptions,
): void {
  if (ev.defaultPrevented || ev.button !== 0 || ev.metaKey || ev.ctrlKey || ev.shiftKey || ev.altKey) {
    return;
  }
  const t = ev.target;
  if (!(t instanceof Element)) {
    return;
  }
  const anchor = t.closest(".markdown-content a[href]");
  if (!(anchor instanceof HTMLAnchorElement)) {
    return;
  }
  const href = anchor.getAttribute("href") ?? "";
  const route = markdownHrefToVueRoute(href, markdownOptions);
  if (route == null) {
    return;
  }
  ev.preventDefault();
  if (markdownOptions?.interceptPush?.(route)) {
    return;
  }

  const confirmCatalog = markdownOptions?.confirmBeforeMarkdownCatalogNavigate;
  if (confirmCatalog != null && markdownRouteIsCatalogHome(route)) {
    void (async () => {
      try {
        if (await confirmCatalog(route)) {
          await router.push(route);
        }
      } catch {
        /* 弹窗或导航失败时忽略 */
      }
    })();
    return;
  }

  void router.push(route);
}
