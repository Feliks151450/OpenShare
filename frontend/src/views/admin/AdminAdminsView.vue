<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";

import { HttpError, httpClient } from "../../lib/http/client";
import { useSessionStore } from "../../stores/session";

interface AdminItem {
  id: string;
  username: string;
  role: string;
  status: "active" | "disabled";
  permissions: string[];
  created_at: string;
  updated_at: string;
}

const sessionStore = useSessionStore();

const permissionOptions = [
  "review_submissions",
  "manage_announcements",
  "edit_resources",
  "delete_resources",
  "manage_tags",
  "review_reports",
  "manage_system",
];

const items = ref<AdminItem[]>([]);
const loading = ref(false);
const error = ref("");
const form = reactive({
  username: "",
  password: "",
  permissions: [] as string[],
});

onMounted(() => {
  if (sessionStore.isSuperAdmin) {
    void loadItems();
  }
});

async function loadItems() {
  loading.value = true;
  error.value = "";
  try {
    const response = await httpClient.get<{ items: AdminItem[] }>("/admin/admins");
    items.value = response.items ?? [];
  } catch {
    error.value = "加载管理员列表失败。";
  } finally {
    loading.value = false;
  }
}

async function createAdmin() {
  try {
    await httpClient.post("/admin/admins", form);
    form.username = "";
    form.password = "";
    form.permissions = [];
    await loadItems();
  } catch (err: unknown) {
    error.value = readApiError(err) ?? "创建管理员失败。";
  }
}

async function saveAdmin(item: AdminItem) {
  await httpClient.request(`/admin/admins/${item.id}`, {
    method: "PUT",
    body: {
      status: item.status,
      permissions: item.permissions,
    },
  });
  await loadItems();
}

async function resetPassword(item: AdminItem) {
  const password = window.prompt(`为 ${item.username} 输入新密码（至少 8 位）`);
  if (!password) {
    return;
  }
  await httpClient.post(`/admin/admins/${item.id}/reset-password`, {
    new_password: password,
  });
}

function togglePermission(item: AdminItem, permission: string) {
  if (item.permissions.includes(permission)) {
    item.permissions = item.permissions.filter((entry) => entry !== permission);
  } else {
    item.permissions = [...item.permissions, permission];
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
      <p class="text-sm font-semibold uppercase tracking-[0.22em] text-blue-300">Admins</p>
      <h2 class="mt-2 text-3xl font-semibold text-white">管理员管理</h2>
    </header>

    <p v-if="!sessionStore.isSuperAdmin" class="rounded-2xl bg-slate-950/70 px-4 py-3 text-sm text-slate-400">
      只有超级管理员可以管理管理员账号。
    </p>

    <template v-else>
      <article class="rounded-[28px] border border-slate-800 bg-slate-950/70 p-6">
        <h3 class="text-2xl font-semibold text-white">创建管理员</h3>
        <form class="mt-6 space-y-4" @submit.prevent="createAdmin">
          <input
            v-model="form.username"
            placeholder="用户名"
            class="w-full rounded-2xl border border-slate-700 bg-slate-900 px-4 py-3 text-sm text-white outline-none focus:border-blue-400"
          />
          <input
            v-model="form.password"
            type="password"
            placeholder="初始密码（至少 8 位）"
            class="w-full rounded-2xl border border-slate-700 bg-slate-900 px-4 py-3 text-sm text-white outline-none focus:border-blue-400"
          />
          <div class="flex flex-wrap gap-2">
            <label
              v-for="permission in permissionOptions"
              :key="permission"
              class="rounded-full border border-slate-700 px-3 py-2 text-xs text-slate-200"
            >
              <input v-model="form.permissions" :value="permission" type="checkbox" class="mr-2" />
              {{ permission }}
            </label>
          </div>
          <button type="submit" class="rounded-2xl bg-blue-500 px-5 py-3 text-sm font-semibold text-slate-950">
            创建
          </button>
        </form>
      </article>

      <p v-if="error" class="rounded-2xl bg-rose-950/60 px-4 py-3 text-sm text-rose-200">{{ error }}</p>

      <article class="rounded-[28px] border border-slate-800 bg-slate-950/70 p-6">
        <div class="flex items-center justify-between gap-4">
          <h3 class="text-2xl font-semibold text-white">管理员列表</h3>
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
                <h4 class="text-lg font-semibold text-white">{{ item.username }}</h4>
                <p class="mt-1 text-sm text-slate-500">{{ item.role }} / {{ item.status }}</p>
              </div>
              <div v-if="item.role !== 'super_admin'" class="flex gap-2">
                <button class="rounded-xl border border-slate-700 px-4 py-2 text-sm text-slate-200" @click="resetPassword(item)">
                  重置密码
                </button>
                <button class="rounded-xl bg-blue-500 px-4 py-2 text-sm font-semibold text-slate-950" @click="saveAdmin(item)">
                  保存
                </button>
              </div>
            </div>

            <div v-if="item.role !== 'super_admin'" class="mt-4 space-y-4">
              <select
                v-model="item.status"
                class="w-full rounded-2xl border border-slate-700 bg-slate-950 px-4 py-3 text-sm text-white outline-none focus:border-blue-400"
              >
                <option value="active">启用</option>
                <option value="disabled">停用</option>
              </select>
              <div class="flex flex-wrap gap-2">
                <button
                  v-for="permission in permissionOptions"
                  :key="permission"
                  type="button"
                  class="rounded-full border px-3 py-2 text-xs"
                  :class="item.permissions.includes(permission) ? 'border-blue-400 bg-blue-500 text-slate-950' : 'border-slate-700 text-slate-200'"
                  @click="togglePermission(item, permission)"
                >
                  {{ permission }}
                </button>
              </div>
            </div>
          </article>
        </div>
      </article>
    </template>
  </section>
</template>
