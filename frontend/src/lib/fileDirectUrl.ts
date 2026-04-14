/** 文件下载/播放地址：单独配置的 playback_url > 文件夹直链前缀拼接 > 本站下载接口 */
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

/** 是否走本站 /api 下载（用于统计下载量等） */
export function fileUsesBackendDownloadHref(href: string): boolean {
  const u = href.trim();
  return u.startsWith("/api/public/files/") && u.includes("/download");
}
