<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRoute, useRouter, type RouteLocationRaw } from "vue-router";
import {
  ChevronLeft,
  ChevronRight,
  Database,
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
  Home,
  LayoutGrid,
  Link2,
  List,
  NotebookText,
  PanelRightOpen,
  Plus,
  Check,
  Trash2,
  Upload,
} from "lucide-vue-next";

import { type InfoPanelCardItem } from "../../components/shared/InfoPanelCard.vue";
import PublicFileDetailView from "./PublicFileDetailView.vue";
import SearchSection from "../../components/resources/SearchSection.vue";
import { useNavActions } from "../../composables/useNavActions";
import { registerHomeConsoleHooks, unregisterHomeConsoleHooks } from "../../lib/homeConsoleBridge";
import { HttpError, httpClient } from "../../lib/http/client";
import { readApiError } from "../../lib/http/helpers";
import { copyPlainTextToClipboard } from "../../lib/clipboard";
import { toastError, toastSuccess, toastWarning } from "../../lib/toast";
import { ensureSessionReceiptCode, readStoredReceiptCode } from "../../lib/receiptCode";
import {
  fileCoverImageHrefFromFields,
  renderSimpleMarkdown,
} from "../../lib/markdown";
import { renderMarkdownAsync } from "../../lib/useAsyncMarkdown";
import {
  hydrateMarkdownCatalogNavigatePresentation,
  markdownCatalogNavigateInitialPresentation,
  type MarkdownCatalogConfirmPresentation,
} from "../../lib/markdownCatalogNavigateDisplay";
import { markdownRoutePublicFileDetailId, onMarkdownLinkClickCapture, isViewportTailwindXlMin } from "../../lib/publicMarkdownLinks";
import { fileEffectiveDownloadHref, fileUsesBackendDownloadHref } from "../../lib/fileDirectUrl";
import { collectDroppedEntries, normalizeFiles, type UploadSelectionEntry } from "../../lib/uploads/fileDrop";
import {
  invalidateDirectoryViewCacheAll,
  invalidateDirectoryViewCacheFolder,
  isDirectoryViewCacheEntryUsable,
  peekDirectoryViewLoadToken,
  readDirectoryViewCache,
  takeDirectoryViewLoadToken,
  sharedRootFolders,
  writeDirectoryViewCache,
  type DirectoryViewCacheEntry,
  type FolderDetailResponse,
  type PublicFileItem,
  type PublicFolderItem,
} from "../../lib/publicHomeDirectoryCache";
import { staticDataLoader } from "../../lib/staticDataLoader";
import FileTagChips from "../../components/public/FileTagChips.vue";
import { type PublicFileTag, readableTextColorForPreset } from "../../lib/publicFileTags";

interface AnnouncementItem {
  id: string;
  title: string;
  content: string;
  is_pinned: boolean;
  creator: {
    id: string;
    username: string;
    display_name: string;
    avatar_url: string;
    role: string;
  };
  published_at?: string;
  updated_at: string;
}

interface HotDownloadItem {
  id: string;
  name: string;
  downloadCount: number;
}

interface LatestItem {
  id: string;
  name: string;
}

interface SidebarDetailItem {
  id: string;
  label: string;
  meta?: string;
}

interface SidebarDetailModalState {
  eyebrow: string;
  title: string;
  description: string;
  items: SidebarDetailItem[];
}

interface SearchResultResponse {
  items: Array<{
    entity_type: "file" | "folder";
    id: string;
    name: string;
    remark?: string;
    extension?: string;
    cover_url?: string;
    playback_url?: string;
    folder_direct_download_url?: string;
    download_allowed?: boolean;
    size?: number;
    download_count?: number;
    uploaded_at?: string;
    updated_at?: string;
    tags?: PublicFileTag[];
  }>;
  page: number;
  page_size: number;
  total: number;
}

const route = useRoute();
const router = useRouter();

const announcements = ref<AnnouncementItem[]>([]);
const selectedAnnouncementId = ref<string | null>(null);
const selectedAnnouncement = computed(() => {
  if (!selectedAnnouncementId.value) return null;
  return announcements.value.find((a) => a.id === selectedAnnouncementId.value) ?? null;
});
const announcementListOpen = ref(false);
const hotDownloadItems = ref<HotDownloadItem[]>([]);
const latestItems = ref<LatestItem[]>([]);
const sidebarDetailModal = ref<SidebarDetailModalState | null>(null);
const { activePanel, closePanel: closeNavPanel } = useNavActions();
watch(activePanel, (panel) => {
  if (panel === "announcements") {
    openAnnouncementList();
  } else if (panel === "hotDownloads") {
    openHotDownloadsModal();
  } else if (panel === "latestItems") {
    openLatestItemsModal();
  }
});

function openPanelFromQuery(panel: string) {
  if (panel === "announcements") {
    openAnnouncementList();
  } else if (panel === "hotDownloads") {
    openHotDownloadsModal();
  } else if (panel === "latestItems") {
    openLatestItemsModal();
  }
}

function clearPanelQuery() {
  if (route.query.panel) {
    const query = { ...route.query };
    delete query.panel;
    router.replace({ query }).catch(() => {});
  }
}
/** 卡片「右侧预览」抽屉：嵌入 PublicFileDetailView */
const fileDetailPanelFileId = ref<string | null>(null);
/** Markdown 站内链接前往目录前确认 */
const markdownCatalogNavigateConfirmRoute = ref<RouteLocationRaw | null>(null);
let markdownCatalogNavigateConfirmResolve: ((ok: boolean) => void) | null = null;
const markdownCatalogNavigatePresentation = ref<MarkdownCatalogConfirmPresentation | null>(null);
let markdownCatalogNavigateHydrateGeneration = 0;

const viewMode = ref<"cards" | "table">("cards");
const sortMode = ref<"name" | "download" | "format" | "modified">("name");
const sortDirection = ref<"asc" | "desc">("desc");
/** 卡片视图：先按是否有封面分组，组内仍沿用当前排序方式 */
const cardCoverFirst = ref(true);
/** 当前文件夹下的标签过滤：空 Set 为不过滤 */
const selectedTagIds = ref<Set<string>>(new Set());
/** 是否有活跃的标签过滤 */
const hasActiveTagFilter = computed(() => selectedTagIds.value.size > 0);

function toggleTagFilter(tagId: string) {
  const next = new Set(selectedTagIds.value);
  if (next.has(tagId)) {
    next.delete(tagId);
  } else {
    next.add(tagId);
  }
  selectedTagIds.value = next;
}

function clearTagFilter() {
  selectedTagIds.value = new Set();
}
const sortMenuOpen = ref(false);
const viewMenuOpen = ref(false);
const toolbarDropdownsRef = ref<HTMLElement | null>(null);
const downloadTimestamps = ref<number[]>([]);
const uploadModalOpen = ref(false);
const uploadSuccessModalOpen = ref(false);
const uploadSubmitting = ref(false);
const uploadMessage = ref("");
const uploadError = ref("");
const uploadFileInput = ref<HTMLInputElement | null>(null);
const currentReceiptCode = ref("");
const uploadForm = ref({
  description: "",
  entries: [] as UploadSelectionEntry[],
});
const uploadDropActive = ref(false);
const uploadCollecting = ref(false);
const feedbackModalOpen = ref(false);
const feedbackSuccessModalOpen = ref(false);
const feedbackTarget = ref<{ id: string; type: "file" | "folder"; name: string } | null>(null);
const feedbackDescription = ref("");
const feedbackSubmitting = ref(false);
const feedbackMessage = ref("");
const feedbackError = ref("");
const feedbackSubmitDisabled = computed(() => feedbackSubmitting.value || !feedbackDescription.value.trim());

const loading = ref(false);
const error = ref("");
const actionMessage = ref("");
const actionError = ref("");
const batchDownloadSubmitting = ref(false);
const DEFAULT_LARGE_DOWNLOAD_CONFIRM = 1024 * 1024 * 1024;
const largeDownloadConfirmBytes = ref(DEFAULT_LARGE_DOWNLOAD_CONFIRM);
let downloadPolicyLoaded = false;
let announcementsLoaded = false;
let hotDownloadsLoaded = false;
let latestTitlesLoaded = false;
type DownloadConfirmState = { mode: "single"; row: DirectoryRow } | { mode: "batch" };
const downloadConfirm = ref<DownloadConfirmState | null>(null);
const folders = ref<PublicFolderItem[]>([]);
const files = ref<PublicFileItem[]>([]);
const searchInput = ref("");
const searchKeyword = ref("");
const searchLoading = ref(false);
const searchError = ref("");
const searchRows = ref<DirectoryRow[]>([]);
const breadcrumbs = ref<Array<{ id: string; name: string }>>([]);
const currentFolderDetail = ref<FolderDetailResponse | null>(null);
const selectedResourceKeys = ref<string[]>([]);
/** 卡片布局下仅在此模式下显示右上角复选框并可点选卡片多选；表格布局始终可多选 */
const cardMultiSelectMode = ref(false);
const canManageResourceDescriptions = ref(false);
const canManageAnnouncements = ref(false);
const homeSessionAdminId = ref("");
const homeSessionIsSuperAdmin = ref(false);
const folderDescriptionEditorOpen = ref(false);
const folderNameDraft = ref("");
const folderDescriptionDraft = ref("");
const folderRemarkDraft = ref("");
const folderDirectPrefixDraft = ref("");
const folderDownloadPolicyDraft = ref<"inherit" | "allow" | "deny">("inherit");
const folderCoverUrlDraft = ref("");
const folderHidePublicCatalogDraft = ref(false);
/* 自定义路径（仅管理员可编辑）：在文件夹编辑弹窗中设置，如 "doc" 对应 /doc 访问该文件夹 */
const folderCustomPathDraft = ref("");

const createFolderModalOpen = ref(false);
const createFolderNameDraft = ref("");
const createFolderSaving = ref(false);
const createFolderError = ref("");

function openCreateFolderModal() {
  createFolderNameDraft.value = "";
  createFolderError.value = "";
  createFolderModalOpen.value = true;
  syncBodyScrollLock();
}

function closeCreateFolderModal() {
  createFolderModalOpen.value = false;
  createFolderSaving.value = false;
  createFolderError.value = "";
  syncBodyScrollLock();
}

// 虚拟目录创建
const virtualFolderModalOpen = ref(false);
const virtualFolderNameDraft = ref("");
const virtualFolderSaving = ref(false);
const virtualFolderError = ref("");

function openCreateVirtualFolderModal() {
  virtualFolderNameDraft.value = "";
  virtualFolderError.value = "";
  virtualFolderModalOpen.value = true;
  syncBodyScrollLock();
}

function closeCreateVirtualFolderModal() {
  virtualFolderModalOpen.value = false;
  virtualFolderSaving.value = false;
  virtualFolderError.value = "";
  syncBodyScrollLock();
}

async function submitCreateVirtualFolder() {
  const name = virtualFolderNameDraft.value.trim();
  if (!name) {
    toastError("请输入文件夹名称。");
    return;
  }
  if (!currentFolderDetail.value) return;
  virtualFolderSaving.value = true;
  virtualFolderError.value = "";
  try {
    await httpClient.request("/admin/resources/virtual-folders", {
      method: "POST",
      body: {
        name,
        parent_id: currentFolderDetail.value.id,
      },
    });
    closeCreateVirtualFolderModal();
    invalidateDirectoryViewCacheFolder(currentFolderDetail.value.id);
    await loadDirectory({ force: true });
  } catch (err: unknown) {
    toastError(readApiError(err, "创建虚拟目录失败。"));
  } finally {
    virtualFolderSaving.value = false;
  }
}

// 虚拟文件添加
const virtualFileModalOpen = ref(false);
// 虚拟文件表单字段
const virtualFileNameDraft = ref("");
const virtualFileUrlDraft = ref("");        // 前台 CDN 直链
const virtualFileFallbackUrlDraft = ref(""); // 前台备用直链
const virtualFileProxySourceDraft = ref(""); // 服务端代理拉取地址
const virtualFileProxyDownload = ref(false);
const virtualFileSaving = ref(false);
const virtualFileError = ref("");
const virtualFileDetectingLink = ref(false);
const virtualFileDetectResult = ref("");

function openAddVirtualFileModal() {
  virtualFileNameDraft.value = "";
  virtualFileUrlDraft.value = "";
  virtualFileFallbackUrlDraft.value = "";
  virtualFileProxySourceDraft.value = "";
  virtualFileProxyDownload.value = false;
  virtualFileError.value = "";
  virtualFileDetectResult.value = "";
  virtualFileModalOpen.value = true;
  syncBodyScrollLock();
}

function closeAddVirtualFileModal() {
  virtualFileModalOpen.value = false;
  virtualFileSaving.value = false;
  virtualFileError.value = "";
  syncBodyScrollLock();
}

/** 检测链接：优先检测代理源地址，否则检测前台直链 */
async function detectVirtualFileLink() {
  const url = (virtualFileProxySourceDraft.value || virtualFileUrlDraft.value).trim();
  if (!url) {
    virtualFileDetectResult.value = "请先输入链接地址。";
    return;
  }
  virtualFileDetectingLink.value = true;
  virtualFileDetectResult.value = "";
  try {
    const resp = await httpClient.post<{
      ok: boolean;
      size: number;
      content_type: string;
      file_name: string;
      error_message?: string;
    }>("/admin/probe-url", { url });
    if (!resp.ok) {
      virtualFileDetectResult.value = `链接检测失败：${resp.error_message || "未知错误"}`;
      return;
    }
    let msg = `链接可达`;
    if (resp.size > 0) msg += `，大小 ${formatSize(resp.size)}`;
    if (resp.content_type) msg += `，类型 ${resp.content_type}`;
    virtualFileDetectResult.value = msg;
    // 文件名输入框为空时自动填入建议文件名
    if (resp.file_name && !virtualFileNameDraft.value.trim()) {
      virtualFileNameDraft.value = resp.file_name;
    }
  } catch (err: unknown) {
    virtualFileDetectResult.value = readApiError(err, "链接检测失败。");
  } finally {
    virtualFileDetectingLink.value = false;
  }
}

async function submitAddVirtualFile() {
  const name = virtualFileNameDraft.value.trim();
  const proxyDownload = virtualFileProxyDownload.value;
  const proxySource = virtualFileProxySourceDraft.value.trim();
  const playbackUrl = virtualFileUrlDraft.value.trim();
  if (!name) {
    toastError("请输入文件名称。");
    return;
  }
  // 代理模式需要代理源地址，否则需要前台直链
  if (proxyDownload && !proxySource) {
    toastError("请输入服务端代理地址。");
    return;
  }
  if (!proxyDownload && !playbackUrl) {
    toastError("请输入前台 CDN 直链地址。");
    return;
  }
  if (!currentFolderDetail.value) return;
  virtualFileSaving.value = true;
  virtualFileError.value = "";
  try {
    await httpClient.request("/admin/resources/virtual-files", {
      method: "POST",
      body: {
        name,
        folder_id: currentFolderDetail.value.id,
        playback_url: playbackUrl,
        playback_fallback_url: virtualFileFallbackUrlDraft.value.trim(),
        proxy_source_url: proxySource,
        proxy_download: proxyDownload,
      },
    });
    closeAddVirtualFileModal();
    invalidateDirectoryViewCacheFolder(currentFolderDetail.value.id);
    await loadDirectory({ force: true });
  } catch (err: unknown) {
    toastError(readApiError(err, "添加虚拟文件失败。"));
  } finally {
    virtualFileSaving.value = false;
  }
}

async function submitCreateFolder() {
  const name = createFolderNameDraft.value.trim();
  if (!name) {
    toastError("请输入文件夹名称。");
    return;
  }
  if (!currentFolderDetail.value) return;
  createFolderSaving.value = true;
  createFolderError.value = "";
  try {
    await httpClient.request("/admin/resources/folders", {
      method: "POST",
      body: {
        name,
        parent_id: currentFolderDetail.value.id,
      },
    });
    closeCreateFolderModal();
    invalidateDirectoryViewCacheFolder(currentFolderDetail.value.id);
    await loadDirectory({ force: true });
  } catch (err: unknown) {
    toastError(readApiError(err, "创建文件夹失败。"));
  } finally {
    createFolderSaving.value = false;
  }
}
const folderDescriptionSaving = ref(false);
const folderDescriptionError = ref("");
const deleteResourceTarget = ref<{ id: string; kind: "folder" | "file"; name: string } | null>(null);
const deleteResourcePassword = ref("");
const deleteResourceMoveToTrash = ref(true);
const deleteResourceSubmitting = ref(false);
const deleteResourceError = ref("");
function folderIdFromRouteQuery(raw: unknown): string {
  if (typeof raw === "string" && raw.trim()) {
    return raw.trim();
  }
  if (Array.isArray(raw)) {
    for (const v of raw) {
      if (typeof v === "string" && v.trim()) {
        return v.trim();
      }
    }
  }
  return "";
}
/* 通过自定义路径（如 /doc）解析到的文件夹 UUID，空字符串表示未通过自定义路径访问 */
const customPathResolvedFolderID = ref("");
const currentFolderID = computed(() => {
  const fromQuery = folderIdFromRouteQuery(route.query.folder);
  if (fromQuery) return fromQuery;
  return customPathResolvedFolderID.value;
});
const canUploadToCurrentFolder = computed(() => currentFolderID.value.length > 0 && !isCurrentFolderVirtual.value);
/** 当前目录是否为虚拟目录（无物理磁盘路径，文件通过 CDN 直链提供） */
const isCurrentFolderVirtual = computed(() => currentFolderDetail.value?.is_virtual === true);
const rootViewLocked = computed(() => route.query.root === "1");

type DirectoryRow = {
  id: string;
  kind: "folder" | "file";
  name: string;
  extension: string;
  description: string;
  /** 单行备注（卡片副标题）；简介仍为 `description` */
  remark: string;
  /** 优先 `cover_url`，否则由简介中 `![cover](url)` 解析，用于卡片/表格缩略图 */
  coverUrl: string | null;
  downloadCount: number;
  fileCount: number;
  /** 原始字节数：文件为 `size`，文件夹可为 `total_size`（用于大文件下载确认） */
  sizeBytes: number;
  sizeText: string;
  updatedAt: string;
  sortTimeMs: number;
  downloadURL: string;
  /** 解析继承后是否允许下载（列表/搜索行内） */
  downloadAllowed: boolean;
  /** 仅 kind === 'file' 时有意义；文件夹固定为空数组 */
  tags: PublicFileTag[];
};

const rows = computed<DirectoryRow[]>(() => [
  ...folders.value.map((folder) => {
    const desc = (folder.description ?? "").trim();
    return {
      id: folder.id,
      kind: "folder" as const,
      name: folder.name,
      extension: "",
      description: desc,
      remark: (folder.remark ?? "").trim(),
      coverUrl: fileCoverImageHrefFromFields((folder as any).cover_url, desc),
      downloadCount: folder.download_count ?? 0,
      fileCount: folder.file_count ?? 0,
      sizeBytes: folder.total_size ?? 0,
      sizeText: formatSize(folder.total_size ?? 0),
      updatedAt: formatDateTime(folder.updated_at),
      sortTimeMs: parseSortTimeMs(folder.updated_at),
      downloadURL: `/api/public/folders/${encodeURIComponent(folder.id)}/download`,
      downloadAllowed: folder.download_allowed !== false,
      tags: [],
    };
  }),
  ...(currentFolderID.value
    ? files.value.map((file) => {
        const desc = (file.description ?? "").trim();
        return {
          id: file.id,
          kind: "file" as const,
          name: file.name,
          extension: normalizeFileExtension(file.extension) || extractExtension(file.name),
          description: desc,
          remark: (file.remark ?? "").trim(),
          coverUrl: fileCoverImageHrefFromFields(file.cover_url, desc),
          downloadCount: file.download_count ?? 0,
          fileCount: 0,
          sizeBytes: file.size ?? 0,
          sizeText: formatSize(file.size),
          updatedAt: formatDateTime(file.uploaded_at),
          sortTimeMs: parseSortTimeMs(file.uploaded_at),
          downloadURL: fileEffectiveDownloadHref(file.id, file.playback_url, file.folder_direct_download_url),
          downloadAllowed: file.download_allowed !== false,
          tags: file.tags ?? [],
        };
      })
    : []),
]);
const displayedRows = computed<DirectoryRow[]>(() => (searchKeyword.value ? searchRows.value : rows.value));

/** 当前文件夹下所有文件的唯一标签（去重，不考虑子文件夹） */
const currentFolderFileTags = computed<PublicFileTag[]>(() => {
  const tagMap = new Map<string, PublicFileTag>();
  for (const row of rows.value) {
    if (row.kind === "file") {
      for (const tag of row.tags) {
        if (!tagMap.has(tag.id)) {
          tagMap.set(tag.id, tag);
        }
      }
    }
  }
  return [...tagMap.values()];
});

const sortedRows = computed(() => {
  const next = [...displayedRows.value];
  next.sort((left, right) => compareRows(left, right, sortMode.value, sortDirection.value));
  if (hasActiveTagFilter.value) {
    return next.filter(
      (row) => row.kind === "folder" || row.tags.some((t) => selectedTagIds.value.has(t.id)),
    );
  }
  return next;
});

type CardDisplayBlock = { key: string; rows: DirectoryRow[] };

const cardDisplayBlocks = computed((): CardDisplayBlock[] => {
  if (!cardCoverFirst.value || hasActiveTagFilter.value) {
    return [{ key: "all", rows: sortedRows.value }];
  }
  const mode = sortMode.value;
  const direction = sortDirection.value;
  const sortChunk = (list: DirectoryRow[]) => {
    const next = [...list];
    next.sort((left, right) => compareRows(left, right, mode, direction));
    return next;
  };
  const withCover = sortChunk(displayedRows.value.filter((r) => !!r.coverUrl));
  const withoutCover = sortChunk(displayedRows.value.filter((r) => !r.coverUrl));
  const blocks: CardDisplayBlock[] = [];
  if (withCover.length) {
    blocks.push({ key: "with-cover", rows: withCover });
  }
  if (withoutCover.length) {
    blocks.push({ key: "without-cover", rows: withoutCover });
  }
  return blocks.length ? blocks : [{ key: "all", rows: sortedRows.value }];
});
const selectedRows = computed(() => sortedRows.value.filter((row) => selectedResourceKeys.value.includes(selectionKey(row))));
const hasSelectedRows = computed(() => selectedRows.value.length > 0);
const selectedRowsDownloadAllowed = computed(() => selectedRows.value.every((row) => row.downloadAllowed));
const downloadConfirmMessage = computed(() => {
  const s = downloadConfirm.value;
  if (!s) {
    return "";
  }
  if (s.mode === "batch") {
    return "所选项目中包含文件夹或超过本站设定阈值的大文件，打包成 ZIP 时体积与耗时不确定，可能较大。确定要继续吗？";
  }
  const row = s.row;
  if (row.kind === "folder") {
    return "您即将打包下载整个文件夹，体积与耗时不确定，可能较大。确定要开始下载吗？";
  }
  return `该文件大小为 ${row.sizeText}，已超过本站设定的大文件阈值（${formatSize(largeDownloadConfirmBytes.value)}）。确定要下载吗？`;
});
const allVisibleRowsSelected = computed(() => sortedRows.value.length > 0 && selectedRows.value.length === sortedRows.value.length);
const currentFolderDescriptionHTML = ref("");
let folderDescRenderSerial = 0;
watch(
  () => currentFolderDetail.value?.description ?? "",
  (desc) => {
    const mySerial = ++folderDescRenderSerial;
    renderMarkdownAsync(desc).then((html) => {
      if (mySerial === folderDescRenderSerial) {
        currentFolderDescriptionHTML.value = html;
      }
    });
  },
  { immediate: true },
);

const useWideDescriptionLayout = computed(() => {
  if (!currentFolderDetail.value) return false;
  const desc = currentFolderDetail.value.description ?? "";
  if (desc.trim().length > 300) return true;
  if (/!\[.*?\]\(.*?\)/.test(desc)) return true;
  return false;
});

const isWideScreen = ref(false);
let wideScreenQuery: MediaQueryList | null = null;
function updateWideScreen() {
  isWideScreen.value = wideScreenQuery?.matches ?? false;
}
onMounted(() => {
  wideScreenQuery = window.matchMedia("(min-width: 1280px)");
  updateWideScreen();
  wideScreenQuery.addEventListener("change", updateWideScreen);
});
onBeforeUnmount(() => {
  wideScreenQuery?.removeEventListener("change", updateWideScreen);
  wideScreenQuery = null;
});

/** 文件夹简介区域：默认限高，可展开全文（仅影响首页目录卡片，不作用于文件详情页）。 */
const folderMarkdownExpanded = ref(false);
const folderMarkdownClampRef = ref<HTMLElement | null>(null);
/** 需要显示「展开 / 收起」时包含：折叠且内容被裁切，或已展开（显示收起） */
const folderMarkdownFooterVisible = ref(false);

function updateFolderMarkdownClampUI() {
  const el = folderMarkdownClampRef.value;
  if (!el) {
    folderMarkdownFooterVisible.value = false;
    return;
  }
  if (!currentFolderDescriptionHTML.value) {
    folderMarkdownFooterVisible.value = false;
    return;
  }
  if (folderMarkdownExpanded.value) {
    folderMarkdownFooterVisible.value = true;
    return;
  }
  folderMarkdownFooterVisible.value = el.scrollHeight > el.clientHeight + 2;
}

const folderMarkdownResizeObserver =
  typeof ResizeObserver !== "undefined"
    ? new ResizeObserver(() => {
        updateFolderMarkdownClampUI();
      })
    : null;

watch(
  folderMarkdownClampRef,
  (el, prev) => {
    if (!folderMarkdownResizeObserver) {
      return;
    }
    if (prev) {
      folderMarkdownResizeObserver.unobserve(prev);
    }
    if (el) {
      folderMarkdownResizeObserver.observe(el);
    }
  },
  { flush: "post" },
);

watch(
  () => [
    currentFolderID.value,
    currentFolderDetail.value?.description,
    currentFolderDescriptionHTML.value,
    folderMarkdownExpanded.value,
  ],
  async () => {
    await nextTick();
    requestAnimationFrame(() => {
      updateFolderMarkdownClampUI();
    });
  },
);

watch(
  () => currentFolderID.value,
  () => {
    folderMarkdownExpanded.value = false;
  },
);
watch(
  () => useWideDescriptionLayout.value && isWideScreen.value,
  (active) => {
    if (active) {
      folderMarkdownExpanded.value = true;
    }
  },
);
function folderDetailIsManagingRoot(d: FolderDetailResponse | null) {
  if (!d) {
    return false;
  }
  const p = d.parent_id;
  return p == null || String(p).trim() === "";
}

const folderEditorMetaDirty = computed(() => {
  if (!currentFolderDetail.value) {
    return false;
  }
  const d = currentFolderDetail.value;
  return (
    folderNameDraft.value.trim() !== d.name ||
    folderDescriptionDraft.value.trim() !== (d.description ?? "") ||
    folderRemarkDraft.value.trim() !== (d.remark ?? "").trim() ||
    folderCoverUrlDraft.value.trim() !== (d.cover_url ?? "").trim() ||
    folderDirectPrefixDraft.value.trim() !== (d.direct_link_prefix ?? "").trim() ||
    folderCustomPathDraft.value.trim() !== (d.custom_path ?? "").trim() ||
    folderDownloadPolicyDraft.value !== (d.download_policy ?? "inherit")
  );
});

const folderEditorHideCatalogDirty = computed(() => {
  if (!currentFolderDetail.value || !folderDetailIsManagingRoot(currentFolderDetail.value)) {
    return false;
  }
  const on = Boolean(currentFolderDetail.value.hide_public_catalog);
  return folderHidePublicCatalogDraft.value !== on;
});

const folderEditorDirty = computed(() => folderEditorMetaDirty.value || folderEditorHideCatalogDirty.value);
const folderDescriptionPreviewHTML = computed(() => renderSimpleMarkdown(folderDescriptionDraft.value));
const currentFolderStats = computed(() => {
  if (!currentFolderDetail.value) {
    return [];
  }

  return [
    { label: "下载量", value: String(currentFolderDetail.value.download_count ?? 0) },
    { label: "文件数", value: `${currentFolderDetail.value.file_count ?? 0} 个文件` },
    { label: "文件夹大小", value: formatSize(currentFolderDetail.value.total_size ?? 0) },
    { label: "更新时间", value: formatDateTime(currentFolderDetail.value.updated_at) },
  ];
});
const canGoUp = computed(() => currentFolderID.value.length > 0);
const backButtonLabel = computed(() => (searchKeyword.value ? "返回所在目录" : "返回上一级"));
const canUseBackButton = computed(() => searchKeyword.value.length > 0 || canGoUp.value);

/** 当前详情为托管根目录（无父级）时可重新扫描磁盘，与后台 rescan API 一致。虚拟目录不可重新扫描。 */
const showRescanCurrentManagedFolder = computed(() => {
  const d = currentFolderDetail.value;
  if (!d || !canManageResourceDescriptions.value) {
    return false;
  }
  if (d.is_virtual) {
    return false;
  }
  const p = d.parent_id;
  return p == null || String(p).trim() === "";
});

const rescanningManagedFolderID = ref("");

async function rescanCurrentManagedFolder() {
  const d = currentFolderDetail.value;
  if (!d || !showRescanCurrentManagedFolder.value) {
    return;
  }
  rescanningManagedFolderID.value = d.id;
  actionError.value = "";
  actionMessage.value = "";
  try {
    const response = await httpClient.post<{
      added_folders: number;
      added_files: number;
      updated_folders: number;
      updated_files: number;
      deleted_folders: number;
      deleted_files: number;
    }>(`/admin/imports/local/${encodeURIComponent(d.id)}/rescan`);
    toastSuccess(
      `重新扫描完成：新增目录 ${response.added_folders} 个、文件 ${response.added_files} 个，` +
      `更新目录 ${response.updated_folders} 个、文件 ${response.updated_files} 个，` +
      `移除目录 ${response.deleted_folders} 个、文件 ${response.deleted_files} 个。`);
    invalidateDirectoryViewCacheAll();
    await loadDirectory({ force: true });
    void loadHotDownloads();
    void loadLatestTitles();
  } catch (err: unknown) {
    toastError(readApiError(err, "重新扫描失败。"));
  } finally {
    rescanningManagedFolderID.value = "";
  }
}

function rowNeedsDownloadConfirm(row: DirectoryRow) {
  if (row.kind === "folder") {
    return true;
  }
  return (row.sizeBytes ?? 0) >= largeDownloadConfirmBytes.value;
}

function performDownloadResource(row: DirectoryRow) {
  actionMessage.value = "";
  actionError.value = "";
  if (!row.downloadAllowed) {
    toastWarning("该资源不允许下载。");
    return;
  }
  if (!allowDownloadRequest()) {
    toastWarning("下载请求过于频繁，请稍后再试。");
    return;
  }

  const link = document.createElement("a");
  link.href = row.downloadURL;
  link.rel = "noopener";
  if (row.downloadURL.startsWith("http://") || row.downloadURL.startsWith("https://")) {
    link.target = "_blank";
  }
  document.body.appendChild(link);
  link.click();
  link.remove();

  applyDownloadCountUpdate(row);
  void loadHotDownloads();
}

function downloadResource(row: DirectoryRow) {
  if (!row.downloadAllowed) {
    toastWarning("该资源不允许下载。");
    return;
  }
  if (rowNeedsDownloadConfirm(row)) {
    downloadConfirm.value = { mode: "single", row };
    syncBodyScrollLock();
    return;
  }
  performDownloadResource(row);
}

function closeDownloadConfirm() {
  downloadConfirm.value = null;
  syncBodyScrollLock();
}

function confirmPendingDownload() {
  const pending = downloadConfirm.value;
  if (!pending) {
    return;
  }
  downloadConfirm.value = null;
  syncBodyScrollLock();
  if (pending.mode === "single") {
    performDownloadResource(pending.row);
  } else {
    void performBatchDownload();
  }
}

function batchNeedsDownloadConfirm() {
  return selectedRows.value.some((row) => rowNeedsDownloadConfirm(row));
}

function selectionKey(row: DirectoryRow) {
  return `${row.kind}:${row.id}`;
}

function isRowSelected(row: DirectoryRow) {
  return selectedResourceKeys.value.includes(selectionKey(row));
}

function toggleRowSelection(row: DirectoryRow) {
  const key = selectionKey(row);
  if (selectedResourceKeys.value.includes(key)) {
    selectedResourceKeys.value = selectedResourceKeys.value.filter((item) => item !== key);
    return;
  }
  selectedResourceKeys.value = [...selectedResourceKeys.value, key];
}

function clearSelection() {
  selectedResourceKeys.value = [];
}

function selectAllVisibleRows() {
  selectedResourceKeys.value = sortedRows.value.map((row) => selectionKey(row));
}

function toggleSelectAllVisibleRows() {
  if (allVisibleRowsSelected.value) {
    clearSelection();
    return;
  }
  selectAllVisibleRows();
}

function toggleCardMultiSelectMode() {
  if (cardMultiSelectMode.value) {
    cardMultiSelectMode.value = false;
    clearSelection();
    return;
  }
  cardMultiSelectMode.value = true;
}

function onCardOpenClick(row: DirectoryRow) {
  if (viewMode.value === "cards" && cardMultiSelectMode.value) {
    toggleRowSelection(row);
    return;
  }
  if (row.kind === "folder") {
    openFolder(row.id);
  } else {
    openFile(row.id);
  }
}

async function downloadSelectedResources() {
  if (!hasSelectedRows.value || batchDownloadSubmitting.value) {
    return;
  }
  if (!selectedRowsDownloadAllowed.value) {
    toastWarning("所选项目中包含不允许下载的项。");
    return;
  }

  if (batchNeedsDownloadConfirm()) {
    downloadConfirm.value = { mode: "batch" };
    syncBodyScrollLock();
    return;
  }

  await performBatchDownload();
}

async function performBatchDownload() {
  if (!hasSelectedRows.value || batchDownloadSubmitting.value) {
    return;
  }
  if (!selectedRowsDownloadAllowed.value) {
    toastWarning("所选项目中包含不允许下载的项。");
    return;
  }

  actionMessage.value = "";
  actionError.value = "";
  if (!allowDownloadRequest()) {
    toastWarning("下载请求过于频繁，请稍后再试。");
    return;
  }

  const fileIDs = selectedRows.value.filter((row) => row.kind === "file").map((row) => row.id);
  const folderIDs = selectedRows.value.filter((row) => row.kind === "folder").map((row) => row.id);

  batchDownloadSubmitting.value = true;
  try {
    const response = await fetch("/api/public/resources/batch-download", {
      method: "POST",
      credentials: "include",
      headers: {
        "Content-Type": "application/json",
        Accept: "application/zip",
      },
      body: JSON.stringify({
        file_ids: fileIDs,
        folder_ids: folderIDs,
      }),
    });

    if (!response.ok) {
      throw new Error("batch download failed");
    }

    const blob = await response.blob();
    const url = window.URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = "openshare-selection.zip";
    document.body.appendChild(link);
    link.click();
    link.remove();
    window.URL.revokeObjectURL(url);

    for (const row of selectedRows.value) {
      applyDownloadCountUpdate(row);
    }
    await loadHotDownloads();
    clearSelection();
    cardMultiSelectMode.value = false;
  } catch (err: unknown) {
    toastError(readApiError(err, "批量下载失败。"));
  } finally {
    batchDownloadSubmitting.value = false;
  }
}

function syncBodyScrollLock() {
  const shouldLock = Boolean(
    announcementListOpen.value
      || sidebarDetailModal.value
      || uploadModalOpen.value
      || uploadSuccessModalOpen.value
      || feedbackModalOpen.value
      || feedbackSuccessModalOpen.value
      || folderDescriptionEditorOpen.value
      || createFolderModalOpen.value
      || virtualFolderModalOpen.value
      || virtualFileModalOpen.value
      || deleteResourceTarget.value
      || downloadConfirm.value
      || Boolean(fileDetailPanelFileId.value)
      || Boolean(markdownCatalogNavigateConfirmRoute.value),
  );
  document.body.style.overflow = shouldLock ? "hidden" : "";
}

function promptMarkdownCatalogNavigateConfirm(route: RouteLocationRaw): Promise<boolean> {
  return new Promise((resolve) => {
    markdownCatalogNavigateHydrateGeneration += 1;
    const gen = markdownCatalogNavigateHydrateGeneration;
    markdownCatalogNavigateConfirmRoute.value = route;
    markdownCatalogNavigatePresentation.value = markdownCatalogNavigateInitialPresentation(route);
    markdownCatalogNavigateConfirmResolve = resolve;
    syncBodyScrollLock();
    void hydrateMarkdownCatalogNavigatePresentation(route).then((presentation) => {
      if (gen !== markdownCatalogNavigateHydrateGeneration) {
        return;
      }
      markdownCatalogNavigatePresentation.value = presentation;
    });
  });
}

function dismissMarkdownCatalogNavigateConfirm(ok: boolean) {
  markdownCatalogNavigateHydrateGeneration += 1;
  markdownCatalogNavigateConfirmRoute.value = null;
  markdownCatalogNavigatePresentation.value = null;
  markdownCatalogNavigateConfirmResolve?.(ok);
  markdownCatalogNavigateConfirmResolve = null;
  syncBodyScrollLock();
}

function interceptHomeMarkdownFilePanelPeek(route: RouteLocationRaw): boolean {
  const fileId = markdownRoutePublicFileDetailId(route);
  if (!fileId) {
    return false;
  }
  if (!isViewportTailwindXlMin()) {
    return false;
  }
  fileDetailPanelFileId.value = fileId;
  syncBodyScrollLock();
  return true;
}

function handleMarkdownInternalLinkNavigate(ev: MouseEvent) {
  onMarkdownLinkClickCapture(ev, router, {
    interceptPush: interceptHomeMarkdownFilePanelPeek,
    confirmBeforeMarkdownCatalogNavigate: promptMarkdownCatalogNavigateConfirm,
  });
}

function onDocumentPointerDownCloseToolbarMenus(event: PointerEvent) {
  if (!sortMenuOpen.value && !viewMenuOpen.value) {
    return;
  }
  const root = toolbarDropdownsRef.value;
  const target = event.target;
  if (!root || !(target instanceof Node) || root.contains(target)) {
    return;
  }
  sortMenuOpen.value = false;
  viewMenuOpen.value = false;
}

onMounted(async () => {
  registerHomeConsoleHooks({
    setListView: setViewMode,
    setListSort: setSortMode,
    setListSortDirection: setSortDirection,
  });
  document.addEventListener("pointerdown", onDocumentPointerDownCloseToolbarMenus, true);
  const storedViewMode = window.localStorage.getItem("public-home-view-mode");
  if (storedViewMode === "cards" || storedViewMode === "table") {
    viewMode.value = storedViewMode;
  }
  const storedSortMode = window.localStorage.getItem("public-home-sort-mode");
  if (storedSortMode === "name" || storedSortMode === "download" || storedSortMode === "format" || storedSortMode === "modified") {
    sortMode.value = storedSortMode;
  }
  const storedSortDirection = window.localStorage.getItem("public-home-sort-direction");
  if (storedSortDirection === "asc" || storedSortDirection === "desc") {
    sortDirection.value = storedSortDirection;
  }
  const storedCardCoverFirst = window.localStorage.getItem("public-home-card-cover-first");
  if (storedCardCoverFirst === "0" || storedCardCoverFirst === "false") {
    cardCoverFirst.value = false;
  } else if (storedCardCoverFirst === "1" || storedCardCoverFirst === "true") {
    cardCoverFirst.value = true;
  }
  currentReceiptCode.value = await syncSessionReceiptCode();
  await Promise.all([
    loadDirectory(),
    loadLargeDownloadPolicy(),
  ]);
  // 管理员权限延后加载：主内容先渲染，编辑/删除按钮稍后出现
  loadAdminPermission();

  const panel = route.query.panel;
  if (typeof panel === "string" && panel) {
    await nextTick();
    openPanelFromQuery(panel);
  }
});

async function loadLargeDownloadPolicy() {
  if (downloadPolicyLoaded || staticDataLoader.policyApplied) return;
  if (staticDataLoader.policyPromise) {
    await staticDataLoader.policyPromise;
    if (staticDataLoader.policyApplied) return;
  }
  // 先同步设 Promise 再启动异步工作，消除竞态窗口
  let taskResolve!: () => void;
  staticDataLoader.setPolicyPromise(new Promise<void>((r) => { taskResolve = r; }));
  try {
    const cached = staticDataLoader.downloadPolicy;
    if (cached) {
      const b = Number(cached.large_download_confirm_bytes);
      if (Number.isFinite(b) && b > 0) largeDownloadConfirmBytes.value = b;
      if (cached.directory_cdn_urls) staticDataLoader.setCdnUrlMapFromObject(cached.directory_cdn_urls);
      downloadPolicyLoaded = true;
      staticDataLoader.markPolicyApplied();
      return;
    }
    const response = await httpClient.get<{ large_download_confirm_bytes: number; directory_cdn_urls?: Record<string, string> }>("/public/download-policy");
    const b = Number(response.large_download_confirm_bytes);
    if (Number.isFinite(b) && b > 0) largeDownloadConfirmBytes.value = b;
    if (response.directory_cdn_urls) staticDataLoader.setCdnUrlMapFromObject(response.directory_cdn_urls);
    if ((response as any).global_cdn_url) staticDataLoader.setGlobalCdnUrl((response as any).global_cdn_url);
    downloadPolicyLoaded = true;
    staticDataLoader.markPolicyApplied();
  } catch {
    /* 使用默认 1 GiB */
  } finally {
    taskResolve();
    staticDataLoader.setPolicyPromise(null);
  }
}

onBeforeUnmount(() => {
  unregisterHomeConsoleHooks();
  document.removeEventListener("pointerdown", onDocumentPointerDownCloseToolbarMenus, true);
  folderMarkdownResizeObserver?.disconnect();
  markdownCatalogNavigateHydrateGeneration += 1;
  markdownCatalogNavigateConfirmResolve?.(false);
  markdownCatalogNavigateConfirmResolve = null;
  markdownCatalogNavigateConfirmRoute.value = null;
  markdownCatalogNavigatePresentation.value = null;
  document.body.style.overflow = "";
});

watch(currentFolderID, () => {
  fileDetailPanelFileId.value = null;
  selectedTagIds.value = new Set();
  if (markdownCatalogNavigateConfirmRoute.value) {
    dismissMarkdownCatalogNavigateConfirm(false);
  }
  clearSearchState();
  void loadDirectory();
});

// 监听自定义路径参数（如 /doc），通过 API 解析为文件夹或文件 UUID
watch(
  () => route.params.customPath as string | undefined,
  async (customPath) => {
    const trimmed = (customPath ?? "").trim();
    if (!trimmed) {
      customPathResolvedFolderID.value = "";
      if (route.meta?.resolvedFolderId) {
        route.meta.resolvedFolderId = undefined;
      }
      return;
    }
    try {
      const resp = await httpClient.get<{
        type: string;
        folder_id: string;
        file_id: string;
        name: string;
      }>(`/public/resolve-custom-path?path=${encodeURIComponent(trimmed)}`);
      if (resp.type === "file" && resp.file_id) {
        // 文件自定义路径 → 跳转到文件详情页
        customPathResolvedFolderID.value = "";
        router.replace({ name: "public-file-detail", params: { fileID: resp.file_id } });
        return;
      }
      customPathResolvedFolderID.value = resp.folder_id ?? "";
      if (resp.folder_id) {
        route.meta.resolvedFolderId = resp.folder_id;
      }
    } catch {
      customPathResolvedFolderID.value = "";
      if (route.meta?.resolvedFolderId) {
        route.meta.resolvedFolderId = undefined;
      }
    }
  },
  { immediate: true },
);

watch(fileDetailPanelFileId, (id, _prev, onCleanup) => {
  syncBodyScrollLock();
  if (!id) {
    return;
  }
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === "Escape") {
      closeFileDetailPanel();
    }
  };
  window.addEventListener("keydown", onKeyDown);
  onCleanup(() => {
    window.removeEventListener("keydown", onKeyDown);
  });
});

watch(markdownCatalogNavigateConfirmRoute, (r, _p, onCleanup) => {
  syncBodyScrollLock();
  if (!r) {
    return;
  }
  const onKeyDownCatalog = (e: KeyboardEvent) => {
    if (e.key === "Escape") {
      dismissMarkdownCatalogNavigateConfirm(false);
    }
  };
  window.addEventListener("keydown", onKeyDownCatalog);
  onCleanup(() => {
    window.removeEventListener("keydown", onKeyDownCatalog);
  });
});

async function loadAnnouncements() {
  if (announcementsLoaded) return;
  const cached = staticDataLoader.announcements;
  if (cached && cached.length > 0) {
    announcements.value = cached as unknown as AnnouncementItem[];
    announcementsLoaded = true;
    return;
  }
  try {
    const response = await httpClient.get<{ items: AnnouncementItem[] }>("/public/announcements");
    announcements.value = response.items ?? [];
    announcementsLoaded = true;
  } catch {
    announcements.value = [];
  }
}

function selectAnnouncement(id: string) {
  selectedAnnouncementId.value = id;
}

async function openAnnouncementList() {
  if (!announcementsLoaded) await loadAnnouncements();
  announcementListOpen.value = true;
  if (announcements.value.length > 0 && !selectedAnnouncementId.value) {
    selectedAnnouncementId.value = announcements.value[0].id;
  }
  syncBodyScrollLock();
}

function closeAnnouncementList() {
  announcementListOpen.value = false;
  selectedAnnouncementId.value = null;
  closeNavPanel();
  clearPanelQuery();
  syncBodyScrollLock();
}

function announcementAuthorName(item: AnnouncementItem) {
  return item.creator?.display_name?.trim() || item.creator?.username?.trim() || "未知用户";
}

function announcementAuthorInitial(item: AnnouncementItem) {
  return announcementAuthorName(item).slice(0, 1).toUpperCase() || "A";
}

function announcementAuthorIsSuperAdmin(item: AnnouncementItem) {
  return item.creator?.role === "super_admin";
}

function canEditAnnouncementOnHome(item: AnnouncementItem | null) {
  if (!item || !canManageAnnouncements.value) {
    return false;
  }
  if (homeSessionIsSuperAdmin.value) {
    return true;
  }
  const creatorId = item.creator?.id?.trim() ?? "";
  return Boolean(creatorId && creatorId === homeSessionAdminId.value);
}

function openAnnouncementInAdminEditor() {
  const d = selectedAnnouncement.value;
  if (!d || !canEditAnnouncementOnHome(d)) {
    return;
  }
  void router.push({ name: "admin-announcements", query: { edit: d.id } });
}

function openSidebarDetailModal(modal: SidebarDetailModalState) {
  sidebarDetailModal.value = modal;
  syncBodyScrollLock();
}

function closeSidebarDetailModal() {
  sidebarDetailModal.value = null;
  closeNavPanel();
  clearPanelQuery();
  syncBodyScrollLock();
}

function openSidebarDetailItem(item: InfoPanelCardItem) {
  sidebarDetailModal.value = null;
  syncBodyScrollLock();
  openFile(item.id);
}

async function openHotDownloadsModal() {
  if (!hotDownloadsLoaded) await loadHotDownloads();
  openSidebarDetailModal({
    eyebrow: "Hot Downloads",
    title: "热门下载",
    description: "展示近七天内下载量最高的前 20 份资料，点击可跳转文件详情页。",
    items: hotDownloadItems.value.map((item) => ({
      id: item.id,
      label: item.name,
      meta: `${item.downloadCount} 次下载`,
    })),
  });
}

async function loadHotDownloads() {
  if (hotDownloadsLoaded) return;
  const cached = staticDataLoader.hotFiles;
  if (cached) {
    const items = (cached as unknown as { items: PublicFileItem[] }).items ?? [];
    hotDownloadItems.value = items.map((item) => ({
      id: item.id,
      name: item.name,
      downloadCount: item.download_count ?? 0,
    }));
    hotDownloadsLoaded = true;
    return;
  }
  try {
    const response = await httpClient.get<{ items: PublicFileItem[] }>("/public/files/hot?limit=20");
    hotDownloadItems.value = (response.items ?? []).map((item) => ({
      id: item.id,
      name: item.name,
      downloadCount: item.download_count ?? 0,
    }));
    hotDownloadsLoaded = true;
  } catch {
    hotDownloadItems.value = [];
  }
}

async function loadLatestTitles() {
  if (latestTitlesLoaded) return;
  const cached = staticDataLoader.latestFiles;
  if (cached) {
    const items = (cached as unknown as { items: PublicFileItem[] }).items ?? [];
    latestItems.value = items.map((item) => ({
      id: item.id,
      name: item.name,
    }));
    latestTitlesLoaded = true;
    return;
  }
  try {
    const response = await httpClient.get<{ items: PublicFileItem[] }>("/public/files/latest?limit=20");
    latestItems.value = (response.items ?? []).map((item) => ({
      id: item.id,
      name: item.name,
    }));
    latestTitlesLoaded = true;
  } catch {
    latestItems.value = [];
  }
}

async function openLatestItemsModal() {
  if (!latestTitlesLoaded) await loadLatestTitles();
  openSidebarDetailModal({
    eyebrow: "Latest Files",
    title: "资料上新",
    description: "展示最新发布的前 20 份资料，点击标题可跳转文件详情页。",
    items: latestItems.value.map((item) => ({
      id: item.id,
      label: item.name,
    })),
  });
}

function snapshotDirectoryViewFromRefs(): DirectoryViewCacheEntry {
  const d = currentFolderDetail.value;
  return {
    folders: folders.value.map((f) => ({ ...f })),
    files: files.value.map((f) => ({ ...f })),
    detail: d ? (JSON.parse(JSON.stringify(d)) as FolderDetailResponse) : null,
  };
}

function applyDirectoryViewToState(entry: DirectoryViewCacheEntry) {
  folders.value = entry.folders.map((f) => ({ ...f }));
  files.value = entry.files.map((f) => ({ ...f }));
  if (entry.detail) {
    const detail = entry.detail;
    currentFolderDetail.value = detail;
    folderNameDraft.value = detail.name;
    folderDescriptionDraft.value = detail.description ?? "";
    folderRemarkDraft.value = (detail.remark ?? "").trim();
    folderCoverUrlDraft.value = (detail.cover_url ?? "").trim();
    folderDirectPrefixDraft.value = (detail.direct_link_prefix ?? "").trim();
    folderCustomPathDraft.value = (detail.custom_path ?? "").trim();
    folderDownloadPolicyDraft.value = detail.download_policy ?? "inherit";
    breadcrumbs.value = detail.breadcrumbs ?? [];
  } else {
    currentFolderDetail.value = null;
    folderNameDraft.value = "";
    folderDescriptionDraft.value = "";
    folderRemarkDraft.value = "";
    folderCoverUrlDraft.value = "";
    folderDirectPrefixDraft.value = "";
    folderCustomPathDraft.value = "";
    folderDownloadPolicyDraft.value = "inherit";
    breadcrumbs.value = [];
  }
}

async function loadDirectory(options?: { force?: boolean }) {
  const gen = takeDirectoryViewLoadToken();
  const requestedKey = currentFolderID.value;

  if (!options?.force) {
    const cached = readDirectoryViewCache(requestedKey);
    if (cached && isDirectoryViewCacheEntryUsable(requestedKey, cached)) {
      if (gen !== peekDirectoryViewLoadToken()) {
        return;
      }
      error.value = "";
      actionMessage.value = "";
      actionError.value = "";
      applyDirectoryViewToState(cached);
      if (!requestedKey) {
        sharedRootFolders.value = [...folders.value];
      }
      loading.value = false;
      return;
    }

    // 尝试从 staticDataLoader 获取预加载的 CDN 数据（按需拉取）
    if (requestedKey) {
      // 若 download-policy 尚未加载，先拉取以获取 directory_cdn_urls 映射
      if (!downloadPolicyLoaded && !staticDataLoader.policyApplied) {
        await loadLargeDownloadPolicy();
      }
      if (gen !== peekDirectoryViewLoadToken() || currentFolderID.value !== requestedKey) return;

      await staticDataLoader.ensureDirectoryLoaded(requestedKey);
      if (gen !== peekDirectoryViewLoadToken() || currentFolderID.value !== requestedKey) return;

      const staticEntry = staticDataLoader.findDirectoryView(requestedKey);
      if (staticEntry) {
        error.value = "";
        actionMessage.value = "";
        actionError.value = "";
        applyDirectoryViewToState({
          folders: staticEntry.folders as unknown as PublicFolderItem[],
          files: staticEntry.files as unknown as PublicFileItem[],
          detail: staticEntry.detail as unknown as FolderDetailResponse,
        });
        loading.value = false;
        return;
      }
    } else {
      // 若 download-policy 尚未加载，先拉取以获取 global_cdn_url
      if (!downloadPolicyLoaded && !staticDataLoader.policyApplied) {
        await loadLargeDownloadPolicy();
      }
      if (gen !== peekDirectoryViewLoadToken()) return;

      // 尝试加载已缓存的全局 CDN JSON
      if (staticDataLoader.globalCdnUrl) {
        await staticDataLoader.preloadGlobalCdn(staticDataLoader.globalCdnUrl);
      }
      if (gen !== peekDirectoryViewLoadToken()) return;

      const staticRoots = staticDataLoader.rootFolders;
      if (staticRoots && staticRoots.length > 0) {
        if (gen !== peekDirectoryViewLoadToken()) return;
        error.value = "";
        actionMessage.value = "";
        actionError.value = "";
        applyDirectoryViewToState({
          folders: staticRoots as unknown as PublicFolderItem[],
          files: [],
          detail: null,
        });
        sharedRootFolders.value = [...folders.value];
        loading.value = false;
        return;
      }
    }
  }

  loading.value = true;
  error.value = "";
  actionMessage.value = "";
  actionError.value = "";
  const fetchKey = requestedKey;

  try {
    const directoryParams = new URLSearchParams();
    if (fetchKey) {
      directoryParams.set("parent_id", fetchKey);
    }

    const requests: Array<Promise<unknown>> = [
      httpClient.get<{ items: PublicFolderItem[] }>(`/public/folders${directoryParams.toString() ? `?${directoryParams.toString()}` : ""}`),
    ];

    if (fetchKey) {
      const folderParams = new URLSearchParams({
        page: "1",
        page_size: "100",
        sort: "name_asc",
      });
      requests.push(
        httpClient.get<{ items: PublicFileItem[] }>(
          `/public/folders/${encodeURIComponent(fetchKey)}/files?${folderParams.toString()}`,
        ),
      );
    }

    if (fetchKey) {
      requests.push(httpClient.get<FolderDetailResponse>(`/public/folders/${encodeURIComponent(fetchKey)}`));
    }

    const [folderResponse, fileResponse, folderDetail] = await Promise.all(requests);

    if (gen !== peekDirectoryViewLoadToken() || currentFolderID.value !== fetchKey) {
      return;
    }

    folders.value = (folderResponse as { items: PublicFolderItem[] }).items ?? [];
    if (!fetchKey) {
      sharedRootFolders.value = [...folders.value];

      // 根目录响应内嵌了 download_policy，应用之
      const embeddedPolicy = (folderResponse as any).download_policy;
      if (embeddedPolicy) {
        const b = Number(embeddedPolicy.large_download_confirm_bytes);
        if (Number.isFinite(b) && b > 0) largeDownloadConfirmBytes.value = b;
        downloadPolicyLoaded = true;
        staticDataLoader.markPolicyApplied();
        if (embeddedPolicy.cdn_mode) {
          staticDataLoader.setCdnUrlMap(folders.value as Array<{ id: string; cdn_url?: string }>);
        }
      }
    }
    files.value = fetchKey ? ((fileResponse as { items: PublicFileItem[] } | undefined)?.items ?? []) : [];

    if (!fetchKey && !rootViewLocked.value && folders.value.length === 1) {
      try {
        writeDirectoryViewCache(fetchKey, snapshotDirectoryViewFromRefs());
      } catch {
        invalidateDirectoryViewCacheFolder(fetchKey);
      }
      void router.replace({ name: "public-home", query: { folder: folders.value[0].id } });
      return;
    }

    if (folderDetail) {
      const detail = folderDetail as FolderDetailResponse;
      currentFolderDetail.value = detail;
      folderNameDraft.value = detail.name;
      folderDescriptionDraft.value = detail.description ?? "";
      folderRemarkDraft.value = (detail.remark ?? "").trim();
      folderCoverUrlDraft.value = (detail.cover_url ?? "").trim();
      folderDirectPrefixDraft.value = (detail.direct_link_prefix ?? "").trim();
      folderDownloadPolicyDraft.value = detail.download_policy ?? "inherit";
      breadcrumbs.value = detail.breadcrumbs ?? [];
    } else {
      currentFolderDetail.value = null;
      folderNameDraft.value = "";
      folderDescriptionDraft.value = "";
      folderRemarkDraft.value = "";
      folderCoverUrlDraft.value = "";
      folderDirectPrefixDraft.value = "";
      folderDownloadPolicyDraft.value = "inherit";
      breadcrumbs.value = [];
    }

    try {
      writeDirectoryViewCache(fetchKey, snapshotDirectoryViewFromRefs());
    } catch {
      invalidateDirectoryViewCacheFolder(fetchKey);
    }
  } catch (err: unknown) {
    if (gen === peekDirectoryViewLoadToken() && currentFolderID.value === fetchKey) {
      invalidateDirectoryViewCacheFolder(fetchKey);
      folders.value = [];
      files.value = [];
      breadcrumbs.value = [];
      currentFolderDetail.value = null;
      folderNameDraft.value = "";
      folderDescriptionDraft.value = "";
      folderRemarkDraft.value = "";
      folderCoverUrlDraft.value = "";
      folderDirectPrefixDraft.value = "";
      folderDownloadPolicyDraft.value = "inherit";
      if (err instanceof HttpError && err.status === 404) {
        toastError("目录不存在或未公开。");
      } else {
        toastError("加载目录失败。");
      }
    }
  } finally {
    if (gen === peekDirectoryViewLoadToken()) {
      loading.value = false;
    }
  }
}

async function loadAdminPermission() {
  canManageResourceDescriptions.value = false;
  canManageAnnouncements.value = false;
  homeSessionAdminId.value = "";
  homeSessionIsSuperAdmin.value = false;
  try {
    const response = await httpClient.get<{
      admin: { id: string; role: string; permissions: string[] };
    }>("/admin/me");
    const a = response.admin;
    const perms = a.permissions ?? [];
    const isSuper = a.role === "super_admin";
    homeSessionIsSuperAdmin.value = isSuper;
    homeSessionAdminId.value = String(a.id ?? "").trim();
    canManageResourceDescriptions.value = isSuper || perms.includes("resource_moderation");
    canManageAnnouncements.value = isSuper || perms.includes("announcements");
  } catch {
    /* 未登录或非管理员 */
  }
}

function openRoot() {
  clearSearchState();
  void router.push({ name: "public-home", query: { root: "1" } });
}

function goUpOneLevel() {
  if (searchKeyword.value) {
    clearSearchState();
    return;
  }
  if (!currentFolderID.value) {
    return;
  }
  clearSearchState();
  const parent = breadcrumbs.value.at(-2);
  if (parent) {
    void router.push({ name: "public-home", query: { folder: parent.id } });
    return;
  }
  openRoot();
}

function openFolder(folderID: string) {
  clearSearchState();
  void router.push({ name: "public-home", query: { folder: folderID } });
}

function openFile(fileID: string) {
  if (searchKeyword.value) {
    clearSearchState();
  }
  void router.push({ name: "public-file-detail", params: { fileID } });
}

function openInNewWindow(row: DirectoryRow) {
  let resolved: ReturnType<typeof router.resolve>;
  if (row.kind === "file") {
    resolved = router.resolve({ name: "public-file-detail", params: { fileID: row.id } });
  } else {
    resolved = router.resolve({ name: "public-home", query: { folder: row.id } });
  }
  window.open(resolved.href, "_blank");
}

function openFileDetailInSidePanel(fileID: string) {
  fileDetailPanelFileId.value = fileID;
}

function closeFileDetailPanel() {
  fileDetailPanelFileId.value = null;
}

function onFileDetailPanelOpenFullPage() {
  const id = fileDetailPanelFileId.value;
  closeFileDetailPanel();
  if (id) {
    openFile(id);
  }
}

function onFileDetailPanelNavigate(nextId: string) {
  fileDetailPanelFileId.value = nextId;
}

function onFileDetailPanelLeaveCatalog() {
  closeFileDetailPanel();
  void router.push({ name: "public-home" });
}

function downloadCurrentFolder() {
  if (!currentFolderDetail.value) {
    return;
  }
  if (currentFolderDetail.value.download_allowed === false) {
    toastWarning("该文件夹不允许下载。");
    return;
  }
  downloadResource({
    id: currentFolderDetail.value.id,
    kind: "folder",
    name: currentFolderDetail.value.name,
    extension: "",
    description: (currentFolderDetail.value.description ?? "").trim(),
    remark: (currentFolderDetail.value.remark ?? "").trim(),
    coverUrl: fileCoverImageHrefFromFields(currentFolderDetail.value.cover_url, (currentFolderDetail.value.description ?? "").trim()),
    downloadCount: currentFolderDetail.value.download_count ?? 0,
    fileCount: currentFolderDetail.value.file_count ?? 0,
    sizeBytes: currentFolderDetail.value.total_size ?? 0,
    sizeText: formatSize(currentFolderDetail.value.total_size ?? 0),
    updatedAt: formatDateTime(currentFolderDetail.value.updated_at),
    sortTimeMs: parseSortTimeMs(currentFolderDetail.value.updated_at),
    downloadURL: `/api/public/folders/${encodeURIComponent(currentFolderDetail.value.id)}/download`,
    downloadAllowed: true,
    tags: [],
  });
}

function openDeleteFolderDialog() {
  if (!currentFolderDetail.value) {
    return;
  }
  deleteResourceTarget.value = {
    id: currentFolderDetail.value.id,
    kind: "folder",
    name: currentFolderDetail.value.name,
  };
  deleteResourcePassword.value = "";
  // 虚拟目录无磁盘文件，只能彻底删除（DB 记录）
  deleteResourceMoveToTrash.value = !isCurrentFolderVirtual.value;
  deleteResourceError.value = "";
}

/** 删除虚拟目录下的文件（仅管理员可用） */
function openDeleteFileDialog(row: DirectoryRow) {
  deleteResourceTarget.value = {
    id: row.id,
    kind: "file",
    name: row.name,
  };
  deleteResourcePassword.value = "";
  // 虚拟文件无磁盘文件，只能从 DB 移除
  deleteResourceMoveToTrash.value = false;
  deleteResourceError.value = "";
}

function closeDeleteResourceDialog() {
  deleteResourceTarget.value = null;
  deleteResourcePassword.value = "";
  deleteResourceMoveToTrash.value = true;
  deleteResourceError.value = "";
  deleteResourceSubmitting.value = false;
}

async function confirmDeleteResource() {
  if (!deleteResourceTarget.value) {
    return;
  }
  if (!deleteResourcePassword.value.trim()) {
    toastError("请输入当前管理员密码。");
    return;
  }

  deleteResourceSubmitting.value = true;
  deleteResourceError.value = "";
  const target = deleteResourceTarget.value;
  const isFile = target.kind === "file";
  const movedToTrash = isFile ? false : deleteResourceMoveToTrash.value;
  const deletedName = target.name;
  try {
    const apiPath = isFile
      ? `/admin/resources/files/${encodeURIComponent(target.id)}`
      : `/admin/resources/folders/${encodeURIComponent(target.id)}`;
    await httpClient.request(apiPath, {
      method: "DELETE",
      body: { password: deleteResourcePassword.value, move_to_trash: movedToTrash },
    });
    if (isFile) {
      // 文件删除后刷新当前目录即可
      invalidateDirectoryViewCacheFolder(currentFolderID.value);
      closeDeleteResourceDialog();
      toastSuccess(`虚拟文件 ${deletedName} 已从数据库中移除。`);
      await loadDirectory({ force: true });
    } else {
      const parentID = currentFolderDetail.value?.parent_id ?? "";
      invalidateDirectoryViewCacheAll();
      closeDeleteResourceDialog();
      if (isCurrentFolderVirtual.value) {
        toastSuccess(`虚拟目录 ${deletedName} 已从数据库中移除。`);
      } else {
        toastSuccess(movedToTrash
          ? `文件夹 ${deletedName} 已移至所在磁盘根目录下的 trash 回收目录。`
          : `文件夹 ${deletedName} 已从磁盘彻底删除。`);
      }
      clearSearchState();
      if (parentID) {
        await router.push({ name: "public-home", query: { folder: parentID } });
      } else {
        await router.push({ name: "public-home", query: { root: "1" } });
      }
    }
  } catch (err: unknown) {
    toastError(readApiError(err, isFile ? "删除文件失败。" : "删除文件夹失败。"));
  } finally {
    deleteResourceSubmitting.value = false;
  }
}

async function runSearch(keyword: string) {
  const normalizedKeyword = keyword.trim();
  if (!normalizedKeyword) {
    clearSearchState();
    return;
  }

  searchInput.value = normalizedKeyword;
  searchKeyword.value = normalizedKeyword;
  searchLoading.value = true;
  searchError.value = "";
  try {
    const query = new URLSearchParams({
      q: normalizedKeyword,
      page: "1",
      page_size: "50",
    });
    if (currentFolderID.value) {
      query.set("folder_id", currentFolderID.value);
    }
    const response = await httpClient.get<SearchResultResponse>(`/public/search?${query.toString()}`);
    searchRows.value = response.items.map((item) => {
      const modRaw =
        item.entity_type === "folder"
          ? item.updated_at
          : (item.updated_at || item.uploaded_at);
      return {
        id: item.id,
        kind: item.entity_type,
        name: item.name,
        extension: item.entity_type === "file" ? normalizeFileExtension(item.extension) || extractExtension(item.name) : "",
        description: "",
        remark: (item.remark ?? "").trim(),
        coverUrl:
          item.entity_type === "file"
            ? fileCoverImageHrefFromFields(item.cover_url, "")
            : fileCoverImageHrefFromFields(item.cover_url, ""),
        downloadCount: item.download_count ?? 0,
        fileCount: 0,
        sizeBytes: item.entity_type === "file" ? (item.size ?? 0) : 0,
        sizeText: item.entity_type === "file" ? formatSize(item.size ?? 0) : "-",
        updatedAt: modRaw ? formatDateTime(modRaw) : "-",
        sortTimeMs: parseSortTimeMs(modRaw),
        downloadURL: item.entity_type === "file"
          ? fileEffectiveDownloadHref(item.id, item.playback_url, item.folder_direct_download_url)
          : `/api/public/folders/${encodeURIComponent(item.id)}/download`,
        downloadAllowed: item.download_allowed !== false,
        tags: item.entity_type === "file" ? (item.tags ?? []) : [],
      };
    });
  } catch (err: unknown) {
    searchRows.value = [];
    toastError(readApiError(err, "搜索失败。"));
  } finally {
    searchLoading.value = false;
  }
}

function clearSearchState() {
  searchInput.value = "";
  searchKeyword.value = "";
  searchLoading.value = false;
  searchError.value = "";
  searchRows.value = [];
  clearSelection();
}

function openUpload() {
  if (!canUploadToCurrentFolder.value) {
    toastWarning("请先进入一个目录后再上传。");
    return;
  }
  uploadModalOpen.value = true;
  uploadError.value = "";
  uploadMessage.value = "";
  uploadForm.value.description = "";
  uploadForm.value.entries = [];
  void syncSessionReceiptCode();
  if (uploadFileInput.value) {
    uploadFileInput.value.value = "";
  }
  syncBodyScrollLock();
}

function closeUploadModal() {
  uploadModalOpen.value = false;
  syncBodyScrollLock();
}

function closeUploadSuccessModal() {
  uploadSuccessModalOpen.value = false;
  syncBodyScrollLock();
}

function onUploadFileChange(event: Event) {
  const target = event.target as HTMLInputElement;
  uploadForm.value.entries = normalizeFiles(Array.from(target.files ?? []).slice(0, 1));
  if (uploadForm.value.entries.length === 0 && (target.files?.length ?? 0) > 0) {
    toastError("已自动忽略 .DS_Store，请重新选择可上传文件。");
  }
}

function triggerUploadFileSelect() {
  uploadFileInput.value?.click();
}

function clearUploadEntries() {
  uploadForm.value.entries = [];
  if (uploadFileInput.value) {
    uploadFileInput.value.value = "";
  }
}

function onUploadDragEnter() {
  uploadDropActive.value = true;
}

function onUploadDragLeave(event: DragEvent) {
  const currentTarget = event.currentTarget as HTMLElement | null;
  if (currentTarget && event.relatedTarget instanceof Node && currentTarget.contains(event.relatedTarget)) {
    return;
  }
  uploadDropActive.value = false;
}

async function onUploadDrop(event: DragEvent) {
  event.preventDefault();
  uploadDropActive.value = false;
  uploadCollecting.value = true;
  uploadError.value = "";
  try {
    const entries = await collectDroppedEntries(event);
    uploadForm.value.entries = entries;
    if (entries.length === 0 && (event.dataTransfer?.files.length ?? 0) > 0) {
      toastError("检测到的内容仅包含 .DS_Store，已自动忽略。");
    }
  } catch {
    toastError("解析拖拽内容失败，请重试。");
  } finally {
    uploadCollecting.value = false;
  }
}

async function submitUpload() {
  if (uploadForm.value.entries.length === 0) {
    toastError("请选择文件，或直接拖入多文件/文件夹。");
    return;
  }

  uploadSubmitting.value = true;
  uploadError.value = "";
  uploadMessage.value = "";
  try {
    const formData = new FormData();
    formData.set("folder_id", currentFolderID.value);
    formData.set("description", uploadForm.value.description.trim());
    formData.set("manifest", JSON.stringify(uploadForm.value.entries.map((entry) => ({
      relative_path: entry.relativePath,
    }))));
    uploadForm.value.entries.forEach((entry) => {
      formData.append("files", entry.file, entry.file.name);
    });
    const response = await httpClient.post<{ receipt_code: string; item_count: number; status: string }>("/public/submissions", formData);
    toastSuccess(response.status === "approved"
      ? `已上传 ${response.item_count} 个文件，请保存回执码 ${response.receipt_code}。`
      : `已提交 ${response.item_count} 个文件进入审核，请保存回执码 ${response.receipt_code}。`);
    window.sessionStorage.setItem("openshare_receipt_code", response.receipt_code);
    currentReceiptCode.value = response.receipt_code;
    uploadForm.value.description = "";
    clearUploadEntries();
    if (response.status === "approved") {
      invalidateDirectoryViewCacheFolder(currentFolderID.value);
      await loadDirectory({ force: true });
    }
    closeUploadModal();
    uploadSuccessModalOpen.value = true;
    syncBodyScrollLock();
  } catch (err) {
    if (err instanceof HttpError && err.status === 400) {
      toastError("上传参数无效。");
    } else if (err instanceof HttpError && err.status === 409) {
      toastError("提交上传失败，请检查名称或者联系管理员");
    } else {
      toastError("提交上传失败。");
    }
  } finally {
    uploadSubmitting.value = false;
  }
}

function applyDownloadCountUpdate(row: DirectoryRow) {
  if (row.kind === "file") {
    if (!fileUsesBackendDownloadHref(row.downloadURL)) {
      return;
    }
    files.value = files.value.map((item) => {
      if (item.id !== row.id) {
        return item;
      }
      return {
        ...item,
        download_count: item.download_count + 1,
      };
    });
    return;
  }

  folders.value = folders.value.map((item) => {
    if (item.id !== row.id) {
      return item;
    }
    return {
      ...item,
      download_count: item.download_count + Math.max(1, item.file_count),
    };
  });
}

function allowDownloadRequest() {
  const now = Date.now();
  const windowMs = 10_000;
  const limit = 10;
  downloadTimestamps.value = downloadTimestamps.value.filter((timestamp) => now - timestamp < windowMs);
  if (downloadTimestamps.value.length >= limit) {
    return false;
  }
  downloadTimestamps.value.push(now);
  return true;
}

/* 复制文件夹的自定义路径链接（如 https://share.linlifei.top/doc） */
async function copyFolderCustomPathUrl() {
  const customPath = (currentFolderDetail.value?.custom_path ?? "").trim();
  if (!customPath) return;
  const url = `${window.location.origin}/${customPath}`;
  const ok = await copyPlainTextToClipboard(url);
  if (ok) {
    toastSuccess(`已复制自定义路径：/${customPath}`);
  } else {
    toastWarning("复制失败，请手动复制地址栏链接。");
  }
}

function setViewMode(mode: "cards" | "table") {
  viewMode.value = mode;
  viewMenuOpen.value = false;
  window.localStorage.setItem("public-home-view-mode", mode);
}

function toggleCardCoverFirst() {
  cardCoverFirst.value = !cardCoverFirst.value;
  window.localStorage.setItem("public-home-card-cover-first", cardCoverFirst.value ? "1" : "0");
}

watch(sortedRows, (rows) => {
  const allowedKeys = new Set(rows.map((row) => selectionKey(row)));
  selectedResourceKeys.value = selectedResourceKeys.value.filter((key) => allowedKeys.has(key));
}, { immediate: true });

watch(viewMode, (mode) => {
  if (mode === "table") {
    cardMultiSelectMode.value = false;
    return;
  }
  if (mode === "cards" && selectedResourceKeys.value.length > 0) {
    cardMultiSelectMode.value = true;
  }
});

function setSortMode(mode: "name" | "download" | "format" | "modified") {
  sortMode.value = mode;
  window.localStorage.setItem("public-home-sort-mode", mode);
}

function setSortDirection(direction: "asc" | "desc") {
  sortDirection.value = direction;
  sortMenuOpen.value = false;
  window.localStorage.setItem("public-home-sort-direction", direction);
}

function sortModeLabel(mode: "name" | "download" | "format" | "modified") {
  switch (mode) {
    case "download":
      return "下载量排序";
    case "format":
      return "格式排序";
    case "modified":
      return "修改日期排序";
    default:
      return "名称排序";
  }
}

function sortDirectionLabel(direction: "asc" | "desc") {
  return direction === "asc" ? "升序" : "降序";
}

function viewModeLabel(mode: "cards" | "table") {
  return mode === "cards" ? "卡片" : "表格";
}

function openFeedbackModal(target: { id: string; type: "file" | "folder"; name: string }) {
  feedbackModalOpen.value = true;
  feedbackTarget.value = target;
  feedbackDescription.value = "";
  feedbackMessage.value = "";
  feedbackError.value = "";
  void syncSessionReceiptCode();
  syncBodyScrollLock();
}

function closeFeedbackModal() {
  feedbackModalOpen.value = false;
  feedbackTarget.value = null;
  syncBodyScrollLock();
}

function closeFeedbackSuccessModal() {
  feedbackSuccessModalOpen.value = false;
  syncBodyScrollLock();
}

function openFolderDescriptionEditor() {
  folderNameDraft.value = currentFolderDetail.value?.name ?? "";
  folderDescriptionDraft.value = currentFolderDetail.value?.description ?? "";
  folderRemarkDraft.value = (currentFolderDetail.value?.remark ?? "").trim();
  folderCoverUrlDraft.value = (currentFolderDetail.value?.cover_url ?? "").trim();
  folderDirectPrefixDraft.value = (currentFolderDetail.value?.direct_link_prefix ?? "").trim();
  folderCustomPathDraft.value = (currentFolderDetail.value?.custom_path ?? "").trim();
  folderDownloadPolicyDraft.value = currentFolderDetail.value?.download_policy ?? "inherit";
  folderHidePublicCatalogDraft.value = Boolean(currentFolderDetail.value?.hide_public_catalog);
  folderDescriptionError.value = "";
  folderDescriptionEditorOpen.value = true;
  syncBodyScrollLock();
}

function closeFolderDescriptionEditor() {
  folderDescriptionEditorOpen.value = false;
  folderDescriptionSaving.value = false;
  folderDescriptionError.value = "";
  folderNameDraft.value = currentFolderDetail.value?.name ?? "";
  folderDescriptionDraft.value = currentFolderDetail.value?.description ?? "";
  folderRemarkDraft.value = (currentFolderDetail.value?.remark ?? "").trim();
  folderCoverUrlDraft.value = (currentFolderDetail.value?.cover_url ?? "").trim();
  folderDirectPrefixDraft.value = (currentFolderDetail.value?.direct_link_prefix ?? "").trim();
  folderCustomPathDraft.value = (currentFolderDetail.value?.custom_path ?? "").trim();
  folderDownloadPolicyDraft.value = currentFolderDetail.value?.download_policy ?? "inherit";
  folderHidePublicCatalogDraft.value = Boolean(currentFolderDetail.value?.hide_public_catalog);
  syncBodyScrollLock();
}

async function saveFolderDescription() {
  if (!currentFolderDetail.value || !folderEditorDirty.value) {
    return;
  }

  folderDescriptionSaving.value = true;
  folderDescriptionError.value = "";
  const d = currentFolderDetail.value;
  const isRoot = folderDetailIsManagingRoot(d);
  try {
    if (folderEditorMetaDirty.value) {
      await httpClient.request(`/admin/resources/folders/${encodeURIComponent(d.id)}`, {
        method: "PUT",
        body: {
          name: folderNameDraft.value.trim(),
          description: folderDescriptionDraft.value.trim(),
          remark: folderRemarkDraft.value.trim(),
          cover_url: folderCoverUrlDraft.value.trim(),
          direct_link_prefix: folderDirectPrefixDraft.value.trim(),
          custom_path: folderCustomPathDraft.value.trim(),
          download_policy: folderDownloadPolicyDraft.value,
        },
      });
    }
    if (folderEditorHideCatalogDirty.value && isRoot) {
      await httpClient.request(
        `/admin/resources/folders/${encodeURIComponent(d.id)}/catalog-visibility`,
        {
          method: "PUT",
          body: { hide_public_catalog: folderHidePublicCatalogDraft.value },
        },
      );
    }

    currentFolderDetail.value = {
      ...d,
      name: folderNameDraft.value.trim(),
      description: folderDescriptionDraft.value.trim(),
      remark: folderRemarkDraft.value.trim(),
      cover_url: folderCoverUrlDraft.value.trim(),
      direct_link_prefix: folderDirectPrefixDraft.value.trim(),
      custom_path: folderCustomPathDraft.value.trim(),
      download_policy: folderDownloadPolicyDraft.value,
      hide_public_catalog: isRoot ? folderHidePublicCatalogDraft.value : d.hide_public_catalog,
    };
    breadcrumbs.value = breadcrumbs.value.map((item, index) => (
      index === breadcrumbs.value.length - 1
        ? { ...item, name: folderNameDraft.value.trim() }
        : item
    ));
    folderDescriptionEditorOpen.value = false;
    syncBodyScrollLock();
    if (currentFolderDetail.value) {
      invalidateDirectoryViewCacheFolder(currentFolderDetail.value.id);
      invalidateDirectoryViewCacheFolder(currentFolderDetail.value.parent_id ?? "");
    }
    await loadDirectory({ force: true });
  } catch (err: unknown) {
    toastError(readApiError(err, "更新文件夹信息失败。"));
  } finally {
    folderDescriptionSaving.value = false;
  }
}

async function submitFeedback() {
  if (!feedbackTarget.value) {
    return;
  }
  if (!feedbackDescription.value.trim()) {
    toastError("请填写问题说明。");
    return;
  }

  feedbackSubmitting.value = true;
  feedbackMessage.value = "";
  feedbackError.value = "";
  try {
    const response = await httpClient.post<{ receipt_code: string }>("/public/feedback", {
      file_id: feedbackTarget.value.type === "file" ? feedbackTarget.value.id : "",
      folder_id: feedbackTarget.value.type === "folder" ? feedbackTarget.value.id : "",
      description: feedbackDescription.value.trim(),
    });
    toastSuccess(`反馈已提交，请保存回执码 ${response.receipt_code}。`);
    window.sessionStorage.setItem("openshare_receipt_code", response.receipt_code);
    currentReceiptCode.value = response.receipt_code;
    closeFeedbackModal();
    feedbackSuccessModalOpen.value = true;
    syncBodyScrollLock();
  } catch (err: unknown) {
    if (err instanceof HttpError && err.status === 400) {
      toastError("请填写问题说明。");
    } else if (err instanceof HttpError && err.status === 404) {
      toastError("目标不存在或已删除。");
    } else {
      toastError("提交反馈失败。");
    }
  } finally {
    feedbackSubmitting.value = false;
  }
}

/** 卡片副标题：仅展示单行备注（不含 Markdown） */
function cardRemarkPreview(remark: string): string {
  return String(remark ?? "")
    .replace(/\r\n/g, "\n")
    .replace(/\r/g, "\n")
    .split("\n")
    .map((line) => line.trim())
    .filter(Boolean)
    .join(" ")
    .trim();
}

function formatSize(size: number) {
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(2)} KB`;
  if (size < 1024 * 1024 * 1024) return `${(size / (1024 * 1024)).toFixed(2)} MB`;
  return `${(size / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

function formatDateTime(value: string) {
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hour12: false,
  }).format(new Date(value));
}

function parseSortTimeMs(raw: string | undefined) {
  if (raw == null || typeof raw !== "string" || !raw.trim()) {
    return 0;
  }
  const ms = Date.parse(raw);
  return Number.isFinite(ms) ? ms : 0;
}

function extractExtension(name: string) {
  const index = name.lastIndexOf(".");
  if (index <= 0 || index === name.length - 1) {
    return "";
  }
  return name.slice(index + 1).toLowerCase();
}

/** 后端等处可能存 `filepath.Ext` 形式（含前导 `.`），图标与排序按无前缀后缀解析 */
function normalizeFileExtension(ext: string | undefined | null): string {
  let s = String(ext ?? "").trim().toLowerCase();
  while (s.startsWith(".")) {
    s = s.slice(1);
  }
  return s;
}

function fileIconComponent(extension: string) {
  const ext = normalizeFileExtension(extension);
  if (["png", "jpg", "jpeg", "gif", "webp", "svg", "bmp", "ico"].includes(ext)) return FileImage;
  if (["mp4", "mov", "avi", "mkv", "webm", "m4v", "ogv"].includes(ext)) return FileVideo;
  if (["mp3", "wav", "flac", "aac", "m4a", "ogg"].includes(ext)) return FileAudio;
  if (["zip", "rar", "7z", "tar", "gz", "bz2", "xz"].includes(ext)) return FileArchive;
  if (["nc"].includes(ext)) return Database;
  if (["ncl"].includes(ext)) return FileCode2;
  if (["md", "markdown"].includes(ext)) return FilePenLine;
  if (ext === "pdf") return NotebookText;
  if (["doc", "docx", "ppt", "pptx"].includes(ext)) return NotebookText;
  if (["xls", "xlsx", "csv", "numbers"].includes(ext)) return FileSpreadsheet;
  if (
    ["js", "ts", "jsx", "tsx", "json", "html", "css", "go", "py", "java", "c", "cpp", "h", "hpp", "rs", "sh", "yaml", "yml", "toml", "xml"].includes(
      ext,
    )
  ) {
    return FileCode2;
  }
  if (["txt", "rtf"].includes(ext)) return FileText;
  return FileType2;
}

function compareRows(
  left: DirectoryRow,
  right: DirectoryRow,
  mode: "name" | "download" | "format" | "modified",
  direction: "asc" | "desc",
) {
  let result = 0;

  if (mode === "download") {
    if (left.downloadCount !== right.downloadCount) {
      result = left.downloadCount - right.downloadCount;
    } else {
      result = left.name.localeCompare(right.name, "zh-CN");
    }
  } else if (mode === "format") {
    const leftRank = formatSortRank(left);
    const rightRank = formatSortRank(right);
    if (leftRank !== rightRank) {
      result = leftRank - rightRank;
    } else {
      result = left.name.localeCompare(right.name, "zh-CN");
    }
  } else if (mode === "modified") {
    if (left.sortTimeMs !== right.sortTimeMs) {
      result = left.sortTimeMs - right.sortTimeMs;
    } else {
      result = left.name.localeCompare(right.name, "zh-CN");
    }
  } else {
    result = left.name.localeCompare(right.name, "zh-CN");
  }

  return direction === "asc" ? result : -result;
}

function formatSortRank(row: DirectoryRow) {
  if (row.kind === "folder") {
    return 0;
  }

  const extension = normalizeFileExtension(row.extension);
  if (extension === "pdf") {
    return 1;
  }
  if (["doc", "docx", "xls", "xlsx", "ppt", "pptx"].includes(extension)) {
    return 2;
  }
  return 3;
}

async function syncSessionReceiptCode() {
  try {
    const receiptCode = await ensureSessionReceiptCode();
    currentReceiptCode.value = receiptCode || readStoredReceiptCode();
    return currentReceiptCode.value;
  } catch {
    currentReceiptCode.value = readStoredReceiptCode();
    return currentReceiptCode.value;
  }
}
</script>

<template>
  <!-- 主页/目录浏览页：面包屑导航 + 文件夹信息描述 + 文件卡片/表格/列表视图 + 搜索 + 公告 + 管理员功能入口 -->
  <main class="app-container py-0 px-0 sm:py-4 sm:px-4 lg:py-8 lg:px-8">
    <div class="space-y-6">
      <div class="grid gap-6">
      <section class="order-1 min-w-0">
        <div class="panel overflow-hidden rounded-none sm:rounded-2xl lg:rounded-2xl">
          <!-- 面包屑导航栏：主页 > 父文件夹 > 当前文件夹 -->
          <div class="border-b border-slate-200 px-4 py-3 sm:px-6 dark:border-slate-800">
            <div class="flex flex-wrap items-center justify-between gap-3">
              <div class="min-w-0 max-w-full overflow-x-auto">
                <div class="flex min-w-max items-center gap-2 text-sm text-slate-500 dark:text-slate-400">
                <button type="button" class="inline-flex items-center gap-2 rounded-full px-2 py-1 transition hover:bg-slate-100 hover:text-slate-900" @click="openRoot">
                  <Home class="h-4 w-4" />
                  <span>主页</span>
                </button>
                <template v-for="item in breadcrumbs" :key="item.id">
                  <ChevronRight class="h-4 w-4 text-slate-300" />
                  <button
                    type="button"
                    class="rounded-full px-2 py-1 transition hover:bg-slate-100 hover:text-slate-900"
                    @click="openFolder(item.id)"
                  >
                    {{ item.name }}
                  </button>
                </template>
                </div>
              </div>

            </div>
          </div>

          <!-- 当前文件夹详情头部：名称、统计信息、操作按钮（编辑/创建文件夹/删除/重新扫描/反馈/下载） -->
          <div v-if="currentFolderDetail" class="border-b border-slate-200 px-4 py-5 sm:px-6 dark:border-slate-800">
            <section>
              <div class="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
                <div class="min-w-0 flex-1 space-y-3">
                  <p class="break-words text-2xl font-semibold leading-snug tracking-tight text-sky-700 sm:text-2xl dark:text-sky-400">
                    {{ currentFolderDetail.name }}
                  </p>
                  <div class="flex flex-wrap items-center gap-x-8 gap-y-3 text-sm text-slate-500">
                    <div
                      v-for="item in currentFolderStats"
                      :key="item.label"
                      class="inline-flex items-center gap-2"
                    >
                      <span>{{ item.label }}</span>
                      <span class="font-medium text-slate-900">{{ item.value }}</span>
                    </div>
                  </div>
                </div>
                <div class="flex flex-wrap items-start gap-3">
                  <button
                    v-if="canManageResourceDescriptions"
                    type="button"
                    class="btn-secondary"
                    @click="openFolderDescriptionEditor"
                  >
                    编辑
                  </button>
                  <button
                    v-if="canManageResourceDescriptions && !isCurrentFolderVirtual"
                    type="button"
                    class="btn-secondary"
                    @click="openCreateFolderModal"
                  >
                    创建文件夹
                  </button>
                  <button
                    v-if="canManageResourceDescriptions"
                    type="button"
                    class="btn-secondary"
                    @click="openCreateVirtualFolderModal"
                  >
                    创建虚拟目录
                  </button>
                  <!-- 虚拟目录下的文件添加按钮 -->
                  <button
                    v-if="canManageResourceDescriptions && isCurrentFolderVirtual"
                    type="button"
                    class="btn-secondary"
                    @click="openAddVirtualFileModal"
                  >
                    添加虚拟文件
                  </button>
                  <button
                    v-if="canManageResourceDescriptions"
                    type="button"
                    class="btn-secondary text-rose-600 hover:border-rose-200 hover:bg-rose-50 hover:text-rose-700"
                    @click="openDeleteFolderDialog"
                  >
                    删除
                  </button>
                  <button
                    v-if="showRescanCurrentManagedFolder"
                    type="button"
                    class="btn-secondary disabled:cursor-not-allowed disabled:opacity-60"
                    :disabled="Boolean(rescanningManagedFolderID)"
                    @click="rescanCurrentManagedFolder"
                  >
                    {{
                      rescanningManagedFolderID === currentFolderDetail.id ? "扫描中…" : "重新扫描"
                    }}
                  </button>
                  <button
                    type="button"
                    class="inline-flex h-11 w-11 items-center justify-center rounded-xl border border-slate-200 bg-white text-slate-500 transition-[transform,background-color,border-color,box-shadow,color] duration-200 hover:-translate-y-0.5 hover:border-slate-300 hover:bg-[#fafafa] hover:text-slate-900 hover:shadow-sm hover:shadow-slate-950/[0.08]"
                    aria-label="反馈文件夹"
                    @click="openFeedbackModal({ id: currentFolderDetail.id, type: 'folder', name: currentFolderDetail.name })"
                  >
                    <Flag class="h-4 w-4" />
                  </button>
                  <!-- 自定义路径复制按钮：仅当该文件夹设置了 custom_path 时显示 -->
                  <button
                    v-if="currentFolderDetail.custom_path"
                    type="button"
                    class="inline-flex h-11 w-11 items-center justify-center rounded-xl border border-slate-200 bg-white text-slate-600 transition-[transform,background-color,border-color,box-shadow,color] duration-200 hover:-translate-y-0.5 hover:border-slate-300 hover:bg-[#fafafa] hover:text-sky-600 hover:shadow-sm hover:shadow-slate-950/[0.08]"
                    :title="`复制自定义路径：/${currentFolderDetail.custom_path}`"
                    aria-label="复制自定义路径链接"
                    @click="copyFolderCustomPathUrl"
                  >
                    <Link2 class="h-4 w-4" />
                  </button>
                  <button
                    v-if="currentFolderDetail.download_allowed !== false && !isCurrentFolderVirtual"
                    type="button"
                    class="inline-flex h-11 w-11 items-center justify-center rounded-xl border border-slate-200 bg-white text-slate-700 transition-[transform,background-color,border-color,box-shadow,color] duration-200 hover:-translate-y-0.5 hover:border-slate-300 hover:bg-[#fafafa] hover:text-slate-900 hover:shadow-sm hover:shadow-slate-950/[0.08]"
                    aria-label="下载文件夹"
                    @click="downloadCurrentFolder"
                  >
                    <Download class="h-4 w-4" />
                  </button>
                </div>
              </div>

            </section>
          </div>

          <div :class="useWideDescriptionLayout ? 'xl:flex xl:gap-0 xl:pr-6 xl:max-h-[calc(100vh-13rem)]' : ''">
            <template v-if="currentFolderDetail">
              <div
                :class="useWideDescriptionLayout
                  ? 'xl:w-[40%] xl:shrink-0 xl:overflow-y-auto xl:border-none xl:border-slate-200 xl:pr-0 xl:py-0'
                  : 'border-b border-slate-200 px-0 py-0'"
              >
                <div v-if="currentFolderDescriptionHTML" :class="useWideDescriptionLayout
                ? 'rounded-3xl border-slate-200 bg-white px-4 py-4 rounded-none sm:border sm:rounded-2xl sm:mx-5 sm:mt-5 sm:px-5 sm:py-5 xl:border-b xl:rounded-none xl:mx-0 xl:mt-0 dark:border-slate-800 dark:bg-slate-900/40'
                : 'rounded-3xl border-slate-200 bg-white px-4 py-4 rounded-none sm:border sm:rounded-2xl sm:mx-6 sm:my-6 sm:px-5 sm:py-5 xl:border-b xl:mx-6 xl:my-6 dark:border-slate-800 dark:bg-slate-900/40'"
                >
                  <div class="space-y-3">
                    <div class="relative">
                      <div
                        ref="folderMarkdownClampRef"
                        class="markdown-content"
                        :class="!folderMarkdownExpanded ? 'max-h-[min(21vh,10rem)] overflow-hidden' : ''"
                        v-html="currentFolderDescriptionHTML"
                        @click.capture="handleMarkdownInternalLinkNavigate"
                      />
                      <div
                        v-if="!folderMarkdownExpanded && folderMarkdownFooterVisible"
                        class="pointer-events-none absolute bottom-0 left-0 right-0 h-14 bg-gradient-to-t from-white to-transparent dark:from-slate-900"
                        aria-hidden="true"
                      />
                    </div>
                    <div v-if="folderMarkdownFooterVisible" class="flex justify-center sm:justify-start">
                      <button
                        type="button"
                        class="inline-flex min-h-10 items-center justify-center rounded-xl border border-slate-200 bg-white px-4 text-sm font-medium text-slate-800 shadow-sm ring-1 ring-slate-950/[0.04] transition hover:border-slate-300 hover:bg-slate-50 dark:border-slate-600 dark:bg-slate-800 dark:text-slate-100 dark:ring-white/[0.06] dark:hover:border-slate-500 dark:hover:bg-slate-800/90"
                        @click="folderMarkdownExpanded = !folderMarkdownExpanded"
                      >
                        {{ folderMarkdownExpanded ? "收起简介" : "展开全文" }}
                      </button>
                    </div>
                  </div>
                </div>
              </div>
            </template>

            <div :class="useWideDescriptionLayout ? 'xl:min-w-0 xl:flex-1 xl:overflow-y-auto xl:py-3' : ''">

          <!-- 搜索栏 + 标签过滤器 + 视图切换 + 操作按钮区 -->
          <div>
            <SearchSection
              v-model="searchInput"
              embedded
              :loading="searchLoading"
              @search="runSearch"
              @clear="clearSearchState"
            />
          </div>
<div
            v-if="searchKeyword"
            class="mx-5 mt-3 rounded-xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-600 sm:mx-6"
          >
            当前搜索：<span class="font-medium text-slate-900">{{ searchKeyword }}</span>
            <span class="ml-2">共 {{ searchRows.length }} 条结果</span>
          </div>

          <div class="px-4 pb-2 sm:px-6">
            <div class="flex flex-wrap items-center gap-3 border-t border-slate-100 pt-3">
              <button
                type="button"
                class="inline-flex items-center gap-2 rounded-xl border border-slate-200 px-3 py-2 text-sm font-medium text-slate-600 transition hover:border-slate-300 hover:text-slate-900 disabled:cursor-not-allowed disabled:opacity-45"
                :disabled="!canUseBackButton"
                @click="goUpOneLevel"
              >
                <ChevronLeft class="h-4 w-4" />
                {{ backButtonLabel }}
              </button>

              <!-- 普通目录上传按钮（虚拟目录不显示） -->
              <button
                v-if="!isCurrentFolderVirtual"
                type="button"
                class="inline-flex items-center gap-2 rounded-xl border border-slate-200 px-3 py-2 text-sm font-medium text-slate-600 transition hover:border-slate-300 hover:text-slate-900"
                :disabled="!canUploadToCurrentFolder"
                :class="!canUploadToCurrentFolder ? 'cursor-not-allowed opacity-45 hover:border-slate-200 hover:text-slate-600' : ''"
                @click="openUpload"
              >
                <Upload class="h-4 w-4" />
                {{ canUploadToCurrentFolder ? "在该目录上传" : "进入目录后上传" }}
              </button>

              <button
                v-if="viewMode === 'cards' && sortedRows.length > 0"
                type="button"
                class="inline-flex items-center gap-2 rounded-xl border px-3 py-2 text-sm font-medium transition hover:border-slate-300 hover:text-slate-900"
                :class="
                  cardMultiSelectMode
                    ? 'border-blue-200 bg-blue-50/90 text-blue-900 dark:border-blue-800 dark:bg-blue-950/50 dark:text-blue-100'
                    : 'border-slate-200 text-slate-600 dark:border-slate-600 dark:text-slate-300'
                "
                @click="toggleCardMultiSelectMode"
              >
                {{ cardMultiSelectMode ? "完成" : "多选" }}
              </button>

              <button
                v-if="sortedRows.length > 0 && (viewMode === 'table' || cardMultiSelectMode)"
                type="button"
                class="inline-flex items-center gap-2 rounded-xl border border-slate-200 px-3 py-2 text-sm font-medium text-slate-600 transition hover:border-slate-300 hover:text-slate-900 dark:border-slate-600 dark:text-slate-300"
                @click="toggleSelectAllVisibleRows"
              >
                {{ allVisibleRowsSelected ? "取消全选" : "全选" }}
              </button>

              <div ref="toolbarDropdownsRef" class="flex w-full flex-wrap items-center gap-3 sm:ml-auto sm:w-auto sm:justify-end">
              <div class="relative">
                <button
                  type="button"
                  class="inline-flex w-full items-center justify-center gap-2 rounded-xl border border-slate-200 px-3 py-2 text-sm font-medium text-slate-600 transition hover:border-slate-300 hover:text-slate-900 sm:w-auto"
                  @click="sortMenuOpen = !sortMenuOpen; viewMenuOpen = false"
                >
                  {{ sortModeLabel(sortMode) }} · {{ sortDirectionLabel(sortDirection) }}
                  <ChevronRight class="h-4 w-4 rotate-90" />
                </button>
                <div v-if="sortMenuOpen" class="absolute left-0 top-full z-20 mt-2 min-w-[176px] rounded-2xl border border-slate-200 bg-white p-1 shadow-lg">
                  <button
                    type="button"
                    class="block w-full rounded-xl px-3 py-2 text-left text-sm transition"
                    :class="sortMode === 'download' ? 'bg-slate-100 font-medium text-slate-900' : 'text-slate-600 hover:bg-slate-50 hover:text-slate-900'"
                    @click="setSortMode('download')"
                  >
                    下载量排序
                  </button>
                  <button
                    type="button"
                    class="block w-full rounded-xl px-3 py-2 text-left text-sm transition"
                    :class="sortMode === 'name' ? 'bg-slate-100 font-medium text-slate-900' : 'text-slate-600 hover:bg-slate-50 hover:text-slate-900'"
                    @click="setSortMode('name')"
                  >
                    名称排序
                  </button>
                  <button
                    type="button"
                    class="block w-full rounded-xl px-3 py-2 text-left text-sm transition"
                    :class="sortMode === 'format' ? 'bg-slate-100 font-medium text-slate-900' : 'text-slate-600 hover:bg-slate-50 hover:text-slate-900'"
                    @click="setSortMode('format')"
                  >
                    格式排序
                  </button>
                  <button
                    type="button"
                    class="block w-full rounded-xl px-3 py-2 text-left text-sm transition"
                    :class="sortMode === 'modified' ? 'bg-slate-100 font-medium text-slate-900' : 'text-slate-600 hover:bg-slate-50 hover:text-slate-900'"
                    @click="setSortMode('modified')"
                  >
                    修改日期排序
                  </button>
                  <div class="mx-2 my-1 border-t border-slate-100"></div>
                  <button
                    type="button"
                    class="block w-full rounded-xl px-3 py-2 text-left text-sm transition"
                    :class="sortDirection === 'desc' ? 'bg-slate-100 font-medium text-slate-900' : 'text-slate-600 hover:bg-slate-50 hover:text-slate-900'"
                    @click="setSortDirection('desc')"
                  >
                    降序
                  </button>
                  <button
                    type="button"
                    class="block w-full rounded-xl px-3 py-2 text-left text-sm transition"
                    :class="sortDirection === 'asc' ? 'bg-slate-100 font-medium text-slate-900' : 'text-slate-600 hover:bg-slate-50 hover:text-slate-900'"
                    @click="setSortDirection('asc')"
                  >
                    升序
                  </button>
                </div>
              </div>

              <div class="relative">
                <button
                  type="button"
                  class="inline-flex w-full items-center justify-center gap-2 rounded-xl border border-slate-200 px-3 py-2 text-sm font-medium text-slate-600 transition hover:border-slate-300 hover:text-slate-900 sm:w-auto"
                  @click="viewMenuOpen = !viewMenuOpen; sortMenuOpen = false"
                >
                  <LayoutGrid v-if="viewMode === 'cards'" class="h-4 w-4" />
                  <List v-else class="h-4 w-4" />
                  {{ viewModeLabel(viewMode) }}
                  <ChevronRight class="h-4 w-4 rotate-90" />
                </button>
                <div v-if="viewMenuOpen" class="absolute left-0 top-full z-20 mt-2 min-w-[200px] rounded-2xl border border-slate-200 bg-white p-1 shadow-lg">
                  <button
                    type="button"
                    class="flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm transition"
                    :class="viewMode === 'cards' ? 'bg-slate-100 font-medium text-slate-900' : 'text-slate-600 hover:bg-slate-50 hover:text-slate-900'"
                    @click="setViewMode('cards')"
                  >
                    <LayoutGrid class="h-4 w-4" />
                    卡片
                  </button>
                  <button
                    type="button"
                    class="flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm transition"
                    :class="viewMode === 'table' ? 'bg-slate-100 font-medium text-slate-900' : 'text-slate-600 hover:bg-slate-50 hover:text-slate-900'"
                    @click="setViewMode('table')"
                  >
                    <List class="h-4 w-4" />
                    表格
                  </button>
                  <div class="mx-2 my-1 border-t border-slate-100"></div>
                  <button
                    type="button"
                    class="flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm transition"
                    :class="cardCoverFirst ? 'bg-slate-100 font-medium text-slate-900' : 'text-slate-600 hover:bg-slate-50 hover:text-slate-900'"
                    @click="toggleCardCoverFirst"
                  >
                    <span class="flex-1">封面卡片优先</span>
                    <Check v-if="cardCoverFirst" class="h-4 w-4 shrink-0 text-slate-700" />
                  </button>
                </div>
              </div>
              </div>
            </div>
          </div>

          <div
            v-if="!searchKeyword && currentFolderFileTags.length > 0"
            class="mx-4 mt-3 flex flex-wrap items-center gap-2 sm:mx-6"
          >
            <button
              type="button"
              class="rounded-lg px-2.5 py-1 text-xs font-medium ring-1 transition"
              :class="!hasActiveTagFilter ? 'bg-slate-800 text-white ring-slate-800' : 'bg-white text-slate-600 ring-slate-200 hover:bg-slate-100'"
              @click="clearTagFilter"
            >
              全部
            </button>
            <button
              v-for="tag in currentFolderFileTags"
              :key="tag.id"
              type="button"
              class="max-w-full shrink-0 truncate rounded-lg px-2.5 py-1 text-xs font-medium ring-1 transition hover:opacity-85"
              :class="selectedTagIds.has(tag.id) ? 'ring-2 ring-offset-1' : ''"
              :style="{
                backgroundColor: tag.color,
                color: readableTextColorForPreset(tag.color),
                ringColor: tag.color,
              }"
              :title="tag.name"
              @click="toggleTagFilter(tag.id)"
            >
              {{ tag.name }}
            </button>
          </div>

          <p v-if="actionMessage" class="mx-4 mt-5 rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700 sm:mx-6">{{ actionMessage }}</p>
<div v-if="loading" class="px-4 py-8 text-sm text-slate-500 sm:px-6">加载中…</div>
          <div v-else-if="error" class="px-4 py-8 text-sm text-rose-600 sm:px-6">{{ error }}</div>
          <div v-else-if="sortedRows.length === 0" class="px-4 py-8 text-sm text-slate-500 sm:px-6">
            {{ searchKeyword ? "没有找到匹配结果。" : "当前目录为空。" }}
          </div>
          <div
            v-else-if="viewMode === 'cards'"
            class="space-y-8 px-4 py-3 sm:px-5"
          >
            <div v-for="block in cardDisplayBlocks" :key="block.key">
              <!-- 卡片视图：大图封面卡片布局，有封面的排前面 -->
              <div class="public-home-card-grid gap-4 md:gap-5">
            <article
              v-for="row in block.rows"
              :key="`${row.kind}-${row.id}`"
              class="group relative min-w-0 flex cursor-pointer flex-col overflow-hidden rounded-3xl border transition hover:shadow-sm"
              :class="[
                row.coverUrl ? 'min-h-0' : 'min-h-[155px] px-2.5 pt-2.5 sm:px-2.5',
                row.kind === 'folder' ? 'border-slate-200 bg-sky-50/50 hover:border-sky-500 hover:bg-sky-50' : 'border-slate-200 bg-white hover:border-slate-300 hover:bg-slate-100',
              ]"
              @click="onCardOpenClick(row)"
            >
              <template v-if="row.coverUrl">
                <div class="relative aspect-[16/10] min-h-[132px] w-full max-h-[220px] shrink-0 overflow-hidden bg-slate-100 sm:min-h-[148px] sm:max-h-[240px]">
                  <img
                    :src="row.coverUrl"
                    :alt="`封面 ${row.name}`"
                    class="absolute inset-0 h-full w-full object-cover"
                    loading="lazy"
                  />
                  <div
                    v-if="cardMultiSelectMode"
                    class="absolute right-3 top-3 z-10 rounded-lg bg-white/90 p-0.5 shadow-sm ring-1 ring-slate-200/80 backdrop-blur-sm"
                  >
                    <input
                      :checked="isRowSelected(row)"
                      type="checkbox"
                      class="h-5 w-5 rounded-lg border-slate-300 text-slate-900 focus:ring-slate-300"
                      @click.stop
                      @change="toggleRowSelection(row)"
                    />
                  </div>
                </div>
                <div class="flex min-h-0 flex-1 flex-col px-2.5 pb-2.5 pt-3 sm:px-2.5">
                  <h3 class="line-clamp-2 text-base font-semibold leading-snug" :class="row.kind === 'folder' ? 'text-sky-900' : 'text-slate-900'">{{ row.name }}</h3>
                  <p
                    v-if="row.kind === 'folder' && cardRemarkPreview(row.remark)"
                    class="mt-1 line-clamp-2 text-sm leading-5 text-slate-500"
                  >
                    {{ cardRemarkPreview(row.remark) }}
                  </p>
                  <FileTagChips
                    v-if="row.kind === 'file' && row.tags.length > 0"
                    :tags="row.tags"
                    class="mt-2"
                  />
                  <div
                    class="mt-3 flex w-full min-w-0 text-xs"
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
                  <div class="mt-2 flex items-center justify-between gap-2 border-t border-slate-100 pt-3">
                    <button
                      type="button"
                      :class="['inline-flex items-center justify-center rounded-xl border p-2.5 transition', row.kind === 'folder' ? 'border-sky-200 bg-sky-50/50 text-sky-700 hover:border-sky-300 hover:bg-sky-100 hover:text-sky-800' : 'border-slate-200 bg-white text-slate-700 hover:border-slate-300 hover:bg-slate-50 hover:text-slate-900']"
                      @click.stop="openFeedbackModal({ id: row.id, type: row.kind, name: row.name })"
                    >
                      <Flag class="h-4 w-4" />
                    </button>
                    <div class="flex items-center gap-2">
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
                        v-if="row.kind === 'file'"
                        type="button"
                        title="右侧打开预览"
                        class="hidden xl:inline-flex items-center justify-center rounded-xl border border-slate-200 bg-white p-2.5 text-slate-700 transition hover:border-slate-300 hover:bg-slate-50 hover:text-slate-900"
                        aria-label="右侧打开预览"
                        @click.stop="openFileDetailInSidePanel(row.id)"
                      >
                        <PanelRightOpen class="h-4 w-4" />
                      </button>
                      <button
                        v-if="row.downloadAllowed"
                        type="button"
                        :class="['inline-flex items-center justify-center rounded-xl border p-2.5 transition', row.kind === 'folder' ? 'border-sky-200 bg-sky-50/50 text-sky-700 hover:border-sky-300 hover:bg-sky-100 hover:text-sky-800' : 'border-slate-200 bg-white text-slate-700 hover:border-slate-300 hover:bg-slate-50 hover:text-slate-900']"
                        @click.stop="downloadResource(row)"
                      >
                        <Download class="h-4 w-4" />
                      </button>
                      <!-- 虚拟目录下的文件删除按钮（仅管理员） -->
                      <button
                        v-if="canManageResourceDescriptions && isCurrentFolderVirtual && row.kind === 'file'"
                        type="button"
                        class="inline-flex items-center justify-center rounded-xl border border-rose-200 bg-white p-2.5 text-rose-600 transition hover:border-rose-300 hover:bg-rose-50 hover:text-rose-700"
                        aria-label="删除文件"
                        @click.stop="openDeleteFileDialog(row)"
                      >
                        <Trash2 class="h-4 w-4" />
                      </button>
                    </div>
                  </div>
                </div>
              </template>
              <template v-else>
                <div v-if="cardMultiSelectMode" class="absolute right-5 top-4 z-10">
                  <input
                    :checked="isRowSelected(row)"
                    type="checkbox"
                    class="h-5 w-5 rounded-lg border-slate-300 text-slate-900 focus:ring-slate-300"
                    @click.stop
                    @change="toggleRowSelection(row)"
                  />
                </div>
                <div class="flex items-start gap-2.5 sm:gap-2.5">
                  <div
                    :class="[
                      'flex h-12 w-12 shrink-0 overflow-hidden rounded-2xl',
                      row.coverUrl ? '' : 'items-center justify-center',
                      row.kind === 'folder' ? 'bg-sky-50/50 text-sky-500' : 'bg-slate-100 text-slate-500',
                    ]"
                  >
                    <img
                      v-if="row.coverUrl"
                      :src="row.coverUrl"
                      :alt="`封面 ${row.name}`"
                      class="h-full w-full object-cover"
                      loading="lazy"
                    />
                    <Folder v-else-if="row.kind === 'folder'" class="h-6 w-6 text-sky-500" />
                    <component v-else :is="fileIconComponent(row.extension)" class="h-6 w-6" />
                  </div>
                  <div
                    class="min-w-0 flex-1 pt-0.5"
                    :class="cardMultiSelectMode ? 'pr-9 sm:pr-10' : 'pr-0'"
                  >
                    <h3
                      class="line-clamp-2 break-words text-base font-semibold leading-snug [overflow-wrap:anywhere]" :class="row.kind === 'folder' ? 'text-sky-900' : 'text-slate-900'"
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
                    <FileTagChips
                      v-if="row.kind === 'file' && row.tags.length > 0"
                      :tags="row.tags"
                      class="mt-2"
                    />
                <div
                  class="mt-3 flex w-full min-w-0 text-xs"
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

                <div class="mt-2 flex items-center justify-between gap-2 border-t border-slate-100 py-2.5">
                  <button
                    type="button"
                    :class="['inline-flex items-center justify-center rounded-xl border p-2.5 transition', row.kind === 'folder' ? 'border-sky-200 bg-sky-50/50 text-sky-700 hover:border-sky-300 hover:bg-sky-100 hover:text-sky-800' : 'border-slate-200 bg-white text-slate-700 hover:border-slate-300 hover:bg-slate-50 hover:text-slate-900']"
                    @click.stop="openFeedbackModal({ id: row.id, type: row.kind, name: row.name })"
                  >
                    <Flag class="h-4 w-4" />
                  </button>
                  <div class="flex items-center gap-2">
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
                      v-if="row.kind === 'file'"
                      type="button"
                      title="右侧打开预览"
                      class="hidden xl:inline-flex items-center justify-center rounded-xl border border-slate-200 bg-white p-2.5 text-slate-700 transition hover:border-slate-300 hover:bg-slate-50 hover:text-slate-900"
                      aria-label="右侧打开预览"
                      @click.stop="openFileDetailInSidePanel(row.id)"
                    >
                      <PanelRightOpen class="h-4 w-4" />
                    </button>
                    <button
                      v-if="row.downloadAllowed"
                      type="button"
                      :class="['inline-flex items-center justify-center rounded-xl border p-2.5 transition', row.kind === 'folder' ? 'border-sky-200 bg-sky-50/50 text-sky-700 hover:border-sky-300 hover:bg-sky-100 hover:text-sky-800' : 'border-slate-200 bg-white text-slate-700 hover:border-slate-300 hover:bg-slate-50 hover:text-slate-900']"
                      @click.stop="downloadResource(row)"
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
          <div v-else class="px-4 py-5 sm:px-6">
            <!-- 表格视图：紧凑行展示文件/文件夹信息 -->
            <table class="data-table table-fixed">
              <thead>
                <tr>
                  <th class="w-10"></th>
                  <th class="text-left">名称</th>
                  <th class="w-[120px] text-right">大小</th>
                  <th class="hidden w-[220px] text-right xl:table-cell">修改时间</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="row in sortedRows"
                  :key="`${row.kind}-${row.id}`"
                  class="cursor-pointer transition hover:bg-slate-50 dark:hover:bg-slate-800/40"
                  @click="row.kind === 'folder' ? openFolder(row.id) : openFile(row.id)"
                >
                  <td @click.stop>
                    <input
                      :checked="isRowSelected(row)"
                      type="checkbox"
                      class="h-5 w-5 rounded-lg border-slate-300 text-slate-900 focus:ring-slate-300"
                      @change="toggleRowSelection(row)"
                    />
                  </td>
                  <td>
                    <div
                      v-if="row.kind === 'folder'"
                      class="flex min-w-0 items-start gap-3 text-left"
                    >
                      <img
                        v-if="row.coverUrl"
                        :src="row.coverUrl"
                        alt=""
                        class="mt-0.5 h-5 w-5 shrink-0 rounded object-cover"
                        loading="lazy"
                      />
                      <Folder v-else class="mt-0.5 h-5 w-5 shrink-0 text-sky-500" />
                      <div class="min-w-0 flex-1">
                        <span
                          class="block truncate text-base font-medium leading-snug text-sky-900 dark:text-sky-400"
                          :title="row.name"
                        >{{ row.name }}</span>
                        <p
                          v-if="cardRemarkPreview(row.remark)"
                          class="mt-0.5 truncate text-xs leading-snug text-slate-500 dark:text-slate-400"
                          :title="cardRemarkPreview(row.remark)"
                        >
                          {{ cardRemarkPreview(row.remark) }}
                        </p>
                      </div>
                    </div>
                    <div
                      v-else
                      class="flex min-w-0 items-start gap-3 text-left"
                    >
                      <component
                        :is="fileIconComponent(row.extension)"
                        class="mt-0.5 h-5 w-5 shrink-0 text-slate-500 dark:text-slate-400"
                      />
                      <div class="min-w-0 flex-1">
                        <span
                          class="block truncate text-base font-medium leading-snug text-slate-900 dark:text-slate-100"
                          :title="row.name"
                        >{{ row.name }}</span>
                        <p
                          v-if="cardRemarkPreview(row.remark)"
                          class="mt-0.5 truncate text-xs leading-snug text-slate-500 dark:text-slate-400"
                          :title="cardRemarkPreview(row.remark)"
                        >
                          {{ cardRemarkPreview(row.remark) }}
                        </p>
                        <FileTagChips
                          v-if="row.tags.length > 0"
                          :tags="row.tags"
                          class="mt-1.5"
                          size="sm"
                        />
                      </div>
                    </div>
                  </td>
                  <td class="w-[120px] whitespace-nowrap text-right tabular-nums">{{ row.sizeText }}</td>
                  <td class="hidden w-[220px] whitespace-nowrap text-right tabular-nums xl:table-cell">{{ row.updatedAt }}</td>
                </tr>
              </tbody>
            </table>
          </div>

        </div>
          </div>
        </div>
      </section>
      </div>
    </div>
  </main>

  <!-- 侧滑面板：文件详情（在当前页右侧滑入展示，不跳转） -->
  <Teleport to="body">
    <Transition name="file-detail-drawer-shell">
      <div
        v-if="fileDetailPanelFileId"
        class="fixed inset-0 z-[118]"
      >
        <div
          class="absolute inset-0 bg-slate-950/40 backdrop-blur-[1px]"
          aria-hidden="true"
          @click="closeFileDetailPanel"
        />
        <div
          role="dialog"
          aria-modal="true"
          aria-label="文件详情预览"
          class="file-detail-drawer-panel absolute right-0 top-0 flex h-full w-[min(100vw,50rem)] min-w-0 flex-col overflow-hidden border-l border-slate-200 bg-[#fafafa] shadow-[0_0_0_1px_rgba(15,23,42,0.06),-12px_0_48px_-24px_rgba(15,23,42,0.25)] dark:border-slate-800 dark:bg-slate-950"
          @click.stop
        >
          <PublicFileDetailView
            class="flex-1 min-h-0 overflow-x-hidden overflow-y-auto"
            :override-file-id="fileDetailPanelFileId"
            panel-presentation
            @close-panel="closeFileDetailPanel"
            @open-full-page="onFileDetailPanelOpenFullPage"
            @navigate-panel-file="onFileDetailPanelNavigate"
            @leave-to-public-catalog="onFileDetailPanelLeaveCatalog"
          />
        </div>
      </div>
    </Transition>
  </Teleport>

  <!-- 弹窗：Markdown 内链跳转确认（从简介中的站内链接跳转前询问） -->
  <Teleport to="body">
    <Transition name="modal-shell">
      <div
        v-if="markdownCatalogNavigateConfirmRoute"
        class="fixed inset-0 z-[126] flex items-center justify-center bg-slate-950/35 px-4 backdrop-blur-[1px]"
        @click.self="dismissMarkdownCatalogNavigateConfirm(false)"
      >
        <div
          class="modal-card panel w-full max-w-md rounded-2xl bg-white p-6 shadow-xl dark:bg-slate-900"
          role="alertdialog"
          aria-modal="true"
          aria-labelledby="home-md-catalog-confirm-title"
          @click.stop
        >
          <h3 id="home-md-catalog-confirm-title" class="text-lg font-semibold text-slate-900 dark:text-slate-100">
            前往文件夹浏览
          </h3>
          <template v-if="markdownCatalogNavigatePresentation">
            <div
              v-if="
                markdownCatalogNavigatePresentation.variant !== 'folder'
                  || markdownCatalogNavigatePresentation.loading
              "
              class="mt-3 space-y-2"
            >
              <p
                class="text-base font-semibold leading-snug text-slate-900 dark:text-slate-50 sm:text-lg sm:leading-snug"
              >
                {{ markdownCatalogNavigatePresentation.headline }}
              </p>
              <p
                v-if="markdownCatalogNavigatePresentation.detail"
                class="text-sm leading-6 text-slate-600 dark:text-slate-300"
              >
                {{ markdownCatalogNavigatePresentation.detail }}
              </p>
              <p
                v-if="markdownCatalogNavigatePresentation.loading"
                class="text-xs leading-5 text-slate-500 dark:text-slate-400"
              >
                正在向服务器查询文件夹名称、路径与简介等信息……
              </p>
            </div>
            <div v-else class="mt-3 max-h-[min(62vh,28rem)] space-y-3 overflow-y-auto pr-1 text-sm leading-relaxed">
              <div>
                <p class="text-xs font-semibold uppercase tracking-[0.12em] text-slate-500 dark:text-slate-400">
                  文件夹名
                </p>
                <p class="mt-1 font-semibold text-slate-900 dark:text-slate-100">
                  {{ markdownCatalogNavigatePresentation.headline }}
                </p>
              </div>
              <div v-if="markdownCatalogNavigatePresentation.detail">
                <p class="text-xs font-semibold uppercase tracking-[0.12em] text-slate-500 dark:text-slate-400">
                  路径
                </p>
                <p class="mt-1 text-slate-700 dark:text-slate-300">
                  {{ markdownCatalogNavigatePresentation.detail }}
                </p>
              </div>
              <div v-if="markdownCatalogNavigatePresentation.remark">
                <p class="text-xs font-semibold uppercase tracking-[0.12em] text-slate-500 dark:text-slate-400">
                  备注
                </p>
                <p class="mt-1 whitespace-pre-wrap text-slate-700 dark:text-slate-300">
                  {{ markdownCatalogNavigatePresentation.remark }}
                </p>
              </div>
              <div v-if="markdownCatalogNavigatePresentation.filesSummary">
                <p class="text-xs font-semibold uppercase tracking-[0.12em] text-slate-500 dark:text-slate-400">
                  内容与大小
                </p>
                <p class="mt-1 tabular-nums text-slate-700 dark:text-slate-300">
                  {{ markdownCatalogNavigatePresentation.filesSummary }}
                </p>
              </div>
              <div v-if="markdownCatalogNavigatePresentation.descriptionHtml">
                <p class="text-xs font-semibold uppercase tracking-[0.12em] text-slate-500 dark:text-slate-400">
                  简介
                </p>
                <div
                  class="markdown-content mt-2 rounded-xl border border-slate-100 bg-slate-50/80 px-3 py-2.5 dark:border-slate-700 dark:bg-slate-800/50"
                  v-html="markdownCatalogNavigatePresentation.descriptionHtml"
                />
              </div>
            </div>
            <p class="mt-4 text-sm text-slate-500 dark:text-slate-400">
              将把当前首页切换到上方所示文件夹的资料列表，确定吗？
            </p>
          </template>
          <div class="mt-6 flex flex-wrap justify-end gap-3">
            <button type="button" class="btn-secondary" @click="dismissMarkdownCatalogNavigateConfirm(false)">
              取消
            </button>
            <button type="button" class="btn-primary" @click="dismissMarkdownCatalogNavigateConfirm(true)">
              前往
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>

  <!-- 底部批量操作栏：多选文件/文件夹后显示，支持批量下载 -->
  <Teleport to="body">
    <Transition
      enter-active-class="transition duration-300 ease-out"
      enter-from-class="translate-y-6 opacity-0"
      enter-to-class="translate-y-0 opacity-100"
      leave-active-class="transition duration-200 ease-in"
      leave-from-class="translate-y-0 opacity-100"
      leave-to-class="translate-y-4 opacity-0"
    >
      <div
        v-if="hasSelectedRows"
        class="pointer-events-none fixed inset-x-0 bottom-6 z-[130] flex justify-center px-4"
      >
        <div class="pointer-events-auto flex w-full max-w-3xl flex-col gap-3 rounded-3xl border border-slate-200 bg-white px-4 py-4 shadow-[0_0_0_1px_rgba(15,23,42,0.06),0_22px_60px_-18px_rgba(15,23,42,0.34)] sm:flex-row sm:items-center sm:justify-between sm:px-6">
          <p class="text-sm text-slate-600">
            已选 <span class="font-semibold text-slate-900">{{ selectedRows.length }}</span> 项
          </p>
          <div class="flex w-full flex-col gap-3 sm:w-auto sm:flex-row sm:items-center">
            <button type="button" class="btn-secondary w-full sm:w-auto" @click="clearSelection">取消选择</button>
            <button
              type="button"
              class="inline-flex h-11 w-full items-center justify-center rounded-xl border border-slate-200 bg-white px-5 text-sm font-medium text-slate-700 transition hover:border-slate-300 hover:bg-slate-50 hover:text-slate-900 disabled:cursor-not-allowed disabled:opacity-60 sm:w-auto"
              :disabled="batchDownloadSubmitting || !selectedRowsDownloadAllowed"
              :title="!selectedRowsDownloadAllowed ? '所选项目中包含不允许下载的项' : undefined"
              @click="downloadSelectedResources"
            >
              {{ batchDownloadSubmitting ? "打包中…" : "批量下载" }}
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>

  <!-- 弹窗：文件夹/文件快速详情侧边栏（管理员预览） -->
  <Teleport to="body">
    <Transition name="modal-shell">
    <div v-if="sidebarDetailModal" class="fixed inset-0 z-[120] flex items-center justify-center bg-slate-950/30 px-4">
      <div class="modal-card panel w-full max-w-3xl p-6">
        <div class="flex items-start justify-between gap-4 border-b border-slate-200 pb-4">
          <div>
            <p class="text-xs font-semibold uppercase tracking-[0.18em] text-blue-600">{{ sidebarDetailModal.eyebrow }}</p>
            <h3 class="mt-2 text-2xl font-semibold tracking-tight text-slate-900">{{ sidebarDetailModal.title }}</h3>
            <p class="mt-2 text-sm text-slate-500">{{ sidebarDetailModal.description }}</p>
          </div>
          <button type="button" class="btn-secondary" @click="closeSidebarDetailModal">关闭</button>
        </div>
        <div class="mt-5 max-h-[70vh] overflow-y-auto pr-1">
          <div v-if="sidebarDetailModal.items.length === 0" class="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-5 text-sm text-slate-500">
            暂无数据
          </div>
          <div v-else class="space-y-3">
            <button
              v-for="(item, index) in sidebarDetailModal.items"
              :key="item.id"
              type="button"
              class="flex w-full items-center gap-4 rounded-2xl border border-slate-200 px-4 py-3 text-left transition hover:border-slate-300 hover:bg-slate-50"
              @click="openSidebarDetailItem({ id: item.id, label: item.label })"
            >
              <span class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-slate-100 text-sm font-semibold text-slate-600">
                {{ index + 1 }}
              </span>
              <div class="min-w-0 flex-1">
                <p class="truncate text-sm font-medium text-slate-900">{{ item.label }}</p>
              </div>
              <span v-if="item.meta" class="shrink-0 text-sm text-slate-500">{{ item.meta }}</span>
            </button>
          </div>
        </div>
      </div>
    </div>
    </Transition>
  </Teleport>

  <!-- 弹窗：公告中心（左侧列表 + 右侧详情，支持 Markdown 渲染） -->
  <Teleport to="body">
    <Transition name="modal-shell">
    <div v-if="announcementListOpen" class="fixed inset-0 z-[120] flex items-center justify-center bg-slate-950/30 px-4">
      <div class="modal-card panel flex h-[85vh] w-full max-w-6xl overflow-hidden">
        <!-- 左侧公告列表 -->
        <div class="flex w-64 shrink-0 flex-col border-r border-slate-200">
          <div class="border-b border-slate-200 px-4 py-3">
            <p class="text-xs font-semibold uppercase tracking-[0.18em] text-blue-600">Announcements</p>
            <h3 class="mt-1 text-lg font-semibold tracking-tight text-slate-900">全部公告</h3>
          </div>
          <div class="flex-1 overflow-y-auto">
            <p v-if="announcements.length === 0" class="px-4 py-6 text-center text-sm text-slate-500">
              暂无公告
            </p>
            <button
              v-for="item in announcements"
              :key="item.id"
              type="button"
              class="block w-full border-b border-slate-100 px-4 py-3 text-left transition"
              :class="
                selectedAnnouncementId === item.id
                  ? 'bg-blue-50/60 border-l-[3px] border-l-blue-500'
                  : 'border-l-[3px] border-l-transparent hover:bg-slate-50'
              "
              @click="selectAnnouncement(item.id)"
            >
              <div class="flex items-start gap-2">
                <span
                  v-if="item.is_pinned"
                  class="mt-0.5 shrink-0 rounded-sm bg-[#dcecff] px-1 py-0.5 text-[11px] font-semibold leading-none text-[#4f8ff7]"
                >
                  置顶
                </span>
                <div class="min-w-0">
                  <p class="truncate text-sm font-medium text-slate-900">{{ item.title }}</p>
                  <p class="mt-1 truncate text-xs text-slate-500">{{ formatDateTime(item.published_at || item.updated_at) }}</p>
                </div>
              </div>
            </button>
          </div>
        </div>

        <!-- 右侧公告详情 -->
        <div class="flex min-w-0 flex-1 flex-col">
          <template v-if="selectedAnnouncement">
            <div class="flex items-start justify-between gap-4 border-b border-slate-200 px-6 py-4">
              <div class="min-w-0">
                <p class="text-xs font-semibold uppercase tracking-[0.18em] text-blue-600">Announcement</p>
                <h3 class="mt-1 text-xl font-semibold tracking-tight text-slate-900">{{ selectedAnnouncement.title }}</h3>
                <div class="mt-2 flex flex-wrap items-center gap-3 text-sm text-slate-500">
                  <div class="flex items-center gap-2">
                    <div class="flex h-7 w-7 items-center justify-center overflow-hidden rounded-full bg-slate-100 text-xs font-semibold text-slate-600">
                      <img v-if="selectedAnnouncement.creator?.avatar_url" :src="selectedAnnouncement.creator.avatar_url" alt="发布人头像" class="h-full w-full object-cover" />
                      <span v-else>{{ announcementAuthorInitial(selectedAnnouncement) }}</span>
                    </div>
                    <span class="font-medium text-slate-700">{{ announcementAuthorName(selectedAnnouncement) }}</span>
                  </div>
                  <span
                    v-if="announcementAuthorIsSuperAdmin(selectedAnnouncement)"
                    class="rounded-full bg-[#fff1e4] px-2.5 py-1 text-xs font-semibold text-[#d07a2d]"
                  >
                    超级管理员
                  </span>
                  <span>{{ formatDateTime(selectedAnnouncement.published_at || selectedAnnouncement.updated_at) }}</span>
                </div>
              </div>
              <div class="flex flex-wrap items-center justify-end gap-2">
                <button
                  v-if="canEditAnnouncementOnHome(selectedAnnouncement)"
                  type="button"
                  class="btn-secondary"
                  @click="openAnnouncementInAdminEditor"
                >
                  编辑
                </button>
                <button type="button" class="btn-secondary" @click="closeAnnouncementList">关闭</button>
              </div>
            </div>
            <div class="flex-1 overflow-y-auto px-6 py-5">
              <div class="markdown-content" v-html="renderSimpleMarkdown(selectedAnnouncement.content)" />
            </div>
          </template>
          <div v-else class="flex flex-1 flex-col">
            <div class="flex items-center justify-end border-b border-slate-200 px-6 py-4">
              <button type="button" class="btn-secondary" @click="closeAnnouncementList">关闭</button>
            </div>
            <div class="flex flex-1 items-center justify-center text-sm text-slate-400">
              请选择一条公告
            </div>
          </div>
        </div>
      </div>
    </div>
    </Transition>
  </Teleport>

  <!-- 弹窗：管理员删除文件夹确认 -->
  <Teleport to="body">
    <Transition name="modal-shell">
    <div v-if="deleteResourceTarget" class="fixed inset-0 z-[120] flex items-center justify-center bg-slate-950/30 px-4">
      <div class="modal-card w-full max-w-md rounded-2xl bg-white p-6 shadow-xl">
        <div>
          <h3 class="text-lg font-semibold text-slate-900">
            {{ deleteResourceTarget.kind === "file" ? "确认删除文件" : "确认删除文件夹" }}
          </h3>
          <p class="mt-2 text-sm leading-6 text-slate-500">
            <!-- 虚拟文件：仅 DB 记录 -->
            <template v-if="deleteResourceTarget.kind === 'file'">
              该文件为虚拟文件（无磁盘数据），将<strong class="text-rose-700">从数据库中移除</strong>，无法恢复。
            </template>
            <!-- 虚拟目录只有 DB 记录 -->
            <template v-else-if="isCurrentFolderVirtual">
              该目录为虚拟目录（无磁盘文件），将<strong class="text-rose-700">从数据库中移除</strong>，无法恢复。
            </template>
            <template v-else-if="deleteResourceMoveToTrash">
              将把该文件夹及其子目录、文件移动到<strong class="text-slate-800">所在磁盘根目录下的 trash</strong> 文件夹（可从文件系统找回）。
            </template>
            <template v-else>
              将<strong class="text-rose-700">彻底删除</strong>该文件夹及其子目录、文件，无法恢复。
            </template>
            确认删除
            <span class="font-medium text-slate-900">{{ deleteResourceTarget.name }}</span>
            吗？
          </p>
        </div>
        <div class="mt-6 space-y-4">
          <!-- 文件和虚拟目录不显示垃圾桶选项 -->
          <div v-if="deleteResourceTarget.kind !== 'file' && !isCurrentFolderVirtual" class="space-y-2 rounded-xl border border-slate-200 bg-slate-50/80 px-4 py-3">
            <label class="flex cursor-pointer items-start gap-3 text-sm text-slate-700">
              <input v-model="deleteResourceMoveToTrash" type="radio" class="mt-1" :value="true" />
              <span>移动到垃圾桶（写入所在磁盘根目录的 <code class="rounded bg-white px-1 text-xs">trash</code>）</span>
            </label>
            <label class="flex cursor-pointer items-start gap-3 text-sm text-slate-700">
              <input v-model="deleteResourceMoveToTrash" type="radio" class="mt-1" :value="false" />
              <span>彻底删除（不经过垃圾桶，不可恢复）</span>
            </label>
          </div>
          <input v-model="deleteResourcePassword" type="password" class="field" placeholder="输入当前管理员密码确认删除" />
<div class="flex justify-end gap-3">
            <button type="button" class="btn-secondary" @click="closeDeleteResourceDialog">取消</button>
            <button
              type="button"
              class="inline-flex h-11 items-center rounded-xl bg-rose-600 px-5 text-sm font-medium text-white transition hover:bg-rose-700"
              :disabled="deleteResourceSubmitting"
              @click="confirmDeleteResource"
            >
              {{ deleteResourceSubmitting ? "删除中…" : "确认删除" }}
            </button>
          </div>
        </div>
      </div>
    </div>
    </Transition>
  </Teleport>

  <!-- 弹窗：下载确认 -->
  <Teleport to="body">
    <Transition name="modal-shell">
    <div
      v-if="downloadConfirm"
      class="fixed inset-0 z-[125] flex items-center justify-center bg-slate-950/30 px-4"
      @click.self="closeDownloadConfirm"
    >
      <div class="modal-card w-full max-w-md rounded-2xl bg-white p-6 shadow-xl" @click.stop>
        <h3 class="text-lg font-semibold text-slate-900">确认下载</h3>
        <p class="mt-3 text-sm leading-6 text-slate-600">{{ downloadConfirmMessage }}</p>
        <div class="mt-6 flex flex-wrap justify-end gap-3">
          <button type="button" class="btn-secondary" @click="closeDownloadConfirm">取消</button>
          <button type="button" class="btn-primary" :disabled="batchDownloadSubmitting" @click="confirmPendingDownload">
            {{ batchDownloadSubmitting ? "处理中…" : "确认下载" }}
          </button>
        </div>
      </div>
    </div>
    </Transition>
  </Teleport>

  <Teleport to="body">
    <Transition name="modal-shell">
    <!-- 弹窗：上传提交成功提示 -->
    <div v-if="uploadSuccessModalOpen" class="fixed inset-0 z-[120] bg-slate-950/40 backdrop-blur-sm">
      <div class="flex min-h-screen items-center justify-center px-4 py-6">
        <div class="modal-card w-full max-w-md rounded-2xl bg-white p-6 shadow-xl">
          <div class="space-y-3">
            <h3 class="text-lg font-semibold text-slate-900">提交成功</h3>
            <p class="text-sm leading-6 text-slate-600">{{ uploadMessage }}</p>
          </div>
          <div class="mt-6 flex justify-end">
            <button type="button" class="btn-primary" @click="closeUploadSuccessModal">知道了</button>
          </div>
        </div>
      </div>
    </div>
    </Transition>
  </Teleport>

  <Teleport to="body">
    <Transition name="modal-shell">
    <!-- 弹窗：访客上传资料（文件选择 + 路径填写 + 说明） -->
    <div v-if="uploadModalOpen" class="fixed inset-0 z-[120] overflow-y-auto bg-slate-950/40 backdrop-blur-sm">
      <div class="flex min-h-screen items-start justify-center px-4 py-6">
        <div class="modal-card panel w-full max-w-2xl overflow-hidden">
          <div class="max-h-[calc(100vh-3rem)] overflow-y-auto p-6">
            <div class="flex items-start justify-between gap-4 border-b border-slate-200 pb-4">
              <div>
                <h3 class="text-lg font-semibold text-slate-900">上传资料</h3>
                <p class="mt-1 text-sm text-slate-500">当前目录下直接上传资料，提交后会进入审核池。</p>
              </div>
              <button type="button" class="btn-secondary" @click="closeUploadModal">关闭</button>
            </div>

            <form class="mt-5 space-y-4" @submit.prevent="submitUpload">
            <div class="panel-muted px-4 py-3 text-sm text-slate-600">
              <p class="text-xs text-slate-400">目标目录</p>
              <p class="mt-1 font-medium text-slate-900">{{ breadcrumbs.length ? breadcrumbs.map((item) => item.name).join(" / ") : "主页根目录" }}</p>
            </div>

            <label class="space-y-2">
              <span class="text-sm font-medium text-slate-700">回执码</span>
              <div class="rounded-xl bg-slate-50 px-4 py-3">
                <p class="text-sm font-semibold tracking-[0.12em] text-slate-900">
                  {{ currentReceiptCode || "当前会话回执码暂未同步" }}
                </p>
              </div>
            </label>

            <label class="space-y-2">
              <span class="text-sm font-medium text-slate-700">资料简介</span>
              <textarea
                v-model="uploadForm.description"
                rows="4"
                class="field-area"
                placeholder="可选，简要介绍资料内容和适用场景，支持简单 Markdown 语法"
              />
            </label>

            <div class="space-y-2">
              <div class="flex items-center justify-between gap-3">
                <span class="text-sm font-medium text-slate-700">上传内容</span>
              </div>

              <input ref="uploadFileInput" type="file" class="hidden" @change="onUploadFileChange" />

              <div
                class="rounded-[28px] border-2 border-dashed px-6 py-10 text-center transition"
                :class="uploadDropActive ? 'border-blue-400 bg-blue-50/60' : 'border-slate-200 bg-slate-50/60'"
                @dragenter.prevent="onUploadDragEnter"
                @dragover.prevent="uploadDropActive = true"
                @dragleave="onUploadDragLeave"
                @drop="onUploadDrop"
              >
                <div class="mx-auto flex h-16 w-16 items-center justify-center rounded-full bg-white text-slate-300 shadow-sm">
                  <Upload class="h-8 w-8" />
                </div>
                <p class="mt-5 text-lg text-slate-600">
                  拖拽文件或整个文件夹到这里，或
                  <button type="button" class="font-semibold text-blue-600 transition hover:text-blue-700" @click="triggerUploadFileSelect">点击选择</button>
                </p>
                <p class="mt-2 text-sm text-slate-400">拖拽支持多文件和文件夹。</p>
                <p v-if="uploadCollecting" class="mt-4 text-sm text-slate-500">正在解析拖拽内容…</p>
              </div>

              <div class="panel-muted px-4 py-3 text-sm text-slate-600">
                <div class="flex flex-wrap items-center justify-between gap-3">
                  <p>
                    已选择
                    <span class="font-semibold text-slate-900">{{ uploadForm.entries.length }}</span>
                    个文件
                  </p>
                  <button v-if="uploadForm.entries.length > 0" type="button" class="text-sm text-slate-500 transition hover:text-slate-900" @click="clearUploadEntries">
                    清空列表
                  </button>
                </div>
                <div v-if="uploadForm.entries.length > 0" class="mt-3 max-h-48 space-y-2 overflow-auto pr-1">
                  <div
                    v-for="entry in uploadForm.entries"
                    :key="entry.relativePath"
                    class="rounded-xl bg-white px-3 py-2 text-sm text-slate-700"
                  >
                    {{ entry.relativePath }}
                  </div>
                </div>
                <p v-else class="mt-2 text-sm text-slate-400">当前还没有选择任何文件。</p>
              </div>
            </div>
<div class="flex justify-end gap-3">
                <button type="button" class="btn-secondary" @click="closeUploadModal">取消</button>
                <button type="submit" class="btn-primary" :disabled="uploadSubmitting || uploadCollecting || uploadForm.entries.length === 0">
                  {{ uploadSubmitting ? "提交中…" : "提交上传" }}
                </button>
              </div>
            </form>
          </div>
        </div>
      </div>
    </div>
    </Transition>
  </Teleport>

  <Teleport to="body">
    <Transition name="modal-shell">
    <!-- 弹窗：反馈提交成功提示 -->
    <div v-if="feedbackSuccessModalOpen" class="fixed inset-0 z-[120] bg-slate-950/40 backdrop-blur-sm">
      <div class="flex min-h-screen items-center justify-center px-4 py-6">
        <div class="modal-card w-full max-w-md rounded-2xl bg-white p-6 shadow-xl">
          <div class="space-y-3">
            <h3 class="text-lg font-semibold text-slate-900">提交成功</h3>
            <p class="text-sm leading-6 text-slate-600">{{ feedbackMessage }}</p>
          </div>
          <div class="mt-6 flex justify-end">
            <button type="button" class="btn-primary" @click="closeFeedbackSuccessModal">知道了</button>
          </div>
        </div>
      </div>
    </div>
    </Transition>
  </Teleport>

  <Teleport to="body">
    <Transition name="modal-shell">
    <!-- 弹窗：访客反馈中心（问题描述 + 联系方式 + 回执码） -->
    <div v-if="feedbackModalOpen" class="fixed inset-0 z-[120] bg-slate-950/40 backdrop-blur-sm">
      <div class="flex min-h-screen items-center justify-center px-4 py-6">
        <div class="modal-card panel w-full max-w-2xl overflow-hidden p-6">
          <div class="flex items-start justify-between gap-4 border-b border-slate-200 pb-5">
            <div class="space-y-1">
              <h3 class="text-lg font-semibold text-slate-900">反馈中心</h3>
              <p class="text-sm text-slate-500">填写问题说明后提交，我们会尽快处理。</p>
            </div>
            <button type="button" class="btn-secondary" @click="closeFeedbackModal">关闭</button>
          </div>

          <div class="mt-6 space-y-5">
            <div v-if="feedbackTarget" class="rounded-2xl border border-slate-200 bg-[#fafafafa] px-4 py-3">
              <p class="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">当前对象</p>
              <p class="mt-1 text-sm leading-6 text-slate-700">{{ feedbackTarget.name }}</p>
            </div>

            <label class="space-y-2">
              <span class="text-sm font-medium text-slate-700">回执码</span>
              <div class="rounded-2xl border border-slate-200 bg-[#fafafafa] px-4 py-3">
                <p class="text-sm font-semibold tracking-[0.12em] text-slate-900">
                  {{ currentReceiptCode || "当前会话回执码暂未同步" }}
                </p>
              </div>
            </label>

            <label class="space-y-2">
              <span class="text-sm font-medium text-slate-700">问题说明</span>
              <textarea
                v-model="feedbackDescription"
                rows="5"
                class="field-area"
              placeholder="信息不当/侵权/内容错误……描述您遇到的问题，我们会尽快改进！"
              />
            </label>

            <p v-if="feedbackMessage" class="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">{{ feedbackMessage }}</p>
<div class="flex justify-end gap-3 pt-1">
              <button type="button" class="btn-secondary" @click="closeFeedbackModal">取消</button>
              <button type="button" class="btn-primary" :disabled="feedbackSubmitDisabled" @click="submitFeedback">
                {{ feedbackSubmitting ? "提交中…" : "提交反馈" }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
    </Transition>
  </Teleport>

  <Teleport to="body">
    <Transition name="modal-shell">
    <!-- 弹窗：管理员编辑当前文件夹简介（Markdown + 实时预览） -->
    <div v-if="folderDescriptionEditorOpen" class="fixed inset-0 z-[120] overflow-y-auto bg-slate-950/40 backdrop-blur-sm">
      <div class="flex min-h-[100dvh] justify-center px-4 py-6 sm:py-10">
        <div
          class="modal-card panel relative my-auto flex w-full max-w-5xl max-h-[min(90dvh,calc(100dvh-2.5rem))] min-h-0 flex-col overflow-hidden p-6"
        >
          <div class="shrink-0 border-b border-slate-200 pb-4">
            <div class="flex flex-wrap items-center justify-between gap-3">
              <h3 class="text-lg font-semibold text-slate-900">编辑文件夹信息</h3>
              <div class="flex shrink-0 flex-wrap justify-end gap-3">
                <button type="button" class="btn-secondary" @click="closeFolderDescriptionEditor">取消</button>
                <button
                  type="button"
                  class="btn-primary"
                  :disabled="folderDescriptionSaving || !folderEditorDirty"
                  @click="saveFolderDescription"
                >
                  {{ folderDescriptionSaving ? "保存中…" : "保存更改" }}
                </button>
              </div>
            </div>
          </div>

          <div class="min-h-0 flex-1 overflow-y-auto overscroll-contain pt-5 [-webkit-overflow-scrolling:touch]">
            <div class="space-y-4 pb-2">
            <label class="space-y-2">
              <span class="text-sm font-medium text-slate-700">文件夹名</span>
              <input
                v-model="folderNameDraft"
                class="field"
                :disabled="!canManageResourceDescriptions"
                placeholder="输入文件夹名"
              />
            </label>

            <label class="space-y-2">
              <span class="text-sm font-medium text-slate-700">备注（单行）</span>
              <input
                v-model="folderRemarkDraft"
                type="text"
                maxlength="500"
                class="field"
                placeholder="展示在首页卡片副标题，不支持换行与 Markdown"
                autocomplete="off"
              />
            </label>

            <label class="space-y-2">
              <span class="text-sm font-medium text-slate-700">封面图地址（可选）</span>
              <input
                v-model="folderCoverUrlDraft"
                type="url"
                class="field"
                placeholder="https://cdn.example.com/cover.jpg（留空则使用简介中 ![cover](...)）"
                autocomplete="off"
              />
              <p class="text-xs leading-5 text-slate-500">
                填写后优先作为首页列表与详情顶部封面；需以 http(s) 开头。清空并保存则回退到简介内封面语法。
              </p>
            </label>

            <div
              class="grid min-h-0 grid-cols-1 gap-6 lg:min-h-[28rem] lg:grid-cols-2 lg:grid-rows-[auto_minmax(17rem,1fr)]"
            >
              <span class="order-1 text-sm font-medium text-slate-700 lg:order-none lg:col-start-1 lg:row-start-1">
                简介（Markdown）
              </span>
              <textarea
                v-model="folderDescriptionDraft"
                class="field-area order-2 min-h-[17rem] w-full resize-y rounded-3xl lg:order-none lg:col-start-1 lg:row-start-2 lg:h-full lg:min-h-0 lg:resize-none"
                rows="10"
                placeholder="进入该文件夹后的详情区展示；支持简单 Markdown。"
              />
              <div class="order-3 shrink-0 lg:order-none lg:col-start-2 lg:row-start-1">
                <h4 class="text-lg font-semibold text-slate-900">简介预览</h4>
              </div>
              <div
                class="order-4 flex min-h-[17rem] flex-col overflow-hidden rounded-3xl border border-slate-200 bg-white lg:order-none lg:col-start-2 lg:row-start-2 lg:h-full lg:min-h-0"
              >
                <div class="min-h-0 flex-1 overflow-y-auto px-5 py-5">
                  <div
                    v-if="folderDescriptionPreviewHTML"
                    class="markdown-content"
                    v-html="folderDescriptionPreviewHTML"
                    @click.capture="handleMarkdownInternalLinkNavigate"
                  />
                  <p v-else class="text-sm text-slate-400">这里会显示简介预览。</p>
                </div>
              </div>
            </div>

            <label class="space-y-2">
              <span class="text-sm font-medium text-slate-700">子文件直链前缀（可选）</span>
              <input
                v-model="folderDirectPrefixDraft"
                type="url"
                class="field"
                placeholder="https://file.test.cn/（其下文件直链 = 此前缀 + 相对路径；最内层有前缀的文件夹优先生效）"
                autocomplete="off"
              />
              <p class="text-xs leading-5 text-slate-500">
                单个文件若配置了播放/下载直链，仍优先使用该链接。留空则回退本站下载。需以 http(s) 开头。
              </p>
            </label>

            <!-- 自定义访问路径：设置后可通过 /doc 之类的短链接访问该文件夹 -->
            <label class="space-y-2">
              <span class="text-sm font-medium text-slate-700">自定义访问路径（可选）</span>
              <input
                v-model="folderCustomPathDraft"
                type="text"
                class="field"
                placeholder="例如 doc 或 doc/sub，设置后可通过 /doc 直接访问此文件夹"
                autocomplete="off"
              />
              <p class="text-xs leading-5 text-slate-500">
                必须以英文字母开头，可包含字母、数字、下划线、连字符和多级路径。不能与 upload/admin/files/api 等系统路径冲突。留空则取消自定义路径。
              </p>
            </label>

            <label class="space-y-2">
              <span class="text-sm font-medium text-slate-700">是否允许下载</span>
              <select v-model="folderDownloadPolicyDraft" class="field">
                <option value="inherit">继承上层文件夹设置</option>
                <option value="allow">允许下载</option>
                <option value="deny">禁止下载</option>
              </select>
              <p class="text-xs leading-5 text-slate-500">
                「继承」时随上层文件夹策略；未设置时默认允许下载。禁止后整包下载与列表下载入口会隐藏，且接口返回 403。
              </p>
            </label>

            <label
              v-if="currentFolderDetail && folderDetailIsManagingRoot(currentFolderDetail)"
              class="flex cursor-pointer items-start gap-3 rounded-xl border border-slate-200 bg-slate-50/80 px-4 py-3"
            >
              <input v-model="folderHidePublicCatalogDraft" type="checkbox" class="mt-1 h-4 w-4 rounded border-slate-300" />
              <span class="min-w-0 text-sm text-slate-700">
                <span class="font-medium text-slate-900">访客首页隐藏此托管根目录</span>
                <span class="mt-1 block text-xs text-slate-500">
                  开启后公开首页的根目录列表不出现此项；书签或链接仍可打开（暂不封锁其它入口）。有资源审核权限的管理员可随时取消勾选恢复。
                </span>
              </span>
            </label>
</div>
          </div>
        </div>
      </div>
    </div>
    </Transition>
  </Teleport>

  <Teleport to="body">
    <Transition name="modal-shell">
    <!-- 弹窗：管理员在当前目录下创建新文件夹 -->
    <div v-if="createFolderModalOpen" class="fixed inset-0 z-[125] overflow-y-auto bg-slate-950/40 backdrop-blur-sm">
      <div class="flex min-h-[100dvh] justify-center px-4 py-6 sm:py-10">
        <div class="modal-card panel relative my-auto flex w-full max-w-md flex-col overflow-hidden p-6">
          <div class="shrink-0 border-b border-slate-200 pb-4">
            <h3 class="text-lg font-semibold text-slate-900">创建文件夹</h3>
            <p class="mt-1 text-sm text-slate-500">
              在「{{ currentFolderDetail?.name ?? "" }}」中创建子文件夹。
            </p>
          </div>
          <div class="py-5 space-y-4">
            <label class="space-y-2">
              <span class="text-sm font-medium text-slate-700">文件夹名称</span>
              <input
                v-model="createFolderNameDraft"
                class="field"
                placeholder="输入文件夹名"
                autocomplete="off"
                @keyup.enter="submitCreateFolder"
              />
            </label>
</div>
          <div class="flex flex-wrap justify-end gap-3 border-t border-slate-200 pt-4">
            <button type="button" class="btn-secondary" :disabled="createFolderSaving" @click="closeCreateFolderModal">取消</button>
            <button type="button" class="btn-primary" :disabled="createFolderSaving" @click="submitCreateFolder">
              {{ createFolderSaving ? "创建中…" : "创建" }}
            </button>
          </div>
        </div>
      </div>
    </div>
    </Transition>
  </Teleport>

  <!-- 弹窗：管理员创建虚拟目录 -->
  <Teleport to="body">
    <Transition name="modal-shell">
    <div v-if="virtualFolderModalOpen" class="fixed inset-0 z-[125] overflow-y-auto bg-slate-950/40 backdrop-blur-sm">
      <div class="flex min-h-[100dvh] justify-center px-4 py-6 sm:py-10">
        <div class="modal-card panel relative my-auto flex w-full max-w-md flex-col overflow-hidden p-6">
          <div class="shrink-0 border-b border-slate-200 pb-4">
            <h3 class="text-lg font-semibold text-slate-900">创建虚拟目录</h3>
            <p class="mt-1 text-sm text-slate-500">
              在「{{ currentFolderDetail?.name ?? "" }}」中创建虚拟目录（无物理磁盘路径，子文件通过 CDN 直链提供）。
            </p>
          </div>
          <div class="py-5 space-y-4">
            <label class="space-y-2">
              <span class="text-sm font-medium text-slate-700">目录名称</span>
              <input
                v-model="virtualFolderNameDraft"
                class="field"
                placeholder="输入虚拟目录名"
                autocomplete="off"
                @keyup.enter="submitCreateVirtualFolder"
              />
            </label>
</div>
          <div class="flex flex-wrap justify-end gap-3 border-t border-slate-200 pt-4">
            <button type="button" class="btn-secondary" :disabled="virtualFolderSaving" @click="closeCreateVirtualFolderModal">取消</button>
            <button type="button" class="btn-primary" :disabled="virtualFolderSaving" @click="submitCreateVirtualFolder">
              {{ virtualFolderSaving ? "创建中…" : "创建" }}
            </button>
          </div>
        </div>
      </div>
    </div>
    </Transition>
  </Teleport>

  <!-- 弹窗：管理员在虚拟目录下添加虚拟文件 -->
  <Teleport to="body">
    <Transition name="modal-shell">
    <div v-if="virtualFileModalOpen" class="fixed inset-0 z-[125] overflow-y-auto bg-slate-950/40 backdrop-blur-sm">
      <div class="flex min-h-[100dvh] justify-center px-4 py-6 sm:py-10">
        <div class="modal-card panel relative my-auto flex w-full max-w-md flex-col overflow-hidden p-6">
          <div class="shrink-0 border-b border-slate-200 pb-4">
            <h3 class="text-lg font-semibold text-slate-900">添加虚拟文件</h3>
            <p class="mt-1 text-sm text-slate-500">
              在虚拟目录「{{ currentFolderDetail?.name ?? "" }}」中添加文件。
            </p>
          </div>
          <div class="py-5 space-y-4">
            <label class="space-y-2">
              <span class="text-sm font-medium text-slate-700">文件名称（含扩展名）</span>
              <input
                v-model="virtualFileNameDraft"
                class="field"
                placeholder="例如：report.pdf"
                autocomplete="off"
              />
            </label>

            <!-- 服务端代理开关 -->
            <label class="flex cursor-pointer items-start gap-3 rounded-xl border border-blue-200 bg-blue-50/60 px-4 py-3 text-sm text-slate-700">
              <input v-model="virtualFileProxyDownload" type="checkbox" class="mt-0.5" />
              <span>
                <span class="font-medium text-slate-900">服务端代理</span>
                <span class="mt-1 block text-xs text-slate-500">由服务端从内网 / LAN 地址拉取文件再流式返回客户端，支持断点续传和视频拖动。</span>
              </span>
            </label>

            <!-- 服务端代理地址（勾选代理时显示） -->
            <label v-if="virtualFileProxyDownload" class="space-y-2">
              <span class="text-sm font-medium text-slate-700">服务端代理地址（必填）</span>
              <input
                v-model="virtualFileProxySourceDraft"
                class="field"
                placeholder="http://10.92.114.62:5244/dav/video.mp4"
                autocomplete="off"
              />
            </label>

            <!-- 前台 CDN 直链 -->
            <label class="space-y-2">
              <span class="text-sm font-medium text-slate-700">{{ virtualFileProxyDownload ? "前台 CDN 直链（可选，降低服务器压力）" : "前台直链地址（必填）" }}</span>
              <input
                v-model="virtualFileUrlDraft"
                class="field"
                placeholder="https://cdn.example.com/files/report.pdf"
                autocomplete="off"
              />
            </label>

            <!-- 前台备用直链 -->
            <label class="space-y-2">
              <span class="text-sm font-medium text-slate-700">前台备用直链（可选）</span>
              <input
                v-model="virtualFileFallbackUrlDraft"
                class="field"
                placeholder="https://cdn2.example.com/files/report.pdf（主直链失效时使用）"
                autocomplete="off"
              />
            </label>

            <!-- 链接检测按钮 -->
            <div class="flex items-center gap-3">
              <button
                type="button"
                class="btn-secondary text-sm"
                :disabled="virtualFileDetectingLink || (!virtualFileProxySourceDraft.trim() && !virtualFileUrlDraft.trim())"
                @click="detectVirtualFileLink"
              >
                {{ virtualFileDetectingLink ? "检测中…" : "检测链接" }}
              </button>
              <span v-if="virtualFileDetectResult" class="text-sm" :class="virtualFileDetectResult.startsWith('链接可达') ? 'text-emerald-600' : 'text-rose-600'">{{ virtualFileDetectResult }}</span>
            </div>
</div>
          <div class="flex flex-wrap justify-end gap-3 border-t border-slate-200 pt-4">
            <button type="button" class="btn-secondary" :disabled="virtualFileSaving" @click="closeAddVirtualFileModal">取消</button>
            <button type="button" class="btn-primary" :disabled="virtualFileSaving" @click="submitAddVirtualFile">
              {{ virtualFileSaving ? "添加中…" : "添加" }}
            </button>
          </div>
        </div>
      </div>
    </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.public-home-card-grid {
  display: grid;
  /* 列数由主内容区宽度决定（非整页视口），避免 xl 出现侧栏时仍按视口算 4 列导致卡片过窄 */
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 17.5rem), 1fr));
}

@keyframes warning-fade-in {
  0% {
    opacity: 0;
    transform: translateY(8px) scale(0.98);
  }

  100% {
    opacity: 1;
    transform: translateY(0) scale(1);
  }
}

@keyframes warning-fade-out {
  0% {
    opacity: 1;
    transform: translateY(0);
  }

  100% {
    opacity: 0;
    transform: translateY(-6px);
  }
}
</style>
