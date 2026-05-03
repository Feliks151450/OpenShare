import type { RouteLocationRaw, Router } from "vue-router";

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
 */
export function markdownHrefToVueRoute(hrefRaw: string): RouteLocationRaw | null {
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

  let u: URL;
  try {
    u = new URL(trimmed, window.location.href);
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

/** 捕获阶段委托：Markdown 正文中的站内链使用 Router 跳转，避免整页刷新。 */
export function onMarkdownLinkClickCapture(ev: MouseEvent, router: Pick<Router, "push">): void {
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
  const route = markdownHrefToVueRoute(href);
  if (route == null) {
    return;
  }
  ev.preventDefault();
  void router.push(route);
}
