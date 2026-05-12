<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";

import SurfaceCard from "../ui/SurfaceCard.vue";
import { httpClient } from "../../lib/http/client";
import { readApiError } from "../../lib/http/helpers";

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
}

interface ManagedFolderNode {
  id: string;
  name: string;
  source_path: string;
  hide_public_catalog?: boolean;
  cdn_url?: string;
  folders: ManagedFolderNode[];
}

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
const managedFolders = ref<Array<{ id: string; name: string; sourcePath: string; hidePublicCatalog: boolean; cdnUrl: string }>>([]);
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
});

onMounted(() => {
  void Promise.all([loadPolicy(), loadDirectories(""), loadManagedFolders()]);
});

async function loadPolicy() {
  loading.value = true;
  error.value = "";
  message.value = "";
  try {
    const response = await httpClient.get<SystemPolicy>("/admin/system/settings");
    Object.assign(form.upload, response.upload);
    Object.assign(form.download, response.download ?? { large_download_confirm_bytes: 1024 * 1024 * 1024 });
    applyUploadSizeFields(response.upload.max_upload_total_bytes);
    applyDownloadSizeFields(form.download.large_download_confirm_bytes);
    uploadSnapshot.value = serializeUploadState();
    downloadSnapshot.value = serializeDownloadState();
  } catch {
    error.value = "加载系统设置失败。";
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
    message.value = "系统策略已更新。";
  } catch (err: unknown) {
    error.value = readApiError(err, "更新系统策略失败。");
  } finally {
    uploadSaving.value = false;
  }
}

function serializeUploadState() {
  return JSON.stringify({
    max_upload_total_bytes: toBytes(uploadSizeValue.value, uploadSizeUnit.value),
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
      importError.value = readApiError(err, "加载目录浏览器失败。");
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
    }));
  } catch (err: unknown) {
    managedFolders.value = [];
    managedFoldersError.value = readApiError(err, "加载已托管目录失败。");
  } finally {
    managedFoldersLoading.value = false;
  }
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
    rescanError.value = readApiError(err, "导出全局数据失败。");
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
    rescanError.value = readApiError(err, `导出 ${folderName} 失败。`);
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
    rescanError.value = readApiError(err, "更新 CDN 地址失败。");
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
    importError.value = "请先选择服务器目录。";
    return;
  }
  if (importPathConflict.value) {
    importError.value = importPathConflict.value;
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
    importMessage.value = `导入完成：${response.imported_folders} 个目录，${response.imported_files} 个文件。`;
    confirmedImportPath.value = "";
    importPath.value = "";
    await loadManagedFolders();
  } catch (err: unknown) {
    importError.value = readApiError(err, "导入目录失败。");
  } finally {
    importLoading.value = false;
  }
}

// 创建虚拟托管根目录（无本地磁盘路径）
async function createVirtualManagedRoot() {
  const name = virtualRootName.value.trim();
  if (!name) {
    virtualRootError.value = "请输入目录名称。";
    return;
  }
  virtualRootCreating.value = true;
  virtualRootError.value = "";
  try {
    await httpClient.post("/admin/resources/virtual-folders", { name, parent_id: "" });
    virtualRootName.value = "";
    await loadManagedFolders();
  } catch (err: unknown) {
    virtualRootError.value = readApiError(err, "创建虚拟托管目录失败。");
  } finally {
    virtualRootCreating.value = false;
  }
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
    managedFoldersError.value = readApiError(err, "更新访客首页可见性失败。");
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
    rescanMessage.value =
      `重新扫描完成：新增目录 ${response.added_folders} 个，新增文件 ${response.added_files} 个，` +
      `更新目录 ${response.updated_folders} 个，更新文件 ${response.updated_files} 个，` +
      `删除目录 ${response.deleted_folders} 个，删除文件 ${response.deleted_files} 个。`;
    await loadManagedFolders();
  } catch (err: unknown) {
    rescanError.value = readApiError(err, "重新扫描托管目录失败。");
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
    unmanageError.value = "请输入超级管理员密码。";
    return;
  }

  unmanageError.value = "";
  unmanageMessage.value = "";
  try {
    await httpClient.request(`/admin/imports/local/${encodeURIComponent(folderID)}`, {
      method: "DELETE",
      body: { password: unmanagePassword.value },
    });
    unmanageMessage.value = "已取消托管，并清理站内关联数据。";
    unmanagingFolderID.value = "";
    unmanagePassword.value = "";
    await loadManagedFolders();
  } catch (err: unknown) {
    unmanageError.value = readApiError(err, "取消托管目录失败。");
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
            <div class="flex items-start justify-between gap-4">
              <div class="min-w-0">
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
                    class="field h-9 flex-1 text-sm"
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
              </div>
              <div class="flex shrink-0 flex-wrap items-center justify-end gap-2">
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
        <p v-if="rescanMessage" class="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">{{ rescanMessage }}</p>
        <p v-if="rescanError" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{{ rescanError }}</p>
        <p v-if="unmanageMessage" class="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">{{ unmanageMessage }}</p>
        <p v-if="unmanageError" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{{ unmanageError }}</p>
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
        <p v-if="virtualRootError" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{{ virtualRootError }}</p>
        <button type="button" class="btn-primary w-full" :disabled="virtualRootCreating || !virtualRootName.trim()" @click="createVirtualManagedRoot">
          {{ virtualRootCreating ? "创建中…" : "创建虚拟托管目录" }}
        </button>
      </SurfaceCard>
      </div>
    </div>

    <p v-if="message" class="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">{{ message }}</p>
    <p v-if="error" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{{ error }}</p>
    <p v-if="importMessage" class="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-700">{{ importMessage }}</p>
    <p v-if="importError" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-700">{{ importError }}</p>

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
