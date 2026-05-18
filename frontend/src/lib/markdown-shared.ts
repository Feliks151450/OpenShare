// Shared markdown utilities — no DOM dependencies, safe for both main thread and Web Workers.

export function escapeHtml(value: string) {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

export function isSafeImageUrlForSrc(url: string): boolean {
  const u = url.trim().toLowerCase();
  if (!u) return false;
  if (u.startsWith("javascript:") || u.startsWith("data:") || u.startsWith("vbscript:")) {
    return false;
  }
  return true;
}

const internalFileCoverRe = /^\/files\/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$/i;

function internalFileCoverHref(path: string): string | null {
  const m = path.trim().match(internalFileCoverRe);
  if (!m) return null;
  return `/api/public/files/${m[1]}/download`;
}

/** 将 Markdown 中的图片地址转为可在当前页展示的绝对 URL */
export function resolveMarkdownImageUrlToHref(raw: string): string {
  const u = raw.trim();
  if (!u) return "";
  if (/^https?:\/\//i.test(u)) return u;
  const internal = internalFileCoverHref(u);
  if (internal) return internal;
  if (typeof window === "undefined") return u;
  try {
    return new URL(u, window.location.href).href;
  } catch {
    return u;
  }
}
