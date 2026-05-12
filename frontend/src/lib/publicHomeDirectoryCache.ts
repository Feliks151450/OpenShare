import { ref } from "vue";

/** 公开首页目录 API 响应形状（与 Home.vue 一致） */
import type { PublicFileTag } from "./publicFileTags";

export interface PublicFolderItem {
  id: string;
  name: string;
  description?: string;
  remark?: string;
  cover_url?: string;
  cdn_url?: string;
  download_allowed?: boolean;
  is_virtual?: boolean;
  updated_at: string;
  file_count: number;
  download_count: number;
  total_size: number;
}

export interface PublicFileItem {
  id: string;
  name: string;
  description: string;
  remark?: string;
  extension: string;
  cover_url?: string;
  folder_direct_download_url?: string;
  playback_url?: string;
  /** 服务端代理下载：为 true 时走 /api/public/files/.../download 由服务端代理拉取 proxy_source_url */
  proxy_download?: boolean;
  /** 服务端代理拉取的目标地址（仅 proxy_download=true 时有效） */
  proxy_source_url?: string;
  download_allowed?: boolean;
  uploaded_at: string;
  download_count: number;
  size: number;
  tags?: PublicFileTag[];
}

export interface FolderDetailResponse {
  id: string;
  name: string;
  description: string;
  remark?: string;
  cover_url?: string;
  parent_id: string | null;
  file_count: number;
  download_count: number;
  total_size: number;
  updated_at: string;
  direct_link_prefix?: string;
  download_allowed?: boolean;
  download_policy?: "inherit" | "allow" | "deny";
  /** 仅托管根目录：为 true 时访客首页根列表不出现该托管树 */
  hide_public_catalog?: boolean;
  /** 虚拟目录：无物理磁盘路径，子文件通过 CDN 直链提供 */
  is_virtual?: boolean;
  breadcrumbs: Array<{
    id: string;
    name: string;
  }>;
}

export type DirectoryViewCacheEntry = {
  folders: PublicFolderItem[];
  files: PublicFileItem[];
  detail: FolderDetailResponse | null;
};

/** 纯 JSON 数据深拷贝，避免 structuredClone 对 Proxy/怪值抛错导致整次加载失败 */
function deepCloneDirectoryEntry(entry: DirectoryViewCacheEntry): DirectoryViewCacheEntry {
  return JSON.parse(JSON.stringify(entry)) as DirectoryViewCacheEntry;
}

/** 根目录视图的缓存不应带详情；子目录缓存的 detail.id 须与当前文件夹 id 一致 */
export function isDirectoryViewCacheEntryUsable(folderID: string, entry: DirectoryViewCacheEntry): boolean {
  const key = folderID.trim();
  if (!key) {
    return entry.detail === null;
  }
  return Boolean(entry.detail && entry.detail.id === key);
}

/** 模块级单例：离开 Home 再进入（如文件详情往返）仍可复用 */
const directoryViewCache = new Map<string, DirectoryViewCacheEntry>();
let directoryViewLoadToken = 0;

export function takeDirectoryViewLoadToken(): number {
  directoryViewLoadToken += 1;
  return directoryViewLoadToken;
}

export function peekDirectoryViewLoadToken(): number {
  return directoryViewLoadToken;
}

export function directoryViewCacheKey(folderID: string): string {
  return folderID.trim() || "__root__";
}

export function writeDirectoryViewCache(folderID: string, entry: DirectoryViewCacheEntry) {
  directoryViewCache.set(directoryViewCacheKey(folderID), deepCloneDirectoryEntry(entry));
}

export function readDirectoryViewCache(folderID: string): DirectoryViewCacheEntry | undefined {
  const raw = directoryViewCache.get(directoryViewCacheKey(folderID));
  return raw ? deepCloneDirectoryEntry(raw) : undefined;
}

export function invalidateDirectoryViewCacheAll() {
  directoryViewCache.clear();
}

export function invalidateDirectoryViewCacheFolder(folderID: string) {
  directoryViewCache.delete(directoryViewCacheKey(folderID));
}

/** 模块级共享响应式数据：根目录文件夹列表。
 *  Home.vue 的 loadDirectory() 在拉取根目录时写入，
 *  GlobalSidebar 直接读取，避免重复请求。 */
export const sharedRootFolders = ref<PublicFolderItem[]>([]);
