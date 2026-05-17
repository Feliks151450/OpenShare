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
  cdn_mode?: boolean;
  directory_cdn_urls?: Record<string, string>;
  global_cdn_url?: string;
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
  /** 下载策略是否已通过任一途径加载（内嵌于 folder 响应 / loadGlobal / loadDownloadPolicy API） */
  private _policyApplied = false;
  /** folderId → cdn_url 映射，cdn_mode 下由根目录 folder 响应填充 */
  private _cdnUrlMap = new Map<string, string>();

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
      this._bootstrapCdnUrlMap();
      this._globalError = null;
      this._globalLoading = false;
      return true;
    }
    const targetUrl = (typeof input === "string" ? input : undefined) ?? this.config.globalUrl;
    if (!targetUrl || this._globalLoading) return false;

    this._globalLoading = true;
    this._globalError = null;
    try {
      const response = await fetch(targetUrl, { cache: "force-cache" });
      if (!response.ok) throw new Error(`HTTP ${response.status}`);
      this._global = (await response.json()) as GlobalExportData;
      this._bootstrapCdnUrlMap();
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

  /** 正在进行的 download-policy 请求 Promise，用于并发调用去重与等待 */
  private _policyPromise: Promise<void> | null = null;

  /** 缓存的 /public/download-policy 原始响应，供 live 页面读取（非 export JSON） */
  private _livePolicy: Record<string, unknown> | null = null;

  get livePolicy(): Record<string, unknown> | null {
    return this._livePolicy;
  }

  setLivePolicy(p: Record<string, unknown> | null): void {
    this._livePolicy = p;
  }

  /** 返回 download-policy 是否已加载完成 */
  get policyApplied(): boolean {
    return this._policyApplied;
  }

  markPolicyApplied(): void {
    this._policyApplied = true;
  }

  /** 若有正在进行的请求则返回其 Promise（供并发者 await），否则返回 null */
  get policyPromise(): Promise<void> | null {
    return this._policyPromise;
  }

  setPolicyPromise(p: Promise<void> | null): void {
    this._policyPromise = p;
  }

  /** 加载全局 CDN JSON（force-cache 优先本地缓存）。
   *  应在拿到 download-policy 后、发起其他 API 请求前调用。
   *  已加载过则跳过。 */
  async preloadGlobalCdn(url: string): Promise<boolean> {
    if (!url || this._global) return false;
    console.log(`[staticData] loading global CDN: ${url}`);
    return this.loadGlobal(url);
  }

  /** 存储从 download-policy 中获取的 global_cdn_url，供后续调用 */
  get globalCdnUrl(): string {
    return this._globalCdnUrl;
  }
  setGlobalCdnUrl(url: string): void {
    this._globalCdnUrl = url;
  }
  private _globalCdnUrl = "";

  /** 从根目录 folder 列表填充 cdn_url 映射（cdn_mode 开启时调用） */
  setCdnUrlMap(folders: Array<{ id: string; cdn_url?: string }>): void {
    this._cdnUrlMap.clear();
    for (const f of folders) {
      const url = (f.cdn_url ?? "").trim();
      if (url) this._cdnUrlMap.set(f.id, url);
    }
  }

  /** 从普通对象 { folderId: cdnUrl } 填充 cdn_url 映射 */
  setCdnUrlMapFromObject(map: Record<string, string>): void {
    for (const [id, url] of Object.entries(map)) {
      if (url && url.trim()) this._cdnUrlMap.set(id, url.trim());
    }
  }

  /** 按需加载目录数据：若该 folderId 有 cdn_url 映射且未缓存，则拉取 */
  async ensureDirectoryLoaded(folderId: string): Promise<void> {
    if (this._directories.has(folderId)) return;
    const url = this._cdnUrlMap.get(folderId);
    if (!url) return;
    await this.loadDirectory(url);
  }

  /** 用 force-cache 探测浏览器缓存中已有哪些 CDN JSON。
   *  拿到 directory_cdn_urls 后立即调用；命中缓存则秒加载（不产生网络请求），
   *  未命中会被浏览器自动回退到网络请求（等同于提前预加载）。
   *  后续 ensureDirectoryLoaded 发现已缓存时会跳过，不再重复拉取。 */
  /** 用 HEAD + 计时法探测浏览器缓存。
   *  已缓存 → 加载完整数据；未缓存 → 跳过（HEAD 流量极小），留给 ensureDirectoryLoaded 按需拉取。 */
  async preloadCachedDirectories(): Promise<void> {
    if (this._cdnUrlMap.size === 0) return;
    const loads: Promise<void>[] = [];
    let checked = 0;
    let cached = 0;
    const CACHE_THRESHOLD_MS = 20; // 本地缓存 < 5ms，网络 > 50ms，20ms 安全分界线

    for (const [id, url] of this._cdnUrlMap) {
      if (this._directories.has(id)) continue;
      checked++;
      loads.push((async () => {
        try {
          const t0 = performance.now();
          const headRes = await fetch(url, { method: "HEAD", cache: "force-cache" });
          const elapsed = performance.now() - t0;
          if (!headRes.ok) return;

          if (elapsed < CACHE_THRESHOLD_MS) {
            // 计时判断是本地缓存命中，加载完整数据
            const res = await fetch(url, { cache: "force-cache" });
            if (!res.ok) return;
            const data = (await res.json()) as DirectoryExportData;
            this._directories.set(data.managed_root.id, data);
            cached++;
            console.log(`[staticData] CDN cache hit (${elapsed.toFixed(1)}ms): ${url}`);
          } else {
            console.log(`[staticData] CDN miss (${elapsed.toFixed(1)}ms), deferred: ${url}`);
          }
        } catch {
          // 跳过
        }
      })());
    }

    if (checked > 0) {
      console.log(`[staticData] CDN probe: ${cached}/${checked} cached, ${checked - cached} deferred`);
      await Promise.all(loads);
    }
  }

  private _bootstrapCdnUrlMap(): void {
    const urls = this._global?.download_policy?.directory_cdn_urls;
    if (urls) this.setCdnUrlMapFromObject(urls);
  }

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
