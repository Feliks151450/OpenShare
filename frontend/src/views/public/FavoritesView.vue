<script setup lang="ts">
/**
 * 我的收藏页面
 * 展示用户已收藏的文件和文件夹列表
 * 数据存储在 localStorage 中，通过公开 API 获取元数据
 * UI 布局逻辑与首页卡片视图完全一致：带封面和不带封面的分开排列
 */

import { computed, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import {
  Download,
  FileArchive,
  FileAudio,
  FileCode2,
  FileImage,
  FilePenLine,
  FileSpreadsheet,
  FileText,
  FileType2,
  FileVideo,
  Flag,
  Folder,
  Plus,
  Star,
} from "lucide-vue-next";
import type { Component } from "vue";

import PageHeader from "../../components/ui/PageHeader.vue";
import EmptyState from "../../components/ui/EmptyState.vue";
import { httpClient } from "../../lib/http/client";
import { useFavorites, type FavoriteItem } from "../../composables/useFavorites";
import { fileCoverImageHrefFromFields } from "../../lib/markdown";

/** 文件详情接口 */
interface FileDetail {
  id: string;
  name: string;
  extension: string;
  folder_id: string;
  path: string;
  description: string;
  remark: string;
  mime_type: string;
  cover_url: string | null;
  download_allowed: boolean;
  size: number;
  download_count: number;
  uploaded_at: string;
  tags: Array<{ id: string; name: string; color: string }>;
}

/** 文件夹详情接口 */
interface FolderDetail {
  id: string;
  name: string;
  description: string;
  remark: string;
  cover_url: string | null;
  parent_id: string | null;
  file_count: number;
  download_count: number;
  total_size: number;
  updated_at: string;
  is_virtual: boolean;
  download_allowed: boolean;
}

/** 统一的卡片行数据（与首页 DirectoryRow 对齐） */
interface FavoriteRow {
  id: string;
  kind: "folder" | "file";
  name: string;
  extension: string;
  description: string;
  remark: string;
  coverUrl: string | null;
  downloadCount: number;
  fileCount: number;
  sizeBytes: number;
  sizeText: string;
  updatedAt: string;
  downloadAllowed: boolean;
  tags: Array<{ id: string; name: string; color: string }>;
}

/** 卡片展示块（与首页 CardDisplayBlock 对齐） */
type CardDisplayBlock = { key: string; rows: FavoriteRow[] };

const router = useRouter();
const { favoriteItems, removeFavorite } = useFavorites();
const items = ref<FavoriteRow[]>([]);
const loadingAll = ref(true);

/** 有收藏项 */
const hasItems = computed(() => items.value.length > 0);

/** 文件扩展名对应的图标组件映射 */
const FILE_ICON_MAP: Record<string, Component> = {
  ".pdf": FileText,
  ".doc": FilePenLine,
  ".docx": FilePenLine,
  ".xls": FileSpreadsheet,
  ".xlsx": FileSpreadsheet,
  ".ppt": FileText,
  ".pptx": FileText,
  ".jpg": FileImage,
  ".jpeg": FileImage,
  ".png": FileImage,
  ".gif": FileImage,
  ".svg": FileImage,
  ".webp": FileImage,
  ".mp4": FileVideo,
  ".avi": FileVideo,
  ".mov": FileVideo,
  ".mkv": FileVideo,
  ".mp3": FileAudio,
  ".wav": FileAudio,
  ".flac": FileAudio,
  ".zip": FileArchive,
  ".rar": FileArchive,
  ".7z": FileArchive,
  ".tar": FileArchive,
  ".gz": FileArchive,
  ".js": FileCode2,
  ".ts": FileCode2,
  ".py": FileCode2,
  ".java": FileCode2,
  ".c": FileCode2,
  ".cpp": FileCode2,
  ".html": FileCode2,
  ".css": FileCode2,
  ".json": FileCode2,
  ".xml": FileCode2,
  ".md": FileText,
  ".txt": FileText,
  ".csv": FileSpreadsheet,
  ".netcdf": FileType2,
  ".nc": FileType2,
};

/** 获取文件图标组件 */
function fileIconComponent(extension: string): Component {
  return FILE_ICON_MAP[extension.toLowerCase()] ?? FileText;
}

/** 格式化文件大小 */
function formatSize(bytes: number): string {
  if (bytes === 0) return "0 B";
  const units = ["B", "KB", "MB", "GB", "TB"];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(i > 0 ? 1 : 0)} ${units[i]}`;
}

/** 格式化日期时间 */
function formatDateTime(dateStr: string): string {
  if (!dateStr) return "-";
  try {
    const date = new Date(dateStr);
    return date.toLocaleDateString("zh-CN", {
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
    });
  } catch {
    return dateStr;
  }
}

/** 截取备注预览（最多 60 字符） */
function cardRemarkPreview(remark: string): string {
  if (!remark) return "";
  return remark.length > 60 ? remark.slice(0, 60) + "…" : remark;
}

/** 规范化文件扩展名 */
function normalizeFileExtension(ext: string): string {
  return (ext ?? "").replace(/^\.+/, "").toLowerCase();
}

/** 从文件名中提取扩展名 */
function extractExtension(name: string): string {
  const match = name.match(/\.([^.]+)$/);
  return match ? match[1].toLowerCase() : "";
}

/** 计算封面 URL（与首页逻辑一致） */
function computeCoverUrl(detail: FileDetail | FolderDetail, kind: "file" | "folder"): string | null {
  const desc = (detail.description ?? "").trim();
  const coverUrl = kind === "file"
    ? fileCoverImageHrefFromFields((detail as FileDetail).cover_url ?? undefined, desc)
    : fileCoverImageHrefFromFields((detail as FolderDetail).cover_url ?? undefined, desc);
  return coverUrl || null;
}

/** 将详情转换为统一的行数据 */
function toFavoriteRow(detail: FileDetail | FolderDetail, kind: "file" | "folder"): FavoriteRow {
  if (kind === "folder") {
    const fd = detail as FolderDetail;
    return {
      id: fd.id,
      kind: "folder",
      name: fd.name,
      extension: "",
      description: (fd.description ?? "").trim(),
      remark: (fd.remark ?? "").trim(),
      coverUrl: computeCoverUrl(fd, "folder"),
      downloadCount: fd.download_count ?? 0,
      fileCount: fd.file_count ?? 0,
      sizeBytes: fd.total_size ?? 0,
      sizeText: formatSize(fd.total_size ?? 0),
      updatedAt: formatDateTime(fd.updated_at),
      downloadAllowed: fd.download_allowed !== false,
      tags: [],
    };
  } else {
    const fd = detail as FileDetail;
    return {
      id: fd.id,
      kind: "file",
      name: fd.name,
      extension: normalizeFileExtension(fd.extension) || extractExtension(fd.name),
      description: (fd.description ?? "").trim(),
      remark: (fd.remark ?? "").trim(),
      coverUrl: computeCoverUrl(fd, "file"),
      downloadCount: fd.download_count ?? 0,
      fileCount: 0,
      sizeBytes: fd.size ?? 0,
      sizeText: formatSize(fd.size),
      updatedAt: formatDateTime(fd.uploaded_at),
      downloadAllowed: fd.download_allowed !== false,
      tags: fd.tags ?? [],
    };
  }
}

/**
 * 卡片展示块（与首页逻辑一致）
 * 带封面和不带封面的分开排列
 */
const cardDisplayBlocks = computed((): CardDisplayBlock[] => {
  const withCover = items.value.filter((r) => !!r.coverUrl);
  const withoutCover = items.value.filter((r) => !r.coverUrl);
  const blocks: CardDisplayBlock[] = [];
  if (withCover.length) {
    blocks.push({ key: "with-cover", rows: withCover });
  }
  if (withoutCover.length) {
    blocks.push({ key: "without-cover", rows: withoutCover });
  }
  return blocks.length ? blocks : [{ key: "all", rows: items.value }];
});

/** 获取资源元数据 */
async function fetchItemMeta(item: FavoriteItem): Promise<FavoriteRow | null> {
  try {
    if (item.kind === "file") {
      const detail = await httpClient.get<FileDetail>(`/public/files/${item.id}`);
      return toFavoriteRow(detail, "file");
    } else {
      const detail = await httpClient.get<FolderDetail>(`/public/folders/${item.id}`);
      return toFavoriteRow(detail, "folder");
    }
  } catch {
    return null;
  }
}

/** 加载所有收藏项的元数据 */
async function loadAllItems() {
  loadingAll.value = true;
  const currentFavorites = [...favoriteItems.value];

  // 并发请求所有收藏项的元数据
  const results = await Promise.all(currentFavorites.map(fetchItemMeta));

  // 过滤掉请求失败的项（资源可能已删除），并从收藏中移除
  const validItems: FavoriteRow[] = [];
  for (let i = 0; i < results.length; i++) {
    const result = results[i];
    if (result === null) {
      // 资源不存在，自动取消收藏
      removeFavorite(currentFavorites[i].id);
    } else {
      validItems.push(result);
    }
  }

  items.value = validItems;
  loadingAll.value = false;
}

/** 取消收藏并从列表中移除 */
function handleRemoveFavorite(id: string) {
  removeFavorite(id);
  items.value = items.value.filter((item) => item.id !== id);
}

/** 跳转到详情页 */
function goToDetail(row: FavoriteRow) {
  if (row.kind === "file") {
    router.push({ name: "public-file-detail", params: { fileID: row.id } });
  } else {
    router.push({ path: "/", query: { folder: row.id } });
  }
}

/** 在新窗口打开 */
function openInNewWindow(row: FavoriteRow) {
  let url: string;
  if (row.kind === "file") {
    url = router.resolve({ name: "public-file-detail", params: { fileID: row.id } }).href;
  } else {
    url = router.resolve({ name: "public-home", query: { folder: row.id } }).href;
  }
  window.open(url, "_blank");
}

onMounted(() => {
  void loadAllItems();
});
</script>

<template>
  <div class="mx-auto w-full max-w-[1360px] px-3 py-6 sm:px-4 md:px-6 lg:px-8 xl:max-w-[2150px]">
    <PageHeader title="我的收藏" description="您收藏的文件和文件夹" />

    <!-- 加载状态 -->
    <div v-if="loadingAll" class="px-4 py-8 text-sm text-slate-500 sm:px-6">加载中…</div>

    <!-- 空状态 -->
    <EmptyState
      v-else-if="!hasItems"
      title="暂无收藏"
      description="在首页卡片中点击星标按钮即可收藏文件或文件夹"
      class="mt-8"
    />

    <!-- 收藏列表 - 与首页完全一致的卡片布局：带封面和不带封面分开排列 -->
    <div v-else class="space-y-8 px-4 pt-3 pb-5 sm:px-5">
      <div v-for="block in cardDisplayBlocks" :key="block.key">
        <div class="public-favorites-card-grid gap-4 md:gap-5">
          <article
            v-for="row in block.rows"
            :key="`${row.kind}-${row.id}`"
            class="group relative min-w-xs flex cursor-pointer flex-col overflow-hidden rounded-3xl border transition hover:shadow-sm"
            :class="[
              row.coverUrl ? 'min-h-0' : 'min-h-[155px] px-2.5 pt-2.5 sm:px-2.5',
              row.kind === 'folder' ? 'border-slate-200 bg-sky-50/50 hover:border-sky-500 hover:bg-sky-50' : 'border-slate-200 bg-white hover:border-slate-300 hover:bg-slate-100',
            ]"
            @click="goToDetail(row)"
          >
            <!-- 有封面图的卡片 -->
            <template v-if="row.coverUrl">
              <div class="relative aspect-[16/10] min-h-[132px] w-full max-h-[220px] shrink-0 overflow-hidden bg-slate-100 sm:min-h-[148px] sm:max-h-[240px]">
                <img
                  :src="row.coverUrl"
                  :alt="`封面 ${row.name}`"
                  class="absolute inset-0 h-full w-full object-cover"
                  loading="lazy"
                />
              </div>
              <div class="flex min-h-0 flex-1 flex-col px-2.5 pb-2.5 pt-3 sm:px-2.5">
                <h3
                  class="line-clamp-2 text-base font-semibold leading-snug"
                  :class="row.kind === 'folder' ? 'text-sky-900' : 'text-slate-900'"
                >
                  {{ row.name }}
                </h3>
                <!-- 文件夹备注 -->
                <p
                  v-if="row.kind === 'folder' && cardRemarkPreview(row.remark)"
                  class="mt-1 line-clamp-2 text-sm leading-5 text-slate-500"
                >
                  {{ cardRemarkPreview(row.remark) }}
                </p>
                <!-- 文件标签 -->
                <div v-if="row.kind === 'file' && row.tags.length > 0" class="mt-2 flex flex-wrap gap-1">
                  <span
                    v-for="tag in row.tags"
                    :key="tag.id"
                    class="inline-flex items-center rounded-md px-1.5 py-0.5 text-xs font-medium"
                    :style="{ backgroundColor: tag.color, color: '#fff' }"
                  >
                    {{ tag.name }}
                  </span>
                </div>
                <!-- 大小信息 -->
                <div
                  class="my-2 flex w-full min-w-0 text-xs"
                  :class="row.kind === 'file' ? 'items-start gap-2' : 'flex-wrap items-center gap-x-4 gap-y-1'"
                >
                  <template v-if="row.kind === 'file'">
                    <div
                      v-if="cardRemarkPreview(row.remark)"
                      class="min-w-0 flex-1 overflow-hidden"
                    >
                      <p class="line-clamp-2 text-left leading-snug text-slate-600">
                        {{ cardRemarkPreview(row.remark) }}
                      </p>
                    </div>
                    <span class="ml-auto shrink-0 tabular-nums text-slate-500">{{ row.sizeText }}</span>
                  </template>
                  <template v-else>
                    <span class="text-slate-500">{{ row.fileCount }} 个文件</span>
                    <span class="text-slate-500">{{ row.sizeText }}</span>
                  </template>
                </div>
                <!-- 底部工具栏 -->
                <div class="mt-auto flex items-center justify-between gap-2 border-t border-slate-100 pt-3">
                  <button
                    type="button"
                    :class="['inline-flex items-center justify-center rounded-xl border p-2.5 transition', row.kind === 'folder' ? 'border-sky-200 bg-sky-50/50 text-sky-700 hover:border-sky-300 hover:bg-sky-100 hover:text-sky-800' : 'border-slate-200 bg-white text-slate-700 hover:border-slate-300 hover:bg-slate-50 hover:text-slate-900']"
                    title="反馈"
                    @click.stop="goToDetail(row)"
                  >
                    <Flag class="h-4 w-4" />
                  </button>
                  <div class="flex items-center gap-2">
                    <button
                      type="button"
                      title="取消收藏"
                      :class="['inline-flex items-center justify-center rounded-xl border p-2.5 transition', row.kind === 'folder' ? 'border-sky-300 bg-sky-100 text-sky-700 hover:border-sky-400 hover:bg-sky-200 hover:text-sky-800' : 'border-slate-300 bg-slate-100 text-slate-700 hover:border-slate-400 hover:bg-slate-200 hover:text-slate-800']"
                      aria-label="取消收藏"
                      @click.stop="handleRemoveFavorite(row.id)"
                    >
                      <Star class="h-4 w-4" fill="currentColor" />
                    </button>
                    <button
                      type="button"
                      title="在新窗口中打开"
                      :class="['inline-flex items-center justify-center rounded-xl border p-2.5 transition', row.kind === 'folder' ? 'border-sky-200 bg-sky-50/50 text-sky-700 hover:border-sky-300 hover:bg-sky-100 hover:text-sky-800' : 'border-slate-200 bg-white text-slate-700 hover:border-slate-300 hover:bg-slate-50 hover:text-slate-900']"
                      aria-label="在新窗口中打开"
                      @click.stop="openInNewWindow(row)"
                    >
                      <Plus class="h-4 w-4" />
                    </button>
                    <button
                      v-if="row.downloadAllowed"
                      type="button"
                      :class="['inline-flex items-center justify-center rounded-xl border p-2.5 transition', row.kind === 'folder' ? 'border-sky-200 bg-sky-50/50 text-sky-700 hover:border-sky-300 hover:bg-sky-100 hover:text-sky-800' : 'border-slate-200 bg-white text-slate-700 hover:border-slate-300 hover:bg-slate-50 hover:text-slate-900']"
                      title="下载"
                      @click.stop="goToDetail(row)"
                    >
                      <Download class="h-4 w-4" />
                    </button>
                  </div>
                </div>
              </div>
            </template>

            <!-- 无封面图的卡片 -->
            <template v-else>
              <div class="flex items-start gap-2.5 sm:gap-2.5">
                <div
                  :class="[
                    'flex h-12 w-12 shrink-0 overflow-hidden rounded-2xl items-center justify-center',
                    row.kind === 'folder' ? 'bg-sky-50/50 text-sky-500' : 'bg-slate-100 text-slate-500',
                  ]"
                >
                  <Folder v-if="row.kind === 'folder'" class="h-6 w-6 text-sky-500" />
                  <component v-else :is="fileIconComponent(row.extension)" class="h-6 w-6" />
                </div>
                <div
                  class="min-w-0 flex-1 pt-0.5"
                >
                  <h3
                    class="line-clamp-2 break-words text-base font-semibold leading-snug [overflow-wrap:anywhere]"
                    :class="row.kind === 'folder' ? 'text-sky-900' : 'text-slate-900'"
                  >
                    {{ row.name }}
                  </h3>
                  <p
                    v-if="row.kind === 'folder' && cardRemarkPreview(row.remark)"
                    class="mt-1 line-clamp-2 text-sm leading-5 text-slate-500"
                  >
                    {{ cardRemarkPreview(row.remark) }}
                  </p>
                </div>
              </div>
              <!-- 文件标签 -->
              <div v-if="row.kind === 'file' && row.tags.length > 0" class="mt-2 flex flex-wrap gap-1">
                <span
                  v-for="tag in row.tags"
                  :key="tag.id"
                  class="inline-flex items-center rounded-md px-1.5 py-0.5 text-xs font-medium"
                  :style="{ backgroundColor: tag.color, color: '#fff' }"
                >
                  {{ tag.name }}
                </span>
              </div>
              <!-- 大小信息 -->
              <div
                class="my-2 flex w-full min-w-0 text-xs"
                :class="row.kind === 'file' ? 'items-start gap-2' : 'flex-wrap items-center gap-x-4 gap-y-1'"
              >
                <template v-if="row.kind === 'file'">
                  <div
                    v-if="cardRemarkPreview(row.remark)"
                    class="min-w-0 flex-1 overflow-hidden"
                  >
                    <p class="line-clamp-2 text-left leading-snug text-slate-600">
                      {{ cardRemarkPreview(row.remark) }}
                    </p>
                  </div>
                  <span class="ml-auto shrink-0 tabular-nums text-slate-500">{{ row.sizeText }}</span>
                </template>
                <template v-else>
                  <span class="text-slate-500">{{ row.fileCount }} 个文件</span>
                  <span class="text-slate-500">{{ row.sizeText }}</span>
                </template>
              </div>
              <!-- 底部工具栏 -->
              <div class="mt-auto flex items-center justify-between gap-2 border-t border-slate-100 py-2.5">
                <button
                  type="button"
                  :class="['inline-flex items-center justify-center rounded-xl border p-2.5 transition', row.kind === 'folder' ? 'border-sky-200 bg-sky-50/50 text-sky-700 hover:border-sky-300 hover:bg-sky-100 hover:text-sky-800' : 'border-slate-200 bg-white text-slate-700 hover:border-slate-300 hover:bg-slate-50 hover:text-slate-900']"
                  title="反馈"
                  @click.stop="goToDetail(row)"
                >
                  <Flag class="h-4 w-4" />
                </button>
                <div class="flex items-center gap-2">
                  <button
                    type="button"
                    title="取消收藏"
                    :class="['inline-flex items-center justify-center rounded-xl border p-2.5 transition', row.kind === 'folder' ? 'border-sky-300 bg-sky-100 text-sky-700 hover:border-sky-400 hover:bg-sky-200 hover:text-sky-800' : 'border-slate-300 bg-slate-100 text-slate-700 hover:border-slate-400 hover:bg-slate-200 hover:text-slate-800']"
                    aria-label="取消收藏"
                    @click.stop="handleRemoveFavorite(row.id)"
                  >
                    <Star class="h-4 w-4" fill="currentColor" />
                  </button>
                  <button
                    type="button"
                    title="在新窗口中打开"
                    :class="['inline-flex items-center justify-center rounded-xl border p-2.5 transition', row.kind === 'folder' ? 'border-sky-200 bg-sky-50/50 text-sky-700 hover:border-sky-300 hover:bg-sky-100 hover:text-sky-800' : 'border-slate-200 bg-white text-slate-700 hover:border-slate-300 hover:bg-slate-50 hover:text-slate-900']"
                    aria-label="在新窗口中打开"
                    @click.stop="openInNewWindow(row)"
                  >
                    <Plus class="h-4 w-4" />
                  </button>
                  <button
                    v-if="row.downloadAllowed"
                    type="button"
                    :class="['inline-flex items-center justify-center rounded-xl border p-2.5 transition', row.kind === 'folder' ? 'border-sky-200 bg-sky-50/50 text-sky-700 hover:border-sky-300 hover:bg-sky-100 hover:text-sky-800' : 'border-slate-200 bg-white text-slate-700 hover:border-slate-300 hover:bg-slate-50 hover:text-slate-900']"
                    title="下载"
                    @click.stop="goToDetail(row)"
                  >
                    <Download class="h-4 w-4" />
                  </button>
                </div>
              </div>
            </template>
          </article>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.public-favorites-card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 20rem), 1fr));
}
</style>
