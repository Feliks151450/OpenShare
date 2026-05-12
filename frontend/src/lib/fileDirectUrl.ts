/** 文件下载/播放地址：playback_url（CDN/直链） > 文件夹直链前缀拼接 > 本站下载接口 */
export function fileEffectiveDownloadHref(
  fileId: string,
  playbackUrl?: string | null,
  folderDirectDownloadUrl?: string | null,
): string {
  const p = (playbackUrl ?? "").trim();
  if (p) return p;
  const f = (folderDirectDownloadUrl ?? "").trim();
  if (f) return f;
  return `/api/public/files/${encodeURIComponent(fileId)}/download`;
}

/** 是否为本站公开文件下载 URL（相对路径或绝对 URL） */
export function isBackendPublicFileDownloadHref(href: string): boolean {
  const u = href.trim();
  if (u.startsWith("/api/public/files/") && u.includes("/download")) {
    return true;
  }
  try {
    const parsed = new URL(u);
    return parsed.pathname.includes("/api/public/files/") && parsed.pathname.includes("/download");
  } catch {
    return false;
  }
}

/** 是否走本站 /api 下载（用于统计下载量等） */
export function fileUsesBackendDownloadHref(href: string): boolean {
  return isBackendPublicFileDownloadHref(href);
}

/**
 * 为本站 /download 地址附加 inline=1，使 PDF 等在 iframe 中使用 Content-Disposition: inline（避免被当作附件下载）。
 * 外链 playback / 文件夹直链不追加参数，以免破坏 CDN 签名等。
 */
export function withBackendDownloadInlinePreviewParam(absoluteUrl: string): string {
  if (!absoluteUrl.trim() || !isBackendPublicFileDownloadHref(absoluteUrl)) {
    return absoluteUrl;
  }
  const sep = absoluteUrl.includes("?") ? "&" : "?";
  return `${absoluteUrl}${sep}inline=1`;
}
