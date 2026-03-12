<script setup lang="ts">
import { onMounted, ref } from "vue";

import { httpClient } from "../../lib/http/client";
import { useSessionStore } from "../../stores/session";

interface ManagedFileItem {
  id: string;
  title: string;
  description: string;
  original_name: string;
  status: "active" | "offline" | "deleted";
  size: number;
  download_count: number;
  folder_name: string;
  tags: string[];
}

const sessionStore = useSessionStore();

const items = ref<ManagedFileItem[]>([]);
const loading = ref(false);
const error = ref("");
const query = ref("");
const status = ref("");

onMounted(() => {
  void loadItems();
});

async function loadItems() {
  loading.value = true;
  error.value = "";
  try {
    const params = new URLSearchParams();
    if (query.value.trim()) {
      params.set("q", query.value.trim());
    }
    if (status.value) {
      params.set("status", status.value);
    }
    const suffix = params.size > 0 ? `?${params.toString()}` : "";
    const response = await httpClient.get<{ items: ManagedFileItem[] }>(`/admin/resources/files${suffix}`);
    items.value = response.items ?? [];
  } catch {
    error.value = "加载资料列表失败。";
  } finally {
    loading.value = false;
  }
}

async function saveItem(item: ManagedFileItem) {
  await httpClient.request(`/admin/resources/files/${item.id}`, {
    method: "PUT",
    body: {
      title: item.title,
      description: item.description,
      tags: item.tags,
    },
  });
  await loadItems();
}

async function offlineItem(item: ManagedFileItem) {
  await httpClient.post(`/admin/resources/files/${item.id}/offline`);
  await loadItems();
}

async function deleteItem(item: ManagedFileItem) {
  if (!window.confirm(`确认删除资料《${item.title}》吗？`)) {
    return;
  }
  await httpClient.request(`/admin/resources/files/${item.id}`, { method: "DELETE" });
  await loadItems();
}

function updateTags(item: ManagedFileItem, raw: string) {
  item.tags = raw
    .split(",")
    .map((entry) => entry.trim())
    .filter(Boolean);
}

function formatSize(size: number) {
  if (size < 1024) {
    return `${size} B`;
  }
  if (size < 1024 * 1024) {
    return `${(size / 1024).toFixed(1)} KB`;
  }
  return `${(size / (1024 * 1024)).toFixed(1)} MB`;
}
</script>

<template>
  <section class="space-y-8">
    <header>
      <p class="text-sm font-semibold uppercase tracking-[0.22em] text-blue-300">Resources</p>
      <h2 class="mt-2 text-3xl font-semibold text-white">资料管理</h2>
    </header>

    <article class="rounded-[28px] border border-slate-800 bg-slate-950/70 p-6">
      <div class="grid gap-4 lg:grid-cols-[1fr_220px_auto]">
        <input
          v-model="query"
          placeholder="按标题、原始文件名或描述搜索"
          class="rounded-2xl border border-slate-700 bg-slate-900 px-4 py-3 text-sm text-white outline-none focus:border-blue-400"
        />
        <select
          v-model="status"
          class="rounded-2xl border border-slate-700 bg-slate-900 px-4 py-3 text-sm text-white outline-none focus:border-blue-400"
        >
          <option value="">全部状态</option>
          <option value="active">active</option>
          <option value="offline">offline</option>
          <option value="deleted">deleted</option>
        </select>
        <button class="rounded-2xl bg-blue-500 px-5 py-3 text-sm font-semibold text-slate-950" @click="loadItems">
          搜索
        </button>
      </div>

      <p v-if="error" class="mt-4 rounded-2xl bg-rose-950/60 px-4 py-3 text-sm text-rose-200">{{ error }}</p>
      <p v-else-if="loading" class="mt-4 text-sm text-slate-400">加载中...</p>

      <div v-else class="mt-6 space-y-4">
        <article
          v-for="item in items"
          :key="item.id"
          class="rounded-[22px] border border-slate-800 bg-slate-900 p-5"
        >
          <div class="flex flex-wrap items-start justify-between gap-4">
            <div>
              <p class="text-xs uppercase tracking-[0.16em] text-slate-500">{{ item.folder_name || "根目录" }}</p>
              <p class="mt-1 text-sm text-slate-500">{{ item.original_name }} · {{ formatSize(item.size) }} · {{ item.status }}</p>
            </div>
            <div class="flex flex-wrap gap-2">
              <button
                v-if="sessionStore.hasPermission('delete_resources')"
                class="rounded-xl border border-slate-700 px-4 py-2 text-sm text-slate-200"
                @click="offlineItem(item)"
              >
                下架
              </button>
              <button
                v-if="sessionStore.hasPermission('delete_resources')"
                class="rounded-xl bg-rose-400 px-4 py-2 text-sm font-semibold text-slate-950"
                @click="deleteItem(item)"
              >
                删除
              </button>
            </div>
          </div>

          <div class="mt-4 grid gap-4 lg:grid-cols-2">
            <input
              v-model="item.title"
              class="rounded-2xl border border-slate-700 bg-slate-950 px-4 py-3 text-sm text-white outline-none focus:border-blue-400"
            />
            <input
              :value="item.tags.join(', ')"
              class="rounded-2xl border border-slate-700 bg-slate-950 px-4 py-3 text-sm text-white outline-none focus:border-blue-400"
              @change="updateTags(item, ($event.target as HTMLInputElement).value)"
            />
          </div>
          <textarea
            v-model="item.description"
            rows="3"
            class="mt-4 w-full rounded-2xl border border-slate-700 bg-slate-950 px-4 py-3 text-sm text-white outline-none focus:border-blue-400"
          />
          <button
            v-if="sessionStore.hasPermission('edit_resources')"
            class="mt-4 rounded-2xl bg-blue-500 px-5 py-3 text-sm font-semibold text-slate-950"
            @click="saveItem(item)"
          >
            保存修改
          </button>
        </article>

        <p v-if="items.length === 0" class="rounded-2xl bg-slate-900 px-4 py-6 text-sm text-slate-400">
          没有匹配的资料。
        </p>
      </div>
    </article>
  </section>
</template>
