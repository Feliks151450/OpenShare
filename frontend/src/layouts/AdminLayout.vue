<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { RouterLink, RouterView, useRoute } from "vue-router";

import { HttpError, httpClient } from "../lib/http/client";
import { useSessionStore } from "../stores/session";

interface AdminMeResponse {
  admin: {
    id: string;
    username: string;
    role: string;
    status: string;
    permissions: string[];
  };
}

const sessionStore = useSessionStore();
const route = useRoute();

const username = ref("superadmin");
const password = ref("");
const loading = ref(true);
const loginLoading = ref(false);
const loginError = ref("");

const navItems = computed(() => [
  { to: "/admin", label: "控制台", visible: true },
  { to: "/admin/announcements", label: "公告", visible: sessionStore.hasPermission("manage_announcements") },
  { to: "/admin/resources", label: "资料", visible: true },
  { to: "/admin/reports", label: "举报", visible: sessionStore.hasPermission("review_reports") },
  { to: "/admin/admins", label: "管理员", visible: sessionStore.isSuperAdmin },
  { to: "/admin/settings", label: "系统设置", visible: sessionStore.isSuperAdmin },
]);

onMounted(async () => {
  await restoreSession();
});

async function restoreSession() {
  loading.value = true;
  try {
    const response = await httpClient.get<AdminMeResponse>("/admin/me");
    applySession(response);
  } catch {
    sessionStore.reset();
  } finally {
    loading.value = false;
  }
}

async function login() {
  loginLoading.value = true;
  loginError.value = "";

  try {
    const response = await httpClient.post<AdminMeResponse>("/admin/session/login", {
      username: username.value,
      password: password.value,
    });
    applySession(response);
    password.value = "";
  } catch (error: unknown) {
    loginError.value = readApiError(error) ?? "登录失败，请重试。";
  } finally {
    loginLoading.value = false;
  }
}

async function logout() {
  await httpClient.post("/admin/session/logout");
  sessionStore.reset();
}

function applySession(response: AdminMeResponse) {
  sessionStore.setAuthenticated(true, response.admin.username, {
    adminId: response.admin.id,
    role: response.admin.role,
    status: response.admin.status,
    permissions: response.admin.permissions,
  });
}

function isActive(path: string) {
  return route.path === path || route.path.startsWith(`${path}/`);
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
  <div class="min-h-screen bg-slate-950 px-4 py-6 text-slate-100 sm:px-6 lg:px-8">
    <div class="mx-auto max-w-7xl">
      <div v-if="loading" class="rounded-[32px] border border-slate-800 bg-slate-900 px-8 py-12 text-sm text-slate-300">
        正在恢复管理会话...
      </div>

      <div
        v-else-if="!sessionStore.authenticated"
        class="mx-auto max-w-xl rounded-[32px] border border-slate-800 bg-slate-900 px-8 py-10 shadow-panel"
      >
        <p class="text-xs font-semibold uppercase tracking-[0.3em] text-blue-300">OpenShare Admin</p>
        <h1 class="mt-3 text-3xl font-semibold text-white">管理后台登录</h1>
        <p class="mt-3 text-sm text-slate-400">阶段八开始需要稳定的后台运营入口，先登录后再进入具体页面。</p>

        <form class="mt-8 space-y-4" @submit.prevent="login">
          <label class="block">
            <span class="mb-2 block text-sm font-medium text-slate-300">用户名</span>
            <input
              v-model="username"
              class="w-full rounded-2xl border border-slate-700 bg-slate-950 px-4 py-3 text-sm text-white outline-none focus:border-blue-400"
            />
          </label>
          <label class="block">
            <span class="mb-2 block text-sm font-medium text-slate-300">密码</span>
            <input
              v-model="password"
              type="password"
              class="w-full rounded-2xl border border-slate-700 bg-slate-950 px-4 py-3 text-sm text-white outline-none focus:border-blue-400"
            />
          </label>
          <button
            type="submit"
            class="rounded-2xl bg-blue-500 px-5 py-3 text-sm font-semibold text-slate-950 transition hover:bg-blue-400 disabled:cursor-not-allowed disabled:bg-slate-600"
            :disabled="loginLoading"
          >
            {{ loginLoading ? "登录中..." : "登录" }}
          </button>
        </form>

        <p v-if="loginError" class="mt-4 rounded-2xl bg-rose-950/60 px-4 py-3 text-sm text-rose-200">
          {{ loginError }}
        </p>
      </div>

      <div
        v-else
        class="grid min-h-[calc(100vh-3rem)] overflow-hidden rounded-[32px] border border-slate-800 bg-slate-900 shadow-panel lg:grid-cols-[260px_1fr]"
      >
        <aside class="border-b border-slate-800 px-6 py-6 lg:border-b-0 lg:border-r">
          <p class="text-xs font-semibold uppercase tracking-[0.3em] text-blue-300">OpenShare Admin</p>
          <h1 class="mt-2 text-2xl font-semibold text-white">管理后台</h1>
          <p class="mt-3 text-sm text-slate-400">{{ sessionStore.displayName }} · {{ sessionStore.role }}</p>

          <nav class="mt-8 flex flex-col gap-2 text-sm">
            <RouterLink
              v-for="item in navItems.filter((entry) => entry.visible)"
              :key="item.to"
              :to="item.to"
              class="rounded-2xl px-4 py-3 transition"
              :class="isActive(item.to) ? 'bg-blue-500 text-slate-950' : 'text-slate-300 hover:bg-slate-800 hover:text-white'"
            >
              {{ item.label }}
            </RouterLink>
            <RouterLink class="rounded-2xl px-4 py-3 text-slate-300 transition hover:bg-slate-800 hover:text-white" to="/">
              返回用户端
            </RouterLink>
          </nav>

          <button
            class="mt-8 rounded-2xl border border-slate-700 px-4 py-3 text-sm font-medium text-slate-200 transition hover:bg-slate-800"
            @click="logout"
          >
            退出登录
          </button>
        </aside>

        <main class="px-6 py-8">
          <RouterView />
        </main>
      </div>
    </div>
  </div>
</template>
