/**
 * StaticDataLoader — 从 CDN 直链加载预导出的 JSON 数据，作为 API 请求的替代。
 *
 * 两种数据文件：
 * 1. 全局数据文件（包含 announcements, hot_files, latest_files, root_folders,
 *    download_policy, file_tags）
 * 2. 按托管目录拆分的数据文件（每个文件包含一个目录树下的所有目录详情、子目录和文件）
 *
 * 使用方式：
 *   import { staticDataLoader } from "...";
 *   staticDataLoader.configure({ globalUrl: "https://cdn.example.com/global.json" });
 *   const ok = await staticDataLoader.loadGlobal();
 *   if (ok) {
 *     const folders = staticDataLoader.rootFolders; // 来自缓存，不请求服务器
 *   }
 */

// ─── 类型定义（与后端导出 JSON 保持一致） ──────────────────────────

export interface ExportAnnouncement {
  id: string;
  title: string;
  content: string;
  status: string;
  is_pinned: boolean;
  published_at: string | null;
  created_at: string;
  updated_at: string;
}

export interface ExportFileTag {
  id: string;
  name: string;
  color: string;
  sort_order: number;
}

export interface ExportDownloadPolicy {
  large_download_confirm_bytes: number;
  wide_layout_extensions: string;
}

export interface ExportPublicFolderItem {
  id: string;
  name: string;
  description: string;
  remark: string;
  cover_url: string;
  download_allowed: boolean;
  updated_at: string;
  file_count: number;
  download_count: number;
  total_size: number;
}

export interface ExportPublicFileItem {
  id: string;
  name: string;
  description: string;
  remark: string;
  extension: string;
  cover_url: string;
  playback_url: string;
  folder_direct_download_url: string;
  download_allowed: boolean;
  uploaded_at: string;
  download_count: number;
  size: number;
  tags?: Array<{ id: string; name: string; color: string }>;
}

export interface ExportPublicFolderDetail {
  id: string;
  name: string;
  description: string;
  remark: string;
  cover_url: string;
  parent_id: string | null;
  breadcrumbs: Array<{ id: string; name: string }>;
  file_count: number;
  download_count: number;
  total_size: number;
  updated_at: string;
  direct_link_prefix: string;
  download_allowed: boolean;
  download_policy: string;
  hide_public_catalog?: boolean;
}

export interface ExportHotFiles {
  items: ExportPublicFileItem[];
}

export interface ExportLatestFiles {
  items: ExportPublicFileItem[];
}

// ─── 全局导出数据文件结构 ──────────────────────────────────────

export interface GlobalExportData {
  version: number;
  exported_at: string;
  announcements: ExportAnnouncement[];
  hot_files: ExportHotFiles;
  latest_files: ExportLatestFiles;
  root_folders: ExportPublicFolderItem[];
  download_policy: ExportDownloadPolicy;
  file_tags: ExportFileTag[];
}

// ─── 目录导出数据文件结构 ──────────────────────────────────────

export interface DirectoryExportManagedRoot {
  id: string;
  name: string;
  description: string;
  cover_url: string;
  source_path: string;
  download_allowed: boolean;
  file_count: number;
  download_count: number;
  total_size: number;
  updated_at: string;
}

export interface DirectoryExportEntry {
  detail: ExportPublicFolderDetail;
  folders: ExportPublicFolderItem[];
  files: ExportPublicFileItem[];
  /** fileId → 完整文件详情（与 GET /public/files/:id 返回体一致） */
  file_details?: Record<string, Record<string, unknown>>;
}

export interface DirectoryExportData {
  version: number;
  exported_at: string;
  managed_root: DirectoryExportManagedRoot;
  directories: Record<string, DirectoryExportEntry>;
}

// ─── 配置 ────────────────────────────────────────────────────────

export interface StaticDataConfig {
  /** 全局数据 JSON 文件的 CDN 直链 */
  globalUrl?: string;
  /** 目录数据 JSON 文件的基础 URL，文件名格式为 {folderName}.json */
  directoryBaseUrl?: string;
}

// ─── 加载器类 ────────────────────────────────────────────────────

class StaticDataLoader {
  private config: StaticDataConfig = {};
  private _global: GlobalExportData | null = null;
  private _directories = new Map<string, DirectoryExportData>();
  private _globalLoading = false;
  private _globalError: string | null = null;

  // ── 配置 ─────────────────────────────────────────────────────

  configure(config: StaticDataConfig) {
    this.config = { ...this.config, ...config };
  }

  // ── 加载 ─────────────────────────────────────────────────────

  /** 加载全局数据，成功返回 true。
   *
   *  三种用法：
   *  - 传 URL 字符串：`loadGlobal("https://cdn.example.com/global.json")`
   *  - 传已配置 globalUrl + 无参：`configure({ globalUrl }); loadGlobal()`
   *  - 直接传 JSON 对象：`loadGlobal({ version: 1, ... })` */
  async loadGlobal(input?: string | GlobalExportData): Promise<boolean> {
    // 直接传 JSON 对象
    if (input != null && typeof input === "object") {
      this._global = input as GlobalExportData;
      this._globalError = null;
      this._globalLoading = false;
      return true;
    }

    const targetUrl = (typeof input === "string" ? input : undefined) ?? this.config.globalUrl;
    if (!targetUrl || this._globalLoading) return false;

    this._globalLoading = true;
    this._globalError = null;
    try {
      const response = await fetch(targetUrl);
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      this._global = (await response.json()) as GlobalExportData;
      return true;
    } catch (err: unknown) {
      this._globalError = err instanceof Error ? err.message : "Unknown error";
      this._global = null;
      return false;
    } finally {
      this._globalLoading = false;
    }
  }

  /** 加载托管目录数据，成功返回 true。
   *
   *  三种用法：
   *  - 直接传完整 URL：`loadDirectory("https://cdn.example.com/my-folder.json")`
   *  - 传文件名 + 已配置 directoryBaseUrl：先 `configure({ directoryBaseUrl })`，再 `loadDirectory("my-folder")`
   *  - 直接传 JSON 对象：`loadDirectory({ version: 1, managed_root: {...}, directories: {...} })` */
  async loadDirectory(input: string | DirectoryExportData, url?: string): Promise<boolean> {
    // 直接传 JSON 对象
    if (typeof input === "object") {
      const data = input as DirectoryExportData;
      if (!data.managed_root?.id) return false;
      this._directories.set(data.managed_root.id, data);
      return true;
    }

    const urlOrName = input as string;
    const targetUrl = url
      ?? (/^https?:\/\//.test(urlOrName) ? urlOrName : this.buildDirectoryUrl(urlOrName));
    if (!targetUrl) return false;

    try {
      const response = await fetch(targetUrl);
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      const data = (await response.json()) as DirectoryExportData;
      this._directories.set(data.managed_root.id, data);
      return true;
    } catch {
      return false;
    }
  }

  private buildDirectoryUrl(name: string): string | undefined {
    if (!this.config.directoryBaseUrl) return undefined;
    return `${this.config.directoryBaseUrl.replace(/\/$/, "")}/${name}.json`;
  }

  // ── 全局数据访问器 ────────────────────────────────────────────

  get hasGlobal(): boolean {
    return this._global !== null;
  }

  get globalLoading(): boolean {
    return this._globalLoading;
  }

  get globalError(): string | null {
    return this._globalError;
  }

  get globalExportedAt(): string | null {
    return this._global?.exported_at ?? null;
  }

  get announcements(): ExportAnnouncement[] | null {
    return this._global?.announcements ?? null;
  }

  get hotFiles(): ExportHotFiles | null {
    return this._global?.hot_files ?? null;
  }

  get latestFiles(): ExportLatestFiles | null {
    return this._global?.latest_files ?? null;
  }

  get rootFolders(): ExportPublicFolderItem[] | null {
    return this._global?.root_folders ?? null;
  }

  get downloadPolicy(): ExportDownloadPolicy | null {
    return this._global?.download_policy ?? null;
  }

  get fileTags(): ExportFileTag[] | null {
    return this._global?.file_tags ?? null;
  }

  // ── 目录数据访问器 ───────────────────────────────────────────

  hasDirectory(folderId: string): boolean {
    return this._directories.has(folderId);
  }

  getManagedRoot(folderId: string): DirectoryExportManagedRoot | null {
    return this._directories.get(folderId)?.managed_root ?? null;
  }

  /** 获取某个文件夹 ID 对应的目录视图数据（详情 + 子文件夹 + 文件），
   *  可直接用于渲染目录页。 */
  getDirectoryView(folderId: string): DirectoryExportEntry | null {
    const data = this._directories.get(folderId);
    if (!data) return null;
    return data.directories[folderId] ?? null;
  }

  /** 在已加载的目录数据中查找某个文件夹 ID 的数据。
   *  遍历所有已加载的目录导出数据，查找匹配的文件夹 ID。 */
  findDirectoryView(folderId: string): DirectoryExportEntry | null {
    for (const data of this._directories.values()) {
      const entry = data.directories[folderId];
      if (entry) return entry;
    }
    return null;
  }

  /** 在已加载的目录数据中查找某个文件的完整详情。
   *  返回与 GET /public/files/:id 相同结构的数据，未找到返回 null。 */
  findFileDetail(fileId: string): Record<string, unknown> | null {
    for (const data of this._directories.values()) {
      for (const entry of Object.values(data.directories)) {
        const fd = entry.file_details?.[fileId];
        if (fd) return fd;
      }
    }
    return null;
  }

  /** 根据托管根 ID 获取该托管树下所有已缓存的文件夹 ID 列表 */
  getFolderIdsForManagedRoot(managedRootId: string): string[] {
    const data = this._directories.get(managedRootId);
    if (!data) return [];
    return Object.keys(data.directories);
  }
}

/** 全局单例 */
export const staticDataLoader = new StaticDataLoader();
