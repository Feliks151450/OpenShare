<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";

import { HttpError, httpClient } from "../../lib/http/client";
import { useSessionStore } from "../../stores/session";

type AnnouncementStatus = "draft" | "published" | "hidden";

interface AnnouncementItem {
  id: string;
  title: string;
  content: string;
  status: AnnouncementStatus;
  published_at?: string | null;
  created_at: string;
  updated_at: string;
}

const sessionStore = useSessionStore();
const items = ref<AnnouncementItem[]>([]);
const loading = ref(false);
const error = ref("");
const saving = ref(false);
const editingId = ref("");
const form = reactive({
  title: "",
  content: "",
  status: "draft" as AnnouncementStatus,
});

onMounted(() => {
  if (sessionStore.hasPermission("manage_announcements")) {
    void loadItems();
  }
});

async function loadItems() {
  loading.value = true;
  error.value = "";
  try {
    const response = await httpClient.get<{ items: AnnouncementItem[] }>("/admin/announcements");
    items.value = response.items ?? [];
  } catch {
    error.value = "加载公告失败。";
  } finally {
    loading.value = false;
  }
}

async function saveAnnouncement() {
  saving.value = true;
  error.value = "";
  try {
    if (editingId.value) {
      await httpClient.request(`/admin/announcements/${editingId.value}`, {
        method: "PUT",
        body: form,
      });
    } else {
      await httpClient.post("/admin/announcements", form);
    }
    resetForm();
    await loadItems();
  } catch (err: unknown) {
    error.value = readApiError(err) ?? "保存公告失败。";
  } finally {
    saving.value = false;
  }
}

async function removeAnnouncement(id: string) {
  if (!window.confirm("确认删除这条公告吗？")) {
    return;
  }
  await httpClient.request(`/admin/announcements/${id}`, { method: "DELETE" });
  await loadItems();
}

function editAnnouncement(item: AnnouncementItem) {
  editingId.value = item.id;
  form.title = item.title;
  form.content = item.content;
  form.status = item.status;
}

function resetForm() {
  editingId.value = "";
  form.title = "";
  form.content = "";
  form.status = "draft";
}

function formatDate(value?: string | null) {
  if (!value) {
    return "未发布";
  }
  return new Intl.DateTimeFormat("zh-CN", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(new Date(value));
}

function readApiError(error: unknown) {
  if (!(error instanceof HttpError) || typeof error.payload !== "object" || error.payload === null) {
    return null;
  }
  const payload = error.payload as Record<string, unknown>;
  return typeof payload.error === "string" ? payload.error : null;
}
</script>

<template>
  <section class="space-y-8">
    <header>
      <p class="text-sm font-semibold uppercase tracking-[0.22em] text-blue-300">Announcements</p>
      <h2 class="mt-2 text-3xl font-semibold text-white">公告管理</h2>
    </header>

    <p
      v-if="!sessionStore.hasPermission('manage_announcements')"
      class="rounded-2xl bg-slate-950/70 px-4 py-3 text-sm text-slate-400"
    >
      当前账号没有公告管理权限。
    </p>

    <template v-else>
      <article class="rounded-[28px] border border-slate-800 bg-slate-950/70 p-6">
        <div class="flex items-center justify-between gap-4">
          <div>
            <h3 class="text-2xl font-semibold text-white">{{ editingId ? "编辑公告" : "新建公告" }}</h3>
            <p class="mt-2 text-sm text-slate-400">已发布公告会出现在首页，隐藏和草稿不会对外展示。</p>
          </div>
          <button class="rounded-2xl border border-slate-700 px-4 py-3 text-sm text-slate-200" @click="resetForm">
            重置
          </button>
        </div>

        <form class="mt-6 space-y-4" @submit.prevent="saveAnnouncement">
          <input
            v-model="form.title"
            placeholder="公告标题"
            class="w-full rounded-2xl border border-slate-700 bg-slate-900 px-4 py-3 text-sm text-white outline-none focus:border-blue-400"
          />
          <textarea
            v-model="form.content"
            rows="5"
            placeholder="公告内容"
            class="w-full rounded-2xl border border-slate-700 bg-slate-900 px-4 py-3 text-sm text-white outline-none focus:border-blue-400"
          />
          <select
            v-model="form.status"
            class="w-full rounded-2xl border border-slate-700 bg-slate-900 px-4 py-3 text-sm text-white outline-none focus:border-blue-400"
          >
            <option value="draft">草稿</option>
            <option value="published">发布</option>
            <option value="hidden">隐藏</option>
          </select>
          <button
            type="submit"
            class="rounded-2xl bg-blue-500 px-5 py-3 text-sm font-semibold text-slate-950"
            :disabled="saving"
          >
            {{ saving ? "保存中..." : editingId ? "保存修改" : "创建公告" }}
          </button>
        </form>
      </article>

      <p v-if="error" class="rounded-2xl bg-rose-950/60 px-4 py-3 text-sm text-rose-200">{{ error }}</p>

      <article class="rounded-[28px] border border-slate-800 bg-slate-950/70 p-6">
        <div class="flex items-center justify-between gap-4">
          <h3 class="text-2xl font-semibold text-white">公告列表</h3>
          <button class="rounded-2xl border border-slate-700 px-4 py-3 text-sm text-slate-200" @click="loadItems">
            刷新
          </button>
        </div>

        <p v-if="loading" class="mt-4 text-sm text-slate-400">加载中...</p>
        <div v-else class="mt-5 space-y-4">
          <article
            v-for="item in items"
            :key="item.id"
            class="rounded-[22px] border border-slate-800 bg-slate-900 p-5"
          >
            <div class="flex flex-wrap items-start justify-between gap-4">
              <div>
                <h4 class="text-lg font-semibold text-white">{{ item.title }}</h4>
                <p class="mt-2 text-sm text-slate-400">{{ item.content }}</p>
                <p class="mt-3 text-xs text-slate-500">状态：{{ item.status }} / 发布时间：{{ formatDate(item.published_at) }}</p>
              </div>
              <div class="flex gap-2">
                <button class="rounded-xl border border-slate-700 px-4 py-2 text-sm text-slate-200" @click="editAnnouncement(item)">
                  编辑
                </button>
                <button class="rounded-xl bg-rose-400 px-4 py-2 text-sm font-semibold text-slate-950" @click="removeAnnouncement(item.id)">
                  删除
                </button>
              </div>
            </div>
          </article>
          <p v-if="items.length === 0" class="rounded-2xl bg-slate-900 px-4 py-6 text-sm text-slate-400">
            还没有公告。
          </p>
        </div>
      </article>
    </template>
  </section>
</template>
