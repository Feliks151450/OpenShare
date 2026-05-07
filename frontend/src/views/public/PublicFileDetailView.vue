<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { RouterLink, useRoute, useRouter, type RouteLocationRaw } from "vue-router";
import {
  ChevronDown,
  Clock,
  Database,
  Download,
  FileText,
  FileVideo,
  Folder,
  Flag,
  Link2,
  NotebookText,
  PanelRightClose,
  PanelRightOpen,
  Server,
  Share2,
} from "lucide-vue-next";

import type { FileTagDefinition, PublicFileTag } from "../../lib/publicFileTags";
import { fetchPublicFileTagDefinitions, readableTextColorForPreset } from "../../lib/publicFileTags";
import FileTagChips from "../../components/public/FileTagChips.vue";
import { HttpError, httpClient } from "../../lib/http/client";
import { readApiError } from "../../lib/http/helpers";
import { ensureSessionReceiptCode, readStoredReceiptCode } from "../../lib/receiptCode";
import { hasAdminPermission } from "../../lib/admin/session";
import {
  fileEffectiveDownloadHref,
  fileUsesBackendDownloadHref,
  withBackendDownloadInlinePreviewParam,
} from "../../lib/fileDirectUrl";
import { copyPlainTextToClipboard } from "../../lib/clipboard";
import { renderSimpleMarkdown } from "../../lib/markdown";
import {
  hydrateMarkdownCatalogNavigatePresentation,
  markdownCatalogNavigateInitialPresentation,
  type MarkdownCatalogConfirmPresentation,
} from "../../lib/markdownCatalogNavigateDisplay";
import { onMarkdownLinkClickCapture, isViewportTailwindXlMin } from "../../lib/publicMarkdownLinks";
import { netcdfStructureToMarkdown, type NetCDFDumpGroup } from "../../lib/netcdfStructureToMarkdown";
/** 递归嵌套「右侧预览」，避免与同名默认导出循环引用告警 */
import PublicFileDetailPeek from "./PublicFileDetailView.vue";

const props = withDefaults(
  defineProps<{
    /** 嵌入首页侧栏时传入，覆盖路由里的 fileID */
    overrideFileId?: string | null;
    /** 是否为右侧抽屉内嵌展示（交互与全文路由区分） */
    panelPresentation?: boolean;
  }>(),
  {
    overrideFileId: null,
    panelPresentation: false,
  },
);

const emit = defineEmits<{
  closePanel: [];
  openFullPage: [];
  navigatePanelFile: [fileId: string];
  leaveToPublicCatalog: [];
}>();

interface FileDetailResponse {
  id: string;
  name: string;
  extension: string;
  folder_id: string;
  /** 站内展示用资料路径（通常为目录层级名）；非磁盘路径 */
  path: string;
  /** 托管存储下磁盘绝对路径（可解析且存在时为非空）；与 `path` 不同 */
  storage_path?: string;
  description: string;
  /** 单行备注；卡片副标题用，与 Markdown 简介分离 */
  remark?: string;
  mime_type: string;
  /** 非空时使用该 http(s) 地址作为播放器与复制下载直链，而非本站下载接口 */
  playback_url?: string;
  /** 非空时优先作为列表/详情封面，高于简介中 ![cover](...) */
  cover_url?: string;
  /** 由文件夹直链前缀生成，不含 playback_url；前端优先 playback */
  folder_direct_download_url?: string;
  /** 解析继承后的站点下载策略；为 false 时隐藏下载与「复制下载直链」 */
  download_allowed?: boolean;
  /** 本文件在库中的三态：inherit | allow | deny（管理端编辑用） */
  download_policy?: "inherit" | "allow" | "deny";
  /** 仅视频：主直链失效时依次尝试；需已配置主直链才有意义 */
  playback_fallback_url?: string;
  size: number;
  uploaded_at: string;
  download_count: number;
  tags?: PublicFileTag[];
}

const route = useRoute();
const router = useRouter();

/** 全文路由页：Markdown 链到站内其它文件时用右侧预览（与子组件抽屉逻辑一致） */
const markdownPeekFileId = ref<string | null>(null);

/** Markdown 内链前往资料目录前：弹窗占位，挂起 Promise 的 resolve */
const markdownCatalogNavigateConfirmRoute = ref<RouteLocationRaw | null>(null);
let markdownCatalogNavigateConfirmResolve: ((ok: boolean) => void) | null = null;
const markdownCatalogNavigatePresentation = ref<MarkdownCatalogConfirmPresentation | null>(null);
let markdownCatalogNavigateHydrateGeneration = 0;

const detail = ref<FileDetailResponse | null>(null);
const loading = ref(false);
const error = ref("");
const message = ref("");
const saveError = ref("");
const saving = ref(false);
const editFileName = ref("");
const editDescription = ref("");
const editRemark = ref("");
const editPlaybackUrl = ref("");
const editPlaybackFallbackUrl = ref("");
const editCoverUrl = ref("");
const editDownloadPolicy = ref<"inherit" | "allow" | "deny">("inherit");
const descriptionEditorOpen = ref(false);
const tagEditorOpen = ref(false);
const tagCatalog = ref<FileTagDefinition[]>([]);
const tagEditorSelected = ref<string[]>([]);
const tagEditorLoading = ref(false);
const tagEditorSaving = ref(false);
const tagEditorError = ref("");
const canManageResourceDescriptions = ref(false);
/** 文件简介区域：默认限高，可展开全文 */
const fileDescriptionExpanded = ref(false);
const fileDescriptionClampRef = ref<HTMLElement | null>(null);
/** 需要显示「展开 / 收起」时包含：折叠且内容被裁切，或已展开（显示收起） */
const fileDescriptionFooterVisible = ref(false);
const deleteDialogOpen = ref(false);
const deletePassword = ref("");
const deleteMoveToTrash = ref(true);
const deleteSubmitting = ref(false);
const deleteError = ref("");
const feedbackModalOpen = ref(false);
const DEFAULT_LARGE_DOWNLOAD_CONFIRM = 1024 * 1024 * 1024;
const largeDownloadConfirmBytes = ref(DEFAULT_LARGE_DOWNLOAD_CONFIRM);
const downloadFileConfirmOpen = ref(false);
const feedbackSuccessModalOpen = ref(false);
const feedbackDescription = ref("");
const feedbackSubmitting = ref(false);
const feedbackMessage = ref("");
const feedbackError = ref("");
const currentReceiptCode = ref("");
const fileID = computed(() => {
  const fromProp = (props.overrideFileId ?? "").trim();
  if (fromProp) {
    return fromProp;
  }
  return String(route.params.fileID ?? "");
});

watch(
  () => fileID.value,
  () => {
    peerListAsideTempHidden.value = false;
  },
);

function extractPublicFileDetailIdFromMarkdownRoute(routeRaw: RouteLocationRaw): string | null {
  if (typeof routeRaw !== "object" || routeRaw == null || !("name" in routeRaw)) {
    return null;
  }
  if (routeRaw.name !== "public-file-detail") {
    return null;
  }
  const rawId = routeRaw.params?.fileID;
  const part =
    rawId === undefined || rawId === null ? "" : Array.isArray(rawId) ? String(rawId[0] ?? "") : String(rawId);
  const nextId = decodeURIComponent(part).trim();
  return nextId || null;
}

/** 抽屉内浏览器地址仍是首页：`./其它文件 id` 须相对 `/files/当前 id` 解析；全文页则仅用拦截逻辑 */
function handleMarkdownInternalLinkNavigate(ev: MouseEvent) {
  let resolutionBaseHref: string | undefined;
  if (props.panelPresentation && fileID.value.trim() && typeof window !== "undefined") {
    try {
      const loc = router.resolve({
        name: "public-file-detail",
        params: { fileID: fileID.value.trim() },
      });
      resolutionBaseHref = new URL(loc.href, window.location.origin).href;
    } catch {
      resolutionBaseHref = undefined;
    }
  }

  function interceptMarkdownInternalFileNavigate(routeRaw: RouteLocationRaw): boolean {
    const nextId = extractPublicFileDetailIdFromMarkdownRoute(routeRaw);
    if (!nextId) {
      return false;
    }

    if (props.panelPresentation) {
      if (!isViewportTailwindXlMin()) {
        return false;
      }
      emit("navigatePanelFile", nextId);
      return true;
    }

    if ((props.overrideFileId ?? "").trim()) {
      return false;
    }

    if (nextId === fileID.value.trim()) {
      return true;
    }
    if (!isViewportTailwindXlMin()) {
      return false;
    }
    markdownPeekFileId.value = nextId;
    return true;
  }

  function promptMarkdownCatalogNavigateConfirm(route: RouteLocationRaw): Promise<boolean> {
    return new Promise((resolve) => {
      markdownCatalogNavigateHydrateGeneration += 1;
      const gen = markdownCatalogNavigateHydrateGeneration;
      markdownCatalogNavigateConfirmRoute.value = route;
      markdownCatalogNavigatePresentation.value = markdownCatalogNavigateInitialPresentation(route);
      markdownCatalogNavigateConfirmResolve = resolve;
      syncMarkdownPeekBodyScrollLock();
      void hydrateMarkdownCatalogNavigatePresentation(route).then((presentation) => {
        if (gen !== markdownCatalogNavigateHydrateGeneration) {
          return;
        }
        markdownCatalogNavigatePresentation.value = presentation;
      });
    });
  }

  onMarkdownLinkClickCapture(ev, router, {
    ...(resolutionBaseHref ? { resolutionBaseHref } : {}),
    interceptPush: interceptMarkdownInternalFileNavigate,
    confirmBeforeMarkdownCatalogNavigate: promptMarkdownCatalogNavigateConfirm,
  });
}

function dismissMarkdownCatalogNavigateConfirm(ok: boolean) {
  markdownCatalogNavigateHydrateGeneration += 1;
  markdownCatalogNavigateConfirmRoute.value = null;
  markdownCatalogNavigatePresentation.value = null;
  markdownCatalogNavigateConfirmResolve?.(ok);
  markdownCatalogNavigateConfirmResolve = null;
  syncMarkdownPeekBodyScrollLock();
}

function closeMarkdownPeekDrawer() {
  markdownPeekFileId.value = null;
  syncMarkdownPeekBodyScrollLock();
}

function onMarkdownPeekOpenFullPage() {
  const id = markdownPeekFileId.value?.trim() ?? "";
  markdownPeekFileId.value = null;
  syncMarkdownPeekBodyScrollLock();
  if (id) {
    void router.push({ name: "public-file-detail", params: { fileID: id } });
  }
}

function onMarkdownPeekNavigate(nextId: string) {
  markdownPeekFileId.value = nextId;
}

function onMarkdownPeekLeaveCatalog() {
  markdownPeekFileId.value = null;
  syncMarkdownPeekBodyScrollLock();
  void router.push({ name: "public-home" });
}

function syncMarkdownPeekBodyScrollLock() {
  if (typeof document === "undefined") {
    return;
  }
  if (props.panelPresentation) {
    return;
  }
  const lock = markdownPeekFileId.value != null || markdownCatalogNavigateConfirmRoute.value != null;
  document.body.style.overflow = lock ? "hidden" : "";
}

const backendDownloadPath = computed(() => `/api/public/files/${encodeURIComponent(fileID.value)}/download`);

const downloadActionsAllowed = computed(() => detail.value?.download_allowed !== false);

/** 播放器与「复制下载直链」：playback_url > 文件夹前缀直链 > 本站下载接口 */
const mediaSourceURL = computed(() => {
  if (!detail.value) {
    return backendDownloadPath.value;
  }
  return fileEffectiveDownloadHref(
    fileID.value,
    detail.value.playback_url,
    detail.value.folder_direct_download_url,
  );
});

/** 含协议与域名的完整 URL，便于站外粘贴 */
const absoluteDownloadURL = computed(() => {
  if (typeof window === "undefined") {
    return "";
  }
  const src = mediaSourceURL.value;
  if (src.startsWith("http://") || src.startsWith("https://")) {
    return src;
  }
  return new URL(src, window.location.origin).href;
});

/** 内嵌预览用：本站 /download 附加 inline=1，避免 PDF 等被 Content-Disposition: attachment 强制下载 */
const previewEmbedDownloadURL = computed(() => {
  if (typeof window === "undefined") {
    return "";
  }
  return withBackendDownloadInlinePreviewParam(absoluteDownloadURL.value);
});

const absoluteDetailPageURL = computed(() => {
  if (typeof window === "undefined") {
    return "";
  }
  const path = router.resolve({
    name: "public-file-detail",
    params: { fileID: fileID.value },
  }).href;
  return new URL(path, window.location.origin).href;
});

/** 与哔哩哔哩等站点类似：`?t=328.2` 表示从该秒数开始播放（秒，可为小数） */
function parseTimestampQuery(raw: unknown): number | null {
  if (raw == null || raw === "") {
    return null;
  }
  const s = Array.isArray(raw) ? raw[0] : raw;
  const n = parseFloat(String(s));
  if (!Number.isFinite(n) || n < 0) {
    return null;
  }
  return n;
}

function formatTimestampParam(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds <= 0) {
    return "0";
  }
  const rounded = Math.round(seconds * 10) / 10;
  return Number.isInteger(rounded) ? String(rounded) : rounded.toFixed(1);
}

function buildAbsoluteDetailPageURL(query?: Record<string, string>): string {
  if (typeof window === "undefined") {
    return "";
  }
  const path = router.resolve({
    name: "public-file-detail",
    params: { fileID: fileID.value },
    query: query ?? {},
  }).href;
  return new URL(path, window.location.origin).href;
}

const videoRef = ref<HTMLVideoElement | null>(null);
/** 视频播放用：主直链 → 备用 → 文件夹前缀 → 本站；与复制下载的 mediaSourceURL 独立 */
const videoPlaybackStep = ref(0);

/** 内嵌播放器完整回退链；禁止下载时仅隐藏下载类 UI，音/视频仍可用本站 /download 以 inline 方式播放（与详情接口 download_allowed 一致）。 */
function buildVideoPlaybackUrlQueue(fileId: string, d: FileDetailResponse): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  const add = (u: string) => {
    const t = u.trim();
    if (!t || seen.has(t)) return;
    seen.add(t);
    out.push(t);
  };
  const playback = (d.playback_url ?? "").trim();
  const fallback = (d.playback_fallback_url ?? "").trim();
  const folder = (d.folder_direct_download_url ?? "").trim();
  const backend = `/api/public/files/${encodeURIComponent(fileId)}/download`;

  if (playback) {
    add(playback);
    if (fallback) {
      add(fallback);
    }
  }
  if (folder) {
    add(folder);
  }
  add(backend);
  return out;
}

const videoPlaybackUrlQueue = computed(() =>
  detail.value ? buildVideoPlaybackUrlQueue(fileID.value, detail.value) : [],
);

const videoPlaybackActiveSrc = computed(() => {
  const q = videoPlaybackUrlQueue.value;
  if (q.length === 0) {
    return "";
  }
  const i = Math.min(videoPlaybackStep.value, q.length - 1);
  return q[i] ?? "";
});

watch(
  () =>
    detail.value
      ? `${fileID.value}|${detail.value.playback_url ?? ""}|${detail.value.playback_fallback_url ?? ""}|${detail.value.folder_direct_download_url ?? ""}`
      : "",
  () => {
    videoPlaybackStep.value = 0;
  },
);

function onVideoPlaybackError() {
  const q = videoPlaybackUrlQueue.value;
  if (videoPlaybackStep.value < q.length - 1) {
    videoPlaybackStep.value++;
  }
}

function applySeekFromRouteQuery() {
  const el = videoRef.value;
  if (!el) {
    return;
  }
  const t = parseTimestampQuery(route.query.t);
  if (t == null) {
    return;
  }
  const dur = el.duration;
  const end = Number.isFinite(dur) && dur > 0 ? dur : Number.POSITIVE_INFINITY;
  el.currentTime = Math.min(Math.max(0, t), Math.max(0, end - 0.05));
}

function onVideoLoadedMetadata() {
  void nextTick(() => {
    applySeekFromRouteQuery();
    setupVideoStageObserver();
  });
}

const linkCopyHint = ref("");
let linkCopyTimer: ReturnType<typeof setTimeout> | null = null;

/** 与首页列表图标逻辑对齐，便于 mime 缺失时仍识别常见视频扩展名 */
const VIDEO_EXTENSIONS = new Set(["mp4", "mov", "avi", "mkv", "webm", "m4v", "ogv"]);

function isVideoDetail(d: FileDetailResponse): boolean {
  const mime = (d.mime_type ?? "").toLowerCase();
  if (mime.startsWith("video/")) {
    return true;
  }
  const ext = (d.extension ?? "").replace(/^\./, "").toLowerCase();
  return VIDEO_EXTENSIONS.has(ext);
}

const isVideo = computed(() => (detail.value ? isVideoDetail(detail.value) : false));

/** 与视频类似：详情页右侧可显示「同目录同后缀」列表（视频=常见视频后缀；PDF/NetCDF 仅本后缀） */
const peerSidebarSameExtLabel = computed((): "video" | "pdf" | "nc" | null => {
  if (!detail.value) {
    return null;
  }
  if (isVideoDetail(detail.value)) {
    return "video";
  }
  const ext = normalizedFileExtension(detail.value);
  if (ext === "pdf") {
    return "pdf";
  }
  if (ext === "nc") {
    return "nc";
  }
  return null;
});

/** 非视频文件的浏览器内预览类型 */
type DetailPreviewVisualKind = "image" | "pdf" | "markdown" | "plain" | "csv" | "netcdf";

const PREVIEW_TEXT_MAX_BYTES = 1_048_576;
/** 本站 PDF：fetch 为 Blob 再交给 iframe，绕过服务端 Content-Disposition 触发的强制下载 */
const PDF_PREVIEW_MAX_BYTES = 52_428_800;

const IMAGE_PREVIEW_EXTENSIONS = new Set(["png", "jpeg", "jpg", "jfif", "gif", "webp", "svg", "bmp"]);

function normalizedFileExtension(d: FileDetailResponse): string {
  return (d.extension ?? "").replace(/^\./, "").toLowerCase();
}

function fileDetailPreviewVisualKind(d: FileDetailResponse): DetailPreviewVisualKind | null {
  if (isVideoDetail(d)) {
    return null;
  }
  const mime = (d.mime_type ?? "").toLowerCase();
  const ext = normalizedFileExtension(d);
  if (mime.startsWith("image/") || IMAGE_PREVIEW_EXTENSIONS.has(ext)) {
    return "image";
  }
  if (mime === "application/pdf" || ext === "pdf") {
    return "pdf";
  }
  if (mime.includes("markdown") || ext === "md" || ext === "markdown") {
    return "markdown";
  }
  if (mime === "text/csv" || mime === "text/tab-separated-values" || ext === "csv" || ext === "tsv") {
    return "csv";
  }
  if (ext === "nc") {
    return "netcdf";
  }
  if (mime === "text/plain" || ext === "txt" || ext === "ncl") {
    return "plain";
  }
  return null;
}

const previewVisualKind = computed((): DetailPreviewVisualKind | null =>
  detail.value ? fileDetailPreviewVisualKind(detail.value) : null,
);

/** NetCDF：工具栏复制托管磁盘绝对路径（非网页 URL） */
const showNcCopyServerStoragePath = computed(() => {
  if (!detail.value) {
    return false;
  }
  if (normalizedFileExtension(detail.value) !== "nc") {
    return false;
  }
  return (detail.value.storage_path ?? "").trim().length > 0;
});

const TEXTUAL_PREVIEW_KINDS = new Set<DetailPreviewVisualKind>(["markdown", "plain", "csv"]);

const needsFetchedTextPreview = computed(() => {
  const k = previewVisualKind.value;
  if (k == null) {
    return false;
  }
  return TEXTUAL_PREVIEW_KINDS.has(k) || k === "netcdf";
});

/** Markdown / PDF / 纯文本(txt、ncl) / NetCDF(nc)：先展示简介再展示内嵌预览（与只读静态页对齐） */
const showFileDescriptionAbovePreview = computed(() => {
  if (!detail.value || isVideo.value) {
    return false;
  }
  const k = previewVisualKind.value;
  return k === "markdown" || k === "pdf" || k === "plain" || k === "netcdf";
});

const previewFetchedText = ref("");
/** NetCDF API 返回的 `structure`，用于 Markdown 结构化预览 */
const previewNetcdfStructure = ref<NetCDFDumpGroup | null>(null);
const previewFallbackUsed = ref(false);
const previewTextLoading = ref(false);
const previewTextError = ref("");
const previewTextTruncated = ref(false);
const fetchedTextPreviewCollapsed = ref(false);
const previewImageFailed = ref(false);
let previewTextAbortController: AbortController | null = null;

function onFetchedPreviewHeaderClick(ev: MouseEvent) {
  if ((ev.target as HTMLElement).closest("button")) {
    return;
  }
  fetchedTextPreviewCollapsed.value = !fetchedTextPreviewCollapsed.value;
}

function toggleFetchedTextPreviewCollapsedKey() {
  fetchedTextPreviewCollapsed.value = !fetchedTextPreviewCollapsed.value;
}

const previewMarkdownRendered = computed(() => {
  if (previewVisualKind.value !== "markdown") {
    return "";
  }
  return renderSimpleMarkdown(previewFetchedText.value);
});

const previewNetcdfMarkdownHtml = computed(() => {
  if (previewVisualKind.value !== "netcdf" || !previewNetcdfStructure.value) {
    return "";
  }
  return renderSimpleMarkdown(netcdfStructureToMarkdown(previewNetcdfStructure.value));
});

const pdfPeerSidebar = computed(
  () =>
    !props.panelPresentation &&
    peerSidebarSameExtLabel.value === "pdf" &&
    Boolean(folderIdForPeers.value),
);

const netcdfPeerSidebar = computed(
  () =>
    !props.panelPresentation &&
    peerSidebarSameExtLabel.value === "nc" &&
    Boolean(folderIdForPeers.value),
);

/** plain 中的 .ncl 与 csv 使用等宽展示 */
const previewFetchedTextUseMonospace = computed(() => {
  if (previewVisualKind.value === "csv" || previewVisualKind.value === "netcdf") {
    return true;
  }
  if (previewVisualKind.value === "plain" && detail.value) {
    return normalizedFileExtension(detail.value) === "ncl";
  }
  return false;
});

const pdfBlobUrl = ref("");
const pdfPreviewLoading = ref(false);
const pdfPreviewError = ref("");
let pdfBlobAbortController: AbortController | null = null;

function revokePdfBlobUrl() {
  if (pdfBlobUrl.value) {
    URL.revokeObjectURL(pdfBlobUrl.value);
    pdfBlobUrl.value = "";
  }
}

function abortPdfBlobFetch() {
  pdfBlobAbortController?.abort();
  pdfBlobAbortController = null;
}

/** PDF 是否走本站 /download（用 fetch+Blob）；外链仍直接 iframe src */
const pdfPreviewUsesBackendBlob = computed(
  () => previewVisualKind.value === "pdf" && fileUsesBackendDownloadHref(mediaSourceURL.value),
);

const pdfIframeSrc = computed(() => {
  if (previewVisualKind.value !== "pdf") {
    return "";
  }
  const direct = absoluteDownloadURL.value;
  if (!direct) {
    return "";
  }
  if (pdfPreviewUsesBackendBlob.value) {
    return pdfBlobUrl.value;
  }
  return direct;
});

watch(
  () =>
    ({
      id: fileID.value,
      kind: previewVisualKind.value,
      fetchSrc: previewEmbedDownloadURL.value,
      useBlob: pdfPreviewUsesBackendBlob.value,
    }) as const,
  async ({ id, kind, fetchSrc, useBlob }) => {
    abortPdfBlobFetch();
    revokePdfBlobUrl();
    pdfPreviewLoading.value = false;
    pdfPreviewError.value = "";

    if (!id || kind !== "pdf" || !fetchSrc || !useBlob) {
      return;
    }

    pdfPreviewLoading.value = true;
    const ac = new AbortController();
    pdfBlobAbortController = ac;

    try {
      const res = await fetch(fetchSrc, { credentials: "include", signal: ac.signal });
      if (!res.ok) {
        pdfPreviewError.value = res.status === 403 ? "不允许访问该文件。" : "加载 PDF 失败。";
        return;
      }
      const cl = res.headers.get("content-length");
      if (cl != null) {
        const n = Number(cl);
        if (Number.isFinite(n) && n > PDF_PREVIEW_MAX_BYTES) {
          pdfPreviewError.value = `PDF 超过内嵌预览上限（约 ${Math.floor(PDF_PREVIEW_MAX_BYTES / 1024 / 1024)} MB），请下载或在新标签页打开。`;
          return;
        }
      }
      const buf = await res.arrayBuffer();
      if (buf.byteLength > PDF_PREVIEW_MAX_BYTES) {
        pdfPreviewError.value = `PDF 超过内嵌预览上限（约 ${Math.floor(PDF_PREVIEW_MAX_BYTES / 1024 / 1024)} MB），请下载或在新标签页打开。`;
        return;
      }
      pdfBlobUrl.value = URL.createObjectURL(new Blob([buf], { type: "application/pdf" }));
    } catch (e: unknown) {
      if (e instanceof DOMException && e.name === "AbortError") {
        return;
      }
      pdfPreviewError.value =
        "无法加载 PDF 预览（网络或浏览器限制）。请尝试「在新标签页打开」或使用下载。";
    } finally {
      pdfPreviewLoading.value = false;
    }
  },
);

function abortInlineTextPreview() {
  previewTextAbortController?.abort();
  previewTextAbortController = null;
}

function onPreviewImageError() {
  previewImageFailed.value = true;
}

watch(previewVisualKind, () => {
  previewImageFailed.value = false;
});

watch(
  () =>
    ({
      id: fileID.value,
      kind: previewVisualKind.value,
      src: previewEmbedDownloadURL.value,
    }) as const,
  async ({ id, kind, src }) => {
    abortInlineTextPreview();
    fetchedTextPreviewCollapsed.value = false;
    previewFetchedText.value = "";
    previewNetcdfStructure.value = null;
    previewFallbackUsed.value = false;
    previewTextError.value = "";
    previewTextTruncated.value = false;
    previewTextLoading.value = false;

    if (!id || !kind) {
      return;
    }

    if (kind === "netcdf") {
      previewTextLoading.value = true;
      const ac = new AbortController();
      previewTextAbortController = ac;
      try {
        const res = await httpClient.get<{
          text: string;
          truncated?: boolean;
          structure?: NetCDFDumpGroup;
        }>(`/public/files/${encodeURIComponent(id)}/netcdf-dump`, { signal: ac.signal });
        previewFetchedText.value = res.text ?? "";
        previewTextTruncated.value = Boolean(res.truncated);
        previewNetcdfStructure.value = res.structure ?? null;
      } catch (e: unknown) {
        if (e instanceof DOMException && e.name === "AbortError") {
          return;
        }
        // 主接口失败，尝试 ncdump 回退
        try {
          const fallbackRes = await httpClient.get<{
            text: string;
            truncated?: boolean;
          }>(`/public/files/${encodeURIComponent(id)}/netcdf-dump-fallback`, { signal: ac.signal });
          previewFetchedText.value = fallbackRes.text ?? "";
          previewTextTruncated.value = Boolean(fallbackRes.truncated);
          previewNetcdfStructure.value = null;
          previewFallbackUsed.value = true;
        } catch (fallbackErr: unknown) {
          if (fallbackErr instanceof DOMException && fallbackErr.name === "AbortError") {
            return;
          }
          if (e instanceof HttpError) {
            previewTextError.value =
              e.status === 403
                ? "不允许访问该文件。"
                : e.status === 400
                  ? readApiError(e, "无法读取 NetCDF 摘要（可能不是有效的 .nc 文件）。")
                  : "加载 NetCDF 摘要失败。";
          } else {
            previewTextError.value = "加载 NetCDF 摘要失败。";
          }
        }
      } finally {
        previewTextLoading.value = false;
      }
      return;
    }

    if (!TEXTUAL_PREVIEW_KINDS.has(kind)) {
      return;
    }
    if (!src) {
      previewTextError.value = "无法解析预览地址。";
      return;
    }

    previewTextLoading.value = true;
    const ac = new AbortController();
    previewTextAbortController = ac;

    try {
      const res = await fetch(src, { credentials: "include", signal: ac.signal });
      if (!res.ok) {
        previewTextError.value = res.status === 403 ? "不允许访问该文件。" : "加载预览失败。";
        return;
      }
      const cl = res.headers.get("content-length");
      if (cl != null) {
        const n = Number(cl);
        if (Number.isFinite(n) && n > PREVIEW_TEXT_MAX_BYTES) {
          previewTextError.value = `文件超过预览上限（约 ${Math.floor(PREVIEW_TEXT_MAX_BYTES / 1024)} KB），请下载后查看。`;
          return;
        }
      }
      const buf = await res.arrayBuffer();
      previewTextTruncated.value = buf.byteLength > PREVIEW_TEXT_MAX_BYTES;
      const slice = previewTextTruncated.value ? buf.slice(0, PREVIEW_TEXT_MAX_BYTES) : buf;
      previewFetchedText.value = new TextDecoder("utf-8", { fatal: false }).decode(slice);
    } catch (e: unknown) {
      if (e instanceof DOMException && e.name === "AbortError") {
        return;
      }
      previewTextError.value =
        "预览加载失败。若文件为外链存储或未允许跨域，请使用下载或通过直链在新标签打开。";
    } finally {
      previewTextLoading.value = false;
    }
  },
);

onBeforeUnmount(() => {
  abortInlineTextPreview();
  abortPdfBlobFetch();
  revokePdfBlobUrl();
  teardownVideoStageObserver();
  fileDescriptionResizeObserver?.disconnect();
  markdownPeekFileId.value = null;
  markdownCatalogNavigateHydrateGeneration += 1;
  markdownCatalogNavigateConfirmResolve?.(false);
  markdownCatalogNavigateConfirmResolve = null;
  markdownCatalogNavigateConfirmRoute.value = null;
  markdownCatalogNavigatePresentation.value = null;
  if (!props.panelPresentation && typeof document !== "undefined") {
    document.body.style.overflow = "";
  }
});

const folderIdForPeers = computed(() => detail.value?.folder_id?.trim() ?? "");

/** 临时收起右侧「同目录」列表以加宽容器内预览；换文件会自动恢复展开 */
const peerListAsideTempHidden = ref(false);

/** 有文件夹且为视频 / PDF / NetCDF 时加宽容器，并显示同目录同后缀列表（抽屉内不展示侧栏） */
const layoutWide = computed(
  () =>
    !props.panelPresentation &&
    Boolean(folderIdForPeers.value) &&
    peerSidebarSameExtLabel.value !== null,
);

const detailInnerMaxWidthClass = computed(() => {
  if (props.panelPresentation) {
    return "max-w-none";
  }
  return layoutWide.value ? "max-w-screen-2xl" : "max-w-6xl";
});

const showVideoPeerAsideExpanded = computed(
  () => layoutWide.value && !peerListAsideTempHidden.value,
);
const showPdfPeerAsideExpanded = computed(() => pdfPeerSidebar.value && !peerListAsideTempHidden.value);
const showNetcdfPeerAsideExpanded = computed(() => netcdfPeerSidebar.value && !peerListAsideTempHidden.value);

const peerSidebarCopy = computed(() => {
  switch (peerSidebarSameExtLabel.value) {
    case "video":
      return {
        title: "同文件夹视频",
        empty: "当前文件夹没有其他视频",
      };
    case "pdf":
      return {
        title: "同文件夹 PDF",
        empty: "当前文件夹没有其他 PDF",
      };
    case "nc":
      return {
        title: "同文件夹 NetCDF",
        empty: "当前文件夹没有其他 NetCDF",
      };
    default:
      return { title: "", empty: "" };
  }
});

const peerSidebarListIcon = computed(() => {
  switch (peerSidebarSameExtLabel.value) {
    case "video":
      return FileVideo;
    case "pdf":
      return NotebookText;
    case "nc":
      return Database;
    default:
      return NotebookText;
  }
});

const videoStageRef = ref<HTMLElement | null>(null);
const videoStageHeightPx = ref<number | null>(null);
let videoStageResizeObserver: ResizeObserver | null = null;

function teardownVideoStageObserver() {
  if (videoStageResizeObserver) {
    videoStageResizeObserver.disconnect();
    videoStageResizeObserver = null;
  }
}

function setupVideoStageObserver() {
  teardownVideoStageObserver();
  if (!layoutWide.value) {
    videoStageHeightPx.value = null;
    return;
  }
  void nextTick(() => {
    const el = videoStageRef.value;
    if (!el) {
      return;
    }
    const ro = new ResizeObserver((entries) => {
      const h = entries[0]?.contentRect.height;
      if (typeof h === "number" && h > 0) {
        videoStageHeightPx.value = Math.round(h);
      }
    });
    videoStageResizeObserver = ro;
    ro.observe(el);
  });
}

const peerAsideMaxStyle = computed((): Record<string, string> => {
  if (videoStageHeightPx.value == null) {
    return { maxHeight: "min(70vh, 720px)" };
  }
  return { maxHeight: `${videoStageHeightPx.value}px` };
});

watch(
  () => [isVideo.value, folderIdForPeers.value, detail.value?.id ?? ""] as const,
  ([video, fid]) => {
    if (video && fid) {
      void nextTick(() => setupVideoStageObserver());
    } else {
      teardownVideoStageObserver();
      videoStageHeightPx.value = null;
    }
  },
  { flush: "post" },
);

interface FolderFileListItem {
  id: string;
  name: string;
  extension: string;
}

const folderVideoPeers = ref<Array<{ id: string; name: string }>>([]);
const folderVideoPeersLoading = ref(false);

/** 视频详情页默认折叠「所属文件夹、下载量」等元数据，由用户点击「文件信息」展开 */
const videoFileMetaVisible = ref(false);

function extensionOfListItem(item: FolderFileListItem): string {
  const ext = (item.extension ?? "").replace(/^\./, "").toLowerCase();
  if (ext) {
    return ext;
  }
  const match = /\.([^.]+)$/.exec(item.name);
  return match ? match[1].toLowerCase() : "";
}

async function loadFolderSameExtensionPeers(folderID: string, currentFileId: string, ext: string) {
  const want = ext.replace(/^\./, "").toLowerCase();
  folderVideoPeersLoading.value = true;
  folderVideoPeers.value = [];
  try {
    const params = new URLSearchParams({
      page: "1",
      page_size: "100",
      sort: "name_asc",
    });
    const response = await httpClient.get<{ items: FolderFileListItem[] }>(
      `/public/folders/${encodeURIComponent(folderID)}/files?${params.toString()}`,
    );
    const items = response.items ?? [];
    folderVideoPeers.value = items
      .filter((f) => f.id !== currentFileId)
      .filter((f) => extensionOfListItem(f) === want)
      .map((f) => ({ id: f.id, name: f.name }));
  } catch {
    folderVideoPeers.value = [];
  } finally {
    folderVideoPeersLoading.value = false;
  }
}

async function loadFolderVideoPeers(folderID: string, currentFileId: string) {
  folderVideoPeersLoading.value = true;
  folderVideoPeers.value = [];
  try {
    const params = new URLSearchParams({
      page: "1",
      page_size: "100",
      sort: "name_asc",
    });
    const response = await httpClient.get<{ items: FolderFileListItem[] }>(
      `/public/folders/${encodeURIComponent(folderID)}/files?${params.toString()}`,
    );
    const items = response.items ?? [];
    folderVideoPeers.value = items
      .filter((f) => f.id !== currentFileId)
      .filter((f) => VIDEO_EXTENSIONS.has(extensionOfListItem(f)))
      .map((f) => ({ id: f.id, name: f.name }));
  } catch {
    folderVideoPeers.value = [];
  } finally {
    folderVideoPeersLoading.value = false;
  }
}

const descriptionHTML = computed(() => renderSimpleMarkdown(detail.value?.description ?? ""));

function updateFileDescriptionClampUI() {
  const el = fileDescriptionClampRef.value;
  if (!el) {
    fileDescriptionFooterVisible.value = false;
    return;
  }
  if (!descriptionHTML.value) {
    fileDescriptionFooterVisible.value = false;
    return;
  }
  if (fileDescriptionExpanded.value) {
    fileDescriptionFooterVisible.value = true;
    return;
  }
  fileDescriptionFooterVisible.value = el.scrollHeight > el.clientHeight + 2;
}

const fileDescriptionResizeObserver =
  typeof ResizeObserver !== "undefined"
    ? new ResizeObserver(() => {
        updateFileDescriptionClampUI();
      })
    : null;

/** 详情页顶部封面：优先 cover_url，否则简介内 ![cover](...) */
const feedbackSubmitDisabled = computed(() => feedbackSubmitting.value || !feedbackDescription.value.trim());
/** 资料目录面包屑（与返回文件夹同目标）；提至标题区展示，避免淹没在「文件信息」里 */
const detailFolderPathLabel = computed(() => (detail.value?.path || "主页根目录").trim() || "主页根目录");

const secondaryDetailRows = computed(() => {
  if (!detail.value) {
    return [];
  }

  const rows: Array<{ label: string; value: string }> = [
    { label: "下载量", value: String(detail.value.download_count) },
    { label: "文件大小", value: formatSize(detail.value.size) },
    { label: "更新时间", value: formatDate(detail.value.uploaded_at) },
  ];
  const rm = (detail.value.remark ?? "").trim();
  if (rm) {
    rows.splice(1, 0, { label: "备注", value: rm });
  }
  return rows;
});
const editorDirty = computed(() => {
  if (!detail.value) {
    return false;
  }

  return (
    editFileName.value.trim() !== detail.value.name ||
    editDescription.value.trim() !== (detail.value.description ?? "") ||
    editRemark.value.trim() !== (detail.value.remark ?? "").trim() ||
    editPlaybackUrl.value.trim() !== (detail.value.playback_url ?? "").trim() ||
    editPlaybackFallbackUrl.value.trim() !== (detail.value.playback_fallback_url ?? "").trim() ||
    editCoverUrl.value.trim() !== (detail.value.cover_url ?? "").trim() ||
    editDownloadPolicy.value !== (detail.value.download_policy ?? "inherit")
  );
});

const fileDescriptionPreviewHTML = computed(() => renderSimpleMarkdown(editDescription.value));

const fileDetailNeedsDownloadConfirm = computed(() => {
  const d = detail.value;
  if (!d || d.download_allowed === false) {
    return false;
  }
  return (d.size ?? 0) >= largeDownloadConfirmBytes.value;
});

const fileDownloadConfirmBody = computed(() => {
  const d = detail.value;
  if (!d) {
    return "";
  }
  return `该文件大小为 ${formatSizeBytes(d.size)}，已超过本站设定的大文件阈值（${formatSizeBytes(largeDownloadConfirmBytes.value)}）。确定要下载吗？`;
});

onMounted(() => {
  void Promise.all([loadDetail(), loadAdminPermission(), syncSessionReceiptCode(), loadLargeDownloadPolicy()]);
});

async function loadLargeDownloadPolicy() {
  try {
    const response = await httpClient.get<{ large_download_confirm_bytes: number }>("/public/download-policy");
    const b = Number(response.large_download_confirm_bytes);
    if (Number.isFinite(b) && b > 0) {
      largeDownloadConfirmBytes.value = b;
    }
  } catch {
    /* 默认 1 GiB */
  }
}

watch(
  fileDescriptionClampRef,
  (el, prev) => {
    if (!fileDescriptionResizeObserver) {
      return;
    }
    if (prev) {
      fileDescriptionResizeObserver.unobserve(prev);
    }
    if (el) {
      fileDescriptionResizeObserver.observe(el);
    }
  },
  { flush: "post" },
);

watch(
  () => [
    fileID.value,
    detail.value?.description,
    descriptionHTML.value,
    fileDescriptionExpanded.value,
  ],
  async () => {
    await nextTick();
    requestAnimationFrame(() => {
      updateFileDescriptionClampUI();
    });
  },
);

watch(
  () => fileID.value,
  () => {
    fileDescriptionExpanded.value = false;
  },
);

watch(fileID, () => {
  markdownPeekFileId.value = null;
  if (markdownCatalogNavigateConfirmRoute.value) {
    dismissMarkdownCatalogNavigateConfirm(false);
  }
  videoFileMetaVisible.value = false;
  void Promise.all([loadDetail(), loadAdminPermission(), syncSessionReceiptCode()]);
});

watch(markdownPeekFileId, (id, _prev, onCleanup) => {
  if (props.panelPresentation) {
    return;
  }
  syncMarkdownPeekBodyScrollLock();
  if (!id) {
    return;
  }
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === "Escape") {
      closeMarkdownPeekDrawer();
    }
  };
  window.addEventListener("keydown", onKeyDown);
  onCleanup(() => {
    window.removeEventListener("keydown", onKeyDown);
  });
});

watch(markdownCatalogNavigateConfirmRoute, (r, _p, onCleanup) => {
  if (!props.panelPresentation) {
    syncMarkdownPeekBodyScrollLock();
  }
  if (!r) {
    return;
  }
  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === "Escape") {
      dismissMarkdownCatalogNavigateConfirm(false);
    }
  };
  window.addEventListener("keydown", onKeyDown);
  onCleanup(() => {
    window.removeEventListener("keydown", onKeyDown);
  });
});

watch(
  () => [String(route.query.t ?? ""), fileID.value] as const,
  () => {
    void nextTick(() => {
      const el = videoRef.value;
      if (!el || el.readyState < HTMLMediaElement.HAVE_METADATA) {
        return;
      }
      applySeekFromRouteQuery();
    });
  },
);

async function loadDetail() {
  loading.value = true;
  error.value = "";
  detail.value = null;
  folderVideoPeers.value = [];
  folderVideoPeersLoading.value = false;
  try {
    detail.value = await httpClient.get<FileDetailResponse>(`/public/files/${encodeURIComponent(fileID.value)}`);
    if (detail.value) {
      editFileName.value = detail.value.name;
      editDescription.value = detail.value.description;
      editRemark.value = (detail.value.remark ?? "").trim();
      editPlaybackUrl.value = (detail.value.playback_url ?? "").trim();
      editPlaybackFallbackUrl.value = (detail.value.playback_fallback_url ?? "").trim();
      editCoverUrl.value = (detail.value.cover_url ?? "").trim();
      editDownloadPolicy.value = detail.value.download_policy ?? "inherit";
      const fid = detail.value.folder_id?.trim() ?? "";
      if (fid && !props.panelPresentation) {
        if (isVideoDetail(detail.value)) {
          void loadFolderVideoPeers(fid, detail.value.id);
        } else {
          const ext = normalizedFileExtension(detail.value);
          if (ext === "pdf" || ext === "nc") {
            void loadFolderSameExtensionPeers(fid, detail.value.id, ext);
          }
        }
      }
    }
  } catch (err: unknown) {
    if (err instanceof HttpError && err.status === 404) {
      error.value = "文件不存在或未公开。";
    } else {
      error.value = "加载文件详情失败。";
    }
  } finally {
    loading.value = false;
  }
}

async function loadAdminPermission() {
  canManageResourceDescriptions.value = await hasAdminPermission("resource_moderation");
}

function openDescriptionEditor() {
  editFileName.value = detail.value?.name ?? "";
  editDescription.value = detail.value?.description ?? "";
  editRemark.value = (detail.value?.remark ?? "").trim();
  editPlaybackUrl.value = (detail.value?.playback_url ?? "").trim();
  editPlaybackFallbackUrl.value = (detail.value?.playback_fallback_url ?? "").trim();
  editCoverUrl.value = (detail.value?.cover_url ?? "").trim();
  editDownloadPolicy.value = detail.value?.download_policy ?? "inherit";
  saveError.value = "";
  message.value = "";
  descriptionEditorOpen.value = true;
}

function closeDescriptionEditor() {
  descriptionEditorOpen.value = false;
  saving.value = false;
  saveError.value = "";
  editFileName.value = detail.value?.name ?? "";
  editDescription.value = detail.value?.description ?? "";
  editRemark.value = (detail.value?.remark ?? "").trim();
  editPlaybackUrl.value = (detail.value?.playback_url ?? "").trim();
  editPlaybackFallbackUrl.value = (detail.value?.playback_fallback_url ?? "").trim();
  editCoverUrl.value = (detail.value?.cover_url ?? "").trim();
  editDownloadPolicy.value = detail.value?.download_policy ?? "inherit";
}

async function openTagEditor() {
  if (!detail.value) {
    return;
  }
  tagEditorError.value = "";
  tagEditorOpen.value = true;
  tagEditorLoading.value = true;
  tagEditorSelected.value = (detail.value.tags ?? []).map((t) => t.id);
  try {
    tagCatalog.value = await fetchPublicFileTagDefinitions();
  } catch {
    tagEditorError.value = "加载标签列表失败。";
    tagCatalog.value = [];
  } finally {
    tagEditorLoading.value = false;
  }
}

function closeTagEditor() {
  tagEditorOpen.value = false;
  tagEditorError.value = "";
}

function toggleTagSelection(tagId: string) {
  const s = new Set(tagEditorSelected.value);
  if (s.has(tagId)) {
    s.delete(tagId);
  } else {
    s.add(tagId);
  }
  tagEditorSelected.value = [...s];
}

async function saveFileTags() {
  if (!detail.value) {
    return;
  }
  tagEditorSaving.value = true;
  tagEditorError.value = "";
  try {
    await httpClient.request(`/admin/resources/files/${encodeURIComponent(detail.value.id)}/tags`, {
      method: "PUT",
      body: { tag_ids: tagEditorSelected.value },
    });
    message.value = "标签已更新。";
    await loadDetail();
    closeTagEditor();
  } catch (err: unknown) {
    tagEditorError.value = readApiError(err, "保存标签失败。");
  } finally {
    tagEditorSaving.value = false;
  }
}

function openDeleteDialog() {
  deletePassword.value = "";
  deleteError.value = "";
  deleteMoveToTrash.value = true;
  deleteDialogOpen.value = true;
}

function openFeedbackModal() {
  feedbackDescription.value = "";
  feedbackError.value = "";
  feedbackMessage.value = "";
  feedbackModalOpen.value = true;
  void syncSessionReceiptCode();
}

function closeFeedbackModal() {
  feedbackModalOpen.value = false;
}

function closeFeedbackSuccessModal() {
  feedbackSuccessModalOpen.value = false;
}

function closeDeleteDialog() {
  deleteDialogOpen.value = false;
  deletePassword.value = "";
  deleteMoveToTrash.value = true;
  deleteError.value = "";
  deleteSubmitting.value = false;
}

async function saveDescription() {
  if (!detail.value || !editorDirty.value) return;
  const normalizedName = editFileName.value.trim();
  if (!normalizedName) {
    saveError.value = "请输入有效的文件名。";
    return;
  }
  saving.value = true;
  saveError.value = "";
  message.value = "";
  try {
    await httpClient.request(`/admin/resources/files/${encodeURIComponent(detail.value.id)}`, {
      method: "PUT",
      body: {
        name: normalizedName,
        description: editDescription.value.trim(),
        remark: editRemark.value.trim(),
        playback_url: editPlaybackUrl.value.trim(),
        playback_fallback_url: editPlaybackUrl.value.trim() ? editPlaybackFallbackUrl.value.trim() : "",
        cover_url: editCoverUrl.value.trim(),
        download_policy: editDownloadPolicy.value,
      },
    });
    message.value = "文件信息已更新。";
    await loadDetail();
    descriptionEditorOpen.value = false;
  } catch (err: unknown) {
    saveError.value = readApiError(err, "更新文件简介失败。");
  } finally {
    saving.value = false;
  }
}

async function confirmDeleteFile() {
  if (!detail.value) {
    return;
  }
  if (!deletePassword.value.trim()) {
    deleteError.value = "请输入当前管理员密码。";
    return;
  }

  deleteSubmitting.value = true;
  deleteError.value = "";
  try {
    await httpClient.request(`/admin/resources/files/${encodeURIComponent(detail.value.id)}`, {
      method: "DELETE",
      body: { password: deletePassword.value, move_to_trash: deleteMoveToTrash.value },
    });
    closeDeleteDialog();
    goBack();
  } catch (err: unknown) {
    deleteError.value = readApiError(err, "删除文件失败。");
  } finally {
    deleteSubmitting.value = false;
  }
}

async function submitFeedback() {
  if (!detail.value || !feedbackDescription.value.trim()) {
    return;
  }

  feedbackSubmitting.value = true;
  feedbackMessage.value = "";
  feedbackError.value = "";
  try {
    const response = await httpClient.post<{ receipt_code: string }>("/public/feedback", {
      file_id: detail.value.id,
      folder_id: "",
      description: feedbackDescription.value.trim(),
    });
    feedbackMessage.value = `反馈已提交，请保存回执码 ${response.receipt_code}。`;
    currentReceiptCode.value = response.receipt_code;
    window.sessionStorage.setItem("openshare_receipt_code", response.receipt_code);
    closeFeedbackModal();
    feedbackSuccessModalOpen.value = true;
  } catch (err: unknown) {
    if (err instanceof HttpError && err.status === 400) {
      feedbackError.value = "请填写问题说明。";
    } else if (err instanceof HttpError && err.status === 404) {
      feedbackError.value = "目标不存在或已删除。";
    } else {
      feedbackError.value = "提交反馈失败。";
    }
  } finally {
    feedbackSubmitting.value = false;
  }
}

async function syncSessionReceiptCode() {
  try {
    currentReceiptCode.value = await ensureSessionReceiptCode();
  } catch {
    currentReceiptCode.value = readStoredReceiptCode();
  }
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

function formatSize(size: number) {
  if (size < 1024) return `${size} B`;
  if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}

/** 用于下载确认等场景，与首页 `formatSize` 一致支持 GB */
function formatSizeBytes(n: number) {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(2)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(2)} MB`;
  return `${(n / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

function goBack() {
  if (props.panelPresentation) {
    emit("closePanel");
    return;
  }
  const folderID = detail.value?.folder_id?.trim() ?? "";
  if (folderID) {
    void router.push({ name: "public-home", query: { folder: folderID } });
    return;
  }
  void router.push({ name: "public-home" });
}

function leaveToPublicCatalogHome() {
  if (props.panelPresentation) {
    emit("leaveToPublicCatalog");
    return;
  }
  void router.push({ name: "public-home" });
}

function emitOpenFullPage() {
  emit("openFullPage");
}

function onPeerListNavigate(peerId: string) {
  if (props.panelPresentation) {
    emit("navigatePanelFile", peerId);
    return;
  }
  void router.push({ name: "public-file-detail", params: { fileID: peerId } });
}

function showLinkCopyHint(text: string) {
  if (linkCopyTimer) {
    clearTimeout(linkCopyTimer);
  }
  linkCopyHint.value = text;
  linkCopyTimer = setTimeout(() => {
    linkCopyHint.value = "";
    linkCopyTimer = null;
  }, 2800);
}

async function copyLink(label: string, url: string) {
  if (!url) {
    showLinkCopyHint("当前环境无法生成链接。");
    return;
  }
  const ok = await copyPlainTextToClipboard(url);
  if (ok) {
    showLinkCopyHint(`已复制${label}`);
  } else {
    showLinkCopyHint("复制失败，请手动长按或右键复制地址栏。");
  }
}

async function copyServerStoragePath() {
  const raw = (detail.value?.storage_path ?? "").trim();
  if (!raw) {
    showLinkCopyHint("暂无服务器磁盘路径。");
    return;
  }
  const ok = await copyPlainTextToClipboard(raw);
  if (ok) {
    showLinkCopyHint("已复制服务器路径");
  } else {
    showLinkCopyHint("复制失败，请手动复制。");
  }
}

async function copyDetailLinkAtCurrentTime() {
  const seconds = videoRef.value?.currentTime ?? 0;
  const url = buildAbsoluteDetailPageURL({ t: formatTimestampParam(seconds) });
  await copyLink("含时间戳的链接", url);
}

async function copyFetchedPreviewText() {
  let text = previewFetchedText.value;
  if (previewVisualKind.value === "netcdf" && previewNetcdfStructure.value) {
    text = netcdfStructureToMarkdown(previewNetcdfStructure.value);
  }
  if (!text) {
    showLinkCopyHint("没有可复制的内容。");
    return;
  }
  const ok = await copyPlainTextToClipboard(text);
  if (ok) {
    if (previewTextTruncated.value) {
      showLinkCopyHint(
        previewVisualKind.value === "netcdf"
          ? "已复制 NetCDF 摘要（可能已截断，完整结构请下载后用专业工具查看）"
          : "已复制预览内容（不含截断以外部分，请下载查看全文）",
      );
    } else {
      showLinkCopyHint("已复制预览内容");
    }
  } else {
    showLinkCopyHint("复制失败，请手动选中预览区文本后复制。");
  }
}

function downloadFile() {
  if (!downloadActionsAllowed.value) {
    return;
  }
  if (fileDetailNeedsDownloadConfirm.value) {
    downloadFileConfirmOpen.value = true;
    return;
  }
  performDownloadFile();
}

function closeDownloadFileConfirm() {
  downloadFileConfirmOpen.value = false;
}

function confirmDownloadFileFromModal() {
  downloadFileConfirmOpen.value = false;
  performDownloadFile();
}

function performDownloadFile() {
  if (!downloadActionsAllowed.value) {
    return;
  }
  const link = document.createElement("a");
  link.href = mediaSourceURL.value;
  link.rel = "noopener";
  if (mediaSourceURL.value.startsWith("http://") || mediaSourceURL.value.startsWith("https://")) {
    link.target = "_blank";
  }
  document.body.appendChild(link);
  link.click();
  link.remove();

  const usesBackendDownload = fileUsesBackendDownloadHref(mediaSourceURL.value);
  if (detail.value && usesBackendDownload) {
    detail.value = {
      ...detail.value,
      download_count: detail.value.download_count + 1,
    };
  }
}
</script>

<template>
  <section :class="panelPresentation ? 'px-3 py-3 sm:px-4 sm:py-4' : 'app-container py-2 sm:py-8 lg:py-8'">
    <div class="mx-auto w-full space-y-6" :class="detailInnerMaxWidthClass">
      <SurfaceCard>
        <p v-if="loading" class="text-sm text-slate-500">加载中…</p>

        <div v-else-if="error" class="space-y-4">
          <p class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{{ error }}</p>
          <div class="flex flex-col gap-3 sm:flex-row">
            <button type="button" class="btn-secondary w-full sm:w-auto" @click="goBack">返回上一页</button>
            <button type="button" class="btn-primary w-full sm:w-auto" @click="leaveToPublicCatalogHome">返回首页</button>
          </div>
        </div>

        <template v-else-if="detail">
          <p v-if="message" class="mb-5 rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">{{ message }}</p>
          <p
            v-if="linkCopyHint"
            class="mb-5 rounded-xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-700"
          >
            {{ linkCopyHint }}
          </p>

          <section>
            <div class="space-y-4">
              <div class="flex flex-col gap-4">
                <div class="min-w-0 w-full space-y-2">
                  <button
                    type="button"
                    class="flex w-full min-w-0 max-w-full cursor-pointer items-start gap-2.5 rounded-xl border border-slate-200/90 bg-gradient-to-br from-slate-50 to-slate-100/90 px-3 py-2.5 text-left shadow-[0_1px_2px_rgb(15_23_42/0.06)] ring-1 ring-slate-900/[0.04] transition hover:border-blue-200/90 hover:from-blue-50/80 hover:to-slate-50 hover:shadow-sm dark:border-slate-700 dark:from-slate-900 dark:to-slate-900/80 dark:hover:border-blue-800/70 dark:hover:from-slate-800/90"
                    :title="props.panelPresentation ? '关闭并回到目录' : '在资料目录中打开该文件夹'"
                    @click="goBack"
                  >
                    <Folder
                      class="mt-0.5 h-5 w-5 shrink-0 text-blue-600 dark:text-blue-400"
                      aria-hidden="true"
                    />
                    <span
                      class="min-w-0 flex-1 break-words text-base font-semibold leading-snug text-slate-900 [overflow-wrap:anywhere] dark:text-slate-100 sm:text-[1.0625rem]"
                      :title="detailFolderPathLabel"
                    >
                      {{ detailFolderPathLabel }}
                    </span>
                  </button>
                  <h3
                    class="min-w-0 break-words text-2xl font-semibold tracking-tight text-slate-900 [overflow-wrap:anywhere] sm:text-3xl"
                    :title="detail.name"
                  >
                    {{ detail.name }}
                  </h3>
                  <p
                    v-if="(detail.remark ?? '').trim()"
                    class="text-sm leading-relaxed text-slate-700"
                  >
                    {{ (detail.remark ?? "").trim() }}
                  </p>
                  <FileTagChips
                    v-if="(detail.tags?.length ?? 0) > 0"
                    :tags="detail.tags ?? []"
                    class="mt-3"
                    size="md"
                  />
                </div>
                <div class="w-full min-w-0">
                  <div
                    class="flex w-full flex-wrap items-center justify-start gap-2 py-1 sm:gap-3"
                  >
                  <button
                    v-if="panelPresentation"
                    type="button"
                    class="btn-primary shrink-0 whitespace-nowrap"
                    @click="emitOpenFullPage"
                  >
                    在单独页中显示
                  </button>
                  <button
                    v-if="isVideo"
                    type="button"
                    class="inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border border-slate-200 bg-white text-slate-600 transition-[transform,background-color,border-color,box-shadow,color] duration-200 hover:-translate-y-0.5 hover:border-slate-300 hover:bg-[#fafafa] hover:text-slate-900 hover:shadow-sm hover:shadow-slate-950/[0.08]"
                    :aria-label="videoFileMetaVisible ? '收起信息' : '文件信息'"
                    :aria-expanded="videoFileMetaVisible"
                    aria-controls="video-file-meta-panel"
                    @click="videoFileMetaVisible = !videoFileMetaVisible"
                  >
                    <FileText class="h-4 w-4" />
                  </button>
                  <button
                    v-if="canManageResourceDescriptions"
                    type="button"
                    class="btn-secondary shrink-0 whitespace-nowrap"
                    @click="openDescriptionEditor"
                  >
                    编辑
                  </button>
                  <button
                    v-if="canManageResourceDescriptions"
                    type="button"
                    class="btn-secondary shrink-0 whitespace-nowrap"
                    @click="openTagEditor"
                  >
                    标签
                  </button>
                  <button
                    v-if="canManageResourceDescriptions"
                    type="button"
                    class="btn-secondary shrink-0 whitespace-nowrap text-rose-600 hover:border-rose-200 hover:bg-rose-50 hover:text-rose-700"
                    @click="openDeleteDialog"
                  >
                    删除
                  </button>
                  <button
                    type="button"
                    class="inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border border-slate-200 bg-white text-slate-500 transition-[transform,background-color,border-color,box-shadow,color] duration-200 hover:-translate-y-0.5 hover:border-slate-300 hover:bg-[#fafafa] hover:text-slate-900 hover:shadow-sm hover:shadow-slate-950/[0.08]"
                    aria-label="反馈文件"
                    @click="openFeedbackModal"
                  >
                    <Flag class="h-4 w-4" />
                  </button>
                  <button
                    v-if="downloadActionsAllowed"
                    type="button"
                    class="inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border border-slate-200 bg-white text-slate-600 transition-[transform,background-color,border-color,box-shadow,color] duration-200 hover:-translate-y-0.5 hover:border-slate-300 hover:bg-[#fafafa] hover:text-slate-900 hover:shadow-sm hover:shadow-slate-950/[0.08]"
                    title="复制下载直链（可直接下载或嵌入播放器）"
                    aria-label="复制下载直链"
                    @click="copyLink('下载直链', absoluteDownloadURL)"
                  >
                    <Link2 class="h-4 w-4" />
                  </button>
                  <button
                    v-if="showNcCopyServerStoragePath"
                    type="button"
                    class="inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border border-slate-200 bg-white text-slate-600 transition-[transform,background-color,border-color,box-shadow,color] duration-200 hover:-translate-y-0.5 hover:border-slate-300 hover:bg-[#fafafa] hover:text-slate-900 hover:shadow-sm hover:shadow-slate-950/[0.08]"
                    title="复制文件在托管服务器磁盘上的路径（POSIX 路径，不是网页链接）"
                    aria-label="复制服务器磁盘路径"
                    @click="copyServerStoragePath"
                  >
                    <Server class="h-4 w-4" />
                  </button>
                  <button
                    type="button"
                    class="inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border border-slate-200 bg-white text-slate-600 transition-[transform,background-color,border-color,box-shadow,color] duration-200 hover:-translate-y-0.5 hover:border-slate-300 hover:bg-[#fafafa] hover:text-slate-900 hover:shadow-sm hover:shadow-slate-950/[0.08]"
                    title="复制本文件详情页链接（不含时间）"
                    aria-label="复制详情页链接"
                    @click="copyLink('详情页链接', absoluteDetailPageURL)"
                  >
                    <Share2 class="h-4 w-4" />
                  </button>
                  <button
                    v-if="isVideo"
                    type="button"
                    class="inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border border-slate-200 bg-white text-slate-600 transition-[transform,background-color,border-color,box-shadow,color] duration-200 hover:-translate-y-0.5 hover:border-slate-300 hover:bg-[#fafafa] hover:text-slate-900 hover:shadow-sm hover:shadow-slate-950/[0.08]"
                    title="复制带 ?t=秒数 的详情页链接（当前播放进度，类似哔哩哔哩）"
                    aria-label="复制含时间戳的详情页链接"
                    @click="copyDetailLinkAtCurrentTime"
                  >
                    <Clock class="h-4 w-4" />
                  </button>
                  <button
                    v-if="downloadActionsAllowed"
                    type="button"
                    class="inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border border-slate-200 bg-white text-slate-700 transition-[transform,background-color,border-color,box-shadow,color] duration-200 hover:-translate-y-0.5 hover:border-slate-300 hover:bg-[#fafafa] hover:text-slate-900 hover:shadow-sm hover:shadow-slate-950/[0.08]"
                    aria-label="下载文件"
                    @click="downloadFile"
                  >
                    <Download class="h-4 w-4" />
                  </button>
                  </div>
                </div>
              </div>

              <div
                v-show="!isVideo || videoFileMetaVisible"
                id="video-file-meta-panel"
                class="min-w-0 space-y-3"
              >
                <div class="grid gap-x-8 gap-y-3 sm:grid-cols-2 lg:grid-cols-3">
                  <div
                    v-for="item in secondaryDetailRows"
                    :key="item.label"
                    class="grid min-w-0 grid-cols-[88px_minmax(0,1fr)] items-baseline gap-x-3 text-sm"
                  >
                    <span class="text-slate-500">{{ item.label }}</span>
                    <span
                      class="min-w-0 truncate font-medium text-slate-900"
                      :title="item.value"
                    >
                      {{ item.value }}
                    </span>
                  </div>
                </div>
              </div>

              <div
                v-if="showFileDescriptionAbovePreview"
                class="rounded-3xl border border-slate-200 bg-white px-4 py-4 sm:px-5 sm:py-5"
              >
                <div v-if="descriptionHTML" class="space-y-3">
                  <div class="relative">
                    <div
                      ref="fileDescriptionClampRef"
                      class="markdown-content"
                      :class="!fileDescriptionExpanded ? 'max-h-[min(42vh,20rem)] overflow-hidden' : ''"
                      v-html="descriptionHTML"
                      @click.capture="handleMarkdownInternalLinkNavigate"
                    />
                    <div
                      v-if="!fileDescriptionExpanded && fileDescriptionFooterVisible"
                      class="pointer-events-none absolute bottom-0 left-0 right-0 h-14 bg-gradient-to-t from-white to-transparent"
                      aria-hidden="true"
                    />
                  </div>
                  <div v-if="fileDescriptionFooterVisible" class="flex justify-center sm:justify-start">
                    <button
                      type="button"
                      class="inline-flex min-h-10 items-center justify-center rounded-xl border border-slate-200 bg-white px-4 text-sm font-medium text-slate-800 shadow-sm ring-1 ring-slate-950/[0.04] transition hover:border-slate-300 hover:bg-slate-50"
                      @click="fileDescriptionExpanded = !fileDescriptionExpanded"
                    >
                      {{ fileDescriptionExpanded ? "收起简介" : "展开全文" }}
                    </button>
                  </div>
                </div>
                <p v-else class="text-sm text-slate-400">该文件暂无简介orz</p>
              </div>

              <div
                v-if="isVideo"
                :class="
                  showVideoPeerAsideExpanded
                    ? 'flex flex-col gap-4 lg:flex-row lg:items-start'
                    : 'relative min-w-0 w-full'
                "
              >
                <button
                  v-if="layoutWide && peerListAsideTempHidden"
                  type="button"
                  class="absolute bottom-3 right-3 z-10 inline-flex items-center gap-1 rounded-full border border-slate-700/40 bg-slate-950/85 px-2.5 py-1 text-xs font-medium text-white shadow-md backdrop-blur-sm transition hover:bg-slate-950"
                  aria-label="显示同目录列表"
                  title="展开同文件夹列表"
                  @click="peerListAsideTempHidden = false"
                >
                  <PanelRightOpen class="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                  同目录
                </button>
                <div
                  ref="videoStageRef"
                  class="min-w-0 overflow-hidden rounded-2xl border border-slate-200 bg-slate-950 shadow-inner ring-1 ring-black/5"
                  :class="showVideoPeerAsideExpanded ? 'flex-1 self-start' : 'w-full'"
                >
                  <video
                    :key="`${fileID}:${videoPlaybackStep}:${videoPlaybackActiveSrc}`"
                    ref="videoRef"
                    class="max-h-[min(70vh,720px)] w-full object-contain"
                    controls
                    playsinline
                    preload="metadata"
                    :src="videoPlaybackActiveSrc"
                    @error="onVideoPlaybackError"
                    @loadedmetadata="onVideoLoadedMetadata"
                  >
                    您的浏览器不支持内嵌视频播放，请使用上方下载按钮获取文件。
                  </video>
                </div>

                <aside
                  v-if="showVideoPeerAsideExpanded"
                  class="flex w-full min-h-0 shrink-0 flex-col overflow-hidden rounded-2xl border border-slate-200 bg-white lg:w-72 lg:self-start xl:w-80"
                  :style="peerAsideMaxStyle"
                >
                  <div class="flex shrink-0 items-center justify-between gap-2 border-b border-slate-100 px-3 py-2 sm:px-4">
                    <p class="min-w-0 text-sm font-medium text-slate-900">
                      {{ peerSidebarCopy.title }}
                    </p>
                    <button
                      type="button"
                      class="inline-flex shrink-0 items-center gap-1 rounded-lg border border-slate-200 bg-white px-2 py-1 text-xs font-medium text-slate-600 shadow-sm transition hover:bg-slate-50"
                      title="收起侧栏以加宽预览区域"
                      aria-label="收起同目录列表"
                      @click="peerListAsideTempHidden = true"
                    >
                      <PanelRightClose class="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                      <span class="hidden sm:inline">收起</span>
                    </button>
                  </div>
                  <div class="min-h-0 flex-1 overflow-y-auto px-2 py-2">
                    <p v-if="folderVideoPeersLoading" class="px-2 py-6 text-center text-sm text-slate-500">
                      加载列表…
                    </p>
                    <ul v-else-if="folderVideoPeers.length > 0" class="space-y-1">
                      <li v-for="peer in folderVideoPeers" :key="peer.id">
                        <button
                          v-if="panelPresentation"
                          type="button"
                          class="flex w-full min-w-0 items-start gap-2 rounded-xl px-2 py-2 text-left text-sm text-slate-700 transition hover:bg-slate-50 hover:text-slate-900"
                          @click="onPeerListNavigate(peer.id)"
                        >
                          <component
                            :is="peerSidebarListIcon"
                            class="mt-0.5 h-4 w-4 shrink-0 text-slate-400"
                            aria-hidden="true"
                          />
                          <span class="min-w-0 break-words leading-snug">{{ peer.name }}</span>
                        </button>
                        <RouterLink
                          v-else
                          :to="{ name: 'public-file-detail', params: { fileID: peer.id } }"
                          class="flex min-w-0 items-start gap-2 rounded-xl px-2 py-2 text-left text-sm text-slate-700 transition hover:bg-slate-50 hover:text-slate-900"
                        >
                          <component
                            :is="peerSidebarListIcon"
                            class="mt-0.5 h-4 w-4 shrink-0 text-slate-400"
                            aria-hidden="true"
                          />
                          <span class="min-w-0 break-words leading-snug">{{ peer.name }}</span>
                        </RouterLink>
                      </li>
                    </ul>
                    <p v-else class="px-2 py-6 text-center text-sm text-slate-500">
                      {{ peerSidebarCopy.empty }}
                    </p>
                  </div>
                </aside>
              </div>

              <div
                v-else-if="previewVisualKind === 'image' && previewEmbedDownloadURL"
                class="overflow-hidden rounded-2xl border border-slate-200 bg-slate-50 shadow-inner ring-1 ring-black/5"
              >
                <template v-if="!previewImageFailed">
                  <img
                    :key="`${fileID}:${previewEmbedDownloadURL}`"
                    :src="previewEmbedDownloadURL"
                    alt=""
                    class="max-h-[min(70vh,720px)] w-full object-contain"
                    loading="lazy"
                    decoding="async"
                    @error="onPreviewImageError"
                  />
                </template>
                <p v-else class="px-4 py-8 text-center text-sm text-slate-600">
                  无法在页面内预览该图片，请使用复制直链在新标签打开或下载查看。
                </p>
              </div>

              <div
                v-else-if="previewVisualKind === 'pdf' && absoluteDownloadURL"
                :class="[
                  showPdfPeerAsideExpanded ? 'flex flex-col gap-4 lg:flex-row lg:items-start' : '',
                  pdfPeerSidebar && peerListAsideTempHidden ? 'relative min-w-0 w-full' : '',
                ]"
              >
                <button
                  v-if="pdfPeerSidebar && peerListAsideTempHidden"
                  type="button"
                  class="absolute bottom-3 right-3 z-10 inline-flex items-center gap-1 rounded-full border border-slate-200 bg-white/95 px-2.5 py-1 text-xs font-medium text-slate-700 shadow-md backdrop-blur-sm transition hover:bg-slate-50"
                  aria-label="显示同目录列表"
                  title="展开同文件夹 PDF 列表"
                  @click="peerListAsideTempHidden = false"
                >
                  <PanelRightOpen class="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                  同目录
                </button>
                <div
                  class="overflow-hidden rounded-2xl border border-slate-200 bg-slate-100 shadow-inner ring-1 ring-black/5"
                  :class="showPdfPeerAsideExpanded && pdfPeerSidebar ? 'min-w-0 flex-1' : ''"
                >
                <div class="relative min-h-[min(70vh,720px)] w-full bg-white">
                  <p
                    v-if="pdfPreviewUsesBackendBlob && pdfPreviewLoading"
                    class="px-4 py-16 text-center text-sm text-slate-600"
                  >
                    正在加载 PDF…
                  </p>
                  <div v-else-if="pdfPreviewError" class="space-y-4 px-4 py-12 text-center">
                    <p class=" px-4 py-3 text-sm text-rose-700">{{ pdfPreviewError }}</p>
                    <a
                      :href="absoluteDownloadURL"
                      target="_blank"
                      rel="noopener noreferrer"
                      class="inline-block text-sm font-medium text-blue-600 underline hover:text-blue-800"
                    >
                      在新标签页打开 PDF
                    </a>
                  </div>
                  <iframe
                    v-else-if="pdfIframeSrc"
                    :key="`${fileID}:${pdfIframeSrc}`"
                    title="PDF 预览"
                    class="block min-h-[min(70vh,720px)] w-full border-0 bg-white"
                    :src="pdfIframeSrc"
                  />
                </div>
                </div>

                <aside
                  v-if="showPdfPeerAsideExpanded"
                  class="flex w-full min-h-0 shrink-0 flex-col overflow-hidden rounded-2xl border border-slate-200 bg-white lg:w-72 lg:self-start xl:w-80"
                  :style="peerAsideMaxStyle"
                >
                  <div class="flex shrink-0 items-center justify-between gap-2 border-b border-slate-100 px-3 py-2 sm:px-4">
                    <p class="min-w-0 text-sm font-medium text-slate-900">
                      {{ peerSidebarCopy.title }}
                    </p>
                    <button
                      type="button"
                      class="inline-flex shrink-0 items-center gap-1 rounded-lg border border-slate-200 bg-white px-2 py-1 text-xs font-medium text-slate-600 shadow-sm transition hover:bg-slate-50"
                      title="收起侧栏以加宽预览区域"
                      aria-label="收起同目录列表"
                      @click="peerListAsideTempHidden = true"
                    >
                      <PanelRightClose class="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                      <span class="hidden sm:inline">收起</span>
                    </button>
                  </div>
                  <div class="min-h-0 flex-1 overflow-y-auto px-2 py-2">
                    <p v-if="folderVideoPeersLoading" class="px-2 py-6 text-center text-sm text-slate-500">
                      加载列表…
                    </p>
                    <ul v-else-if="folderVideoPeers.length > 0" class="space-y-1">
                      <li v-for="peer in folderVideoPeers" :key="peer.id">
                        <button
                          v-if="panelPresentation"
                          type="button"
                          class="flex w-full min-w-0 items-start gap-2 rounded-xl px-2 py-2 text-left text-sm text-slate-700 transition hover:bg-slate-50 hover:text-slate-900"
                          @click="onPeerListNavigate(peer.id)"
                        >
                          <component
                            :is="peerSidebarListIcon"
                            class="mt-0.5 h-4 w-4 shrink-0 text-slate-400"
                            aria-hidden="true"
                          />
                          <span class="min-w-0 break-words leading-snug">{{ peer.name }}</span>
                        </button>
                        <RouterLink
                          v-else
                          :to="{ name: 'public-file-detail', params: { fileID: peer.id } }"
                          class="flex min-w-0 items-start gap-2 rounded-xl px-2 py-2 text-left text-sm text-slate-700 transition hover:bg-slate-50 hover:text-slate-900"
                        >
                          <component
                            :is="peerSidebarListIcon"
                            class="mt-0.5 h-4 w-4 shrink-0 text-slate-400"
                            aria-hidden="true"
                          />
                          <span class="min-w-0 break-words leading-snug">{{ peer.name }}</span>
                        </RouterLink>
                      </li>
                    </ul>
                    <p v-else class="px-2 py-6 text-center text-sm text-slate-500">
                      {{ peerSidebarCopy.empty }}
                    </p>
                  </div>
                </aside>
              </div>

              <div
                v-else-if="needsFetchedTextPreview"
                :class="[
                  showNetcdfPeerAsideExpanded ? 'flex flex-col gap-4 lg:flex-row lg:items-start' : '',
                  netcdfPeerSidebar && peerListAsideTempHidden ? 'relative min-w-0 w-full' : '',
                ]"
              >
              <button
                v-if="netcdfPeerSidebar && peerListAsideTempHidden"
                type="button"
                class="absolute bottom-3 right-3 z-10 inline-flex items-center gap-1 rounded-full border border-slate-200 bg-white/95 px-2.5 py-1 text-xs font-medium text-slate-700 shadow-md backdrop-blur-sm transition hover:bg-slate-50"
                aria-label="显示同目录列表"
                title="展开同文件夹 NetCDF 列表"
                @click="peerListAsideTempHidden = false"
              >
                <PanelRightOpen class="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                同目录
              </button>
              <div
                class="rounded-2xl border border-slate-200 bg-slate-50 shadow-inner ring-1 ring-black/5"
                :class="showNetcdfPeerAsideExpanded && netcdfPeerSidebar ? 'min-w-0 flex-1' : ''"
              >
                <div
                  class="flex cursor-pointer select-none flex-wrap items-center justify-between gap-2 border-b border-slate-100 px-4 py-2 transition hover:bg-slate-50/70"
                  role="button"
                  tabindex="0"
                  aria-label="展开或收起文件预览"
                  :aria-expanded="!fetchedTextPreviewCollapsed"
                  aria-controls="fetched-text-preview-panel"
                  @click="onFetchedPreviewHeaderClick"
                  @keydown.enter.prevent="toggleFetchedTextPreviewCollapsedKey"
                  @keydown.space.prevent="toggleFetchedTextPreviewCollapsedKey"
                >
                  <span class="flex min-w-0 flex-1 items-center gap-2">
                    <ChevronDown
                      class="h-4 w-4 shrink-0 text-slate-500 transition-transform duration-200 ease-out"
                      :class="{ '-rotate-90': fetchedTextPreviewCollapsed }"
                      aria-hidden="true"
                    />
                    <span class="text-xs font-medium uppercase tracking-[0.12em] text-slate-500">文件预览</span>
                  </span>
                  <button
                    v-if="!previewTextLoading && !previewTextError"
                    type="button"
                    class="shrink-0 rounded-lg border border-slate-200 bg-white px-2.5 py-1 text-xs font-medium text-slate-700 shadow-sm transition hover:bg-slate-50 disabled:cursor-not-allowed disabled:opacity-50"
                    :disabled="!previewFetchedText.length && !previewNetcdfStructure"
                    aria-label="复制预览区文本"
                    @click.stop="copyFetchedPreviewText"
                  >
                    复制
                  </button>
                </div>
                <div
                  v-show="!fetchedTextPreviewCollapsed"
                  id="fetched-text-preview-panel"
                >
                  <p v-if="previewTextLoading" class="text-sm text-slate-500">
                    {{ previewVisualKind === "netcdf" ? "正在加载 NetCDF 结构摘要…" : "正在加载预览内容…" }}
                  </p>
                  <p v-else-if="previewTextError" class="px-4 py-3 text-sm text-rose-700">{{ previewTextError }}</p>
                  <template v-else>
                    <p
                      v-if="previewFallbackUsed"
                      class="mb-3 rounded-lg border border-blue-200 bg-blue-50 px-3 py-2 text-xs text-blue-900"
                    >
                      该文件可能不完整，已通过 ncdump 回退方案读取头部信息；完整结构请下载后查看。
                    </p>
                    <p
                      v-if="previewTextTruncated"
                      class="mb-3 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-900"
                    >
                      <template v-if="previewVisualKind === 'netcdf'">
                        摘要因长度上限已截断；完整结构请下载后使用 ncdump、Python 等工具查看。
                      </template>
                      <template v-else>
                        文件较大，已截断至约 {{ Math.floor(PREVIEW_TEXT_MAX_BYTES / 1024) }} KB；完整内容请下载查看。
                      </template>
                    </p>
                    <div
                      v-if="previewVisualKind === 'markdown'"
                      class="max-h-[min(70vh,720px)] overflow-auto rounded-xl bg-white px-4 py-3 ring-1 ring-slate-950/[0.04]"
                    >
                      <p v-if="!previewFetchedText.trim()" class="text-sm text-slate-400">（空白文件）</p>
                      <div
                        v-else
                        class="markdown-content"
                        v-html="previewMarkdownRendered"
                        @click.capture="handleMarkdownInternalLinkNavigate"
                      />
                    </div>
                    <div
                      v-else-if="previewVisualKind === 'netcdf' && previewNetcdfStructure"
                      class="max-h-[min(70vh,720px)] overflow-auto rounded-xl bg-white px-4 py-3 ring-1 ring-slate-950/[0.04]"
                    >
                      <div class="markdown-content" v-html="previewNetcdfMarkdownHtml" @click.capture="handleMarkdownInternalLinkNavigate" />
                    </div>
                    <pre
                      v-else
                      class="max-h-[min(70vh,720px)] overflow-auto whitespace-pre-wrap break-words rounded-2xl bg-white px-4 py-3 text-slate-800 ring-1 ring-slate-950/[0.04]"
                      :class="previewFetchedTextUseMonospace ? 'font-mono text-xs leading-relaxed' : 'font-sans text-sm leading-relaxed'"
                    ><template v-if="previewFetchedText.length">{{ previewFetchedText }}</template><span v-else class="text-slate-400">（空白文件）</span></pre>
                  </template>
                </div>
              </div>

                <aside
                  v-if="showNetcdfPeerAsideExpanded"
                  class="flex w-full min-h-0 shrink-0 flex-col overflow-hidden rounded-2xl border border-slate-200 bg-white lg:w-72 lg:self-start xl:w-80"
                  :style="peerAsideMaxStyle"
                >
                  <div class="flex shrink-0 items-center justify-between gap-2 border-b border-slate-100 px-3 py-2 sm:px-4">
                    <p class="min-w-0 text-sm font-medium text-slate-900">
                      {{ peerSidebarCopy.title }}
                    </p>
                    <button
                      type="button"
                      class="inline-flex shrink-0 items-center gap-1 rounded-lg border border-slate-200 bg-white px-2 py-1 text-xs font-medium text-slate-600 shadow-sm transition hover:bg-slate-50"
                      title="收起侧栏以加宽预览区域"
                      aria-label="收起同目录列表"
                      @click="peerListAsideTempHidden = true"
                    >
                      <PanelRightClose class="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
                      <span class="hidden sm:inline">收起</span>
                    </button>
                  </div>
                  <div class="min-h-0 flex-1 overflow-y-auto px-2 py-2">
                    <p v-if="folderVideoPeersLoading" class="px-2 py-6 text-center text-sm text-slate-500">
                      加载列表…
                    </p>
                    <ul v-else-if="folderVideoPeers.length > 0" class="space-y-1">
                      <li v-for="peer in folderVideoPeers" :key="peer.id">
                        <button
                          v-if="panelPresentation"
                          type="button"
                          class="flex w-full min-w-0 items-start gap-2 rounded-xl px-2 py-2 text-left text-sm text-slate-700 transition hover:bg-slate-50 hover:text-slate-900"
                          @click="onPeerListNavigate(peer.id)"
                        >
                          <component
                            :is="peerSidebarListIcon"
                            class="mt-0.5 h-4 w-4 shrink-0 text-slate-400"
                            aria-hidden="true"
                          />
                          <span class="min-w-0 break-words leading-snug">{{ peer.name }}</span>
                        </button>
                        <RouterLink
                          v-else
                          :to="{ name: 'public-file-detail', params: { fileID: peer.id } }"
                          class="flex min-w-0 items-start gap-2 rounded-xl px-2 py-2 text-left text-sm text-slate-700 transition hover:bg-slate-50 hover:text-slate-900"
                        >
                          <component
                            :is="peerSidebarListIcon"
                            class="mt-0.5 h-4 w-4 shrink-0 text-slate-400"
                            aria-hidden="true"
                          />
                          <span class="min-w-0 break-words leading-snug">{{ peer.name }}</span>
                        </RouterLink>
                      </li>
                    </ul>
                    <p v-else class="px-2 py-6 text-center text-sm text-slate-500">
                      {{ peerSidebarCopy.empty }}
                    </p>
                  </div>
                </aside>
              </div>
            </div>

            <div
              v-if="!showFileDescriptionAbovePreview"
              class="mt-4 rounded-3xl border border-slate-200 bg-white px-4 py-4 sm:px-5 sm:py-5"
            >
              <div v-if="descriptionHTML" class="space-y-3">
                <div class="relative">
                  <div
                    ref="fileDescriptionClampRef"
                    class="markdown-content"
                    :class="!fileDescriptionExpanded ? 'max-h-[min(42vh,20rem)] overflow-hidden' : ''"
                    v-html="descriptionHTML"
                    @click.capture="handleMarkdownInternalLinkNavigate"
                  />
                  <div
                    v-if="!fileDescriptionExpanded && fileDescriptionFooterVisible"
                    class="pointer-events-none absolute bottom-0 left-0 right-0 h-14 bg-gradient-to-t from-white to-transparent"
                    aria-hidden="true"
                  />
                </div>
                <div v-if="fileDescriptionFooterVisible" class="flex justify-center sm:justify-start">
                  <button
                    type="button"
                    class="inline-flex min-h-10 items-center justify-center rounded-xl border border-slate-200 bg-white px-4 text-sm font-medium text-slate-800 shadow-sm ring-1 ring-slate-950/[0.04] transition hover:border-slate-300 hover:bg-slate-50"
                    @click="fileDescriptionExpanded = !fileDescriptionExpanded"
                  >
                    {{ fileDescriptionExpanded ? "收起简介" : "展开全文" }}
                  </button>
                </div>
              </div>
              <p v-else class="text-sm text-slate-400">该文件暂无简介orz</p>
            </div>
          </section>
        </template>
      </SurfaceCard>
    </div>

    <Teleport to="body">
      <Transition name="modal-shell">
      <div v-if="deleteDialogOpen && detail" class="fixed inset-0 z-[120] flex items-center justify-center bg-slate-950/30 px-4">
        <div class="modal-card w-full max-w-md rounded-2xl bg-white p-6 shadow-xl">
          <div>
            <h3 class="text-lg font-semibold text-slate-900">确认删除文件</h3>
            <p class="mt-2 text-sm leading-6 text-slate-500">
              <template v-if="deleteMoveToTrash">
                将移动到该文件所在磁盘根目录下的 <span class="font-medium text-slate-800">trash</span> 文件夹，可从文件系统中找回。
              </template>
              <template v-else>
                将直接从磁盘<strong class="text-rose-700">彻底删除</strong>，无法恢复。
              </template>
              确认删除
              <span class="font-medium text-slate-900">{{ detail.name }}</span>
              吗？
            </p>
          </div>
          <div class="mt-6 space-y-4">
            <div class="space-y-2 rounded-xl border border-slate-200 bg-slate-50/80 px-4 py-3">
              <label class="flex cursor-pointer items-start gap-3 text-sm text-slate-700">
                <input v-model="deleteMoveToTrash" type="radio" class="mt-1" :value="true" />
                <span>移动到垃圾桶（写入所在磁盘根目录的 <code class="rounded bg-white px-1 text-xs">trash</code>）</span>
              </label>
              <label class="flex cursor-pointer items-start gap-3 text-sm text-slate-700">
                <input v-model="deleteMoveToTrash" type="radio" class="mt-1" :value="false" />
                <span>彻底删除（不经过垃圾桶，不可恢复）</span>
              </label>
            </div>
            <input v-model="deletePassword" type="password" class="field" placeholder="输入当前管理员密码确认删除" />
            <p v-if="deleteError" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
              {{ deleteError }}
            </p>
            <div class="flex justify-end gap-3">
              <button type="button" class="btn-secondary" @click="closeDeleteDialog">取消</button>
              <button
                type="button"
                class="inline-flex h-11 items-center rounded-xl bg-rose-600 px-5 text-sm font-medium text-white transition hover:bg-rose-700"
                :disabled="deleteSubmitting"
                @click="confirmDeleteFile"
              >
                {{ deleteSubmitting ? "删除中…" : "确认删除" }}
              </button>
            </div>
          </div>
        </div>
      </div>
      </Transition>
    </Teleport>

    <Teleport to="body">
      <Transition name="modal-shell">
      <div
        v-if="downloadFileConfirmOpen && detail"
        class="fixed inset-0 z-[125] flex items-center justify-center bg-slate-950/30 px-4"
        @click.self="closeDownloadFileConfirm"
      >
        <div class="modal-card w-full max-w-md rounded-2xl bg-white p-6 shadow-xl" @click.stop>
          <h3 class="text-lg font-semibold text-slate-900">确认下载</h3>
          <p class="mt-3 text-sm leading-6 text-slate-600">{{ fileDownloadConfirmBody }}</p>
          <div class="mt-6 flex flex-wrap justify-end gap-3">
            <button type="button" class="btn-secondary" @click="closeDownloadFileConfirm">取消</button>
            <button type="button" class="btn-primary" @click="confirmDownloadFileFromModal">确认下载</button>
          </div>
        </div>
      </div>
      </Transition>
    </Teleport>

    <Teleport to="body">
      <Transition name="modal-shell">
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
      <div v-if="feedbackModalOpen && detail" class="fixed inset-0 z-[120] bg-slate-950/40 backdrop-blur-sm">
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
              <div class="rounded-2xl border border-slate-200 bg-[#fafafafa] px-4 py-3">
                <p class="text-xs font-semibold uppercase tracking-[0.12em] text-slate-400">当前对象</p>
                <p class="mt-1 text-sm leading-6 text-slate-700">{{ detail.name }}</p>
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

              <p v-if="feedbackMessage" class="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">
                {{ feedbackMessage }}
              </p>
              <p v-if="feedbackError" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
                {{ feedbackError }}
              </p>

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
      <div v-if="descriptionEditorOpen" class="fixed inset-0 z-[120] overflow-y-auto bg-slate-950/40 backdrop-blur-sm">
        <div class="flex min-h-[100dvh] justify-center px-4 py-6 sm:py-10">
          <div
            class="modal-card panel relative my-auto flex w-full max-w-5xl max-h-[min(90dvh,calc(100dvh-2.5rem))] min-h-0 flex-col overflow-hidden p-6"
          >
            <div class="shrink-0 border-b border-slate-200 pb-4">
              <div class="flex flex-wrap items-center justify-between gap-3">
                <h3 class="text-lg font-semibold text-slate-900">编辑文件信息</h3>
                <div class="flex shrink-0 flex-wrap justify-end gap-3">
                  <button type="button" class="btn-secondary" @click="closeDescriptionEditor">取消</button>
                  <button type="button" class="btn-primary" :disabled="saving || !editorDirty" @click="saveDescription">
                    {{ saving ? "保存中…" : "保存更改" }}
                  </button>
                </div>
              </div>
            </div>

            <div class="min-h-0 flex-1 overflow-y-auto overscroll-contain pt-5 [-webkit-overflow-scrolling:touch]">
              <div class="space-y-4 pb-2">
              <label class="space-y-2">
                <span class="text-sm font-medium text-slate-700">文件名</span>
                <input
                  v-model="editFileName"
                  class="field"
                  :disabled="!canManageResourceDescriptions"
                  placeholder="输入完整文件名，例如 example.xlsx"
                />
              </label>

              <label class="space-y-2">
                <span class="text-sm font-medium text-slate-700">备注（单行）</span>
                <input
                  v-model="editRemark"
                  type="text"
                  maxlength="500"
                  class="field"
                  placeholder="展示在首页卡片副标题，不支持换行与 Markdown"
                  autocomplete="off"
                />
              </label>

              <div
                class="grid min-h-0 grid-cols-1 gap-4 lg:min-h-[28rem] lg:grid-cols-2 lg:grid-rows-[auto_minmax(17rem,1fr)]"
              >
                <span class="order-1 text-sm font-medium text-slate-700 lg:order-none lg:col-start-1 lg:row-start-1">
                  简介（Markdown）
                </span>
                <textarea
                  v-model="editDescription"
                  class="field-area order-2 min-h-[17rem] w-full resize-y rounded-3xl lg:order-none lg:col-start-1 lg:row-start-2 lg:h-full lg:min-h-0 lg:resize-none"
                  rows="10"
                  placeholder="仅在文件详情页展示；支持简单 Markdown。"
                />
                <div class="order-3 shrink-0 lg:order-none lg:col-start-2 lg:row-start-1">
                  <h4 class="text-sm font-medium text-slate-700">简介预览</h4>
                </div>
                <div
                  class="order-4 flex min-h-[17rem] flex-col overflow-hidden rounded-3xl border border-slate-200 bg-white lg:order-none lg:col-start-2 lg:row-start-2 lg:h-full lg:min-h-0"
                >
                  <div class="min-h-0 flex-1 overflow-y-auto px-5 py-5">
                    <div
                      v-if="fileDescriptionPreviewHTML"
                      class="markdown-content"
                      v-html="fileDescriptionPreviewHTML"
                      @click.capture="handleMarkdownInternalLinkNavigate"
                    />
                    <p v-else class="text-sm text-slate-400">这里会显示简介预览。</p>
                  </div>
                </div>
              </div>

              <label class="space-y-2">
                <span class="text-sm font-medium text-slate-700">封面图地址（可选）</span>
                <input
                  v-model="editCoverUrl"
                  type="url"
                  class="field"
                  placeholder="https://cdn.example.com/cover.jpg（留空则使用简介中 ![cover](...)）"
                  autocomplete="off"
                />
                <p class="text-xs leading-5 text-slate-500">
                  填写后优先作为首页列表与详情顶部封面；需以 http(s) 开头。清空并保存则回退到简介内封面语法。
                </p>
              </label>

              <label class="space-y-2">
                <span class="text-sm font-medium text-slate-700">播放 / 下载直链（可选）</span>
                <input
                  v-model="editPlaybackUrl"
                  type="url"
                  class="field"
                  placeholder="https://cdn.example.com/path/video.mp4（留空则使用本站下载接口）"
                  autocomplete="off"
                />
                <p class="text-xs leading-5 text-slate-500">
                  填写后，详情页播放器与「复制下载直链」均使用该地址；需以 http(s) 开头。清空并保存可恢复为默认。
                </p>
              </label>

              <label class="space-y-2">
                <span class="text-sm font-medium text-slate-700">是否允许下载</span>
                <select v-model="editDownloadPolicy" class="field">
                  <option value="inherit">继承所在文件夹设置</option>
                  <option value="allow">允许下载</option>
                  <option value="deny">禁止下载</option>
                </select>
                <p class="text-xs leading-5 text-slate-500">
                  「继承」时随文件夹策略；未单独设置文件与文件夹时默认允许下载。
                </p>
              </label>

              <label v-if="editPlaybackUrl.trim()" class="space-y-2">
                <span class="text-sm font-medium text-slate-700">备用播放直链（可选）</span>
                <input
                  v-model="editPlaybackFallbackUrl"
                  type="url"
                  class="field"
                  placeholder="主直链失效时播放器会优先尝试此地址，再回退文件夹前缀与本站下载"
                  autocomplete="off"
                />
                <p class="text-xs leading-5 text-slate-500">
                  仅用于内嵌播放器；复制下载仍使用上方主直链逻辑。需以 http(s) 开头；无单独配置主直链时不可填。
                </p>
              </label>

              <p v-if="saveError" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
                {{ saveError }}
              </p>
              </div>
            </div>
          </div>
        </div>
      </div>
      </Transition>
    </Teleport>

    <Teleport to="body">
      <Transition name="modal-shell">
        <div
          v-if="tagEditorOpen"
          class="fixed inset-0 z-[121] overflow-y-auto bg-slate-950/40 backdrop-blur-sm"
        >
          <div class="flex min-h-[100dvh] justify-center px-4 py-6 sm:py-10">
            <div
              class="modal-card panel relative my-auto flex w-full max-w-lg flex-col overflow-hidden p-6"
              role="dialog"
              aria-modal="true"
              aria-labelledby="tag-editor-title"
            >
              <h3 id="tag-editor-title" class="text-lg font-semibold text-slate-900">文件标签</h3>
              <div class="mt-4 min-h-[8rem]">
                <p v-if="tagEditorLoading" class="text-sm text-slate-500">加载中…</p>
                <p v-else-if="tagCatalog.length === 0 && !tagEditorError" class="text-sm text-slate-500">
                  暂无预设标签。请先在管理后台「文件标签」中添加。
                </p>
                <ul v-else class="max-h-[min(50vh,20rem)] space-y-2 overflow-y-auto overscroll-contain pr-1">
                  <li v-for="t in tagCatalog" :key="t.id">
                    <label
                      class="flex cursor-pointer items-center gap-3 rounded-xl border border-slate-200 px-3 py-2.5 transition hover:border-slate-300"
                    >
                      <input
                        type="checkbox"
                        class="h-4 w-4 rounded border-slate-300"
                        :checked="tagEditorSelected.includes(t.id)"
                        @change="toggleTagSelection(t.id)"
                      />
                      <span
                        class="inline-flex max-w-full items-center rounded-md px-2 py-0.5 text-xs font-medium ring-1 ring-black/10"
                        :style="{
                          backgroundColor: t.color,
                          color: readableTextColorForPreset(t.color),
                        }"
                      >{{ t.name }}</span>
                    </label>
                  </li>
                </ul>
              </div>
              <p v-if="tagEditorError" class="mt-3 rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">
                {{ tagEditorError }}
              </p>
              <div class="mt-6 flex flex-wrap justify-end gap-3">
                <button type="button" class="btn-secondary" @click="closeTagEditor">取消</button>
                <button
                  type="button"
                  class="btn-primary"
                  :disabled="tagEditorSaving || tagEditorLoading"
                  @click="saveFileTags"
                >
                  {{ tagEditorSaving ? "保存中…" : "保存" }}
                </button>
              </div>
            </div>
          </div>
        </div>
      </Transition>
    </Teleport>

    <Teleport to="body">
      <Transition name="modal-shell">
        <div
          v-if="markdownCatalogNavigateConfirmRoute"
          class="fixed inset-0 z-[126] flex items-center justify-center bg-slate-950/35 px-4 backdrop-blur-[1px]"
          @click.self="dismissMarkdownCatalogNavigateConfirm(false)"
        >
          <div
            class="modal-card panel w-full max-w-lg rounded-2xl bg-white p-6 shadow-xl dark:bg-slate-900"
            role="alertdialog"
            aria-modal="true"
            aria-labelledby="md-catalog-confirm-title"
            @click.stop
          >
            <h3 id="md-catalog-confirm-title" class="text-lg font-semibold text-slate-900 dark:text-slate-100">
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
              <div
                v-else-if="
                  markdownCatalogNavigatePresentation.variant === 'folder'
                    && markdownCatalogNavigatePresentation.folderDetailLoaded
                "
                class="mt-3 max-h-[min(62vh,28rem)] space-y-3 overflow-y-auto pr-1 text-sm leading-relaxed"
              >
                <div>
                  <p class="text-xs font-semibold uppercase tracking-[0.12em] text-slate-500 dark:text-slate-400">
                    文件夹名
                  </p>
                  <p class="mt-1 font-semibold text-slate-900 dark:text-slate-100">
                    {{ markdownCatalogNavigatePresentation.headline }}
                  </p>
                </div>
                <div v-if="markdownCatalogNavigatePresentation.folderPathTrail">
                  <p class="text-xs font-semibold uppercase tracking-[0.12em] text-slate-500 dark:text-slate-400">
                    路径
                  </p>
                  <p class="mt-1 text-slate-700 dark:text-slate-300">
                    {{ markdownCatalogNavigatePresentation.folderPathTrail }}
                  </p>
                </div>
                <div v-if="markdownCatalogNavigatePresentation.folderIdLine">
                  <p class="text-xs font-semibold uppercase tracking-[0.12em] text-slate-500 dark:text-slate-400">
                    标识
                  </p>
                  <p class="mt-1 font-mono text-xs text-slate-600 dark:text-slate-400">
                    {{ markdownCatalogNavigatePresentation.folderIdLine }}
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
              <div v-else class="mt-3 space-y-2">
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
              </div>
              <p class="mt-4 text-sm text-slate-500 dark:text-slate-400">
                将从当前视图切换到上方所示目录的资料列表（相当于在首页打开对应文件夹）。
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

    <Teleport to="body">
      <Transition name="file-detail-drawer-shell">
        <div v-if="markdownPeekFileId" class="fixed inset-0 z-[118]">
          <div
            class="absolute inset-0 bg-slate-950/40 backdrop-blur-[1px]"
            aria-hidden="true"
            @click="closeMarkdownPeekDrawer"
          />
          <div
            role="dialog"
            aria-modal="true"
            aria-label="文件详情预览"
            class="file-detail-drawer-panel absolute right-0 top-0 flex h-full w-[min(100vw,50rem)] min-w-0 flex-col overflow-hidden border-l border-slate-200 bg-[#fafafa] shadow-[0_0_0_1px_rgba(15,23,42,0.06),-12px_0_48px_-24px_rgba(15,23,42,0.25)] dark:border-slate-800 dark:bg-slate-950"
            @click.stop
          >
            <PublicFileDetailPeek
              class="flex-1 min-h-0 overflow-x-hidden overflow-y-auto"
              :override-file-id="markdownPeekFileId"
              panel-presentation
              @close-panel="closeMarkdownPeekDrawer"
              @open-full-page="onMarkdownPeekOpenFullPage"
              @navigate-panel-file="onMarkdownPeekNavigate"
              @leave-to-public-catalog="onMarkdownPeekLeaveCatalog"
            />
          </div>
        </div>
      </Transition>
    </Teleport>
  </section>
</template>
