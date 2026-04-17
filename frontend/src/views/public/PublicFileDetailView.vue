<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
import { ChevronLeft, Clock, Download, FileText, FileVideo, Flag, Link2, Share2 } from "lucide-vue-next";

import SurfaceCard from "../../components/ui/SurfaceCard.vue";
import { HttpError, httpClient } from "../../lib/http/client";
import { readApiError } from "../../lib/http/helpers";
import { ensureSessionReceiptCode, readStoredReceiptCode } from "../../lib/receiptCode";
import { hasAdminPermission } from "../../lib/admin/session";
import { fileEffectiveDownloadHref, fileUsesBackendDownloadHref } from "../../lib/fileDirectUrl";
import { fileCoverImageHrefFromFields, renderSimpleMarkdown } from "../../lib/markdown";

interface FileDetailResponse {
  id: string;
  name: string;
  extension: string;
  folder_id: string;
  path: string;
  description: string;
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
}

const route = useRoute();
const router = useRouter();
const detail = ref<FileDetailResponse | null>(null);
const loading = ref(false);
const error = ref("");
const message = ref("");
const saveError = ref("");
const saving = ref(false);
const editFileName = ref("");
const editDescription = ref("");
const editPlaybackUrl = ref("");
const editPlaybackFallbackUrl = ref("");
const editCoverUrl = ref("");
const editDownloadPolicy = ref<"inherit" | "allow" | "deny">("inherit");
const descriptionEditorOpen = ref(false);
const canManageResourceDescriptions = ref(false);
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
const fileID = computed(() => String(route.params.fileID ?? ""));
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
  void nextTick(() => applySeekFromRouteQuery());
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

const folderIdForPeers = computed(() => detail.value?.folder_id?.trim() ?? "");

/** 有文件夹且为视频时拉取同目录列表，用于加宽布局 */
const layoutWide = computed(() => isVideo.value && Boolean(folderIdForPeers.value));

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
/** 详情页顶部封面：优先 cover_url，否则简介内 ![cover](...) */
const detailCoverImageHref = computed(() =>
  detail.value ? fileCoverImageHrefFromFields(detail.value.cover_url, detail.value.description ?? "") : null,
);
const feedbackSubmitDisabled = computed(() => feedbackSubmitting.value || !feedbackDescription.value.trim());
const primaryDetailRows = computed(() => {
  if (!detail.value) {
    return [];
  }

  return [{ label: "所属文件夹", value: detail.value.path || "主页根目录" }];
});
const secondaryDetailRows = computed(() => {
  if (!detail.value) {
    return [];
  }

  return [
    { label: "下载量", value: String(detail.value.download_count) },
    { label: "文件大小", value: formatSize(detail.value.size) },
    { label: "更新时间", value: formatDate(detail.value.uploaded_at) },
  ];
});
const editorDirty = computed(() => {
  if (!detail.value) {
    return false;
  }

  return (
    editFileName.value.trim() !== detail.value.name ||
    editDescription.value.trim() !== (detail.value.description ?? "") ||
    editPlaybackUrl.value.trim() !== (detail.value.playback_url ?? "").trim() ||
    editPlaybackFallbackUrl.value.trim() !== (detail.value.playback_fallback_url ?? "").trim() ||
    editCoverUrl.value.trim() !== (detail.value.cover_url ?? "").trim() ||
    editDownloadPolicy.value !== (detail.value.download_policy ?? "inherit")
  );
});

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

watch(fileID, () => {
  videoFileMetaVisible.value = false;
  void Promise.all([loadDetail(), loadAdminPermission(), syncSessionReceiptCode()]);
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
      editPlaybackUrl.value = (detail.value.playback_url ?? "").trim();
      editPlaybackFallbackUrl.value = (detail.value.playback_fallback_url ?? "").trim();
      editCoverUrl.value = (detail.value.cover_url ?? "").trim();
      editDownloadPolicy.value = detail.value.download_policy ?? "inherit";
      if (isVideoDetail(detail.value)) {
        const fid = detail.value.folder_id?.trim() ?? "";
        if (fid) {
          void loadFolderVideoPeers(fid, detail.value.id);
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
  editPlaybackUrl.value = (detail.value?.playback_url ?? "").trim();
  editPlaybackFallbackUrl.value = (detail.value?.playback_fallback_url ?? "").trim();
  editCoverUrl.value = (detail.value?.cover_url ?? "").trim();
  editDownloadPolicy.value = detail.value?.download_policy ?? "inherit";
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
  const folderID = detail.value?.folder_id?.trim() ?? "";
  if (folderID) {
    void router.push({ name: "public-home", query: { folder: folderID } });
    return;
  }
  void router.push({ name: "public-home" });
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
  try {
    await navigator.clipboard.writeText(url);
    showLinkCopyHint(`已复制${label}`);
  } catch {
    showLinkCopyHint("复制失败，请手动长按或右键复制地址栏。");
  }
}

async function copyDetailLinkAtCurrentTime() {
  const seconds = videoRef.value?.currentTime ?? 0;
  const url = buildAbsoluteDetailPageURL({ t: formatTimestampParam(seconds) });
  await copyLink("含时间戳的链接", url);
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
  <section class="app-container py-2 sm:py-8 lg:py-10">
    <div class="mx-auto w-full space-y-6" :class="layoutWide ? 'max-w-6xl' : 'max-w-4xl'">
      <SurfaceCard>
        <p v-if="loading" class="text-sm text-slate-500">加载中…</p>

        <div v-else-if="error" class="space-y-4">
          <p class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{{ error }}</p>
          <div class="flex flex-col gap-3 sm:flex-row">
            <button type="button" class="btn-secondary w-full sm:w-auto" @click="goBack">返回上一页</button>
            <button type="button" class="btn-primary w-full sm:w-auto" @click="$router.push({ name: 'public-home' })">返回首页</button>
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
              <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
                <div class="space-y-2">
                  <div class="flex items-center gap-2">
                    <button
                      type="button"
                      class="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-slate-200 bg-white text-slate-600 transition hover:border-slate-300 hover:bg-slate-50 hover:text-slate-900 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-400 dark:hover:border-slate-600 dark:hover:bg-slate-800 dark:hover:text-slate-100"
                      aria-label="返回"
                      @click="goBack"
                    >
                      <ChevronLeft class="h-4 w-4" />
                    </button>
                    <p class="text-xs font-semibold uppercase tracking-[0.18em] text-blue-600">File Info</p>
                  </div>
                  <h3 class="break-words text-2xl font-semibold tracking-tight text-slate-900 sm:text-3xl" :title="detail.name">
                    {{ detail.name }}
                  </h3>
                </div>
                <div class="min-w-0 max-w-full">
                  <div
                    class="flex min-w-0 max-w-full flex-nowrap items-center gap-2 overflow-x-auto py-2 [scrollbar-width:thin] sm:gap-3 lg:justify-end"
                  >
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
                <div class="grid gap-x-8 gap-y-3 lg:grid-cols-2">
                  <div
                    v-for="item in primaryDetailRows"
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
                v-if="isVideo"
                class="flex flex-col gap-4 lg:flex-row lg:items-stretch"
              >
                <div
                  class="min-w-0 flex-1 overflow-hidden rounded-2xl border border-slate-200 bg-slate-950 shadow-inner ring-1 ring-black/5"
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
                  v-if="folderIdForPeers"
                  class="flex w-full shrink-0 flex-col rounded-2xl border border-slate-200 bg-white lg:w-72 xl:w-80"
                >
                  <div class="border-b border-slate-100 px-4 py-3">
                    <p class="text-xs font-semibold uppercase tracking-[0.14em] text-slate-400">Playlist</p>
                    <p class="mt-1 text-sm font-medium text-slate-900">同文件夹视频</p>
                  </div>
                  <div
                    class="min-h-[120px] max-h-[min(70vh,720px)] overflow-y-auto px-2 py-2"
                  >
                    <p v-if="folderVideoPeersLoading" class="px-2 py-6 text-center text-sm text-slate-500">
                      加载列表…
                    </p>
                    <ul v-else-if="folderVideoPeers.length > 0" class="space-y-1">
                      <li v-for="peer in folderVideoPeers" :key="peer.id">
                        <RouterLink
                          :to="{ name: 'public-file-detail', params: { fileID: peer.id } }"
                          class="flex min-w-0 items-start gap-2 rounded-xl px-2 py-2 text-left text-sm text-slate-700 transition hover:bg-slate-50 hover:text-slate-900"
                        >
                          <FileVideo class="mt-0.5 h-4 w-4 shrink-0 text-slate-400" aria-hidden="true" />
                          <span class="min-w-0 break-words leading-snug">{{ peer.name }}</span>
                        </RouterLink>
                      </li>
                    </ul>
                    <p v-else class="px-2 py-6 text-center text-sm text-slate-500">
                      当前文件夹没有其他视频
                    </p>
                  </div>
                </aside>
              </div>
            </div>

            <div class="mt-4 rounded-3xl border border-slate-200 bg-white px-4 py-4 sm:px-5 sm:py-5">
              <div
                v-if="detailCoverImageHref && !isVideo"
                class="mb-4 overflow-hidden rounded-2xl border border-slate-100 bg-slate-50 ring-1 ring-slate-950/[0.04]"
              >
                <img
                  :src="detailCoverImageHref"
                  alt=""
                  class="max-h-72 w-full object-cover"
                  loading="lazy"
                  decoding="async"
                />
              </div>
              <div
                v-if="descriptionHTML"
                class="markdown-content"
                v-html="descriptionHTML"
              />
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
      <div v-if="descriptionEditorOpen" class="fixed inset-0 z-[120] bg-slate-950/40 backdrop-blur-sm">
        <div class="flex min-h-screen items-center justify-center px-4 py-6">
          <div class="modal-card panel w-full max-w-3xl overflow-hidden p-6">
            <div class="border-b border-slate-200 pb-4">
              <div>
                <h3 class="text-lg font-semibold text-slate-900">编辑文件信息</h3>
              </div>
            </div>

            <div class="mt-5 space-y-4">
              <label class="space-y-2">
                <span class="text-sm font-medium text-slate-700">文件名</span>
                <input
                  v-model="editFileName"
                  class="field"
                  :disabled="!canManageResourceDescriptions"
                  placeholder="输入完整文件名，例如 example.xlsx"
                />
              </label>

              <textarea
                v-model="editDescription"
                rows="10"
                class="field-area"
                placeholder="输入文件简介，简介支持简单 Markdown。"
              />

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

              <div class="flex justify-end gap-3">
                <button type="button" class="btn-secondary" @click="closeDescriptionEditor">取消</button>
                <button type="button" class="btn-primary" :disabled="saving || !editorDirty" @click="saveDescription">
                  {{ saving ? "保存中…" : "保存更改" }}
                </button>
              </div>
            </div>
          </div>
        </div>
      </div>
      </Transition>
    </Teleport>
  </section>
</template>
