<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";

import { HttpError, httpClient } from "../../lib/http/client";
import { useSessionStore } from "../../stores/session";

interface SystemPolicy {
  guest: {
    allow_direct_publish: boolean;
    extra_permissions_enabled: boolean;
    allow_guest_resource_edit: boolean;
    allow_guest_resource_delete: boolean;
  };
  upload: {
    max_file_size_bytes: number;
    max_tag_count: number;
    allowed_extensions: string[];
  };
  search: {
    enable_fuzzy_match: boolean;
    enable_tag_filter: boolean;
    enable_folder_scope: boolean;
    result_window: number;
  };
}

const sessionStore = useSessionStore();
const loading = ref(false);
const saving = ref(false);
const error = ref("");
const form = reactive<SystemPolicy>({
  guest: {
    allow_direct_publish: false,
    extra_permissions_enabled: false,
    allow_guest_resource_edit: false,
    allow_guest_resource_delete: false,
  },
  upload: {
    max_file_size_bytes: 0,
    max_tag_count: 10,
    allowed_extensions: [],
  },
  search: {
    enable_fuzzy_match: true,
    enable_tag_filter: true,
    enable_folder_scope: true,
    result_window: 50,
  },
});
const extensionsInput = ref("");

onMounted(() => {
  if (sessionStore.isSuperAdmin) {
    void loadPolicy();
  }
});

async function loadPolicy() {
  loading.value = true;
  error.value = "";
  try {
    const response = await httpClient.get<SystemPolicy>("/admin/system/settings");
    Object.assign(form.guest, response.guest);
    Object.assign(form.upload, response.upload);
    Object.assign(form.search, response.search);
    extensionsInput.value = response.upload.allowed_extensions.join(", ");
  } catch {
    error.value = "加载系统设置失败。";
  } finally {
    loading.value = false;
  }
}

async function savePolicy() {
  saving.value = true;
  error.value = "";
  form.upload.allowed_extensions = extensionsInput.value
    .split(",")
    .map((entry) => entry.trim())
    .filter(Boolean);
  try {
    await httpClient.request("/admin/system/settings", {
      method: "PUT",
      body: form,
    });
  } catch (err: unknown) {
    error.value = readApiError(err) ?? "保存系统设置失败。";
  } finally {
    saving.value = false;
  }
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
      <p class="text-sm font-semibold uppercase tracking-[0.22em] text-blue-300">System Settings</p>
      <h2 class="mt-2 text-3xl font-semibold text-white">系统策略配置</h2>
    </header>

    <p v-if="!sessionStore.isSuperAdmin" class="rounded-2xl bg-slate-950/70 px-4 py-3 text-sm text-slate-400">
      只有超级管理员可以修改系统策略。
    </p>

    <article v-else class="rounded-[28px] border border-slate-800 bg-slate-950/70 p-6">
      <p v-if="loading" class="text-sm text-slate-400">加载中...</p>
      <form v-else class="space-y-8" @submit.prevent="savePolicy">
        <section class="space-y-4">
          <h3 class="text-2xl font-semibold text-white">访客策略</h3>
          <label class="flex items-center gap-3 text-sm text-slate-200">
            <input v-model="form.guest.allow_direct_publish" type="checkbox" />
            允许游客免审核上传
          </label>
          <label class="flex items-center gap-3 text-sm text-slate-200">
            <input v-model="form.guest.extra_permissions_enabled" type="checkbox" />
            开放额外访客权限
          </label>
          <label class="flex items-center gap-3 text-sm text-slate-200">
            <input v-model="form.guest.allow_guest_resource_edit" type="checkbox" />
            允许访客编辑资料
          </label>
          <label class="flex items-center gap-3 text-sm text-slate-200">
            <input v-model="form.guest.allow_guest_resource_delete" type="checkbox" />
            允许访客删除资料
          </label>
        </section>

        <section class="space-y-4">
          <h3 class="text-2xl font-semibold text-white">上传策略</h3>
          <input
            v-model.number="form.upload.max_file_size_bytes"
            type="number"
            class="w-full rounded-2xl border border-slate-700 bg-slate-900 px-4 py-3 text-sm text-white outline-none focus:border-blue-400"
            placeholder="最大文件大小（字节）"
          />
          <input
            v-model.number="form.upload.max_tag_count"
            type="number"
            class="w-full rounded-2xl border border-slate-700 bg-slate-900 px-4 py-3 text-sm text-white outline-none focus:border-blue-400"
            placeholder="最大 Tag 数量"
          />
          <input
            v-model="extensionsInput"
            class="w-full rounded-2xl border border-slate-700 bg-slate-900 px-4 py-3 text-sm text-white outline-none focus:border-blue-400"
            placeholder=".pdf, .zip, .md"
          />
        </section>

        <section class="space-y-4">
          <h3 class="text-2xl font-semibold text-white">搜索策略</h3>
          <label class="flex items-center gap-3 text-sm text-slate-200">
            <input v-model="form.search.enable_fuzzy_match" type="checkbox" />
            启用模糊匹配
          </label>
          <label class="flex items-center gap-3 text-sm text-slate-200">
            <input v-model="form.search.enable_tag_filter" type="checkbox" />
            启用 Tag 过滤
          </label>
          <label class="flex items-center gap-3 text-sm text-slate-200">
            <input v-model="form.search.enable_folder_scope" type="checkbox" />
            启用目录范围搜索
          </label>
          <input
            v-model.number="form.search.result_window"
            type="number"
            class="w-full rounded-2xl border border-slate-700 bg-slate-900 px-4 py-3 text-sm text-white outline-none focus:border-blue-400"
            placeholder="搜索结果窗口"
          />
        </section>

        <button type="submit" class="rounded-2xl bg-blue-500 px-5 py-3 text-sm font-semibold text-slate-950" :disabled="saving">
          {{ saving ? "保存中..." : "保存系统设置" }}
        </button>
      </form>

      <p v-if="error" class="mt-4 rounded-2xl bg-rose-950/60 px-4 py-3 text-sm text-rose-200">{{ error }}</p>
    </article>
  </section>
</template>
