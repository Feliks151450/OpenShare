<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";

import EmptyState from "../../components/ui/EmptyState.vue";
import PageHeader from "../../components/ui/PageHeader.vue";
import SurfaceCard from "../../components/ui/SurfaceCard.vue";
import { httpClient } from "../../lib/http/client";
import { readApiError } from "../../lib/http/helpers";
import { renderSimpleMarkdown } from "../../lib/markdown";
import { toastError, toastSuccess } from "../../lib/toast";
import { useSessionStore } from "../../stores/session";

type AnnouncementStatus = "draft" | "published" | "hidden";

interface AnnouncementItem {
  id: string;
  title: string;
  content: string;
  status: AnnouncementStatus;
  is_pinned: boolean;
  created_by_id: string;
  published_at?: string;
  created_at: string;
  updated_at: string;
}

const sessionStore = useSessionStore();
const route = useRoute();
const router = useRouter();
const items = ref<AnnouncementItem[]>([]);
const loading = ref(false);
const loaded = ref(false);
const error = ref("");
const message = ref("");
const editorOpen = ref(false);
const editingItem = ref<AnnouncementItem | null>(null);
const saving = ref(false);
const deleteTarget = ref<AnnouncementItem | null>(null);
const deleting = ref(false);

const form = reactive({
  title: "",
  content: "",
  status: "published" as AnnouncementStatus,
  isPinned: false,
});

const canManageAnnouncements = computed(() => sessionStore.hasPermission("announcements"));
const editorTitle = computed(() => editingItem.value ? "编辑公告" : "新建公告");
const previewHTML = computed(() => renderSimpleMarkdown(form.content));
const formDirty = computed(() => {
  if (!editingItem.value) {
    return form.title.trim().length > 0 || form.content.trim().length > 0;
  }
  return (
    form.title.trim() !== editingItem.value.title ||
    form.content.trim() !== editingItem.value.content ||
    form.status !== editingItem.value.status ||
    form.isPinned !== editingItem.value.is_pinned
  );
});

onMounted(() => {
  if (canManageAnnouncements.value) {
    void (async () => {
      await loadItems();
      await tryOpenEditorFromQuery();
    })();
  }
});

watch(
  () => route.query.edit,
  () => {
    if (!canManageAnnouncements.value || !loaded.value) {
      return;
    }
    void tryOpenEditorFromQuery();
  },
);

async function loadItems() {
  loading.value = true;
  error.value = "";
  try {
    const response = await httpClient.get<{ items: AnnouncementItem[] }>("/admin/announcements");
    items.value = response.items ?? [];
  } catch (err: unknown) {
    toastError(readApiError(err, "加载公告列表失败。"));
  } finally {
    loaded.value = true;
    loading.value = false;
  }
}

function normalizeRouteEditId(raw: unknown): string {
  if (typeof raw === "string") {
    return raw.trim();
  }
  if (Array.isArray(raw) && typeof raw[0] === "string") {
    return raw[0].trim();
  }
  return "";
}

async function tryOpenEditorFromQuery() {
  const id = normalizeRouteEditId(route.query.edit);
  if (!id) {
    return;
  }
  if (!canManageAnnouncements.value) {
    return;
  }
  const item = items.value.find((x) => x.id === id);
  if (item && canEdit(item)) {
    openEditEditor(item);
    await router.replace({ name: "admin-announcements", query: {} });
  } else if (loaded.value) {
    await router.replace({ name: "admin-announcements", query: {} });
  }
}

function canEdit(item: AnnouncementItem) {
  return sessionStore.isSuperAdmin || item.created_by_id === sessionStore.adminId;
}

function canDelete(item: AnnouncementItem) {
  return sessionStore.isSuperAdmin || item.created_by_id === sessionStore.adminId;
}

function openCreateEditor() {
  editingItem.value = null;
  form.title = "";
  form.content = "";
  form.status = "published";
  form.isPinned = false;
  editorOpen.value = true;
}

function openEditEditor(item: AnnouncementItem) {
  editingItem.value = item;
  form.title = item.title;
  form.content = item.content;
  form.status = item.status;
  form.isPinned = item.is_pinned;
  editorOpen.value = true;
}

function closeEditor() {
  editorOpen.value = false;
  editingItem.value = null;
  saving.value = false;
}

async function saveAnnouncement() {
  if (!formDirty.value) {
    return;
  }
  saving.value = true;
  error.value = "";
  message.value = "";
  try {
    const body = {
      title: form.title.trim(),
      content: form.content.trim(),
      status: form.status,
      ...(sessionStore.isSuperAdmin ? { is_pinned: form.isPinned } : {}),
    };
    if (editingItem.value) {
      await httpClient.request(`/admin/announcements/${editingItem.value.id}`, {
        method: "PUT",
        body,
      });
      toastSuccess("公告已更新。");
    } else {
      await httpClient.post("/admin/announcements", body);
      toastSuccess("公告已发布。");
    }
    closeEditor();
    await loadItems();
  } catch (err: unknown) {
    toastError(readApiError(err, editingItem.value ? "更新公告失败。" : "创建公告失败。"));
  } finally {
    saving.value = false;
  }
}

function requestDelete(item: AnnouncementItem) {
  deleteTarget.value = item;
}

function closeDeleteDialog() {
  deleteTarget.value = null;
  deleting.value = false;
}

async function confirmDelete() {
  if (!deleteTarget.value) {
    return;
  }
  deleting.value = true;
  error.value = "";
  message.value = "";
  try {
    await httpClient.request(`/admin/announcements/${deleteTarget.value.id}`, { method: "DELETE" });
    toastSuccess("公告已删除。");
    closeDeleteDialog();
    await loadItems();
  } catch (err: unknown) {
    toastError(readApiError(err, "删除公告失败。"));
  } finally {
    deleting.value = false;
  }
}

function formatDate(value?: string) {
  if (!value) {
    return "未发布";
  }
  return new Intl.DateTimeFormat("zh-CN", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(new Date(value));
}

function statusLabel(status: AnnouncementStatus) {
  switch (status) {
    case "draft":
      return "草稿";
    case "hidden":
      return "隐藏";
    default:
      return "已发布";
  }
}

function statusClass(status: AnnouncementStatus) {
  switch (status) {
    case "draft":
      return "bg-slate-100 text-slate-600";
    case "hidden":
      return "bg-amber-50 text-amber-700";
    default:
      return "bg-emerald-50 text-emerald-700";
  }
}

function pinLabel(item: AnnouncementItem) {
  return item.is_pinned ? "已置顶" : "普通公告";
}
</script>

<template>
  <!-- 公告管理页：管理员可查看公告列表、新建/编辑/删除公告；新建编辑弹窗支持 Markdown 实时预览 -->
  <section class="space-y-8">
    <PageHeader eyebrow="Announcements" title="公告" />

    <!-- 公告列表卡片 -->
    <SurfaceCard class="space-y-5">
      <div class="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h2 class="text-lg font-semibold text-slate-900">公告列表</h2>
        </div>
        <div class="flex gap-3">
          <button class="btn-secondary" @click="loadItems">刷新</button>
          <button class="btn-primary" @click="openCreateEditor">新建公告</button>
        </div>
      </div>

      <!-- 操作反馈消息 -->
<div v-if="!canManageAnnouncements" class="text-sm text-slate-500">当前账号没有公告权限。</div>
      <div v-else-if="loading && !loaded" class="text-sm text-slate-500">加载中…</div>
      <div v-else class="space-y-4">
        <!-- 单条公告卡片：标题 + 状态/置顶标签 + 时间 + Markdown 内容预览（最多4行） + 编辑/删除按钮 -->
        <article v-for="item in items" :key="item.id" class="rounded-2xl border border-slate-200 p-5">
          <div class="flex flex-wrap items-start justify-between gap-4">
            <div class="min-w-0 flex-1">
              <div class="flex flex-wrap items-center gap-2">
                <h3 class="text-lg font-semibold text-slate-900">{{ item.title }}</h3>
                <span class="rounded-lg px-2.5 py-1 text-xs font-medium" :class="statusClass(item.status)">
                  {{ statusLabel(item.status) }}
                </span>
                <span
                  class="rounded-lg px-2.5 py-1 text-xs font-medium"
                  :class="item.is_pinned ? 'bg-blue-50 text-blue-700' : 'bg-slate-100 text-slate-500'"
                >
                  {{ pinLabel(item) }}
                </span>
              </div>
              <div class="mt-2 flex flex-wrap gap-4 text-sm text-slate-500">
                <span>发布时间：{{ formatDate(item.published_at) }}</span>
                <span>更新时间：{{ formatDate(item.updated_at) }}</span>
                <span>{{ item.created_by_id === sessionStore.adminId ? "我发布的" : `发布者：${item.created_by_id}` }}</span>
              </div>
              <!-- 公告正文 Markdown 渲染预览（截断至4行） -->
              <div class="markdown-content mt-4 line-clamp-4 text-sm text-slate-600" v-html="renderSimpleMarkdown(item.content)" />
            </div>
            <!-- 操作按钮：仅创建者或超管可编辑/删除 -->
            <div class="flex shrink-0 flex-wrap gap-2">
              <button v-if="canEdit(item)" class="btn-secondary" @click="openEditEditor(item)">编辑</button>
              <button
                v-if="canDelete(item)"
                class="inline-flex h-11 items-center rounded-xl border border-rose-200 px-4 text-sm font-medium text-rose-700 transition hover:bg-rose-50"
                @click="requestDelete(item)"
              >
                删除
              </button>
            </div>
          </div>
        </article>

        <EmptyState v-if="!loading && items.length === 0" title="暂无公告" />
      </div>
    </SurfaceCard>
  </section>

  <!-- 新建/编辑公告弹窗：左右分栏布局，左侧编辑区（标题 + 发布设置 + 正文），右侧 Markdown 实时预览 -->
  <Teleport to="body">
    <Transition name="modal-shell">
    <div v-if="editorOpen" class="fixed inset-0 z-[120] overflow-y-auto bg-slate-950/30 px-4 py-6">
      <div class="mx-auto w-full max-w-5xl">
        <SurfaceCard class="modal-card space-y-6">
          <div class="flex items-start justify-between gap-4 border-b border-slate-200 pb-4">
            <div>
              <p class="text-xs font-semibold uppercase tracking-[0.18em] text-blue-600">Announcement Editor</p>
              <h3 class="mt-2 text-2xl font-semibold tracking-tight text-slate-900">{{ editorTitle }}</h3>
              <p class="mt-2 text-sm text-slate-500">支持简单 Markdown 语法，暂不支持图片插入。</p>
            </div>
            <button class="btn-secondary" @click="closeEditor">关闭</button>
          </div>

          <div class="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
            <!-- 左侧：编辑区 -->
            <div class="space-y-4">
              <label class="space-y-2">
                <span class="text-sm font-medium text-slate-700">公告标题</span>
                <input v-model="form.title" class="field" placeholder="输入公告标题" />
              </label>

              <!-- 发布设置：公开/草稿切换 + 置顶切换（仅超管可置顶） -->
              <div class="space-y-2">
                <span class="text-sm font-medium text-slate-700">发布设置</span>
                <div class="grid gap-3 sm:grid-cols-2">
                  <button
                    type="button"
                    class="flex items-center justify-between rounded-2xl border px-4 py-3 text-left transition"
                    :class="form.status === 'published' ? 'border-blue-200 bg-blue-50 text-blue-700' : 'border-slate-200 bg-white text-slate-600 hover:bg-slate-50'"
                    @click="form.status = form.status === 'published' ? 'draft' : 'published'"
                  >
                    <div>
                      <p class="text-sm font-semibold">是否公开</p>
                      <p class="mt-1 text-xs" :class="form.status === 'published' ? 'text-blue-600/80' : 'text-slate-400'">
                        {{ form.status === 'published' ? '公开展示' : '暂不公开' }}
                      </p>
                    </div>
                    <span class="rounded-full px-3 py-1 text-xs font-semibold" :class="form.status === 'published' ? 'bg-white text-blue-700' : 'bg-slate-100 text-slate-500'">
                      {{ form.status === 'published' ? '公开' : '不公开' }}
                    </span>
                  </button>

                  <button
                    v-if="sessionStore.isSuperAdmin"
                    type="button"
                    class="flex items-center justify-between rounded-2xl border px-4 py-3 text-left transition"
                    :class="form.isPinned ? 'border-blue-200 bg-blue-50 text-blue-700' : 'border-slate-200 bg-white text-slate-600 hover:bg-slate-50'"
                    @click="form.isPinned = !form.isPinned"
                  >
                    <div>
                      <p class="text-sm font-semibold">是否置顶</p>
                      <p class="mt-1 text-xs" :class="form.isPinned ? 'text-blue-600/80' : 'text-slate-400'">
                        {{ form.isPinned ? '优先展示在前面' : '按时间顺序展示' }}
                      </p>
                    </div>
                    <span class="rounded-full px-3 py-1 text-xs font-semibold" :class="form.isPinned ? 'bg-white text-blue-700' : 'bg-slate-100 text-slate-500'">
                      {{ form.isPinned ? '置顶' : '普通' }}
                    </span>
                  </button>
                </div>
              </div>

              <label class="space-y-2">
                <span class="text-sm font-medium text-slate-700">公告内容</span>
                <textarea
                  v-model="form.content"
                  rows="16"
                  class="field-area"
                  placeholder="输入公告正文，支持简单 Markdown 语法"
                />
              </label>
            </div>

            <!-- 右侧：Markdown 实时预览区 -->
            <div class="space-y-3">
              <div>
                <p class="text-xs font-semibold uppercase tracking-[0.18em] text-blue-600">Preview</p>
                <h4 class="mt-2 text-lg font-semibold text-slate-900">{{ form.title.trim() || "公告预览" }}</h4>
              </div>
              <div class="min-h-[420px] rounded-3xl border border-slate-200 bg-white px-5 py-5">
                <div v-if="previewHTML" class="markdown-content" v-html="previewHTML" />
                <p v-else class="text-sm text-slate-400">这里会显示公告预览。</p>
              </div>
            </div>
          </div>

          <div class="flex justify-end gap-3 border-t border-slate-200 pt-4">
            <button class="btn-secondary" @click="closeEditor">取消</button>
            <button class="btn-primary" :disabled="saving || !formDirty" @click="saveAnnouncement">
              {{ saving ? "保存中…" : "保存公告" }}
            </button>
          </div>
        </SurfaceCard>
      </div>
    </div>
    </Transition>
  </Teleport>

  <!-- 删除公告确认弹窗 -->
  <Teleport to="body">
    <Transition name="modal-shell">
    <div v-if="deleteTarget" class="fixed inset-0 z-[120] flex items-center justify-center bg-slate-950/30 px-4">
      <div class="modal-card w-full max-w-md rounded-2xl bg-white p-6 shadow-xl">
        <h3 class="text-lg font-semibold text-slate-900">确认删除公告</h3>
        <p class="mt-2 text-sm leading-6 text-slate-500">
          删除后首页将不再展示这条公告。确认删除
          <span class="font-medium text-slate-900">{{ deleteTarget.title }}</span>
          吗？
        </p>
        <div class="mt-6 flex justify-end gap-3">
          <button class="btn-secondary" @click="closeDeleteDialog">取消</button>
          <button
            class="inline-flex h-11 items-center rounded-xl bg-rose-600 px-5 text-sm font-medium text-white transition hover:bg-rose-700"
            :disabled="deleting"
            @click="confirmDelete"
          >
            {{ deleting ? "删除中…" : "确认删除" }}
          </button>
        </div>
      </div>
    </div>
    </Transition>
  </Teleport>
</template>
