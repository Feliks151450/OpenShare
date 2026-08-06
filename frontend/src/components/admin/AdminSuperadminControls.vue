<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { useRouter } from "vue-router";

import SurfaceCard from "../ui/SurfaceCard.vue";
import { httpClient } from "../../lib/http/client";
import { readApiError } from "../../lib/http/helpers";
import { toastError, toastInfo, toastSuccess, toastWarning } from "../../lib/toast";

interface SystemPolicy {
  upload: {
    max_upload_total_bytes: number;
  };
  download: {
    large_download_confirm_bytes: number;
    wide_layout_extensions?: string;
    cdn_mode?: boolean;
    global_cdn_url?: string;
  };
  cover_upload_dir?: string;
}

interface ManagedFolderNode {
  id: string;
  name: string;
  source_path: string;
  hide_public_catalog?: boolean;
  cdn_url?: string;
  folders: ManagedFolderNode[];
}

const router = useRouter();

const loading = ref(false);
const loaded = ref(false);
const uploadSaving = ref(false);
const error = ref("");
const message = ref("");
const importPath = ref("");
const importCurrentPath = ref("");
const importParentPath = ref("");
const importItems = ref<Array<{ name: string; path: string }>>([]);
const importLoading = ref(false);
const importMessage = ref("");
const importError = ref("");
const directoryPickerOpen = ref(false);
const pendingImportPath = ref("");
const manualBrowsePath = ref("");
const confirmedImportPath = ref("");
const importFilter = ref("");
const managedFolders = ref<Array<{ id: string; name: string; sourcePath: string; hidePublicCatalog: boolean; cdnUrl: string; guestKeyRequired: boolean; allowedGuestKeyIds: string[] }>>([]);
const managedFoldersLoading = ref(false);
const managedFoldersError = ref("");
const catalogVisibilitySaving = ref("");
const unmanagingFolderID = ref("");
const unmanagePassword = ref("");
const unmanageError = ref("");
const unmanageMessage = ref("");
const rescanningFolderID = ref("");
const rescanError = ref("");
const rescanMessage = ref("");

// 访客密钥访问：全局配置
interface GuestAccessKeyDraft {
  id: string;
  name: string;
  value: string;
  hint: string;
}
const guestAccessEnabled = ref(false);
const guestAccessKeys = ref<GuestAccessKeyDraft[]>([]);
const guestAccessSnapshot = ref("{}");
const guestAccessSaving = ref(false);
const guestAccessMessage = ref("");
const guestAccessError = ref("");
const guestAccessDirty = computed(() => JSON.stringify(serializeGuestAccess()) !== guestAccessSnapshot.value);

function serializeGuestAccess() {
  return {
    enabled: guestAccessEnabled.value,
    keys: guestAccessKeys.value.map((k) => ({
      id: (k.id ?? "").trim(),
      name: (k.name ?? "").trim(),
      value: (k.value ?? "").trim(),
      hint: (k.hint ?? "").trim(),
    })),
  };
}

function addGuestAccessKeyDraft() {
  guestAccessKeys.value.push({ id: "", name: "", value: "", hint: "" });
}

function removeGuestAccessKeyDraft(idx: number) {
  guestAccessKeys.value.splice(idx, 1);
}

async function loadGuestAccessConfig() {
  try {
    const resp = await httpClient.get<{ enabled: boolean; keys: GuestAccessKeyDraft[] }>("/admin/system/guest-access");
    guestAccessEnabled.value = Boolean(resp.enabled);
    guestAccessKeys.value = (resp.keys ?? []).map((k) => ({
      id: k.id ?? "",
      name: k.name ?? "",
      value: k.value ?? "",
      hint: k.hint ?? "",
    }));
    guestAccessSnapshot.value = JSON.stringify(serializeGuestAccess());
  } catch (err: unknown) {
    toastError(readApiError(err, "加载访客密钥配置失败。"));
  }
}

async function saveGuestAccessConfig() {
  guestAccessSaving.value = true;
  guestAccessError.value = "";
  guestAccessMessage.value = "";
  try {
    const payload = serializeGuestAccess();
    // 给未填 ID 的密钥生成 UUID（保持引用稳定）
    payload.keys = payload.keys.map((k) => {
      if (!k.id) {
        const fallback = `key-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
        // typeof 守卫避免在不支持 crypto 的环境下触发 ReferenceError
        const newId = (typeof crypto !== "undefined" && typeof crypto.randomUUID === "function")
          ? crypto.randomUUID()
          : fallback;
        return { ...k, id: newId };
      }
      return k;
    });
    const resp = await httpClient.put<{ enabled: boolean; keys: GuestAccessKeyDraft[] }>(
      "/admin/system/guest-access",
      payload,
    );
    guestAccessEnabled.value = Boolean(resp.enabled);
    guestAccessKeys.value = (resp.keys ?? []).map((k) => ({
      id: k.id ?? "",
      name: k.name ?? "",
      value: k.value ?? "",
      hint: k.hint ?? "",
    }));
    guestAccessSnapshot.value = JSON.stringify(serializeGuestAccess());
    guestAccessMessage.value = "访客密钥配置已保存。";
  } catch (err: unknown) {
    toastError(readApiError(err, "保存访客密钥配置失败。"));
  } finally {
    guestAccessSaving.value = false;
  }
}
// 虚拟托管根目录创建
const virtualRootName = ref("");
const virtualRootCreating = ref(false);
const virtualRootError = ref("");
const exportingGlobal = ref(false);
const exportingFolderId = ref("");
const uploadSizeValue = ref(5);
const uploadSizeUnit = ref<"B" | "KB" | "MB" | "GB">("GB");
const uploadSnapshot = ref("");
const downloadConfirmSizeValue = ref(1);
const downloadConfirmSizeUnit = ref<"B" | "KB" | "MB" | "GB">("GB");
const downloadSnapshot = ref("");
const form = reactive<SystemPolicy>({
  upload: {
    max_upload_total_bytes: 0,
  },
  download: {
    large_download_confirm_bytes: 1024 * 1024 * 1024,
    wide_layout_extensions: "",
    cdn_mode: false,
    global_cdn_url: "",
  },
  cover_upload_dir: "",
});

onMounted(() => {
  void Promise.all([loadPolicy(), loadDirectories(""), loadManagedFolders(), loadGuestAccessConfig()]);
});

async function loadPolicy() {
  loading.value = true;
  error.value = "";
  message.value = "";
  try {
    const response = await httpClient.get<SystemPolicy>("/admin/system/settings");
    Object.assign(form.upload, response.upload);
    Object.assign(form.download, response.download ?? { large_download_confirm_bytes: 1024 * 1024 * 1024 });
    form.cover_upload_dir = response.cover_upload_dir ?? "";
    applyUploadSizeFields(response.upload.max_upload_total_bytes);
    applyDownloadSizeFields(form.download.large_download_confirm_bytes);
    uploadSnapshot.value = serializeUploadState();
    downloadSnapshot.value = serializeDownloadState();
  } catch {
    toastError("加载系统设置失败。");
  } finally {
    loaded.value = true;
    loading.value = false;
  }
}

async function saveUploadPolicy() {
  uploadSaving.value = true;
  error.value = "";
  message.value = "";
  form.upload.max_upload_total_bytes = toBytes(uploadSizeValue.value, uploadSizeUnit.value);
  form.download.large_download_confirm_bytes = toBytes(downloadConfirmSizeValue.value, downloadConfirmSizeUnit.value);

  try {
    await httpClient.request("/admin/system/settings", {
      method: "PUT",
      body: form,
    });
    uploadSnapshot.value = serializeUploadState();
    downloadSnapshot.value = serializeDownloadState();
    toastSuccess("系统策略已更新。");
  } catch (err: unknown) {
    toastError(readApiError(err, "更新系统策略失败。"));
  } finally {
    uploadSaving.value = false;
  }
}

function serializeUploadState() {
  return JSON.stringify({
    max_upload_total_bytes: toBytes(uploadSizeValue.value, uploadSizeUnit.value),
    cover_upload_dir: (form.cover_upload_dir ?? "").trim(),
  });
}

function serializeDownloadState() {
  return JSON.stringify({
    large_download_confirm_bytes: toBytes(downloadConfirmSizeValue.value, downloadConfirmSizeUnit.value),
    wide_layout_extensions: (form.download.wide_layout_extensions ?? "").trim(),
    cdn_mode: form.download.cdn_mode ?? false,
    global_cdn_url: (form.download.global_cdn_url ?? "").trim(),
  });
}

function applyDownloadSizeFields(bytes: number) {
  if (bytes >= 1024 * 1024 * 1024 && bytes % (1024 * 1024 * 1024) === 0) {
    downloadConfirmSizeValue.value = bytes / (1024 * 1024 * 1024);
    downloadConfirmSizeUnit.value = "GB";
    return;
  }
  if (bytes >= 1024 * 1024 && bytes % (1024 * 1024) === 0) {
    downloadConfirmSizeValue.value = bytes / (1024 * 1024);
    downloadConfirmSizeUnit.value = "MB";
    return;
  }
  if (bytes >= 1024 && bytes % 1024 === 0) {
    downloadConfirmSizeValue.value = bytes / 1024;
    downloadConfirmSizeUnit.value = "KB";
    return;
  }
  downloadConfirmSizeValue.value = bytes;
  downloadConfirmSizeUnit.value = "B";
}

function applyUploadSizeFields(bytes: number) {
  if (bytes >= 1024 * 1024 * 1024 && bytes % (1024 * 1024 * 1024) === 0) {
    uploadSizeValue.value = bytes / (1024 * 1024 * 1024);
    uploadSizeUnit.value = "GB";
    return;
  }
  if (bytes >= 1024 * 1024 && bytes % (1024 * 1024) === 0) {
    uploadSizeValue.value = bytes / (1024 * 1024);
    uploadSizeUnit.value = "MB";
    return;
  }
  if (bytes >= 1024 && bytes % 1024 === 0) {
    uploadSizeValue.value = bytes / 1024;
    uploadSizeUnit.value = "KB";
    return;
  }
  uploadSizeValue.value = bytes;
  uploadSizeUnit.value = "B";
}

function toBytes(value: number, unit: "B" | "KB" | "MB" | "GB") {
  const normalized = Math.max(1, Math.floor(value || 0));
  switch (unit) {
    case "GB":
      return normalized * 1024 * 1024 * 1024;
    case "MB":
      return normalized * 1024 * 1024;
    case "KB":
      return normalized * 1024;
    default:
      return normalized;
  }
}

const uploadDirty = computed(() => loaded.value && uploadSnapshot.value !== serializeUploadState());
const downloadDirty = computed(() => loaded.value && downloadSnapshot.value !== serializeDownloadState());
const systemPolicyDirty = computed(() => uploadDirty.value || downloadDirty.value);
const strictDirectoryInputKeyword = computed(() => {
  const current = normalizeManualBrowsePath(importCurrentPath.value);
  const manual = normalizeManualBrowsePath(manualBrowsePath.value);
  if (!current || !manual || manual === current) {
    return "";
  }
  const prefix = `${current}/`;
  if (!manual.startsWith(prefix)) {
    return "";
  }
  const remainder = manual.slice(prefix.length);
  if (!remainder || remainder.includes("/")) {
    return "";
  }
  return remainder.toLowerCase();
});
const filteredImportItems = computed(() => {
  const strictKeyword = strictDirectoryInputKeyword.value.trim();
  const looseKeyword = importFilter.value.trim().toLowerCase();

  return importItems.value.filter((item) => {
    const name = item.name.toLowerCase();
    const path = item.path.toLowerCase();

    if (strictKeyword && !name.startsWith(strictKeyword)) {
      return false;
    }

    if (looseKeyword && !name.includes(looseKeyword) && !path.includes(looseKeyword)) {
      return false;
    }

    return true;
  });
});
const importPathConflict = computed(() => {
  const selectedPath = normalizeManagedRootClientPath(importPath.value);
  if (!selectedPath) {
    return "";
  }

  for (const folder of managedFolders.value) {
    const existingPath = normalizeManagedRootClientPath(folder.sourcePath);
    if (!existingPath) {
      continue;
    }
    if (selectedPath === existingPath) {
      return "该目录已托管，请使用“重新扫描”。";
    }
    if (isManagedRootClientChild(selectedPath, existingPath)) {
      return "该目录位于已托管目录内，请对上级托管目录执行“重新扫描”。";
    }
    if (isManagedRootClientChild(existingPath, selectedPath)) {
      return "该目录包含已托管目录，不能重复导入父目录。";
    }
  }

  return "";
});

async function loadDirectories(path: string, options?: { silent?: boolean }) {
  importLoading.value = true;
  if (!options?.silent) {
    importError.value = "";
  }
  try {
    const suffix = path ? `?path=${encodeURIComponent(path)}` : "";
    const response = await httpClient.get<{
      current_path: string;
      parent_path: string;
      items: Array<{ name: string; path: string }>;
    }>(`/admin/imports/directories${suffix}`);
    importCurrentPath.value = response.current_path;
    importParentPath.value = response.parent_path;
    importItems.value = response.items ?? [];
    manualBrowsePath.value = withTrailingSlash(response.current_path);
    if (!importPath.value) {
      importPath.value = response.current_path;
    }
  } catch (err: unknown) {
    if (!options?.silent) {
      toastError(readApiError(err, "加载目录浏览器失败。"));
    }
  } finally {
    importLoading.value = false;
  }
}

async function loadManagedFolders() {
  managedFoldersLoading.value = true;
  managedFoldersError.value = "";
  try {
    const response = await httpClient.get<{ items: ManagedFolderNode[] }>("/admin/folders/tree");
    managedFolders.value = (response.items ?? []).map((item) => ({
      id: item.id,
      name: item.name,
      sourcePath: item.source_path,
      hidePublicCatalog: Boolean(item.hide_public_catalog),
      cdnUrl: (item as any).cdn_url ?? "",
      guestKeyRequired: Boolean((item as any).guest_key_required),
      allowedGuestKeyIds: Array.isArray((item as any).allowed_guest_key_ids) ? (item as any).allowed_guest_key_ids : [],
    }));
  } catch (err: unknown) {
    managedFolders.value = [];
    toastError(readApiError(err, "加载已托管目录失败。"));
  } finally {
    managedFoldersLoading.value = false;
  }
}

async function patchFolderGuestKey(folderID: string, required: boolean, allowedKeyIDs: string[]) {
  try {
    await httpClient.request(`/admin/resources/folders/${encodeURIComponent(folderID)}/guest-keys`, {
      method: "PATCH",
      body: { required, allowed_key_ids: allowedKeyIDs },
    });
    await loadManagedFolders();
  } catch (err: unknown) {
    toastError(readApiError(err, "更新访客密钥授权失败。"));
  }
}

// 切换目录的"要求密钥访问"开关。
// 后端要求开启时必须至少允许 1 个密钥，因此：
// - 关闭：直接保存（后端自动清空允许的密钥列表）
// - 开启：先乐观更新本地状态，让密钥选择下拉立即出现（下拉有 v-if="folder.guestKeyRequired"），
//   由下拉的 change 事件发送真正的保存请求；密钥池为空则回滚并提示
function toggleFolderGuestKey(folder: { guestKeyRequired: boolean; allowedGuestKeyIds: string[]; id: string }, checked: boolean) {
  if (!checked) {
    void patchFolderGuestKey(folder.id, false, []);
    return true;
  }
  if (guestAccessKeys.value.length === 0) {
    toastWarning("请先在下方密钥池中添加密钥，再开启密钥访问。");
    return false;
  }
  folder.guestKeyRequired = true;
  if (folder.allowedGuestKeyIds.length > 0) {
    // 已有已选密钥：直接按当前选择保存
    void patchFolderGuestKey(folder.id, true, [...folder.allowedGuestKeyIds]);
  } else {
    // 尚无已选密钥：提示用户在刚出现的下拉中选择密钥以完成保存
    toastInfo("已开启，请在下拉中选择允许访问的密钥以完成保存。");
  }
  return true;
}

// —— 目录密钥选择器：按钮 + 弹出面板（替代原生多选框，同一时刻只打开一个）——
const guestKeyPickerOpen = ref(false); // 面板是否打开
const guestKeyPickerFolderId = ref(""); // 当前打开面板的目录 ID
const guestKeyPickerDraft = ref<string[]>([]); // 面板内的临时选择，点"确定"才保存

// 打开某目录的密钥选择面板：以当前已选密钥初始化草稿
function openFolderGuestKeyPicker(folder: { id: string; allowedGuestKeyIds: string[] }) {
  guestKeyPickerFolderId.value = folder.id;
  guestKeyPickerDraft.value = [...folder.allowedGuestKeyIds];
  guestKeyPickerOpen.value = true;
}

// 在面板内勾选/取消一个密钥（只改草稿，不请求后端）
function toggleGuestKeyDraft(keyId: string) {
  const draft = guestKeyPickerDraft.value;
  const idx = draft.indexOf(keyId);
  if (idx >= 0) {
    draft.splice(idx, 1);
  } else {
    draft.push(keyId);
  }
}

// 确定面板选择：草稿非空时按草稿保存；全部取消等价于关闭密钥访问（后端不接受空密钥列表）
function confirmFolderGuestKeyPicker(folder: { id: string }) {
  guestKeyPickerOpen.value = false;
  if (guestKeyPickerDraft.value.length > 0) {
    void patchFolderGuestKey(folder.id, true, [...guestKeyPickerDraft.value]);
  } else {
    void patchFolderGuestKey(folder.id, false, []);
  }
}

// 密钥 ID → 显示名称（未找到时回退到 ID 本身）
function guestKeyNameById(id: string) {
  return guestAccessKeys.value.find((k) => k.id === id)?.name || id;
}

// 选择按钮文案：已选密钥的名称摘要
function folderGuestKeySummary(folder: { allowedGuestKeyIds: string[] }) {
  if (folder.allowedGuestKeyIds.length === 0) return "选择密钥";
  const names = folder.allowedGuestKeyIds.map(guestKeyNameById);
  return `已选 ${names.length} 个：${names.join("、")}`;
}

function downloadJsonBlob(data: unknown, filename: string) {
  const blob = new Blob([JSON.stringify(data, null, 2)], { type: "application/json" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

async function exportGlobalData() {
  exportingGlobal.value = true;
  try {
    const data = await httpClient.get("/admin/export/global");
    const date = new Date().toISOString().slice(0, 10);
    downloadJsonBlob(data, `openshare-global-${date}.json`);
  } catch (err: unknown) {
    toastError(readApiError(err, "导出全局数据失败。"));
  } finally {
    exportingGlobal.value = false;
  }
}

async function exportDirectoryData(folderId: string, folderName: string) {
  exportingFolderId.value = folderId;
  try {
    const data = await httpClient.get(`/admin/export/directory/${folderId}`);
    downloadJsonBlob(data, `${folderName}.json`);
  } catch (err: unknown) {
    toastError(readApiError(err, `导出 ${folderName} 失败。`));
  } finally {
    exportingFolderId.value = "";
  }
}

const savingCdnUrlFolderId = ref("");

async function saveFolderCdnUrl(folderId: string, cdnUrl: string) {
  savingCdnUrlFolderId.value = folderId;
  try {
    await httpClient.request(`/admin/resources/folders/${encodeURIComponent(folderId)}/cdn-url`, {
      method: "PATCH",
      body: { cdn_url: cdnUrl.trim() },
    });
  } catch (err: unknown) {
    toastError(readApiError(err, "更新 CDN 地址失败。"));
  } finally {
    savingCdnUrlFolderId.value = "";
  }
}

async function openDirectoryPicker() {
  directoryPickerOpen.value = true;
  importFilter.value = "";
  pendingImportPath.value = importPath.value.trim();
  await loadDirectories(importPath.value.trim());
  if (!pendingImportPath.value) {
    pendingImportPath.value = importCurrentPath.value;
  }
}

function closeDirectoryPicker() {
  directoryPickerOpen.value = false;
}

function selectCurrentDirectory() {
  confirmedImportPath.value = pendingImportPath.value || importCurrentPath.value;
  importPath.value = confirmedImportPath.value;
  directoryPickerOpen.value = false;
}

async function browseDirectory(path: string) {
  pendingImportPath.value = path;
  importFilter.value = "";
  await loadDirectories(path);
}

async function applyManualBrowsePath() {
  const nextPath = normalizeManualBrowsePath(manualBrowsePath.value);
  if (!nextPath) {
    return;
  }
  pendingImportPath.value = nextPath;
  importFilter.value = "";
  await loadDirectories(nextPath);
}

function updateManualBrowsePath(value: string) {
  manualBrowsePath.value = value;
}

function normalizeManualBrowsePath(value: string) {
  return value.trim().replace(/\/+$/, "");
}

function withTrailingSlash(value: string) {
  const trimmed = value.trim();
  if (!trimmed) {
    return "";
  }
  return trimmed.endsWith("/") ? trimmed : `${trimmed}/`;
}

async function importDirectory() {
  if (!importPath.value.trim()) {
    toastError("请先选择服务器目录。");
    return;
  }
  if (importPathConflict.value) {
    toastError(importPathConflict.value);
    return;
  }
  importLoading.value = true;
  importError.value = "";
  importMessage.value = "";
  try {
    const response = await httpClient.post<{
      imported_folders: number;
      imported_files: number;
    }>("/admin/imports/local", {
      root_path: importPath.value.trim(),
    });
    toastSuccess(`导入完成：${response.imported_folders} 个目录，${response.imported_files} 个文件。`);
    confirmedImportPath.value = "";
    importPath.value = "";
    await loadManagedFolders();
  } catch (err: unknown) {
    toastError(readApiError(err, "导入目录失败。"));
  } finally {
    importLoading.value = false;
  }
}

// 创建虚拟托管根目录（无本地磁盘路径）
async function createVirtualManagedRoot() {
  const name = virtualRootName.value.trim();
  if (!name) {
    toastError("请输入目录名称。");
    return;
  }
  virtualRootCreating.value = true;
  virtualRootError.value = "";
  try {
    await httpClient.post("/admin/resources/virtual-folders", { name, parent_id: "" });
    virtualRootName.value = "";
    await loadManagedFolders();
  } catch (err: unknown) {
    toastError(readApiError(err, "创建虚拟托管目录失败。"));
  } finally {
    virtualRootCreating.value = false;
  }
}

/* 在新标签页中打开托管目录，方便查看隐藏目录的内容 */
function goToFolder(folderID: string) {
  const url = router.resolve({ name: "public-home", query: { folder: folderID } }).href;
  window.open(url, "_blank");
}

async function patchManagedRootCatalogVisibility(folderID: string, hide: boolean) {
  catalogVisibilitySaving.value = folderID;
  managedFoldersError.value = "";
  try {
    await httpClient.request(`/admin/resources/folders/${encodeURIComponent(folderID)}/catalog-visibility`, {
      method: "PUT",
      body: { hide_public_catalog: hide },
    });
    await loadManagedFolders();
  } catch (err: unknown) {
    toastError(readApiError(err, "更新访客首页可见性失败。"));
  } finally {
    catalogVisibilitySaving.value = "";
  }
}

async function rescanManagedFolder(folderID: string) {
  rescanningFolderID.value = folderID;
  rescanError.value = "";
  rescanMessage.value = "";
  try {
    const response = await httpClient.post<{
      added_folders: number;
      added_files: number;
      updated_folders: number;
      updated_files: number;
      deleted_folders: number;
      deleted_files: number;
    }>(`/admin/imports/local/${encodeURIComponent(folderID)}/rescan`);
    toastSuccess(
      `重新扫描完成：新增目录 ${response.added_folders} 个，新增文件 ${response.added_files} 个，` +
      `更新目录 ${response.updated_folders} 个，更新文件 ${response.updated_files} 个，` +
      `删除目录 ${response.deleted_folders} 个，删除文件 ${response.deleted_files} 个。`);
    await loadManagedFolders();
  } catch (err: unknown) {
    toastError(readApiError(err, "重新扫描托管目录失败。"));
  } finally {
    rescanningFolderID.value = "";
  }
}

function beginUnmanageManagedFolder(folderID: string) {
  unmanageError.value = "";
  unmanageMessage.value = "";
  unmanagePassword.value = "";
  unmanagingFolderID.value = folderID;
}

function cancelUnmanageManagedFolder() {
  unmanagingFolderID.value = "";
  unmanagePassword.value = "";
}

async function confirmUnmanageManagedFolder(folderID: string) {
  if (!unmanagePassword.value.trim()) {
    toastError("请输入超级管理员密码。");
    return;
  }

  unmanageError.value = "";
  unmanageMessage.value = "";
  try {
    await httpClient.request(`/admin/imports/local/${encodeURIComponent(folderID)}`, {
      method: "DELETE",
      body: { password: unmanagePassword.value },
    });
    toastSuccess("已取消托管，并清理站内关联数据。");
    unmanagingFolderID.value = "";
    unmanagePassword.value = "";
    await loadManagedFolders();
  } catch (err: unknown) {
    toastError(readApiError(err, "取消托管目录失败。"));
  }
}

function normalizeManagedRootClientPath(value: string) {
  return value.trim().replace(/\\/g, "/").replace(/\/+$/, "");
}

function isManagedRootClientChild(path: string, root: string) {
  return path !== root && path.startsWith(`${root}/`);
}
</script>

<template>
  <section class="space-y-4">
    <div>
      <h2 class="text-lg font-semibold tracking-tight text-slate-900">系统配置</h2>
    </div>

    <div v-if="!loaded && loading" class="text-sm text-slate-500">加载中…</div>

    <div v-else class="space-y-6">
      <SurfaceCard class="space-y-4">
        <div class="flex items-center justify-between gap-4">
          <div>
            <h3 class="text-lg font-semibold text-slate-900">当前已托管文件目录</h3>
          </div>
          <div class="flex items-center gap-2">
            <button type="button" class="btn-secondary" :disabled="managedFoldersLoading || exportingGlobal" @click="exportGlobalData">
              {{ exportingGlobal ? "导出中…" : "导出全局数据" }}
            </button>
            <button type="button" class="btn-secondary" :disabled="managedFoldersLoading" @click="loadManagedFolders">
              {{ managedFoldersLoading ? "刷新中…" : "刷新" }}
            </button>
          </div>
        </div>

        <div v-if="managedFoldersLoading" class="text-sm text-slate-500">正在加载托管目录…</div>
        <p v-else-if="managedFoldersError" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{{ managedFoldersError }}</p>
        <div v-else-if="managedFolders.length === 0" class="panel-muted px-4 py-3 text-sm text-slate-500">
          暂无已托管目录。
        </div>
        <div v-else class="grid gap-3">
          <div
            v-for="folder in managedFolders"
            :key="folder.id"
            class="panel-muted px-4 py-3"
          >
            <!-- 窄屏：左右两栏堆叠成上下两行；>=md：恢复左右并排。左右都加 min-w-0/flex-1 + 内部按钮 flex-wrap，确保路径不被挤压。 -->
            <div class="flex flex-col gap-3 md:flex-row md:items-start md:justify-between md:gap-4">
              <div class="min-w-0 flex-1">
                <p class="text-sm font-medium text-slate-900">
                  {{ folder.name }}
                  <span
                    v-if="folder.hidePublicCatalog"
                    class="ml-2 inline-block rounded-md bg-amber-100 px-2 py-0.5 text-xs font-medium text-amber-900"
                  >访客首页已隐藏</span>
                </p>
                <p class="mt-1 break-all text-sm text-slate-500">{{ folder.sourcePath || "未记录源目录" }}</p>
                <div class="mt-2 flex items-center gap-2">
                  <input
                    :value="folder.cdnUrl"
                    type="url"
                    class="field h-9 min-w-0 flex-1 text-sm"
                    placeholder="CDN JSON 直链（可选）"
                    @change="(e) => { const target = e.target as HTMLInputElement; folder.cdnUrl = target.value; }"
                    @blur="(e) => { const target = e.target as HTMLInputElement; const v = target.value.trim(); if (v !== (folder.cdnUrl ?? '')) { folder.cdnUrl = v; saveFolderCdnUrl(folder.id, v); } }"
                  />
                  <button
                    type="button"
                    class="inline-flex h-9 shrink-0 items-center rounded-lg border border-slate-200 bg-white px-3 text-xs font-medium text-slate-600 transition hover:bg-slate-100 disabled:opacity-60"
                    :disabled="savingCdnUrlFolderId === folder.id"
                    @click="saveFolderCdnUrl(folder.id, folder.cdnUrl)"
                  >
                    {{ savingCdnUrlFolderId === folder.id ? '保存中…' : '保存' }}
                  </button>
                </div>
                <!-- 访客密钥访问：每目录开关与允许的密钥选择 -->
                <div class="mt-2 flex flex-wrap items-center gap-2">
                  <label class="inline-flex shrink-0 items-center gap-2 cursor-pointer">
                    <input
                      type="checkbox"
                      :checked="folder.guestKeyRequired"
                      class="h-4 w-4 rounded border-slate-300 text-indigo-600 focus:ring-indigo-500"
                      @change="(e) => {
                        const target = e.target as HTMLInputElement;
                        // 开启但未选密钥时不会保存（返回 false），回滚勾选视觉状态
                        if (!toggleFolderGuestKey(folder, target.checked)) {
                          target.checked = false;
                        }
                      }"
                    />
                    <span class="text-xs font-medium text-slate-700">要求密钥访问</span>
                  </label>
                  <div v-if="folder.guestKeyRequired" class="relative min-w-0 flex-1">
                    <!-- 打开密钥选择面板的按钮，展示当前已选摘要 -->
                    <button
                      type="button"
                      class="field h-9 w-full min-w-0 truncate text-left text-sm"
                      @click="openFolderGuestKeyPicker(folder)"
                    >
                      {{ folderGuestKeySummary(folder) }}
                    </button>
                    <!-- 透明遮罩：点击面板外任意位置关闭并丢弃草稿 -->
                    <div
                      v-if="guestKeyPickerOpen && guestKeyPickerFolderId === folder.id"
                      class="fixed inset-0 z-10"
                      @click="guestKeyPickerOpen = false"
                    ></div>
                    <!-- 密钥选择面板：本地草稿，点"确定"才保存 -->
                    <div
                      v-if="guestKeyPickerOpen && guestKeyPickerFolderId === folder.id"
                      class="absolute left-0 right-0 top-[calc(100%+8px)] z-20 rounded-xl border border-slate-200 bg-white p-3 shadow-sm shadow-slate-950/[0.06]"
                      @click.stop
                    >
                      <div class="mb-2 text-xs font-semibold text-slate-700">允许访问的密钥</div>
                      <div v-if="guestAccessKeys.length === 0" class="text-xs text-slate-500">
                        暂无密钥，请先在下方密钥池中添加。
                      </div>
                      <div v-else class="max-h-44 space-y-1 overflow-y-auto">
                        <label
                          v-for="k in guestAccessKeys"
                          :key="k.id"
                          class="flex cursor-pointer items-center gap-2 rounded-lg px-2 py-1.5 text-sm text-slate-700 hover:bg-slate-100"
                        >
                          <input
                            type="checkbox"
                            :checked="guestKeyPickerDraft.includes(k.id)"
                            class="h-4 w-4 rounded border-slate-300 text-indigo-600 focus:ring-indigo-500"
                            @change="toggleGuestKeyDraft(k.id)"
                          />
                          <span class="truncate">{{ k.name || k.id }}</span>
                        </label>
                      </div>
                      <div class="mt-2 flex items-center justify-end gap-2 border-t border-slate-100 pt-2">
                        <button type="button" class="btn-secondary" @click="guestKeyPickerOpen = false">取消</button>
                        <button
                          type="button"
                          class="btn-primary"
                          :disabled="guestAccessKeys.length === 0"
                          @click="confirmFolderGuestKeyPicker(folder)"
                        >
                          确定
                        </button>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
              <div class="flex flex-wrap items-center gap-2 md:shrink-0 md:justify-end">
                <button
                  type="button"
                  class="inline-flex h-11 items-center justify-center rounded-xl border border-slate-200 bg-white px-4 text-sm font-medium text-slate-700 transition hover:bg-slate-100 disabled:cursor-not-allowed disabled:opacity-60"
                  :disabled="
                    managedFoldersLoading ||
                    rescanningFolderID === folder.id ||
                    catalogVisibilitySaving === folder.id
                  "
                  @click="patchManagedRootCatalogVisibility(folder.id, !folder.hidePublicCatalog)"
                >
                  {{
                    catalogVisibilitySaving === folder.id
                      ? "更新中…"
                      : folder.hidePublicCatalog
                        ? "恢复访客首页展示"
                        : "访客首页隐藏"
                  }}
                </button>
                <button
                  type="button"
                  class="inline-flex h-11 items-center justify-center rounded-xl border border-slate-200 bg-white px-5 text-sm font-medium text-slate-700 transition hover:bg-slate-100 disabled:cursor-not-allowed disabled:opacity-60"
                  :disabled="managedFoldersLoading || rescanningFolderID === folder.id || catalogVisibilitySaving === folder.id"
                  @click="rescanManagedFolder(folder.id)"
                >
                  {{ rescanningFolderID === folder.id ? "扫描中…" : "重新扫描" }}
                </button>
                <button
                  type="button"
                  class="inline-flex h-11 items-center justify-center rounded-xl border border-slate-200 bg-white px-4 text-sm font-medium text-slate-700 transition hover:bg-slate-100 disabled:cursor-not-allowed disabled:opacity-60"
                  :disabled="managedFoldersLoading || rescanningFolderID === folder.id || catalogVisibilitySaving === folder.id || exportingFolderId === folder.id"
                  @click="exportDirectoryData(folder.id, folder.name)"
                >
                  {{ exportingFolderId === folder.id ? "导出中…" : "导出数据" }}
                </button>
                <!-- 隐藏目录显示打开按钮，方便直接访问内容 -->
                <button
                  v-if="folder.hidePublicCatalog"
                  type="button"
                  class="inline-flex h-11 items-center justify-center rounded-xl border border-indigo-200 bg-indigo-50 px-4 text-sm font-medium text-indigo-700 transition hover:bg-indigo-100 disabled:cursor-not-allowed disabled:opacity-60"
                  :disabled="managedFoldersLoading"
                  @click="goToFolder(folder.id)"
                >
                  打开目录
                </button>
                <button
                  type="button"
                  class="inline-flex h-11 items-center justify-center rounded-xl bg-rose-600 px-5 text-sm font-medium text-white transition hover:bg-rose-700 disabled:cursor-not-allowed disabled:opacity-60"
                  :disabled="managedFoldersLoading || rescanningFolderID === folder.id || catalogVisibilitySaving === folder.id"
                  @click="beginUnmanageManagedFolder(folder.id)"
                >
                  取消托管
                </button>
              </div>
            </div>
            <div v-if="unmanagingFolderID === folder.id" class="mt-4 space-y-3 rounded-xl border border-rose-200 bg-white px-4 py-4">
              <p class="text-sm text-rose-700">该操作会取消此目录的托管并清理站内关联数据，原目录和文件会保留在原位置。</p>
              <input v-model="unmanagePassword" type="password" class="field" placeholder="输入 superadmin 密码确认取消托管" />
              <div class="flex items-center justify-end gap-3">
                <button type="button" class="inline-flex h-11 items-center justify-center rounded-xl border border-slate-200 bg-white px-5 text-sm font-medium text-slate-700 transition hover:bg-slate-100" @click="cancelUnmanageManagedFolder">取消</button>
                <button type="button" class="inline-flex h-11 items-center justify-center rounded-xl bg-rose-600 px-5 text-sm font-medium text-white transition hover:bg-rose-700" @click="confirmUnmanageManagedFolder(folder.id)">确认取消托管</button>
              </div>
            </div>
          </div>
        </div>

</SurfaceCard>

      <div class="grid gap-6 xl:grid-cols-2">
      <form class="panel space-y-6 p-6" @submit.prevent="saveUploadPolicy">
        <div>
          <h3 class="text-lg font-semibold text-slate-900">访客策略</h3>
          <p class="mt-2 text-sm text-slate-500">上传总大小限制与大文件下载确认阈值。保存时会一并写入系统策略。</p>
        </div>
        <div>
          <h4 class="text-sm font-semibold text-slate-800">上传</h4>
          <p class="mt-1 text-sm text-slate-500">访客只允许发起上传，所有公开上传都会先进入审核。单次提交里的文件总大小不能超过这里设置的上限。</p>
        </div>
        <div class="grid gap-4 md:grid-cols-[minmax(0,1fr)_140px]">
          <div class="space-y-2">
            <label class="text-sm font-medium text-slate-700">单次提交总大小</label>
            <input v-model.number="uploadSizeValue" type="number" min="1" class="field" placeholder="请输入大小" />
          </div>
          <div class="space-y-2">
            <label class="text-sm font-medium text-slate-700">单位</label>
            <select v-model="uploadSizeUnit" class="field">
              <option value="GB">GB</option>
              <option value="MB">MB</option>
              <option value="KB">KB</option>
              <option value="B">B</option>
            </select>
          </div>
        </div>
        <!-- 封面图片上传目录 -->
        <div class="border-t border-slate-200 pt-5">
          <h4 class="text-sm font-semibold text-slate-800">封面图片</h4>
          <p class="mt-1 text-sm text-slate-500">管理员拖拽上传封面时，图片将存入此磁盘目录（绝对路径）。首次使用时自动创建为隐藏托管根目录。留空则禁用拖拽上传封面功能。</p>
        </div>
        <div class="space-y-2">
          <label class="text-sm font-medium text-slate-700">封面存储目录</label>
          <input v-model="form.cover_upload_dir" class="field" placeholder="/data/openshare/cover-images" />
        </div>

        <div class="border-t border-slate-200 pt-5">
          <h4 class="text-sm font-semibold text-slate-800">下载确认</h4>
          <p class="mt-1 text-sm text-slate-500">访客下载<strong class="text-slate-700">整个文件夹（ZIP）</strong>时始终弹出确认。单文件在达到下列大小时也会要求确认（默认 1 GB）。</p>
        </div>
        <div class="grid gap-4 md:grid-cols-[minmax(0,1fr)_140px]">
          <div class="space-y-2">
            <label class="text-sm font-medium text-slate-700">大文件确认阈值</label>
            <input v-model.number="downloadConfirmSizeValue" type="number" min="1" class="field" placeholder="请输入大小" />
          </div>
          <div class="space-y-2">
            <label class="text-sm font-medium text-slate-700">单位</label>
            <select v-model="downloadConfirmSizeUnit" class="field">
              <option value="GB">GB</option>
              <option value="MB">MB</option>
              <option value="KB">KB</option>
              <option value="B">B</option>
            </select>
          </div>
        </div>
        <div class="border-t border-slate-200 pt-5">
          <h4 class="text-sm font-semibold text-slate-800">文件详情宽屏布局</h4>
          <p class="mt-1 text-sm text-slate-500">当文件后缀匹配以下列表（逗号分隔，如 <code class="text-slate-700">.md,.txt,.nc</code>），且简介内容较长或包含图片时，在宽屏幕上启用左右分栏布局。留空则对所有文件禁用。</p>
        </div>
        <div class="space-y-2">
          <label class="text-sm font-medium text-slate-700">启用后缀列表</label>
          <input v-model="form.download.wide_layout_extensions" class="field" placeholder="例如：.md,.txt,.nc" />
        </div>
        <div class="border-t border-slate-200 pt-5">
          <h4 class="text-sm font-semibold text-slate-800">CDN 模式</h4>
          <p class="mt-1 text-sm text-slate-500">开启后，访客首页将优先从各托管目录配置的 CDN JSON 直链加载数据，减少源服务器请求。</p>
        </div>
        <label class="mt-3 inline-flex items-center gap-3 cursor-pointer">
          <input type="checkbox" v-model="form.download.cdn_mode" class="h-5 w-5 rounded border-slate-300 text-indigo-600 focus:ring-indigo-500" />
          <span class="text-sm font-medium text-slate-700">{{ form.download.cdn_mode ? '已开启' : '已关闭' }}</span>
        </label>
        <div v-if="form.download.cdn_mode" class="mt-3 space-y-2">
          <label class="text-sm font-medium text-slate-700">全局数据 CDN 直链</label>
          <input v-model="form.download.global_cdn_url" type="url" class="field" placeholder="https://cdn.example.com/openshare-global.json" />
          <p class="text-sm text-slate-500">填入导出全局数据 JSON 后在 CDN 上的直链地址。</p>
        </div>
        <button type="submit" class="btn-primary" :disabled="uploadSaving || !systemPolicyDirty">
          {{ uploadSaving ? "更新中…" : "确认更新" }}
        </button>
      </form>

      <!-- 访客密钥访问：管理全局开关与密钥池 -->
      <SurfaceCard class="space-y-4">
        <div class="flex items-start justify-between gap-4">
          <div>
            <h3 class="text-lg font-semibold text-slate-900">访客密钥访问</h3>
            <p class="mt-1 text-sm text-slate-500">
              启用后，访客在网页端浏览被标记为"要求密钥访问"的托管目录前必须先输入密钥。
              密钥校验成功后保存在浏览器本地存储，后续浏览自动附带。
              <strong class="text-slate-700">下载与直链不受密钥保护</strong>。
            </p>
          </div>
        </div>

        <label class="mt-2 inline-flex items-center gap-3 cursor-pointer">
          <input
            type="checkbox"
            v-model="guestAccessEnabled"
            class="h-5 w-5 rounded border-slate-300 text-indigo-600 focus:ring-indigo-500"
          />
          <span class="text-sm font-medium text-slate-700">{{ guestAccessEnabled ? "已启用" : "已关闭" }}</span>
        </label>

        <div class="space-y-3">
          <div class="flex items-center justify-between gap-3">
            <h4 class="text-sm font-semibold text-slate-800">密钥池</h4>
            <button type="button" class="btn-secondary" :disabled="guestAccessSaving" @click="addGuestAccessKeyDraft">添加密钥</button>
          </div>
          <p v-if="guestAccessKeys.length === 0" class="text-sm text-slate-500">尚未配置密钥。</p>
          <ul v-else class="space-y-3">
            <li v-for="(key, idx) in guestAccessKeys" :key="idx" class="panel-muted px-4 py-3 space-y-3">
              <div class="grid gap-3 md:grid-cols-2">
                <label class="space-y-1">
                  <span class="text-xs font-medium text-slate-600">名称</span>
                  <input v-model="key.name" type="text" class="field h-9 text-sm" placeholder="显示名称" />
                </label>
                <label class="space-y-1">
                  <span class="text-xs font-medium text-slate-600">密钥值</span>
                  <input v-model="key.value" type="text" class="field h-9 text-sm" placeholder="访客需要输入的密钥" />
                </label>
              </div>
              <label class="space-y-1 block">
                <span class="text-xs font-medium text-slate-600">提示文案（可选，错误时展示）</span>
                <input v-model="key.hint" type="text" class="field h-9 text-sm" placeholder="例如：8-12位字母数字" />
              </label>
              <div class="flex justify-end">
                <button type="button" class="btn-secondary" :disabled="guestAccessSaving" @click="removeGuestAccessKeyDraft(idx)">删除</button>
              </div>
            </li>
          </ul>
        </div>

        <div class="flex justify-end">
          <button type="button" class="btn-primary" :disabled="guestAccessSaving || !guestAccessDirty" @click="saveGuestAccessConfig">
            {{ guestAccessSaving ? "保存中…" : "保存访客密钥" }}
          </button>
        </div>
        <p v-if="guestAccessMessage" class="text-sm text-emerald-600">{{ guestAccessMessage }}</p>
      </SurfaceCard>

      <SurfaceCard class="space-y-6">
        <div>
          <h3 class="text-lg font-semibold text-slate-900">本地目录导入</h3>
        </div>
        <div class="space-y-4">
          <div class="panel-muted px-4 py-3">
            <p class="text-xs font-medium uppercase tracking-[0.12em] text-slate-400">已选目录</p>
            <p class="mt-2 break-all text-sm text-slate-700">{{ importPath || "尚未选择服务器目录" }}</p>
          </div>
          <p v-if="importPathConflict" class="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-700">{{ importPathConflict }}</p>
        </div>
        <div class="space-y-3">
          <button type="button" class="btn-secondary w-full" :disabled="importLoading" @click="openDirectoryPicker">
            选择服务器目录
          </button>
          <button type="button" class="btn-primary w-full" :disabled="importLoading || !confirmedImportPath.trim() || !!importPathConflict" @click="importDirectory">
            {{ importLoading ? "导入中…" : "确认导入" }}
          </button>
        </div>
      </SurfaceCard>

      <!-- 创建虚拟托管目录（无物理磁盘路径，仅存数据库，文件通过 CDN 直链提供） -->
      <SurfaceCard class="space-y-6">
        <div>
          <h3 class="text-lg font-semibold text-slate-900">创建虚拟托管目录</h3>
          <p class="mt-1 text-sm text-slate-500">无需本地磁盘路径，目录结构与文件仅存储在数据库中，文件通过 CDN 直链提供下载。</p>
        </div>
        <div class="space-y-4">
          <label class="space-y-2">
            <span class="text-sm font-medium text-slate-700">目录名称</span>
            <input v-model="virtualRootName" type="text" class="field" placeholder="输入虚拟托管根目录名称" @keyup.enter="createVirtualManagedRoot" />
          </label>
        </div>
<button type="button" class="btn-primary w-full" :disabled="virtualRootCreating || !virtualRootName.trim()" @click="createVirtualManagedRoot">
          {{ virtualRootCreating ? "创建中…" : "创建虚拟托管目录" }}
        </button>
      </SurfaceCard>
      </div>
    </div>
<Teleport to="body">
    <Transition name="modal-shell">
    <div v-if="directoryPickerOpen" class="fixed inset-0 z-[120] overflow-hidden bg-slate-950/40 backdrop-blur-sm">
      <div class="flex h-full items-start justify-center px-4 py-6">
      <SurfaceCard class="modal-card w-full max-w-3xl overflow-hidden">
        <div class="flex items-start justify-between gap-4 border-b border-slate-200 pb-4">
          <div>
            <h3 class="text-lg font-semibold text-slate-900">选择服务器目录</h3>
            <p class="mt-1 text-sm text-slate-500">浏览服务器目录，确认后将当前目录作为导入源。</p>
          </div>
          <button type="button" class="btn-secondary" @pointerdown.prevent="closeDirectoryPicker">关闭</button>
        </div>

        <div class="mt-4 space-y-4">
          <div class="space-y-2">
            <label class="text-sm font-medium text-slate-700">当前目录</label>
            <input
              :value="manualBrowsePath"
              type="text"
              class="field"
              placeholder="/Users/quan/Desktop/test/"
              @input="updateManualBrowsePath(($event.target as HTMLInputElement).value)"
              @keydown.enter.prevent="applyManualBrowsePath"
              @blur="applyManualBrowsePath"
            />
          </div>

          <div class="space-y-2">
            <label class="text-sm font-medium text-slate-700">搜索子目录</label>
            <input v-model="importFilter" type="text" class="field" placeholder="输入关键字筛选目标目录" />
          </div>

          <div class="flex items-center justify-between gap-3">
            <button
              v-if="importParentPath"
              type="button"
              class="btn-secondary"
              @pointerdown.prevent="browseDirectory(importParentPath)"
            >
              上一级
            </button>
            <div v-else></div>
            <button
              type="button"
              class="btn-primary"
              :disabled="importLoading || !(pendingImportPath || importCurrentPath)"
              @pointerdown.prevent="selectCurrentDirectory"
            >
              选择当前目录
            </button>
          </div>

          <div class="max-h-[42vh] overflow-y-auto rounded-xl border border-slate-200 p-3">
            <div v-if="importLoading" class="py-6 text-center text-sm text-slate-500">目录加载中…</div>
            <div v-else-if="filteredImportItems.length === 0" class="py-6 text-center text-sm text-slate-500">没有匹配的目录，请继续输入或切换上级目录。</div>
            <div v-else class="space-y-2">
              <button
                v-for="item in filteredImportItems"
                :key="item.path"
                type="button"
                class="flex w-full items-center justify-between rounded-lg border border-slate-200 px-3 py-2.5 text-left text-sm text-slate-600 transition hover:bg-slate-50 hover:text-slate-900"
                @pointerdown.prevent="browseDirectory(item.path)"
              >
                <span>{{ item.name }}</span>
                <span class="text-xs text-slate-400">打开</span>
              </button>
            </div>
          </div>
        </div>
      </SurfaceCard>
      </div>
    </div>
    </Transition>
    </Teleport>
  </section>
</template>
