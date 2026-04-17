/**
 * OpenShare 只读静态页：无登录、上传、反馈、批量下载、管理端操作。
 * 大文件与文件夹下载前会弹出确认（阈值来自 GET /public/download-policy）。
 * API 基址默认 /api（同域），可通过 localStorage「openshare_readonly_api_base」或页面内设置保存。
 * 所有请求 credentials: omit，不发送 Cookie。
 */
const LS_API = "openshare_readonly_api_base";
const LS_VIEW = "public-home-view-mode";
const LS_SORT = "public-home-sort-mode";
const LS_SORT_DIR = "public-home-sort-direction";

function escapeHtml(value) {
  return String(value)
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function getApiBase() {
  try {
    const v = localStorage.getItem(LS_API);
    if (v != null && String(v).trim() !== "") return String(v).trim().replace(/\/+$/, "");
  } catch {
    /* ignore */
  }
  if (window.apiBaseFallback) {
    return window.apiBaseFallback;
  }
  return "/api";
}

function setApiBase(next) {
  const t = String(next ?? "").trim().replace(/\/+$/, "");
  localStorage.setItem(LS_API, t || "/api");
}

function apiUrl(path) {
  const base = getApiBase();
  const p = path.startsWith("/") ? path : `/${path}`;
  if (/^https?:\/\//i.test(base)) {
    return `${base}${p}`;
  }
  return `${base}${p}`;
}

class HttpError extends Error {
  constructor(message, status, payload) {
    super(message);
    this.name = "HttpError";
    this.status = status;
    this.payload = payload;
  }
}

async function parsePayload(response) {
  if (response.status === 204) return null;
  const contentType = response.headers.get("content-type") ?? "";
  if (contentType.includes("application/json")) return response.json();
  return response.text();
}

async function apiRequest(path, options = {}) {
  const headers = new Headers({ Accept: "application/json" });
  if (options.headers) new Headers(options.headers).forEach((v, k) => headers.set(k, v));
  let body = options.body;
  if (body != null && typeof body === "object" && !(body instanceof FormData) && !(body instanceof URLSearchParams) && typeof body !== "string" && !(body instanceof Blob)) {
    headers.set("Content-Type", "application/json");
    body = JSON.stringify(body);
  }
  const response = await fetch(apiUrl(path), {
    ...options,
    headers,
    body,
    credentials: "omit",
  });
  const payload = await parsePayload(response);
  if (!response.ok) {
    throw new HttpError(response.statusText || "Request failed", response.status, payload);
  }
  return payload;
}

function readApiError(err, fallback = "请求失败。") {
  if (!(err instanceof HttpError) || typeof err.payload !== "object" || err.payload === null) return fallback;
  const e = err.payload.error;
  return typeof e === "string" && e.trim() !== "" ? e : fallback;
}

/* --- Markdown（marked + DOMPurify，与 src/lib/markdown.ts 对齐）--- */
function extractCoverImageUrlFromMarkdown(source) {
  const normalized = source.replace(/\r\n/g, "\n");
  const re = /!\[([^\]]*)\]\(([^)]+)\)/g;
  let m;
  while ((m = re.exec(normalized)) !== null) {
    if (m[1].trim().toLowerCase() === "cover") return m[2].trim();
  }
  return null;
}

function stripCoverImageMarkdown(source) {
  return source
    .replace(/\r\n/g, "\n")
    .replace(/!\[cover\]\([^)]*\)/gi, "")
    .replace(/\n{3,}/g, "\n\n")
    .trim();
}

function isSafeImageUrlForSrc(url) {
  const u = url.trim().toLowerCase();
  if (!u) return false;
  if (u.startsWith("javascript:") || u.startsWith("data:") || u.startsWith("vbscript:")) return false;
  return true;
}

function resolveMarkdownImageUrlToHref(raw) {
  const u = raw.trim();
  if (!u) return "";
  if (/^https?:\/\//i.test(u)) return u;
  try {
    return new URL(u, window.location.href).href;
  } catch {
    return u;
  }
}

function coverImageHrefFromDescription(description) {
  const raw = extractCoverImageUrlFromMarkdown(description);
  if (!raw || !isSafeImageUrlForSrc(raw)) return null;
  return resolveMarkdownImageUrlToHref(raw) || null;
}

function fileCoverImageHrefFromFields(coverUrlField, description) {
  const direct = (coverUrlField ?? "").trim();
  if (direct) {
    if (!isSafeImageUrlForSrc(direct)) return null;
    return resolveMarkdownImageUrlToHref(direct) || null;
  }
  return coverImageHrefFromDescription(description);
}

function encodeHrefLikeMarked(href) {
  const h = String(href ?? "").trim();
  if (!h) return null;
  try {
    return encodeURI(decodeURI(h));
  } catch {
    try {
      return encodeURI(h);
    } catch {
      return null;
    }
  }
}

let markdownRendererConfigured = false;
function ensureMarkdownRenderer() {
  if (markdownRendererConfigured) return;
  markdownRendererConfigured = true;
  if (typeof marked === "undefined") return;
  marked.use({
    gfm: true,
    breaks: false,
    renderer: {
      image(token) {
        let altPlain = token.text ?? "";
        if (token.tokens?.length) {
          altPlain = this.parser.parseInline(token.tokens, this.parser.textRenderer);
        }
        const rawHref = String(token.href ?? "").trim();
        if (!isSafeImageUrlForSrc(rawHref)) {
          return escapeHtml(token.raw ?? altPlain);
        }
        const resolved = resolveMarkdownImageUrlToHref(rawHref);
        const src = escapeHtml(resolved);
        const alt = escapeHtml(altPlain);
        const title =
          token.title != null && String(token.title).trim() !== ""
            ? ` title="${escapeHtml(String(token.title))}"`
            : "";
        return `<img src="${src}" alt="${alt}" class="markdown-img" loading="lazy" decoding="async"${title} />`;
      },
      link(token) {
        const inner = this.parser.parseInline(token.tokens);
        const encoded = encodeHrefLikeMarked(String(token.href ?? ""));
        if (encoded === null) return inner;
        const title =
          token.title != null && String(token.title).trim() !== ""
            ? ` title="${escapeHtml(String(token.title))}"`
            : "";
        const hrefAttr = escapeHtml(encoded);
        if (/^https?:\/\//i.test(encoded)) {
          return `<a href="${hrefAttr}" target="_blank" rel="noopener noreferrer"${title}>${inner}</a>`;
        }
        return `<a href="${hrefAttr}"${title}>${inner}</a>`;
      },
    },
  });
}

function renderSimpleMarkdown(source) {
  const normalized = source.replace(/\r\n/g, "\n");
  if (!normalized.trim()) return "";
  ensureMarkdownRenderer();
  if (typeof marked === "undefined" || typeof DOMPurify === "undefined") {
    return escapeHtml(normalized);
  }
  try {
    const html = marked.parse(normalized, { async: false });
    return DOMPurify.sanitize(html, {
      ADD_ATTR: ["target", "rel", "loading", "decoding", "align", "start"],
      ADD_TAGS: ["input"],
    });
  } catch {
    return escapeHtml(normalized);
  }
}

function fileEffectiveDownloadHref(fileId, playbackUrl, folderDirectDownloadUrl) {
  const p = (playbackUrl ?? "").trim();
  if (p) return p;
  const f = (folderDirectDownloadUrl ?? "").trim();
  if (f) return f;
  return apiUrl(`/public/files/${encodeURIComponent(fileId)}/download`);
}

/* --- 路由（hash，便于静态部署与 file://）--- */
function parseHashRoute() {
  const raw = (location.hash || "#/").replace(/^#/, "") || "/";
  const q = raw.indexOf("?");
  const pathPart = q >= 0 ? raw.slice(0, q) : raw;
  const search = q >= 0 ? raw.slice(q + 1) : "";
  const path = pathPart.startsWith("/") ? pathPart : `/${pathPart}`;
  const sp = new URLSearchParams(search);
  if (path.startsWith("/files/")) {
    const fileId = decodeURIComponent(path.slice("/files/".length).split("/")[0] || "");
    return {
      view: "file",
      fileId,
      folder: "",
      root: "",
      t: sp.get("t") || "",
    };
  }
  return {
    view: "home",
    fileId: "",
    folder: sp.get("folder")?.trim() ?? "",
    root: sp.get("root")?.trim() ?? "",
    t: "",
  };
}

function setHashRoute(route) {
  if (route.view === "file" && route.fileId) {
    const t = route.t && String(route.t).trim() !== "" ? `?t=${encodeURIComponent(String(route.t))}` : "";
    location.hash = `#/files/${encodeURIComponent(route.fileId)}${t}`;
    return;
  }
  const sp = new URLSearchParams();
  if (route.folder) sp.set("folder", route.folder);
  if (route.root === "1") sp.set("root", "1");
  const qs = sp.toString();
  location.hash = qs ? `#/?${qs}` : "#/";
}

function formatSize(size) {
  const n = Number(size) || 0;
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(2)} KB`;
  if (n < 1024 * 1024 * 1024) return `${(n / (1024 * 1024)).toFixed(2)} MB`;
  return `${(n / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

function formatDateTime(value) {
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

function parseSortTimeMs(raw) {
  if (raw == null || raw === "" || (typeof raw === "string" && !raw.trim())) return 0;
  const ms = Date.parse(String(raw));
  return Number.isFinite(ms) ? ms : 0;
}

function extractExtension(name) {
  const index = name.lastIndexOf(".");
  if (index <= 0 || index === name.length - 1) return "";
  return name.slice(index + 1).toLowerCase();
}

const VIDEO_EXT = new Set(["mp4", "mov", "avi", "mkv", "webm", "m4v", "ogv"]);
/** 与后端默认一致：超过该大小的单文件下载前确认；文件夹打包始终确认 */
const DEFAULT_LARGE_DOWNLOAD_CONFIRM = 1024 * 1024 * 1024;

function isVideoDetail(d) {
  const mime = (d.mime_type ?? "").toLowerCase();
  if (mime.startsWith("video/")) return true;
  const ext = (d.extension ?? "").replace(/^\./, "").toLowerCase();
  return VIDEO_EXT.has(ext);
}

/* --- 简易 SVG 图标 --- */
const Ico = {
  home: '<svg class="h-4 w-4" xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="m3 9 9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/><polyline points="9 22 9 12 15 12 15 22"/></svg>',
  chevronLeft: '<svg class="h-4 w-4" xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="m15 18-6-6 6-6"/></svg>',
  chevronRight: '<svg class="h-4 w-4" xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="m9 18 6-6-6-6"/></svg>',
  download: '<svg class="h-4 w-4" xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><polyline points="7 10 12 15 17 10"/><line x1="12" x2="12" y1="15" y2="3"/></svg>',
  clock: '<svg class="h-3.5 w-3.5" xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>',
  folder: '<svg class="h-7 w-7 text-blue-500" xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M20 20a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.9a2 2 0 0 1-1.69-.9L9.04 3.6a2 2 0 0 0-1.69-.9H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2Z"/></svg>',
  file: '<svg class="h-7 w-7" xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"/><path d="M14 2v4h4"/></svg>',
  search: '<svg class="pointer-events-none absolute left-5 top-1/2 h-5 w-5 -translate-y-1/2 text-slate-400" xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/></svg>',
  x: '<svg class="h-4 w-4" xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>',
  grid: '<svg class="h-4 w-4" xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect width="7" height="7" x="3" y="3" rx="1"/><rect width="7" height="7" x="14" y="3" rx="1"/><rect width="7" height="7" x="14" y="14" rx="1"/><rect width="7" height="7" x="3" y="14" rx="1"/></svg>',
  list: '<svg class="h-4 w-4" xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><line x1="8" x2="21" y1="6" y2="6"/><line x1="8" x2="21" y1="12" y2="12"/><line x1="8" x2="21" y1="18" y2="18"/><line x1="3" x2="3.01" y1="6" y2="6"/><line x1="3" x2="3.01" y1="12" y2="12"/><line x1="3" x2="3.01" y1="18" y2="18"/></svg>',
  github: '<svg class="h-[17.2px] w-[17.2px]" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="currentColor"><path d="M12 0c-6.626 0-12 5.373-12 12 0 5.302 3.438 9.8 8.207 11.387.599.111.793-.261.793-.577v-2.234c-3.338.726-4.033-1.416-4.033-1.416-.546-1.387-1.333-1.756-1.333-1.756-1.089-.745.083-.729.083-.729 1.205.084 1.839 1.237 1.839 1.237 1.07 1.834 2.807 1.304 3.492.997.107-.775.418-1.305.762-1.604-2.665-.305-5.467-1.334-5.467-5.931 0-1.311.469-2.381 1.236-3.221-.124-.303-.535-1.524.117-3.176 0 0 1.008-.322 3.301 1.23.957-.266 1.983-.399 3.003-.404 1.02.005 2.047.138 3.006.404 2.291-1.552 3.297-1.23 3.297-1.23.653 1.653.242 2.874.118 3.176.77.84 1.235 1.911 1.235 3.221 0 4.609-2.807 5.624-5.479 5.921.43.372.823 1.102.823 2.222v3.293c0 .319.192.694.801.576 4.765-1.589 8.199-6.086 8.199-11.386 0-6.627-5.373-12-12-12z"/></svg>',
  link2: '<svg class="h-4 w-4" xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>',
  share: '<svg class="h-4 w-4" xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="18" cy="5" r="3"/><circle cx="6" cy="12" r="3"/><circle cx="18" cy="19" r="3"/><line x1="8.59" x2="15.42" y1="13.51" y2="17.49"/><line x1="15.41" x2="8.59" y1="6.51" y2="10.49"/></svg>',
  fileVideo: '<svg class="h-4 w-4 shrink-0 text-slate-400" xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="m16 13 5.223 3.482a.5.5 0 0 0 .777-.416V7.87a.5.5 0 0 0-.752-.432L16 10.5"/><rect x="2" y="6" width="14" height="12" rx="2"/></svg>',
  fileText: '<svg class="h-4 w-4" xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z"/><path d="M14 2v4h4"/><path d="M10 9H8"/><path d="M16 13H8"/><path d="M16 17H8"/></svg>',
  clockBig: '<svg class="h-4 w-4" xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>',
};

function fileIconSvg(ext) {
  const e = String(ext || "").toLowerCase();
  if (["png", "jpg", "jpeg", "gif", "webp", "svg"].includes(e)) {
    return '<svg class="h-7 w-7 text-slate-500" xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect width="18" height="18" x="3" y="3" rx="2"/><circle cx="9" cy="9" r="2"/><path d="m21 15-3.086-3.086a2 2 0 0 0-2.828 0L6 21"/></svg>';
  }
  if (VIDEO_EXT.has(e)) {
    return '<svg class="h-7 w-7 text-slate-500" xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="m16 13 5.223 3.482a.5.5 0 0 0 .777-.416V7.87a.5.5 0 0 0-.752-.432L16 10.5"/><rect x="2" y="6" width="14" height="12" rx="2"/></svg>';
  }
  return Ico.file;
}

/* --- 全局 UI 状态 --- */
const state = {
  route: parseHashRoute(),
  announcements: [],
  hotDownloadItems: [],
  latestItems: [],
  folders: [],
  files: [],
  folderDetail: null,
  breadcrumbs: [],
  loading: false,
  error: "",
  searchInput: "",
  searchKeyword: "",
  searchLoading: false,
  searchError: "",
  searchRows: [],
  viewMode: "cards",
  sortMode: "name",
  sortDirection: "desc",
  sortMenuOpen: false,
  viewMenuOpen: false,
  folderMarkdownExpanded: false,
  modalAnnouncement: null,
  modalAnnouncementList: false,
  modalSidebar: null,
  transientWarning: "",
  downloadTimestamps: [],
  largeDownloadConfirmBytes: DEFAULT_LARGE_DOWNLOAD_CONFIRM,
  /** @type {null | { kind: "row"; row: object } | { kind: "folderToolbar" } | { kind: "fileDetail" }} */
  downloadConfirm: null,
  /** 详情页 */
  fileDetail: null,
  fileLoading: false,
  fileError: "",
  folderVideoPeers: [],
  folderVideoPeersLoading: false,
  videoPlaybackStep: 0,
  videoFileMetaVisible: false,
  linkCopyHint: "",
  settingsOpen: false,
};

function loadPrefs() {
  const vm = localStorage.getItem(LS_VIEW);
  if (vm === "cards" || vm === "table") state.viewMode = vm;
  const sm = localStorage.getItem(LS_SORT);
  if (sm === "name" || sm === "download" || sm === "format" || sm === "modified") state.sortMode = sm;
  const sd = localStorage.getItem(LS_SORT_DIR);
  if (sd === "asc" || sd === "desc") state.sortDirection = sd;
}

function savePref(key, value) {
  localStorage.setItem(key, value);
}

function showWarning(msg) {
  state.transientWarning = msg;
  render();
  clearTimeout(showWarning._t);
  showWarning._t = setTimeout(() => {
    state.transientWarning = "";
    render();
  }, 2200);
}

function allowDownloadRequest() {
  const now = Date.now();
  const windowMs = 10000;
  const limit = 10;
  state.downloadTimestamps = state.downloadTimestamps.filter((t) => now - t < windowMs);
  if (state.downloadTimestamps.length >= limit) return false;
  state.downloadTimestamps.push(now);
  return true;
}

function buildRows() {
  const folderId = state.route.folder;
  const folderRows = state.folders.map((folder) => {
    const desc = (folder.description ?? "").trim();
    return {
      id: folder.id,
      kind: "folder",
      name: folder.name,
      extension: "",
      description: desc,
      coverUrl: coverImageHrefFromDescription(desc),
      downloadCount: folder.download_count ?? 0,
      fileCount: folder.file_count ?? 0,
      sizeBytes: folder.total_size ?? 0,
      sizeText: formatSize(folder.total_size ?? 0),
      updatedAt: formatDateTime(folder.updated_at),
      sortTimeMs: parseSortTimeMs(folder.updated_at),
      downloadURL: apiUrl(`/public/folders/${encodeURIComponent(folder.id)}/download`),
      downloadAllowed: folder.download_allowed !== false,
    };
  });
  const fileRows = folderId
    ? state.files.map((file) => {
        const desc = (file.description ?? "").trim();
        return {
          id: file.id,
          kind: "file",
          name: file.name,
          extension: file.extension || extractExtension(file.name),
          description: desc,
          coverUrl: fileCoverImageHrefFromFields(file.cover_url, desc),
          downloadCount: file.download_count ?? 0,
          fileCount: 0,
          sizeBytes: file.size ?? 0,
          sizeText: formatSize(file.size),
          updatedAt: formatDateTime(file.uploaded_at),
          sortTimeMs: parseSortTimeMs(file.uploaded_at),
          downloadURL: fileEffectiveDownloadHref(file.id, file.playback_url, file.folder_direct_download_url),
          downloadAllowed: file.download_allowed !== false,
        };
      })
    : [];
  return [...folderRows, ...fileRows];
}

function displayedRows() {
  if (state.searchKeyword) return state.searchRows;
  return buildRows();
}

function formatSortRank(row) {
  if (row.kind === "folder") return 0;
  const extension = row.extension.toLowerCase();
  if (extension === "pdf") return 1;
  if (["doc", "docx", "xls", "xlsx", "ppt", "pptx"].includes(extension)) return 2;
  return 3;
}

function compareRows(left, right, mode, direction) {
  let result = 0;
  if (mode === "download") {
    if (left.downloadCount !== right.downloadCount) result = left.downloadCount - right.downloadCount;
    else result = left.name.localeCompare(right.name, "zh-CN");
  } else if (mode === "format") {
    const lr = formatSortRank(left);
    const rr = formatSortRank(right);
    if (lr !== rr) result = lr - rr;
    else result = left.name.localeCompare(right.name, "zh-CN");
  } else if (mode === "modified") {
    if (left.sortTimeMs !== right.sortTimeMs) result = left.sortTimeMs - right.sortTimeMs;
    else result = left.name.localeCompare(right.name, "zh-CN");
  } else {
    result = left.name.localeCompare(right.name, "zh-CN");
  }
  return direction === "asc" ? result : -result;
}

function sortedRows() {
  const rows = [...displayedRows()];
  rows.sort((a, b) => compareRows(a, b, state.sortMode, state.sortDirection));
  return rows;
}

function currentFolderStats() {
  const d = state.folderDetail;
  if (!d) return [];
  return [
    { label: "文件夹名", value: d.name },
    { label: "下载量", value: String(d.download_count ?? 0) },
    { label: "文件数", value: `${d.file_count ?? 0} 个文件` },
    { label: "文件夹大小", value: formatSize(d.total_size ?? 0) },
    { label: "更新时间", value: formatDateTime(d.updated_at) },
  ];
}

function canGoUp() {
  return Boolean(state.route.folder);
}

function rootViewLocked() {
  return state.route.root === "1";
}

async function loadAnnouncements() {
  try {
    const response = await apiRequest(`/public/announcements`);
    state.announcements = response.items ?? [];
  } catch {
    state.announcements = [];
  }
}

async function loadHotDownloads() {
  try {
    const response = await apiRequest(`/public/files/hot?limit=20`);
    state.hotDownloadItems = (response.items ?? []).map((item) => ({
      id: item.id,
      name: item.name,
      downloadCount: item.download_count ?? 0,
    }));
  } catch {
    state.hotDownloadItems = [];
  }
}

async function loadLatestTitles() {
  try {
    const response = await apiRequest(`/public/files/latest?limit=20`);
    state.latestItems = (response.items ?? []).map((item) => ({
      id: item.id,
      name: item.name,
    }));
  } catch {
    state.latestItems = [];
  }
}

async function loadDirectory() {
  const folderId = state.route.folder;
  state.loading = true;
  state.error = "";
  try {
    const directoryParams = new URLSearchParams();
    if (folderId) directoryParams.set("parent_id", folderId);
    const folderPath = `/public/folders${directoryParams.toString() ? `?${directoryParams.toString()}` : ""}`;
    const requests = [apiRequest(folderPath)];
    if (folderId) {
      const fp = new URLSearchParams({ page: "1", page_size: "100", sort: "name_asc" });
      requests.push(apiRequest(`/public/folders/${encodeURIComponent(folderId)}/files?${fp.toString()}`));
      requests.push(apiRequest(`/public/folders/${encodeURIComponent(folderId)}`));
    }
    const results = await Promise.all(requests);
    const folderResponse = results[0];
    state.folders = folderResponse.items ?? [];
    state.files = folderId && results[1] ? results[1].items ?? [] : [];
    const folderDetail = folderId && results[2] ? results[2] : null;

    if (!folderId && !rootViewLocked() && state.folders.length === 1) {
      setHashRoute({ view: "home", folder: state.folders[0].id, root: "", fileId: "", t: "" });
      return;
    }

    if (folderDetail) {
      state.folderDetail = folderDetail;
      state.breadcrumbs = folderDetail.breadcrumbs ?? [];
    } else {
      state.folderDetail = null;
      state.breadcrumbs = [];
    }
  } catch (err) {
    state.folders = [];
    state.files = [];
    state.breadcrumbs = [];
    state.folderDetail = null;
    if (err instanceof HttpError && err.status === 404) state.error = "目录不存在或未公开。";
    else state.error = "加载目录失败。";
  } finally {
    state.loading = false;
  }
}

async function runSearch(keyword) {
  const normalizedKeyword = keyword.trim();
  if (!normalizedKeyword) {
    clearSearchState();
    render();
    return;
  }
  state.searchInput = normalizedKeyword;
  state.searchKeyword = normalizedKeyword;
  state.searchLoading = true;
  state.searchError = "";
  try {
    const query = new URLSearchParams({
      q: normalizedKeyword,
      page: "1",
      page_size: "50",
    });
    if (state.route.folder) query.set("folder_id", state.route.folder);
    const response = await apiRequest(`/public/search?${query.toString()}`);
    state.searchRows = (response.items ?? []).map((item) => {
      const modRaw =
        item.entity_type === "folder" ? item.updated_at : (item.updated_at || item.uploaded_at);
      return {
        id: item.id,
        kind: item.entity_type,
        name: item.name,
        extension: item.entity_type === "file" ? item.extension || extractExtension(item.name) : "",
        description: "",
        coverUrl:
          item.entity_type === "file" ? fileCoverImageHrefFromFields(item.cover_url, "") : null,
        downloadCount: item.download_count ?? 0,
        fileCount: 0,
        sizeBytes: item.entity_type === "file" ? (item.size ?? 0) : 0,
        sizeText: item.entity_type === "file" ? formatSize(item.size ?? 0) : "-",
        updatedAt: modRaw ? formatDateTime(modRaw) : "-",
        sortTimeMs: parseSortTimeMs(modRaw),
        downloadURL:
          item.entity_type === "file"
            ? fileEffectiveDownloadHref(item.id, item.playback_url, item.folder_direct_download_url)
            : apiUrl(`/public/folders/${encodeURIComponent(item.id)}/download`),
        downloadAllowed: item.download_allowed !== false,
      };
    });
  } catch (err) {
    state.searchRows = [];
    state.searchError = readApiError(err, "搜索失败。");
  } finally {
    state.searchLoading = false;
  }
}

function clearSearchState() {
  state.searchInput = "";
  state.searchKeyword = "";
  state.searchLoading = false;
  state.searchError = "";
  state.searchRows = [];
}

async function loadFileDetail() {
  const id = state.route.fileId;
  if (!id) return;
  state.fileLoading = true;
  state.fileError = "";
  state.fileDetail = null;
  state.folderVideoPeers = [];
  state.folderVideoPeersLoading = false;
  state.videoPlaybackStep = 0;
  state.videoFileMetaVisible = false;
  try {
    const d = await apiRequest(`/public/files/${encodeURIComponent(id)}`);
    state.fileDetail = d;
    if (d && isVideoDetail(d)) {
      const fid = (d.folder_id ?? "").trim();
      if (fid) await loadFolderVideoPeers(fid, d.id);
    }
  } catch (err) {
    if (err instanceof HttpError && err.status === 404) state.fileError = "文件不存在或未公开。";
    else state.fileError = "加载文件详情失败。";
  } finally {
    state.fileLoading = false;
  }
}

async function loadFolderVideoPeers(folderID, currentFileId) {
  state.folderVideoPeersLoading = true;
  state.folderVideoPeers = [];
  try {
    const params = new URLSearchParams({ page: "1", page_size: "100", sort: "name_asc" });
    const response = await apiRequest(`/public/folders/${encodeURIComponent(folderID)}/files?${params.toString()}`);
    const items = response.items ?? [];
    state.folderVideoPeers = items
      .filter((f) => f.id !== currentFileId)
      .filter((f) => VIDEO_EXT.has(((f.extension ?? "").replace(/^\./, "") || extractExtension(f.name)).toLowerCase()))
      .map((f) => ({ id: f.id, name: f.name }));
  } catch {
    state.folderVideoPeers = [];
  } finally {
    state.folderVideoPeersLoading = false;
  }
}

function buildVideoPlaybackUrlQueue(fileId, d) {
  const seen = new Set();
  const out = [];
  const add = (u) => {
    const t = u.trim();
    if (!t || seen.has(t)) return;
    seen.add(t);
    out.push(t);
  };
  const playback = (d.playback_url ?? "").trim();
  const fallback = (d.playback_fallback_url ?? "").trim();
  const folder = (d.folder_direct_download_url ?? "").trim();
  const backend = apiUrl(`/public/files/${encodeURIComponent(fileId)}/download`);
  if (playback) {
    add(playback);
    if (fallback) add(fallback);
  }
  if (folder) add(folder);
  add(backend);
  return out;
}

function mediaSourceURL(detail, fileId) {
  if (!detail) return apiUrl(`/public/files/${encodeURIComponent(fileId)}/download`);
  return fileEffectiveDownloadHref(fileId, detail.playback_url, detail.folder_direct_download_url);
}

function parseTimestampQuery(raw) {
  if (raw == null || raw === "") return null;
  const n = parseFloat(String(raw));
  if (!Number.isFinite(n) || n < 0) return null;
  return n;
}

function formatTimestampParam(seconds) {
  if (!Number.isFinite(seconds) || seconds <= 0) return "0";
  const rounded = Math.round(seconds * 10) / 10;
  return Number.isInteger(rounded) ? String(rounded) : rounded.toFixed(1);
}

function goBackFromDetail() {
  const folderID = state.fileDetail?.folder_id?.trim() ?? "";
  if (folderID) setHashRoute({ view: "home", folder: folderID, root: "", fileId: "", t: "" });
  else setHashRoute({ view: "home", folder: "", root: "", fileId: "", t: "" });
}

function rowNeedsDownloadConfirm(row) {
  if (row.kind === "folder") return true;
  const sz = Number(row.sizeBytes) || 0;
  return sz >= state.largeDownloadConfirmBytes;
}

function performDownloadRow(row) {
  if (!row.downloadAllowed) {
    showWarning("该资源不允许下载。");
    return;
  }
  if (!allowDownloadRequest()) {
    showWarning("下载请求过于频繁，请稍后再试。");
    return;
  }
  const link = document.createElement("a");
  link.href = row.downloadURL;
  link.rel = "noopener";
  if (row.downloadURL.startsWith("http://") || row.downloadURL.startsWith("https://")) link.target = "_blank";
  document.body.appendChild(link);
  link.click();
  link.remove();
}

function downloadRow(row) {
  if (!row.downloadAllowed) {
    showWarning("该资源不允许下载。");
    return;
  }
  if (rowNeedsDownloadConfirm(row)) {
    state.downloadConfirm = { kind: "row", row };
    render();
    return;
  }
  performDownloadRow(row);
}

function fileDetailNeedsDownloadConfirm() {
  const d = state.fileDetail;
  if (!d || d.download_allowed === false) return false;
  return (Number(d.size) || 0) >= state.largeDownloadConfirmBytes;
}

function performDownloadCurrentFolder() {
  const d = state.folderDetail;
  if (!d || d.download_allowed === false) {
    showWarning("该文件夹不允许下载。");
    return;
  }
  if (!allowDownloadRequest()) {
    showWarning("下载请求过于频繁，请稍后再试。");
    return;
  }
  const href = apiUrl(`/public/folders/${encodeURIComponent(d.id)}/download`);
  const link = document.createElement("a");
  link.href = href;
  link.rel = "noopener";
  document.body.appendChild(link);
  link.click();
  link.remove();
}

function downloadCurrentFolder() {
  const d = state.folderDetail;
  if (!d || d.download_allowed === false) {
    showWarning("该文件夹不允许下载。");
    return;
  }
  state.downloadConfirm = { kind: "folderToolbar" };
  render();
}

function performDownloadFileFromDetail() {
  const d = state.fileDetail;
  if (!d || d.download_allowed === false) return;
  const src = mediaSourceURL(d, d.id);
  const link = document.createElement("a");
  link.href = src;
  link.rel = "noopener";
  if (src.startsWith("http://") || src.startsWith("https://")) link.target = "_blank";
  document.body.appendChild(link);
  link.click();
  link.remove();
}

function downloadFileFromDetail() {
  const d = state.fileDetail;
  if (!d || d.download_allowed === false) return;
  if (fileDetailNeedsDownloadConfirm()) {
    state.downloadConfirm = { kind: "fileDetail" };
    render();
    return;
  }
  performDownloadFileFromDetail();
}

async function loadDownloadSettings() {
  try {
    const r = await apiRequest("/public/download-policy");
    const b = Number(r?.large_download_confirm_bytes);
    if (Number.isFinite(b) && b > 0) state.largeDownloadConfirmBytes = b;
    else state.largeDownloadConfirmBytes = DEFAULT_LARGE_DOWNLOAD_CONFIRM;
  } catch {
    state.largeDownloadConfirmBytes = DEFAULT_LARGE_DOWNLOAD_CONFIRM;
  }
}

/** 更新复制提示：在视频详情页只改 DOM，避免 render() 重建 video 节点导致重新加载转圈。 */
function applyFileDetailCopyHint(message) {
  state.linkCopyHint = message;
  const el = document.getElementById("file-detail-copy-hint");
  if (el && state.route.view === "file") {
    el.textContent = message;
    el.classList.toggle("hidden", !message);
    return;
  }
  render();
}

/** 视频详情：展开/收起元数据区，不调用 render()，避免重建 video。 */
function toggleFileDetailVideoMetaFromDOM() {
  if (state.route.view !== "file" || !state.fileDetail || !isVideoDetail(state.fileDetail)) {
    return false;
  }
  const panel = document.getElementById("file-detail-meta-panel");
  const btn = document.getElementById("file-detail-toggle-meta-btn");
  if (!panel) {
    return false;
  }
  state.videoFileMetaVisible = !state.videoFileMetaVisible;
  panel.classList.toggle("hidden", !state.videoFileMetaVisible);
  if (btn) {
    btn.setAttribute("aria-expanded", state.videoFileMetaVisible ? "true" : "false");
  }
  return true;
}

async function copyText(label, text) {
  if (!text) {
    applyFileDetailCopyHint("当前环境无法生成链接。");
    setTimeout(() => applyFileDetailCopyHint(""), 2800);
    return;
  }
  try {
    await navigator.clipboard.writeText(text);
    applyFileDetailCopyHint(`已复制${label}`);
  } catch {
    applyFileDetailCopyHint("复制失败，请手动长按或右键复制地址栏。");
  }
  setTimeout(() => applyFileDetailCopyHint(""), 2800);
}

function absoluteDetailPageURL(fileId) {
  const path = `#/files/${encodeURIComponent(fileId)}`;
  return new URL(path, window.location.href).href;
}

function buildDetailPageURLWithT(fileId, seconds) {
  const t = formatTimestampParam(seconds);
  const path = `#/files/${encodeURIComponent(fileId)}?t=${encodeURIComponent(t)}`;
  return new URL(path, window.location.href).href;
}

function syncRouteFromHash() {
  state.route = parseHashRoute();
}

function renderNavbar() {
  return `
  <header class="fixed inset-x-0 top-0 z-[60] border-b border-slate-200 bg-white/95 backdrop-blur">
    <div class="mx-auto grid h-16 w-full max-w-[1360px] grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)] items-center gap-3 px-3 sm:px-4 md:px-6 md:gap-4 lg:px-8">
      <div class="min-w-0 flex items-center justify-start">
        <a href="#/" class="inline-flex min-w-0 items-center gap-2 sm:gap-2.5" data-nav="home">
          <span class="flex h-8 w-8 items-center justify-center rounded-lg bg-slate-900 text-xs font-bold text-white">OS</span>
          <span class="truncate font-serif text-[15px] font-extrabold tracking-tight text-slate-900 sm:text-[16px]">OpenShare</span>
        </a>
      </div>
      <nav class="flex items-center justify-center gap-1 overflow-x-auto">
        <a href="#/" class="shrink-0 rounded-lg bg-slate-200/70 px-2.5 py-2 text-sm font-medium text-slate-900 sm:px-4">首页</a>
      </nav>
      <div class="flex min-w-0 items-center justify-end gap-2">
        <button type="button" class="rounded-lg border border-slate-200 bg-white px-2 py-1.5 text-xs font-medium text-slate-600 hover:bg-slate-50" data-action="toggle-settings" title="API 基址">API</button>
        <a href="https://github.com/zzzzquan/OpenShare" target="_blank" rel="noreferrer" aria-label="GitHub" class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-black text-white transition hover:bg-neutral-800">${Ico.github}</a>
      </div>
    </div>
  </header>`;
}

function renderSettingsPanel() {
  const apiVal = escapeHtml(getApiBase());
  const topClass = state.route.view === "file" ? "top-0" : "top-16";
  return `
  <div id="settings-panel" class="${state.settingsOpen ? "" : "hidden"} fixed inset-x-0 ${topClass} z-[55] border-b border-slate-200 bg-white px-4 py-3 shadow-sm sm:px-8">
    <div class="mx-auto flex max-w-[1360px] flex-col gap-2 sm:flex-row sm:items-end sm:gap-4">
      <label class="min-w-0 flex-1 space-y-1">
        <span class="text-xs font-medium text-slate-600">API 基址（含 <code class="rounded bg-slate-100 px-1">/api</code>，与主站同源时可填 <code class="rounded bg-slate-100 px-1">/api</code>）</span>
        <input type="text" class="field h-10" data-input="api-base" value="${apiVal}" placeholder="/api 或 https://后端域名/api" autocomplete="off" />
      </label>
      <div class="flex gap-2">
        <button type="button" class="btn-primary h-10 px-5" data-action="save-api">保存并刷新</button>
        <button type="button" class="btn-secondary h-10" data-action="close-settings">关闭</button>
      </div>
    </div>
  </div>`;
}

function renderInfoPanel(title, items, emptyText, actionAttr, panelKind) {
  const kind = panelKind || "file";
  const dataPick = kind === "announcement" ? "data-ann-pick" : "data-sidebar-item";
  const rows = items.length
    ? items
        .map(
          (it) => `
      <button type="button" class="block w-full rounded-lg px-2 py-2 text-left text-sm leading-6 text-slate-600 transition hover:bg-slate-50 hover:text-slate-900" ${dataPick}="${escapeHtml(it.id)}">
        <span class="flex items-start gap-2">
          ${it.badge ? `<span class="mt-0.5 inline-flex shrink-0 rounded-md bg-[#dcecff] px-2 py-0.5 text-xs font-semibold text-[#4f8ff7]">${escapeHtml(it.badge)}</span>` : ""}
          <span class="line-clamp-2">${escapeHtml(it.label)}</span>
        </span>
      </button>`,
        )
        .join("")
    : `<div class="rounded-lg bg-[#fafafa] px-3 py-3 text-sm text-slate-500">${escapeHtml(emptyText)}</div>`;
  return `
  <section class="panel p-4">
    <header class="flex flex-wrap items-center justify-between gap-2 sm:gap-3">
      <h2 class="text-sm font-medium tracking-tight text-slate-900">${escapeHtml(title)}</h2>
      <button type="button" class="shrink-0 text-xs font-medium text-slate-500 transition hover:text-slate-900" ${actionAttr}>详情</button>
    </header>
    <div class="mt-4">
      <div class="space-y-2">${rows}</div>
    </div>
  </section>`;
}

function renderHome() {
  const rows = sortedRows();
  const fd = state.folderDetail;
  const descHtml = fd ? renderSimpleMarkdown(fd.description ?? "") : "";
  const folderStats = currentFolderStats();
  const breadcrumbs = state.breadcrumbs;
  const searchBanner = state.searchKeyword
    ? `<div class="mx-5 mt-3 rounded-xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-600 sm:mx-6">当前搜索：<span class="font-medium text-slate-900">${escapeHtml(state.searchKeyword)}</span><span class="ml-2">共 ${state.searchRows.length} 条结果</span></div>`
    : "";

  const breadcrumbHtml = `
    <div class="flex min-w-max items-center gap-2 text-sm text-slate-500">
      <button type="button" class="inline-flex items-center gap-2 rounded-full px-2 py-1 transition hover:bg-slate-100 hover:text-slate-900" data-action="open-root">${Ico.home}<span>主页</span></button>
      ${breadcrumbs
        .map(
          (item) => `
        ${Ico.chevronRight}
        <button type="button" class="rounded-full px-2 py-1 transition hover:bg-slate-100 hover:text-slate-900" data-folder="${escapeHtml(item.id)}">${escapeHtml(item.name)}</button>`,
        )
        .join("")}
    </div>`;

  const folderInfoBlock =
    fd && !state.searchKeyword
      ? `
    <div class="border-b border-slate-200 px-4 py-5 sm:px-6">
      <section>
        <div class="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
          <div class="min-w-0 flex-1 space-y-3">
            <p class="text-xs font-semibold uppercase tracking-[0.18em] text-blue-600">Folder Info</p>
            <div class="flex flex-wrap items-center gap-x-8 gap-y-3 text-sm text-slate-500">
              ${folderStats.map((item) => `<div class="inline-flex items-center gap-2"><span>${escapeHtml(item.label)}</span><span class="font-medium text-slate-900">${escapeHtml(item.value)}</span></div>`).join("")}
            </div>
          </div>
          <div class="flex flex-wrap items-start gap-3">
            <button type="button" class="inline-flex h-11 w-11 items-center justify-center rounded-xl border border-slate-200 bg-white text-slate-700 transition hover:-translate-y-0.5 hover:border-slate-300 hover:bg-[#fafafa] hover:text-slate-900 hover:shadow-sm" data-action="dl-folder" aria-label="下载文件夹" ${fd.download_allowed === false ? "disabled" : ""}>${Ico.download}</button>
          </div>
        </div>
        <div class="mt-4 rounded-3xl border border-slate-200 bg-white px-4 py-4 sm:px-5 sm:py-5">
          ${
            descHtml
              ? `<div class="space-y-3">
            <div class="relative">
              <div id="folder-md" class="markdown-content ${state.folderMarkdownExpanded ? "" : "max-h-[min(42vh,20rem)] overflow-hidden"}">${descHtml}</div>
            </div>
            <div class="flex justify-center sm:justify-start">
              <button type="button" class="inline-flex min-h-10 items-center justify-center rounded-xl border border-slate-200 bg-white px-4 text-sm font-medium text-slate-800 shadow-sm" data-action="toggle-folder-md">${state.folderMarkdownExpanded ? "收起简介" : "展开全文"}</button>
            </div>
          </div>`
              : `<p class="text-sm text-slate-400">该文件夹暂无简介orz</p>`
          }
        </div>
      </section>
    </div>`
      : "";

  const searchSection = `
  <section class="px-5 py-4 sm:px-6">
    <form class="flex flex-col gap-3 xl:flex-row xl:items-center" data-form="search">
      <label class="relative block min-w-0 flex-1">
        ${Ico.search}
        <input type="text" name="q" value="${escapeHtml(state.searchInput)}" placeholder="在该目录下搜索文件/文件夹" class="h-14 w-full rounded-lg border border-slate-300 bg-white pl-14 pr-14 text-[15px] text-slate-900 outline-none transition placeholder:text-slate-400 focus:border-slate-400 focus:ring-4 focus:ring-slate-100" />
        ${state.searchInput ? `<button type="button" class="absolute right-4 top-1/2 inline-flex h-8 w-8 -translate-y-1/2 items-center justify-center rounded-full text-slate-400 hover:bg-slate-100 hover:text-slate-700" data-action="clear-search" aria-label="清除">${Ico.x}</button>` : ""}
      </label>
      <button type="submit" class="h-11 rounded-lg px-6 text-sm font-medium xl:shrink-0 ${state.searchInput.trim() ? "bg-slate-900 text-white hover:bg-slate-800" : "cursor-not-allowed bg-slate-200 text-slate-500"}" ${!state.searchInput.trim() || state.searchLoading ? "disabled" : ""}>${state.searchLoading ? "搜索中…" : "搜索"}</button>
    </form>
  </section>`;

  const backLabel = state.searchKeyword ? "返回所在目录" : "返回上一级";
  const canBack = state.searchKeyword || canGoUp();

  const toolbar = `
  <div class="px-4 pb-2 sm:px-6">
    <div class="flex flex-wrap items-center gap-3 border-t border-slate-100 pt-3">
      <button type="button" class="inline-flex items-center gap-2 rounded-xl border border-slate-200 px-3 py-2 text-sm font-medium text-slate-600 transition hover:border-slate-300 hover:text-slate-900 disabled:cursor-not-allowed disabled:opacity-45" data-action="go-up" ${canBack ? "" : "disabled"}>${Ico.chevronLeft}${backLabel}</button>
      <div class="flex w-full flex-wrap items-center gap-3 sm:ml-auto sm:w-auto sm:justify-end" data-toolbar-dropdowns>
        <div class="relative">
          <button type="button" class="inline-flex w-full items-center justify-center gap-2 rounded-xl border border-slate-200 px-3 py-2 text-sm font-medium text-slate-600 transition hover:border-slate-300 hover:text-slate-900 sm:w-auto" data-action="toggle-sort-menu">${escapeHtml(sortModeLabel())} · ${escapeHtml(sortDirectionLabel())} ${Ico.chevronRight.replace("class=\"h-4 w-4\"", "class=\"h-4 w-4 rotate-90\"")}</button>
          <div class="${state.sortMenuOpen ? "" : "hidden"} absolute left-0 top-full z-20 mt-2 min-w-[176px] rounded-2xl border border-slate-200 bg-white p-1 shadow-lg">
            <button type="button" class="block w-full rounded-xl px-3 py-2 text-left text-sm transition ${state.sortMode === "download" ? "bg-slate-100 font-medium text-slate-900" : "text-slate-600 hover:bg-slate-50 hover:text-slate-900"}" data-set-sort="download">下载量排序</button>
            <button type="button" class="block w-full rounded-xl px-3 py-2 text-left text-sm transition ${state.sortMode === "name" ? "bg-slate-100 font-medium text-slate-900" : "text-slate-600 hover:bg-slate-50 hover:text-slate-900"}" data-set-sort="name">名称排序</button>
            <button type="button" class="block w-full rounded-xl px-3 py-2 text-left text-sm transition ${state.sortMode === "format" ? "bg-slate-100 font-medium text-slate-900" : "text-slate-600 hover:bg-slate-50 hover:text-slate-900"}" data-set-sort="format">格式排序</button>
            <button type="button" class="block w-full rounded-xl px-3 py-2 text-left text-sm transition ${state.sortMode === "modified" ? "bg-slate-100 font-medium text-slate-900" : "text-slate-600 hover:bg-slate-50 hover:text-slate-900"}" data-set-sort="modified">修改日期排序</button>
            <div class="mx-2 my-1 border-t border-slate-100"></div>
            <button type="button" class="block w-full rounded-xl px-3 py-2 text-left text-sm transition ${state.sortDirection === "desc" ? "bg-slate-100 font-medium text-slate-900" : "text-slate-600 hover:bg-slate-50 hover:text-slate-900"}" data-set-sort-dir="desc">降序</button>
            <button type="button" class="block w-full rounded-xl px-3 py-2 text-left text-sm transition ${state.sortDirection === "asc" ? "bg-slate-100 font-medium text-slate-900" : "text-slate-600 hover:bg-slate-50 hover:text-slate-900"}" data-set-sort-dir="asc">升序</button>
          </div>
        </div>
        <div class="relative">
          <button type="button" class="inline-flex w-full items-center justify-center gap-2 rounded-xl border border-slate-200 px-3 py-2 text-sm font-medium text-slate-600 transition hover:border-slate-300 hover:text-slate-900 sm:w-auto" data-action="toggle-view-menu">${state.viewMode === "cards" ? Ico.grid : Ico.list}${state.viewMode === "cards" ? "卡片" : "表格"} ${Ico.chevronRight.replace("class=\"h-4 w-4\"", "class=\"h-4 w-4 rotate-90\"")}</button>
          <div class="${state.viewMenuOpen ? "" : "hidden"} absolute left-0 top-full z-20 mt-2 min-w-[124px] rounded-2xl border border-slate-200 bg-white p-1 shadow-lg">
            <button type="button" class="flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm transition ${state.viewMode === "cards" ? "bg-slate-100 font-medium text-slate-900" : "text-slate-600 hover:bg-slate-50 hover:text-slate-900"}" data-set-view="cards">${Ico.grid} 卡片</button>
            <button type="button" class="flex w-full items-center gap-2 rounded-xl px-3 py-2 text-left text-sm transition ${state.viewMode === "table" ? "bg-slate-100 font-medium text-slate-900" : "text-slate-600 hover:bg-slate-50 hover:text-slate-900"}" data-set-view="table">${Ico.list} 表格</button>
          </div>
        </div>
      </div>
    </div>
  </div>`;

  let mainList = "";
  if (state.loading) mainList = `<div class="px-4 py-8 text-sm text-slate-500 sm:px-6">加载中…</div>`;
  else if (state.error) mainList = `<div class="px-4 py-8 text-sm text-rose-600 sm:px-6">${escapeHtml(state.error)}</div>`;
  else if (rows.length === 0) {
    mainList = `<div class="px-4 py-8 text-sm text-slate-500 sm:px-6">${state.searchKeyword ? "没有找到匹配结果。" : "当前目录为空。"}</div>`;
  } else if (state.viewMode === "cards") {
    mainList = `<div class="grid grid-cols-1 gap-4 px-4 py-3 sm:grid-cols-2 sm:px-5 md:gap-5 lg:grid-cols-3 lg:gap-5 xl:grid-cols-4 2xl:grid-cols-4">${rows.map((row) => renderCard(row)).join("")}</div>`;
  } else {
    mainList = `<div class="px-4 py-5 sm:px-6"><table class="data-table table-fixed"><thead><tr><th class="text-left">名称</th><th class="w-[120px] text-right">大小</th><th class="hidden w-[220px] text-right xl:table-cell">修改时间</th></tr></thead><tbody>${rows.map((row) => renderTableRow(row)).join("")}</tbody></table></div>`;
  }

  const ann = state.announcements.slice(0, 5).map((a) => ({
    id: a.id,
    label: a.title,
    badge: a.is_pinned ? "置顶" : undefined,
  }));
  const hot = state.hotDownloadItems.slice(0, 5).map((h) => ({ id: h.id, label: h.name }));
  const latest = state.latestItems.slice(0, 5).map((l) => ({ id: l.id, label: l.name }));

  const aside = `
  <aside class="order-2 min-w-0 space-y-4">
    ${renderInfoPanel("公告栏", ann, "暂无公告", 'data-action="announcement-list"', "announcement")}
    ${renderInfoPanel("热门下载", hot, "暂无下载数据", 'data-action="hot-modal"', "file")}
    ${renderInfoPanel("资料上新", latest, "暂无最新资料", 'data-action="latest-modal"', "file")}
  </aside>`;

  return `
  <main class="pt-16">
    <div class="app-container py-2 sm:py-8 lg:py-10">
    <div class="grid gap-6 xl:grid-cols-[minmax(0,1fr)_248px]">
      <section class="order-1 min-w-0">
        <div class="panel overflow-hidden">
          <div class="border-b border-slate-200 px-4 py-3 sm:px-6">
            <div class="min-w-0 max-w-full overflow-x-auto">${breadcrumbHtml}</div>
          </div>
          ${folderInfoBlock}
          ${searchSection}
          ${state.searchError ? `<p class="mx-5 mt-3 rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700 sm:mx-6">${escapeHtml(state.searchError)}</p>` : ""}
          ${searchBanner}
          ${toolbar}
          ${mainList}
        </div>
      </section>
      ${aside}
    </div>
    </div>
  </main>`;
}

function sortModeLabel() {
  switch (state.sortMode) {
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

function sortDirectionLabel() {
  return state.sortDirection === "asc" ? "升序" : "降序";
}

function closeToolbarMenusIfOutside(e) {
  const app = document.getElementById("app");
  if (!app || !(e.target instanceof Node) || !app.contains(e.target)) return;
  if (!state.sortMenuOpen && !state.viewMenuOpen) return;
  const t = e.target instanceof Element ? e.target : e.target.parentElement;
  if (!(t instanceof Element)) return;
  if (t.closest("[data-toolbar-dropdowns]")) return;
  state.sortMenuOpen = false;
  state.viewMenuOpen = false;
  render();
}

function renderCard(row) {
  const prev = stripCoverImageMarkdown(row.description || "");
  const descPreview = prev.trim();
  const cover = row.coverUrl;
  const dlBtn =
    row.downloadAllowed && row.kind === "file"
      ? `<button type="button" class="inline-flex items-center justify-center rounded-xl border border-slate-200 bg-white p-2.5 text-slate-700 hover:border-slate-300 hover:bg-slate-50" data-download-row="${escapeHtml(row.kind)}:${escapeHtml(row.id)}">${Ico.download}</button>`
      : row.downloadAllowed && row.kind === "folder"
        ? `<button type="button" class="inline-flex items-center justify-center rounded-xl border border-slate-200 bg-white p-2.5 text-slate-700 hover:border-slate-300 hover:bg-slate-50" data-download-row="${escapeHtml(row.kind)}:${escapeHtml(row.id)}">${Ico.download}</button>`
        : "";

  if (cover) {
    return `
    <article class="group relative min-w-0 flex cursor-pointer flex-col overflow-hidden rounded-3xl border border-slate-200 bg-white transition hover:border-slate-300 hover:shadow-sm ${cover ? "min-h-0" : "min-h-[168px] px-4 pt-3.5 sm:px-5"}" data-open-row="${escapeHtml(row.kind)}:${escapeHtml(row.id)}">
      <div class="relative aspect-[16/10] min-h-[132px] w-full max-h-[220px] shrink-0 overflow-hidden bg-slate-100 sm:min-h-[148px] sm:max-h-[240px]">
        <img src="${escapeHtml(cover)}" alt="" class="absolute inset-0 h-full w-full object-cover" loading="lazy" />
      </div>
      <div class="flex min-h-0 flex-1 flex-col px-4 pb-3 pt-3 sm:px-5">
        <h3 class="line-clamp-2 text-base font-semibold leading-snug text-slate-900">${escapeHtml(row.name)}</h3>
        ${descPreview ? `<p class="mt-1 line-clamp-2 text-sm leading-5 text-slate-500">${escapeHtml(descPreview)}</p>` : ""}
        <div class="mt-3 flex min-w-0 flex-wrap items-center gap-x-4 gap-y-1 text-xs text-slate-500">
          ${row.kind === "file" ? `<span class="inline-flex items-center gap-1.5">${Ico.download} ${row.downloadCount}</span><span>${escapeHtml(row.sizeText)}</span>` : `<span class="inline-flex items-center gap-1.5">${Ico.download} ${row.downloadCount}</span><span>${row.fileCount} 个文件</span><span>${escapeHtml(row.sizeText)}</span>`}
          <span class="inline-flex min-w-0 max-w-full items-center gap-1.5">${Ico.clock}<span class="truncate">${escapeHtml(row.updatedAt)}</span></span>
        </div>
        <div class="mt-auto flex items-center justify-end border-t border-slate-100 pt-3">${dlBtn}</div>
      </div>
    </article>`;
  }
  const icon = row.kind === "folder" ? Ico.folder : fileIconSvg(row.extension);
  return `
  <article class="group relative min-w-0 flex min-h-[168px] cursor-pointer flex-col overflow-hidden rounded-3xl border border-slate-200 bg-white px-4 pt-3.5 transition hover:border-slate-300 hover:shadow-sm sm:px-5" data-open-row="${escapeHtml(row.kind)}:${escapeHtml(row.id)}">
    <div class="flex items-start gap-4">
      <div class="flex h-14 w-14 shrink-0 items-center justify-center overflow-hidden rounded-2xl bg-slate-100 text-slate-500">${icon}</div>
      <div class="min-w-0 flex-1 pr-2 pt-0.5">
        <h3 class="truncate text-base font-semibold leading-6 text-slate-900">${escapeHtml(row.name)}</h3>
        ${descPreview ? `<p class="mt-1 line-clamp-1 text-sm leading-5 text-slate-500">${escapeHtml(descPreview)}</p>` : ""}
      </div>
    </div>
    <div class="mt-3 flex min-w-0 flex-wrap items-center gap-x-4 gap-y-1 text-xs text-slate-500">
      ${row.kind === "file" ? `<span class="inline-flex items-center gap-1.5">${Ico.download} ${row.downloadCount}</span><span>${escapeHtml(row.sizeText)}</span>` : `<span class="inline-flex items-center gap-1.5">${Ico.download} ${row.downloadCount}</span><span>${row.fileCount} 个文件</span><span>${escapeHtml(row.sizeText)}</span>`}
      <span class="inline-flex min-w-0 max-w-full items-center gap-1.5">${Ico.clock}<span class="truncate">${escapeHtml(row.updatedAt)}</span></span>
    </div>
    <div class="mt-auto flex items-center justify-end border-t border-slate-100 py-2.5">${dlBtn}</div>
  </article>`;
}

function renderTableRow(row) {
  const cover = row.coverUrl;
  const icon =
    row.kind === "folder"
      ? cover
        ? `<img src="${escapeHtml(cover)}" alt="" class="h-5 w-5 shrink-0 rounded object-cover" loading="lazy" />`
        : Ico.folder.replace("h-7 w-7", "h-5 w-5 shrink-0")
      : cover
        ? `<img src="${escapeHtml(cover)}" alt="" class="h-5 w-5 shrink-0 rounded object-cover" loading="lazy" />`
        : fileIconSvg(row.extension).replace("h-7 w-7", "h-5 w-5 shrink-0");
  return `
  <tr class="cursor-pointer transition hover:bg-slate-50" data-open-row="${escapeHtml(row.kind)}:${escapeHtml(row.id)}">
    <td>
      <div class="flex min-w-0 items-center gap-3 text-left">${icon}<span class="truncate text-slate-900" title="${escapeHtml(row.name)}">${escapeHtml(row.name)}</span></div>
    </td>
    <td class="w-[120px] whitespace-nowrap text-right tabular-nums">${escapeHtml(row.sizeText)}</td>
    <td class="hidden w-[220px] whitespace-nowrap text-right tabular-nums xl:table-cell">${escapeHtml(row.updatedAt)}</td>
  </tr>`;
}

function renderFileDetail() {
  const d = state.fileDetail;
  const loading = state.fileLoading;
  const err = state.fileError;
  if (loading) {
    return `<section class="app-container py-2 sm:py-8 lg:py-10"><div class="mx-auto max-w-4xl space-y-6"><div class="panel p-6"><p class="text-sm text-slate-500">加载中…</p></div></div></section>`;
  }
  if (err) {
    return `<section class="app-container py-2 sm:py-8 lg:py-10"><div class="mx-auto max-w-4xl space-y-6"><div class="panel p-6 space-y-4"><p class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">${escapeHtml(err)}</p><div class="flex gap-3"><button type="button" class="btn-secondary" data-action="detail-back">返回上一页</button><button type="button" class="btn-primary" data-action="detail-home">返回首页</button></div></div></div></section>`;
  }
  if (!d) {
    return `<section class="app-container py-2 sm:py-8 lg:py-10"><div class="mx-auto max-w-4xl"><p class="text-sm text-slate-500">无数据</p></div></section>`;
  }

  const isVideo = isVideoDetail(d);
  const layoutWide = isVideo && Boolean((d.folder_id ?? "").trim());
  const descHtml = renderSimpleMarkdown(d.description ?? "");
  const coverHref = fileCoverImageHrefFromFields(d.cover_url, d.description ?? "");
  const absDl = mediaSourceURL(d, d.id);
  const absDlFull = absDl.startsWith("http") ? absDl : new URL(absDl, window.location.origin).href;
  const detailUrl = absoluteDetailPageURL(d.id);
  const q = buildVideoPlaybackUrlQueue(d.id, d);
  const activeSrc = q[Math.min(state.videoPlaybackStep, q.length - 1)] ?? "";

  const metaPrimary = `<div class="grid gap-x-8 gap-y-3 lg:grid-cols-2"><div class="grid min-w-0 grid-cols-[88px_minmax(0,1fr)] items-baseline gap-x-3 text-sm"><span class="text-slate-500">所属文件夹</span><span class="min-w-0 truncate font-medium text-slate-900" title="${escapeHtml(d.path || "")}">${escapeHtml(d.path || "主页根目录")}</span></div></div>`;
  const metaSecondary = `
  <div class="grid gap-x-8 gap-y-3 sm:grid-cols-2 lg:grid-cols-3">
    <div class="grid min-w-0 grid-cols-[88px_minmax(0,1fr)] items-baseline gap-x-3 text-sm"><span class="text-slate-500">下载量</span><span class="font-medium text-slate-900">${d.download_count}</span></div>
    <div class="grid min-w-0 grid-cols-[88px_minmax(0,1fr)] items-baseline gap-x-3 text-sm"><span class="text-slate-500">文件大小</span><span class="font-medium text-slate-900">${formatSize(d.size)}</span></div>
    <div class="grid min-w-0 grid-cols-[88px_minmax(0,1fr)] items-baseline gap-x-3 text-sm"><span class="text-slate-500">更新时间</span><span class="font-medium text-slate-900">${formatDateTime(d.uploaded_at)}</span></div>
  </div>`;

  const videoBlock =
    isVideo && activeSrc
      ? `
  <div class="flex flex-col gap-4 lg:flex-row lg:items-stretch">
    <div class="min-w-0 flex-1 overflow-hidden rounded-2xl border border-slate-200 bg-slate-950 shadow-inner ring-1 ring-black/5">
      <video id="detail-video" class="max-h-[min(70vh,720px)] w-full object-contain" controls playsinline preload="metadata" src="${escapeHtml(activeSrc)}"></video>
    </div>
    ${
      (d.folder_id ?? "").trim()
        ? `<aside class="flex w-full shrink-0 flex-col rounded-2xl border border-slate-200 bg-white lg:w-72 xl:w-80">
      <div class="border-b border-slate-100 px-4 py-3">
        <p class="text-xs font-semibold uppercase tracking-[0.14em] text-slate-400">Playlist</p>
        <p class="mt-1 text-sm font-medium text-slate-900">同文件夹视频</p>
      </div>
      <div class="min-h-[120px] max-h-[min(70vh,720px)] overflow-y-auto px-2 py-2">
        ${state.folderVideoPeersLoading ? `<p class="px-2 py-6 text-center text-sm text-slate-500">加载列表…</p>` : state.folderVideoPeers.length ? `<ul class="space-y-1">${state.folderVideoPeers.map((p) => `<li><a href="#/files/${encodeURIComponent(p.id)}" class="flex min-w-0 items-start gap-2 rounded-xl px-2 py-2 text-left text-sm text-slate-700 hover:bg-slate-50">${Ico.fileVideo}<span class="min-w-0 break-words leading-snug">${escapeHtml(p.name)}</span></a></li>`).join("")}</ul>` : `<p class="px-2 py-6 text-center text-sm text-slate-500">当前文件夹没有其他视频</p>`}
      </div>
    </aside>`
        : ""
    }
  </div>`
      : "";

  const hint = `<p id="file-detail-copy-hint" role="status" class="mb-5 rounded-xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-700 ${state.linkCopyHint ? "" : "hidden"}">${state.linkCopyHint ? escapeHtml(state.linkCopyHint) : ""}</p>`;

  const metaVisible = !isVideo || state.videoFileMetaVisible;
  const dlAllowed = d.download_allowed !== false;

  return `
  <section class="app-container py-2 sm:py-8 lg:py-10">
    <div class="mx-auto w-full space-y-6 ${layoutWide ? "max-w-6xl" : "max-w-4xl"}">
      <div class="panel p-6">
        ${hint}
        <section>
          <div class="space-y-4">
            <div class="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
              <div class="space-y-2">
                <div class="flex items-center gap-2">
                  <button type="button" class="inline-flex h-8 w-8 shrink-0 items-center justify-center rounded-lg border border-slate-200 bg-white text-slate-600 hover:border-slate-300" data-action="detail-back" aria-label="返回">${Ico.chevronLeft}</button>
                  <p class="text-xs font-semibold uppercase tracking-[0.18em] text-blue-600">File Info</p>
                </div>
                <h3 class="break-words text-2xl font-semibold tracking-tight text-slate-900 sm:text-3xl">${escapeHtml(d.name)}</h3>
              </div>
              <div class="min-w-0 max-w-full">
                <div class="flex min-w-0 max-w-full flex-nowrap items-center gap-2 overflow-x-auto py-2 sm:gap-3 lg:justify-end">
                  ${isVideo ? `<button id="file-detail-toggle-meta-btn" type="button" class="inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border border-slate-200 bg-white text-slate-600" data-action="toggle-video-meta" aria-label="文件信息" aria-controls="file-detail-meta-panel" aria-expanded="${state.videoFileMetaVisible ? "true" : "false"}">${Ico.fileText}</button>` : ""}
                  ${dlAllowed ? `<button type="button" class="inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border border-slate-200 bg-white text-slate-600" data-copy="${escapeHtml(absDlFull)}" data-copy-label="下载直链" title="复制下载直链" aria-label="复制下载直链">${Ico.link2}</button>` : ""}
                  <button type="button" class="inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border border-slate-200 bg-white text-slate-600" data-copy="${escapeHtml(detailUrl)}" data-copy-label="详情页链接" title="复制详情页链接" aria-label="复制详情页链接">${Ico.share}</button>
                  ${isVideo && dlAllowed ? `<button type="button" class="inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border border-slate-200 bg-white text-slate-600" data-action="copy-at-time" title="复制带时间戳的链接" aria-label="复制含时间戳的详情页链接">${Ico.clockBig}</button>` : ""}
                  ${dlAllowed ? `<button type="button" class="inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border border-slate-200 bg-white text-slate-700" data-action="dl-file" aria-label="下载文件">${Ico.download}</button>` : ""}
                </div>
              </div>
            </div>
            <div id="file-detail-meta-panel" class="${metaVisible ? "" : "hidden"} min-w-0 space-y-3">
              ${metaPrimary}
              ${metaSecondary}
            </div>
            ${videoBlock}
            ${
              !isVideo && coverHref
                ? `<div class="mb-4 overflow-hidden rounded-2xl border border-slate-100 bg-slate-50 ring-1 ring-slate-950/[0.04]"><img src="${escapeHtml(coverHref)}" alt="" class="max-h-72 w-full object-cover" loading="lazy" /></div>`
                : ""
            }
            <div class="mt-4 rounded-3xl border border-slate-200 bg-white px-4 py-4 sm:px-5 sm:py-5">
              ${descHtml ? `<div class="markdown-content">${descHtml}</div>` : `<p class="text-sm text-slate-400">该文件暂无简介orz</p>`}
            </div>
          </div>
        </section>
      </div>
    </div>
  </section>`;
}

function renderModals() {
  let html = "";
  if (state.downloadConfirm) {
    const dc = state.downloadConfirm;
    let body = "";
    if (dc.kind === "row") {
      const r = dc.row;
      body =
        r.kind === "folder"
          ? "您即将打包下载整个文件夹，体积与耗时不确定，可能较大。确定要开始下载吗？"
          : `该文件大小为 ${escapeHtml(r.sizeText)}，已超过本站设定的大文件阈值（${escapeHtml(formatSize(state.largeDownloadConfirmBytes))}）。确定要下载吗？`;
    } else if (dc.kind === "folderToolbar") {
      body = "您即将打包下载整个文件夹，体积与耗时不确定，可能较大。确定要开始下载吗？";
    } else if (dc.kind === "fileDetail") {
      const fd = state.fileDetail;
      body = fd
        ? `该文件大小为 ${escapeHtml(formatSize(fd.size))}，已超过本站设定的大文件阈值（${escapeHtml(formatSize(state.largeDownloadConfirmBytes))}）。确定要下载吗？`
        : "";
    }
    html += `
    <div class="fixed inset-0 z-[125] flex items-center justify-center bg-slate-950/30 px-4">
      <div class="modal-card panel w-full max-w-md rounded-2xl bg-white p-6 shadow-xl" data-stop-modal="1">
        <h3 class="text-lg font-semibold text-slate-900">确认下载</h3>
        <p class="mt-3 text-sm leading-6 text-slate-600">${body}</p>
        <div class="mt-6 flex flex-wrap justify-end gap-3">
          <button type="button" class="btn-secondary" data-action="download-confirm-no">取消</button>
          <button type="button" class="btn-primary" data-action="download-confirm-yes">确认下载</button>
        </div>
      </div>
    </div>`;
  }
  if (state.modalAnnouncementList) {
    const items = state.announcements
      .map(
        (item) => `
      <button type="button" class="flex w-full items-start justify-between gap-4 rounded-2xl border border-slate-200 bg-white px-4 py-4 text-left hover:border-blue-200 hover:bg-blue-50/40" data-announcement-open="${escapeHtml(item.id)}">
        <div class="min-w-0">
          <div class="flex flex-wrap items-center gap-2">
            ${item.is_pinned ? `<span class="rounded-md bg-[#dcecff] px-2 py-0.5 text-xs font-semibold text-[#4f8ff7]">置顶</span>` : ""}
            <p class="text-base font-semibold text-slate-900">${escapeHtml(item.title)}</p>
          </div>
          <p class="mt-2 line-clamp-2 text-sm text-slate-500">${escapeHtml(item.content)}</p>
        </div>
        <span class="shrink-0 text-sm text-slate-400">${escapeHtml(formatDateTime(item.published_at || item.updated_at))}</span>
      </button>`,
      )
      .join("");
    html += `
    <div class="fixed inset-0 z-[120] flex items-center justify-center bg-slate-950/30 px-4" data-close-modal="1">
      <div class="modal-card panel w-full max-w-3xl p-6" data-stop-modal="1">
        <div class="flex items-start justify-between gap-4 border-b border-slate-200 pb-4">
          <div><p class="text-xs font-semibold uppercase tracking-[0.18em] text-blue-600">Announcements</p><h3 class="mt-2 text-2xl font-semibold text-slate-900">全部公告</h3></div>
          <button type="button" class="btn-secondary" data-action="close-announcement-list">关闭</button>
        </div>
        <div class="mt-5 max-h-[70vh] space-y-3 overflow-auto pr-1">${items || `<p class="text-center text-sm text-slate-500">暂无公告</p>`}</div>
      </div>
    </div>`;
  }
  if (state.modalAnnouncement) {
    const item = state.modalAnnouncement;
    html += `
    <div class="fixed inset-0 z-[120] flex items-center justify-center bg-slate-950/30 px-4" data-close-modal="1">
      <div class="modal-card panel w-full max-w-2xl p-6" data-stop-modal="1">
        <div class="flex items-start justify-between gap-4 border-b border-slate-200 pb-4">
          <div class="min-w-0">
            <p class="text-xs font-semibold uppercase tracking-[0.18em] text-blue-600">Announcement</p>
            <h3 class="mt-2 text-2xl font-semibold tracking-tight text-slate-900">${escapeHtml(item.title)}</h3>
            <p class="mt-2 text-sm text-slate-500">${escapeHtml(formatDateTime(item.published_at || item.updated_at))}</p>
          </div>
          <div class="flex gap-2">
            <button type="button" class="btn-secondary" data-action="back-announcement-list">返回</button>
            <button type="button" class="btn-secondary" data-action="close-announcement">关闭</button>
          </div>
        </div>
        <div class="mt-5 rounded-3xl border border-slate-200 bg-white px-5 py-5"><div class="markdown-content">${renderSimpleMarkdown(item.content)}</div></div>
      </div>
    </div>`;
  }
  if (state.modalSidebar) {
    const m = state.modalSidebar;
    const list = m.items
      .map(
        (it, i) => `
      <button type="button" class="flex w-full items-center gap-4 rounded-2xl border border-slate-200 px-4 py-3 text-left hover:border-slate-300 hover:bg-slate-50" data-sidebar-open="${escapeHtml(it.id)}">
        <span class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-slate-100 text-sm font-semibold text-slate-600">${i + 1}</span>
        <div class="min-w-0 flex-1"><p class="truncate text-sm font-medium text-slate-900">${escapeHtml(it.label)}</p></div>
        ${it.meta ? `<span class="shrink-0 text-sm text-slate-500">${escapeHtml(it.meta)}</span>` : ""}
      </button>`,
      )
      .join("");
    html += `
    <div class="fixed inset-0 z-[120] flex items-center justify-center bg-slate-950/30 px-4" data-close-modal="1">
      <div class="modal-card panel w-full max-w-3xl p-6" data-stop-modal="1">
        <div class="flex items-start justify-between gap-4 border-b border-slate-200 pb-4">
          <div>
            <p class="text-xs font-semibold uppercase tracking-[0.18em] text-blue-600">${escapeHtml(m.eyebrow)}</p>
            <h3 class="mt-2 text-2xl font-semibold tracking-tight text-slate-900">${escapeHtml(m.title)}</h3>
            <p class="mt-2 text-sm text-slate-500">${escapeHtml(m.description)}</p>
          </div>
          <button type="button" class="btn-secondary" data-action="close-sidebar">关闭</button>
        </div>
        <div class="mt-5 max-h-[70vh] overflow-y-auto pr-1 space-y-3">${list || `<p class="text-sm text-slate-500">暂无数据</p>`}</div>
      </div>
    </div>`;
  }
  return html;
}

function renderWarning() {
  if (!state.transientWarning) return "";
  return `<div class="fixed inset-0 z-[130] flex items-center justify-center px-4 pointer-events-none"><div class="rounded-2xl border border-rose-200 bg-white px-4 py-3 text-sm text-rose-700 shadow-lg">${escapeHtml(state.transientWarning)}</div></div>`;
}

function render() {
  const app = document.getElementById("app");
  if (!app) return;
  const fileFab =
    state.route.view === "file"
      ? `<button type="button" class="fixed bottom-6 right-6 z-[70] rounded-full border border-slate-200 bg-white px-4 py-2 text-xs font-medium text-slate-700 shadow-lg hover:bg-slate-50" data-action="toggle-settings">API</button>`
      : "";
  const main =
    state.route.view === "file"
      ? `${renderFileDetail()}${fileFab}`
      : `<div class="app-shell">${renderNavbar()}${renderHome()}</div>`;
  app.innerHTML = `${main}${renderSettingsPanel()}${renderModals()}${renderWarning()}`;
  attachListeners();
  if (state.route.view === "file" && state.fileDetail && isVideoDetail(state.fileDetail)) {
    const vid = document.getElementById("detail-video");
    if (vid) {
      const t = parseTimestampQuery(state.route.t);
      if (t != null) {
        vid.addEventListener(
          "loadedmetadata",
          () => {
            try {
              vid.currentTime = Math.min(Math.max(0, t), Math.max(0, vid.duration - 0.05));
            } catch {
              /* ignore */
            }
          },
          { once: true },
        );
      }
      vid.addEventListener("error", onVideoError);
    }
  }
}

function onVideoError() {
  const d = state.fileDetail;
  if (!d || !isVideoDetail(d)) return;
  const q = buildVideoPlaybackUrlQueue(d.id, d);
  if (state.videoPlaybackStep < q.length - 1) {
    state.videoPlaybackStep++;
    render();
  }
}

function getRowByKey(key) {
  const [kind, id] = key.split(":");
  return sortedRows().find((r) => r.kind === kind && r.id === id);
}

let globalHandlersInstalled = false;

function installGlobalHandlersOnce() {
  if (globalHandlersInstalled) return;
  globalHandlersInstalled = true;
  document.addEventListener("pointerdown", closeToolbarMenusIfOutside, true);
  document.addEventListener("click", appClickHandler, true);
  document.addEventListener("submit", searchSubmitHandler, true);
  document.addEventListener("change", apiBaseChangeHandler, true);
}

function apiBaseChangeHandler(e) {
  const t = e.target;
  if (!(t instanceof HTMLInputElement) || !t.matches("[data-input=\"api-base\"]")) return;
  const app = document.getElementById("app");
  if (!app || !app.contains(t)) return;
  setApiBase(t.value);
}

function searchSubmitHandler(e) {
  const f = e.target;
  if (!(f instanceof HTMLFormElement) || f.getAttribute("data-form") !== "search") return;
  const app = document.getElementById("app");
  if (!app || !app.contains(f)) return;
  e.preventDefault();
  const fd = new FormData(f);
  const q = String(fd.get("q") ?? "");
  void runSearch(q).then(() => render());
}

function appClickHandler(e) {
  const app = document.getElementById("app");
  if (!app || !(e.target instanceof Node) || !app.contains(e.target)) return;
  const t = e.target instanceof Element ? e.target : e.target.parentElement;
  if (!(t instanceof Element)) return;
  if (t.closest("[data-stop-modal]")) {
    e.stopPropagation();
  }

  if (t.closest("[data-action=\"download-confirm-yes\"]")) {
    e.preventDefault();
    const dc = state.downloadConfirm;
    state.downloadConfirm = null;
    if (dc?.kind === "row") performDownloadRow(dc.row);
    else if (dc?.kind === "folderToolbar") performDownloadCurrentFolder();
    else if (dc?.kind === "fileDetail") performDownloadFileFromDetail();
    render();
    return;
  }
  if (t.closest("[data-action=\"download-confirm-no\"]")) {
    e.preventDefault();
    state.downloadConfirm = null;
    render();
    return;
  }

      const openAnn = t.closest("[data-announcement-open]");
      if (openAnn) {
        const id = openAnn.getAttribute("data-announcement-open");
        state.modalAnnouncement = state.announcements.find((a) => a.id === id) || null;
        state.modalAnnouncementList = false;
        render();
        return;
      }

      const annPick = t.closest("[data-ann-pick]");
      if (annPick) {
        const id = annPick.getAttribute("data-ann-pick");
        const item = state.announcements.find((a) => a.id === id);
        if (item) {
          state.modalAnnouncement = item;
          state.modalAnnouncementList = false;
          render();
        }
        return;
      }

      const sidebarOpen = t.closest("[data-sidebar-open]");
      if (sidebarOpen) {
        const id = sidebarOpen.getAttribute("data-sidebar-open");
        setHashRoute({ view: "file", fileId: id, folder: "", root: "", t: "" });
        state.modalSidebar = null;
        return;
      }

      const sidebarItem = t.closest("[data-sidebar-item]");
      if (sidebarItem) {
        const id = sidebarItem.getAttribute("data-sidebar-item");
        setHashRoute({ view: "file", fileId: id, folder: "", root: "", t: "" });
        return;
      }

      const row = t.closest("[data-open-row]");
      if (row) {
        const key = row.getAttribute("data-open-row");
        if (!key) return;
        const [kind, id] = key.split(":");
        if (kind === "folder") setHashRoute({ view: "home", folder: id, root: "", fileId: "", t: "" });
        else setHashRoute({ view: "file", fileId: id, folder: "", root: "", t: "" });
        return;
      }

      const dl = t.closest("[data-download-row]");
      if (dl) {
        const key = dl.getAttribute("data-download-row");
        if (!key) return;
        const r = getRowByKey(key);
        if (r) {
          e.preventDefault();
          e.stopPropagation();
          downloadRow(r);
        }
        return;
      }

      const copyBtn = t.closest("[data-copy]");
      if (copyBtn) {
        const url = copyBtn.getAttribute("data-copy") || "";
        const label = copyBtn.getAttribute("data-copy-label") || "链接";
        e.preventDefault();
        void copyText(label, url);
        return;
      }

      if (t.closest("[data-action=\"toggle-settings\"]")) {
        state.settingsOpen = !state.settingsOpen;
        render();
        return;
      }
      if (t.closest("[data-action=\"save-api\"]")) {
        const input = app.querySelector("[data-input=\"api-base\"]");
        if (input && "value" in input) setApiBase(input.value);
        state.settingsOpen = false;
        location.reload();
        return;
      }
      if (t.closest("[data-action=\"close-settings\"]")) {
        state.settingsOpen = false;
        render();
        return;
      }
      if (t.closest("[data-action=\"toggle-folder-md\"]")) {
        state.folderMarkdownExpanded = !state.folderMarkdownExpanded;
        render();
        return;
      }
      if (t.closest("[data-action=\"dl-folder\"]")) {
        downloadCurrentFolder();
        return;
      }
      if (t.closest("[data-action=\"go-up\"]")) {
        if (state.searchKeyword) {
          clearSearchState();
          render();
          return;
        }
        const bc = state.breadcrumbs;
        const parent = bc.length >= 2 ? bc[bc.length - 2] : null;
        if (parent) setHashRoute({ view: "home", folder: parent.id, root: "", fileId: "", t: "" });
        else setHashRoute({ view: "home", folder: "", root: "1", fileId: "", t: "" });
        return;
      }
      if (t.closest("[data-action=\"open-root\"]")) {
        clearSearchState();
        setHashRoute({ view: "home", folder: "", root: "1", fileId: "", t: "" });
        return;
      }
      const folderBtn = t.closest("[data-folder]");
      if (folderBtn) {
        const id = folderBtn.getAttribute("data-folder");
        if (id) {
          clearSearchState();
          setHashRoute({ view: "home", folder: id, root: "", fileId: "", t: "" });
        }
        return;
      }
      if (t.closest("[data-action=\"toggle-sort-menu\"]")) {
        state.sortMenuOpen = !state.sortMenuOpen;
        state.viewMenuOpen = false;
        render();
        return;
      }
      if (t.closest("[data-action=\"toggle-view-menu\"]")) {
        state.viewMenuOpen = !state.viewMenuOpen;
        state.sortMenuOpen = false;
        render();
        return;
      }
      const sort = t.closest("[data-set-sort]");
      if (sort) {
        const mode = sort.getAttribute("data-set-sort");
        if (mode === "name" || mode === "download" || mode === "format" || mode === "modified") {
          state.sortMode = mode;
          savePref(LS_SORT, mode);
        }
        render();
        return;
      }
      const sortDir = t.closest("[data-set-sort-dir]");
      if (sortDir) {
        const d = sortDir.getAttribute("data-set-sort-dir");
        if (d === "asc" || d === "desc") {
          state.sortDirection = d;
          savePref(LS_SORT_DIR, d);
        }
        state.sortMenuOpen = false;
        render();
        return;
      }
      const vw = t.closest("[data-set-view]");
      if (vw) {
        const v = vw.getAttribute("data-set-view");
        if (v === "cards" || v === "table") {
          state.viewMode = v;
          savePref(LS_VIEW, v);
        }
        state.viewMenuOpen = false;
        render();
        return;
      }
      if (t.closest("[data-action=\"announcement-list\"]")) {
        state.modalAnnouncementList = true;
        render();
        return;
      }
      if (t.closest("[data-action=\"hot-modal\"]")) {
        state.modalSidebar = {
          eyebrow: "Hot Downloads",
          title: "热门下载",
          description: "展示近七天内下载量最高的前 20 份资料。",
          items: state.hotDownloadItems.map((item) => ({
            id: item.id,
            label: item.name,
            meta: `${item.downloadCount} 次下载`,
          })),
        };
        render();
        return;
      }
      if (t.closest("[data-action=\"latest-modal\"]")) {
        state.modalSidebar = {
          eyebrow: "Latest Files",
          title: "资料上新",
          description: "展示最新发布的前 20 份资料。",
          items: state.latestItems.map((item) => ({ id: item.id, label: item.name })),
        };
        render();
        return;
      }
      if (t.closest("[data-action=\"close-announcement-list\"]")) {
        state.modalAnnouncementList = false;
        render();
        return;
      }
      if (t.closest("[data-action=\"close-announcement\"]")) {
        state.modalAnnouncement = null;
        render();
        return;
      }
      if (t.closest("[data-action=\"back-announcement-list\"]")) {
        state.modalAnnouncement = null;
        state.modalAnnouncementList = true;
        render();
        return;
      }
      if (t.closest("[data-action=\"close-sidebar\"]")) {
        state.modalSidebar = null;
        render();
        return;
      }
      if (t.closest("[data-close-modal]") && !t.closest("[data-stop-modal]")) {
        state.modalAnnouncement = null;
        state.modalAnnouncementList = false;
        state.modalSidebar = null;
        render();
        return;
      }
      if (t.closest("[data-action=\"detail-back\"]")) {
        goBackFromDetail();
        return;
      }
      if (t.closest("[data-action=\"detail-home\"]")) {
        setHashRoute({ view: "home", folder: "", root: "", fileId: "", t: "" });
        return;
      }
      if (t.closest("[data-action=\"toggle-video-meta\"]")) {
        if (!toggleFileDetailVideoMetaFromDOM()) {
          state.videoFileMetaVisible = !state.videoFileMetaVisible;
          render();
        }
        return;
      }
      if (t.closest("[data-action=\"copy-at-time\"]")) {
        if (!state.fileDetail) return;
        const vid = document.getElementById("detail-video");
        const sec = vid ? vid.currentTime : 0;
        const u = buildDetailPageURLWithT(state.fileDetail.id, sec);
        void copyText("含时间戳的链接", u);
        return;
      }
      if (t.closest("[data-action=\"dl-file\"]")) {
        downloadFileFromDetail();
        return;
      }
      if (t.closest("[data-action=\"clear-search\"]")) {
        clearSearchState();
        render();
        return;
      }
}

function attachListeners() {
  installGlobalHandlersOnce();
}

async function bootstrapRoute() {
  syncRouteFromHash();
  state.sortMenuOpen = false;
  state.viewMenuOpen = false;
  state.modalAnnouncement = null;
  state.modalAnnouncementList = false;
  state.modalSidebar = null;
  state.folderMarkdownExpanded = false;
  state.videoPlaybackStep = 0;
  state.downloadConfirm = null;
  if (state.route.view === "file") {
    await Promise.all([loadFileDetail(), loadDownloadSettings()]);
  } else {
    clearSearchState();
    await Promise.all([loadAnnouncements(), loadHotDownloads(), loadLatestTitles(), loadDirectory(), loadDownloadSettings()]);
  }
  render();
}

window.addEventListener("hashchange", () => {
  consumeApiFromHashQuery();
  void bootstrapRoute();
});

/** 页面级查询串：`page.html?api=https://host/api`（写入后从地址栏移除） */
function consumeApiFromQueryOnce() {
  try {
    const u = new URL(window.location.href);
    const api = u.searchParams.get("api");
    if (api != null && api.trim() !== "") {
      setApiBase(api.trim());
      u.searchParams.delete("api");
      history.replaceState({}, "", u.pathname + u.search + u.hash);
    }
  } catch {
    /* ignore */
  }
}

/**
 * Hash 内查询串：`#/files/xxx?t=1&api=https://host/api`
 * 写入 localStorage 后从 hash 中去掉 api，避免泄露与重复应用。
 * 兼容误写为 `?t=1&https://host/api`（整段 URL 被当成参数名）的情况。
 */
function consumeApiFromHashQuery() {
  try {
    const raw = (location.hash || "").replace(/^#/, "");
    if (!raw.includes("?")) {
      return;
    }
    const qi = raw.indexOf("?");
    const pathPart = raw.slice(0, qi);
    const search = raw.slice(qi + 1);
    const sp = new URLSearchParams(search);
    let api = sp.get("api");
    if (api != null) {
      api = api.trim();
    }
    if (api) {
      sp.delete("api");
    } else {
      for (const [k, v] of sp.entries()) {
        if (v === "" && /^https?:\/\//i.test(k)) {
          api = k.trim();
          sp.delete(k);
          break;
        }
      }
    }
    if (!api) {
      return;
    }
    setApiBase(api);
    const newSearch = sp.toString();
    const newHash = newSearch ? `#${pathPart}?${newSearch}` : `#${pathPart}`;
    const u = new URL(window.location.href);
    history.replaceState({}, "", `${u.pathname}${u.search}${newHash}`);
  } catch {
    /* ignore */
  }
}

document.addEventListener("DOMContentLoaded", () => {
  loadPrefs();
  consumeApiFromQueryOnce();
  consumeApiFromHashQuery();
  void bootstrapRoute();
});



