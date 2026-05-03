/** 控制台 `getFileInfo` 返回结构与 GET `/api/public/files/:id` 一致字段的归一形态。 */

export interface PublicFileDetailPayload {
  id: string;
  name: string;
  extension: string;
  folder_id: string;
  path: string;
  /** 托管磁盘绝对路径（可解析时为非空） */
  storage_path?: string;
  description?: string;
  remark?: string;
  mime_type: string;
  playback_url?: string;
  playback_fallback_url?: string;
  cover_url?: string;
  folder_direct_download_url?: string;
  download_allowed?: boolean;
  download_policy?: string;
  size: number;
  uploaded_at: string;
  download_count?: number;
}

/** 供控制台脚本使用的可读文件信息快照 */
export type OpenSharePublicFileInfo = {
  id: string;
  name: string;
  extension: string;
  sizeBytes: number;
  mimeType: string;
  folderId: string;
  /** 服务端返回的资料目录层级展示路径（通常为目录名层级） */
  path: string;
  /** 同上接口字段：磁盘上的绝对路径；无托管路径时省略 */
  storagePath?: string;
  description: string;
  remark: string;
  uploadedAt: string;
  downloadCount: number;
  downloadAllowed: boolean;
  downloadPolicy?: string;
  coverUrl?: string;
  playbackUrl: string | null;
  playbackFallbackUrl: string | null;
  folderDirectDownloadUrl: string | null;
  /**
   * 与站内播放器/复制直链优先级一致：`playback_url` > `folder_direct_download_url`
   * > `/api/public/files/:id/download`。
   */
  effectiveDownloadHref: string;
  /**
   * 上述有效地址的同源绝对 URL（外链 http(s) 时与 `effectiveDownloadHref` 相同）。
   */
  effectiveDownloadAbsoluteUrl: string;
  /** 本站公开下载路由（不含域名），用于辨认是否走本站统计接口 */
  siteDownloadHref: string;
};

function stripOrNull(raw: unknown): string | null {
  const s = String(raw ?? "").trim();
  return s ? s : null;
}

function toEffectiveAbsoluteHref(effectiveHref: string): string {
  const href = effectiveHref.trim();
  if (!href) {
    return "";
  }
  if (href.startsWith("http://") || href.startsWith("https://")) {
    return href;
  }
  if (typeof window === "undefined") {
    return href;
  }
  return new URL(href.startsWith("/") ? href : `/${href}`, window.location.origin).href;
}

/** @param resolveSiteDownloadHref 返回本站 `/download` 的完整请求路径（含 API 前缀，可为相对 `/api/...`） */
export function buildOpenSharePublicFileInfo(
  payload: PublicFileDetailPayload,
  resolveSiteDownloadHref: (fileId: string) => string,
  resolveEffectiveDownloadHref: (fileId: string, playbackUrl?: string | null, folderDirect?: string | null) => string,
): OpenSharePublicFileInfo {
  const id = String(payload.id ?? "").trim();
  const eff = resolveEffectiveDownloadHref(id, payload.playback_url, payload.folder_direct_download_url).trim();
  return {
    id,
    name: String(payload.name ?? ""),
    extension: String(payload.extension ?? ""),
    sizeBytes: Number(payload.size) || 0,
    mimeType: String(payload.mime_type ?? ""),
    folderId: String(payload.folder_id ?? ""),
    path: String(payload.path ?? ""),
    storagePath:
      typeof payload.storage_path === "string" && payload.storage_path.trim() !== ""
        ? payload.storage_path.trim()
        : undefined,
    description: String(payload.description ?? ""),
    remark: String(payload.remark ?? ""),
    uploadedAt: String(payload.uploaded_at ?? ""),
    downloadCount: Number(payload.download_count) || 0,
    downloadAllowed: payload.download_allowed !== false,
    downloadPolicy: typeof payload.download_policy === "string" ? payload.download_policy : undefined,
    coverUrl: typeof payload.cover_url === "string" && payload.cover_url.trim() ? payload.cover_url.trim() : undefined,
    playbackUrl: stripOrNull(payload.playback_url),
    playbackFallbackUrl: stripOrNull(payload.playback_fallback_url),
    folderDirectDownloadUrl: stripOrNull(payload.folder_direct_download_url),
    effectiveDownloadHref: eff,
    effectiveDownloadAbsoluteUrl: toEffectiveAbsoluteHref(eff),
    siteDownloadHref: resolveSiteDownloadHref(id),
  };
}
