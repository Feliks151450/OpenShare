<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { ChevronRight, Folder } from "lucide-vue-next";
import { httpClient } from "../../lib/http/client";

interface AdminFolderItem {
  id: string;
  name: string;
  parent_id: string | null;
}

interface MoveFileItem {
  id: string;
  name: string;
}

interface FolderEntry {
  id: string;
  name: string;
  parentID: string;
}

const props = defineProps<{
  files: MoveFileItem[];
  /** 当前所在文件夹 ID（目标选择器中排除，不可选为目标） */
  currentFolderId: string;
  open: boolean;
}>();

const emit = defineEmits<{
  close: [];
  moved: [];
}>();

const submitting = ref(false);
const error = ref("");
const selectedTargetFolderId = ref("");
const selectedTargetFolderName = ref("");
const loading = ref(false);

/** 一次性从管理员接口加载所有非虚拟文件夹（不受 hide_public_catalog 影响） */
const allFolders = ref<AdminFolderItem[]>([]);

/** 当前浏览的文件夹 ID（空字符串=根层级） */
const browseParentID = ref("");
const browsePath = ref<{ id: string; name: string }[]>([]);

/** 从 allFolders 中筛选当前层级子文件夹 */
const entries = computed<FolderEntry[]>(() => {
  const pid = browseParentID.value;
  return allFolders.value
    .filter((f) => pid ? f.parent_id === pid : !f.parent_id)
    .map((f) => ({ id: f.id, name: f.name, parentID: pid }));
});

/** 加载所有文件夹（管理员接口，仅打开弹窗时调用一次） */
async function loadAllFolders() {
  loading.value = true;
  try {
    const resp = await httpClient.get<{ items: AdminFolderItem[] }>("/admin/resources/folders");
    allFolders.value = resp.items;
    loadLastTarget();
  } catch {
    error.value = "加载文件夹列表失败";
  } finally {
    loading.value = false;
  }
}

/** 进入子文件夹 */
function navigateInto(folder: FolderEntry) {
  browsePath.value.push({ id: folder.id, name: folder.name });
  browseParentID.value = folder.id;
  selectedTargetFolderId.value = "";
  selectedTargetFolderName.value = "";
}

/** 返回上级 */
function goBack() {
  browsePath.value.pop();
  browseParentID.value = browsePath.value.length > 0
    ? browsePath.value[browsePath.value.length - 1].id
    : "";
  selectedTargetFolderId.value = "";
  selectedTargetFolderName.value = "";
}

/** 当前浏览的文件夹是否是源文件夹（不可选为目标） */
const isCurrentFolderSelected = computed(() => {
  if (browsePath.value.length === 0) return false;
  return browsePath.value[browsePath.value.length - 1].id === props.currentFolderId;
});

/* ── 上次选择的目标文件夹（localStorage 持久化）── */
const LAST_TARGET_KEY = "openshare_move_file_last_target";
interface LastTarget {
  id: string;
  /** 面包屑路径拼接的显示名 */
  displayName: string;
}
const lastTarget = ref<LastTarget | null>(null);

/** 加载 localStorage 中上次选择的目标文件夹 */
function loadLastTarget() {
  try {
    const raw = localStorage.getItem(LAST_TARGET_KEY);
    if (raw) {
      const parsed = JSON.parse(raw) as LastTarget;
      if (parsed.id && parsed.displayName) {
        lastTarget.value = parsed;
      }
    }
  } catch {
    lastTarget.value = null;
  }
}

/** 上次选择的目标文件夹在 allFolders 中仍然存在，可以快速选择 */
const lastTargetAvailable = computed(() => {
  if (!lastTarget.value) return false;
  return allFolders.value.some((f) => f.id === lastTarget.value!.id && f.id !== props.currentFolderId);
});

/** 快速选择上次的目标文件夹 */
function selectLastTarget() {
  if (!lastTargetAvailable.value || !lastTarget.value) return;
  selectedTargetFolderId.value = lastTarget.value.id;
  selectedTargetFolderName.value = lastTarget.value.displayName;
}

/** 保存上次选择的目标文件夹到 localStorage */
function saveLastTarget() {
  if (!selectedTargetFolderId.value || !selectedTargetFolderName.value) return;
  try {
    localStorage.setItem(LAST_TARGET_KEY, JSON.stringify({
      id: selectedTargetFolderId.value,
      displayName: selectedTargetFolderName.value,
    }));
    lastTarget.value = {
      id: selectedTargetFolderId.value,
      displayName: selectedTargetFolderName.value,
    };
  } catch {
    // localStorage 不可用时忽略
  }
}

/** 选中当前浏览的文件夹作为目标 */
function selectCurrentFolder() {
  if (browsePath.value.length > 0 && !isCurrentFolderSelected.value) {
    const current = browsePath.value[browsePath.value.length - 1];
    selectedTargetFolderId.value = current.id;
    selectedTargetFolderName.value = browsePath.value.map((p) => p.name).join(" / ");
  }
}

/** 跳转到面包屑中某级 */
function jumpToBreadcrumb(idx: number) {
  browsePath.value = browsePath.value.slice(0, idx + 1);
  browseParentID.value = browsePath.value[idx].id;
  selectedTargetFolderId.value = "";
  selectedTargetFolderName.value = "";
}

/** 确认移动 */
async function confirmMove() {
  if (!selectedTargetFolderId.value || submitting.value) return;

  submitting.value = true;
  error.value = "";

  let moved = 0;
  for (const file of props.files) {
    try {
      await httpClient.request(
        `/admin/resources/files/${encodeURIComponent(file.id)}/move`,
        {
          method: "PUT",
          body: JSON.stringify({ target_folder_id: selectedTargetFolderId.value }),
        },
      );
      moved++;
    } catch {
      // 继续处理其他文件
    }
  }

  submitting.value = false;

  if (moved > 0) {
    saveLastTarget();
    emit("moved");
  } else {
    error.value = "移动失败，请重试";
  }
}

function reset() {
  selectedTargetFolderId.value = "";
  selectedTargetFolderName.value = "";
  browseParentID.value = "";
  browsePath.value = [];
  allFolders.value = [];
  error.value = "";
}

watch(
  () => props.open,
  (val) => {
    if (val) {
      reset();
      loadAllFolders();
    }
  },
);
</script>

<template>
  <Teleport to="body">
    <Transition name="modal-shell">
      <div
        v-if="open"
        class="fixed inset-0 z-[120] flex items-center justify-center bg-slate-950/30 px-4"
        @click.self="emit('close')"
      >
        <div class="modal-card w-full max-w-md rounded-2xl bg-white p-6 shadow-xl" @click.stop>
          <h3 class="text-lg font-semibold text-slate-900">移动文件到其他文件夹</h3>

          <!-- 待移动文件列表（只读） -->
          <div class="mt-3">
            <p class="text-sm text-slate-500">待移动文件：</p>
            <ul class="mt-1 max-h-28 space-y-0.5 overflow-y-auto rounded-lg border border-slate-200 bg-slate-50 px-3 py-2">
              <li v-for="file in files" :key="file.id" class="truncate text-sm text-slate-700">
                {{ file.name }}
              </li>
            </ul>
          </div>

          <!-- 目标文件夹选择器——浏览器风格 -->
          <div class="mt-4">
            <p class="text-sm text-slate-500">
              目标文件夹：
              <span v-if="selectedTargetFolderName" class="font-medium text-sky-600">{{ selectedTargetFolderName }}</span>
              <span v-else class="text-slate-400">请先浏览并点击"选为目的地"</span>
            </p>

            <!-- 上次选择的目标目录快捷按钮 -->
            <button
              v-if="lastTargetAvailable && !selectedTargetFolderId"
              type="button"
              class="mt-2 flex w-full items-center gap-2 rounded-lg border border-dashed border-sky-200 bg-sky-50/60 px-3 py-2 text-sm text-sky-700 transition hover:bg-sky-100"
              @click="selectLastTarget"
            >
              <Folder class="h-4 w-4 shrink-0" />
              <span class="truncate">上次选择：{{ lastTarget?.displayName }}</span>
            </button>

            <!-- 面包屑导航 -->
            <div class="mt-2 flex flex-wrap items-center gap-1 text-xs text-slate-500">
              <button
                type="button"
                class="rounded-md px-1.5 py-0.5 transition hover:bg-slate-100 hover:text-slate-800"
                :class="{ 'font-medium text-slate-800': browsePath.length === 0 }"
                :disabled="browsePath.length === 0"
                @click="browsePath = []; browseParentID = ''; selectedTargetFolderId = ''; selectedTargetFolderName = ''"
              >
                根目录
              </button>
              <template v-for="(seg, idx) in browsePath" :key="seg.id">
                <ChevronRight class="h-3 w-3 shrink-0" />
                <button
                  type="button"
                  class="max-w-[120px] truncate rounded-md px-1.5 py-0.5 transition hover:bg-slate-100 hover:text-slate-800"
                  :class="{ 'font-medium text-slate-800': idx === browsePath.length - 1 }"
                  @click="jumpToBreadcrumb(idx)"
                >
                  {{ seg.name }}
                </button>
              </template>
            </div>

            <!-- 文件夹列表 -->
            <div
              class="mt-2 max-h-48 min-h-[80px] overflow-y-auto rounded-lg border border-slate-200 bg-slate-50 p-1"
            >
              <!-- 返回上级按钮 -->
              <button
                v-if="browsePath.length > 0"
                type="button"
                class="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm text-slate-500 transition hover:bg-slate-100"
                @click="goBack"
              >
                <ChevronRight class="h-4 w-4 rotate-180" />
                ..
              </button>

              <div v-if="loading" class="px-3 py-4 text-center text-sm text-slate-400">加载中…</div>

              <div v-else-if="entries.length === 0 && !loading" class="px-3 py-4 text-center text-sm text-slate-400">
                此层级没有文件夹
              </div>

              <button
                v-for="entry in entries"
                :key="entry.id"
                type="button"
                class="flex w-full items-center gap-2 rounded-lg px-3 py-2 text-sm transition hover:bg-slate-100"
                :class="{ 'bg-amber-50': entry.id === currentFolderId }"
                :title="entry.id === currentFolderId ? '当前文件夹（不可选为目标）' : entry.name"
                @click="navigateInto(entry)"
              >
                <Folder class="h-4 w-4 shrink-0 text-sky-500" />
                <span class="truncate" :class="entry.id === currentFolderId ? 'text-amber-700' : 'text-slate-700'">{{ entry.name }}</span>
                <span v-if="entry.id === currentFolderId" class="shrink-0 text-xs text-amber-500">(当前)</span>
              </button>
            </div>

            <!-- 选为目的地按钮 -->
            <div v-if="browsePath.length > 0" class="mt-2">
              <button
                v-if="!isCurrentFolderSelected"
                type="button"
                class="btn-secondary w-full text-sm"
                @click="selectCurrentFolder"
              >
                将「{{ browsePath[browsePath.length - 1].name }}」选为目的地
              </button>
              <p v-else class="text-xs text-amber-600">
                当前文件夹为待移动文件的来源，不可选为目标
              </p>
            </div>
          </div>

          <!-- 错误信息 -->
          <p v-if="error" class="mt-3 text-sm text-rose-600">{{ error }}</p>

          <!-- 操作按钮 -->
          <div class="mt-6 flex justify-end gap-3">
            <button type="button" class="btn-secondary" :disabled="submitting" @click="emit('close')">
              取消
            </button>
            <button
              type="button"
              class="btn-primary"
              :disabled="!selectedTargetFolderId || submitting"
              @click="confirmMove"
            >
              {{ submitting ? "移动中…" : `确认移动 (${files.length})` }}
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
