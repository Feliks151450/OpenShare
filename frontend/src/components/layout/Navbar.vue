<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue";
import { Check, PanelLeftClose, PanelLeftOpen, UserRound } from "lucide-vue-next";
import { useRouter } from "vue-router";

import { useSidebar } from "../../composables/useSidebar";

import { HttpError, httpClient } from "../../lib/http/client";
import { useSessionStore } from "../../stores/session";

export interface NavbarItem {
  label: string;
  to: string;
  action?: () => void;
}

interface AdminMeResponse {
  admin: {
    id: string;
    username: string;
    display_name: string;
    avatar_url: string;
    role: string;
    status: string;
    permissions: string[];
  };
}

interface AdminDashboardStatsResponse {
  pending_audit_count: number;
}

const props = withDefaults(
  defineProps<{
    items?: NavbarItem[];
    currentPath?: string;
  }>(),
  {
    items: () => [
      { label: "首页", to: "/" },
      { label: "回执查询", to: "/upload" },
    ],
    currentPath: "/",
  },
);

const router = useRouter();
const sessionStore = useSessionStore();
const { expanded: sidebarExpanded, toggle: toggleSidebar } = useSidebar();
const panelRef = ref<HTMLElement | null>(null);
const panelOpen = ref(false);
const username = ref("");
const password = ref("");
const loginLoading = ref(false);
const loginSuccess = ref(false);
const loginError = ref("");

const userButtonLabel = computed(() => {
  if (sessionStore.authenticated) {
    return sessionStore.displayName.slice(0, 1).toUpperCase() || "A";
  }
  return "";
});

onMounted(async () => {
  document.addEventListener("pointerdown", onPointerDown);

  try {
    const response = await httpClient.get<AdminMeResponse>("/admin/me");
    applySession(response);
    await loadPendingAuditCount();
  } catch {
    sessionStore.reset();
    sessionStore.setPendingAuditCount(0);
  }
});

onUnmounted(() => {
  document.removeEventListener("pointerdown", onPointerDown);
});

function isActive(path: string) {
  return props.currentPath === path;
}

async function onUserAction() {
  if (sessionStore.authenticated) {
    await router.push("/admin");
    return;
  }

  panelOpen.value = !panelOpen.value;
  loginError.value = "";
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
    await loadPendingAuditCount();
    password.value = "";
    loginSuccess.value = true;

    window.setTimeout(() => {
      loginSuccess.value = false;
      panelOpen.value = false;
    }, 1100);
  } catch (error: unknown) {
    loginError.value = readApiError(error) ?? "登录失败，请检查账号或密码。";
  } finally {
    loginLoading.value = false;
  }
}

function applySession(response: AdminMeResponse) {
  sessionStore.setAuthenticated(true, response.admin.display_name || response.admin.username, {
    username: response.admin.username,
    adminId: response.admin.id,
    avatarUrl: response.admin.avatar_url,
    role: response.admin.role,
    status: response.admin.status,
    permissions: response.admin.permissions,
  });
}

async function loadPendingAuditCount() {
  try {
    const response = await httpClient.get<AdminDashboardStatsResponse>("/admin/dashboard/stats");
    sessionStore.setPendingAuditCount(response.pending_audit_count ?? 0);
  } catch {
    sessionStore.setPendingAuditCount(0);
  }
}

function readApiError(error: unknown) {
  if (!(error instanceof HttpError) || typeof error.payload !== "object" || error.payload === null) {
    return null;
  }

  const payload = error.payload as Record<string, unknown>;
  return typeof payload.error === "string" ? payload.error : null;
}

function onPointerDown(event: PointerEvent) {
  if (!panelOpen.value || !panelRef.value) {
    return;
  }

  const target = event.target;
  if (target instanceof Node && !panelRef.value.contains(target)) {
    panelOpen.value = false;
    loginError.value = "";
  }
}
</script>

<template>
  <header class="fixed inset-x-0 top-0 z-[60] border-b border-slate-200 bg-white/95 backdrop-blur dark:border-slate-800 dark:bg-slate-950/95">
    <div
      class="mx-auto flex h-16 w-full max-w-[1360px] items-center justify-between gap-3 px-3 sm:px-4 md:gap-4 md:px-6 lg:px-8 xl:max-w-[2150px]"
    >
      <button
        type="button"
        class="hidden h-8 w-8 shrink-0 items-center justify-center rounded-lg text-slate-500 transition hover:bg-slate-100 hover:text-slate-700 xl:inline-flex dark:text-slate-400 dark:hover:bg-slate-800 dark:hover:text-slate-200"
        :title="sidebarExpanded ? '收起侧栏' : '展开侧栏'"
        @click="toggleSidebar"
      >
        <PanelLeftClose v-if="sidebarExpanded" class="h-4 w-4" />
        <PanelLeftOpen v-else class="h-4 w-4" />
      </button>
      <nav class="flex min-w-0 flex-1 items-center justify-start gap-1 overflow-x-auto">
        <template v-for="item in items" :key="item.to">
          <button
            v-if="item.action"
            type="button"
            class="shrink-0 rounded-lg px-2.5 py-2 text-sm font-medium transition sm:px-4 text-slate-600 hover:bg-slate-200/60 hover:text-slate-900 dark:text-slate-400 dark:hover:bg-slate-900 dark:hover:text-slate-100"
            @click="item.action"
          >
            {{ item.label }}
          </button>
          <RouterLink
            v-else
            :to="item.to"
            class="shrink-0 rounded-lg px-2.5 py-2 text-sm font-medium transition sm:px-4"
            :class="
              isActive(item.to)
                ? 'bg-slate-200/70 text-slate-900 dark:bg-slate-800 dark:text-slate-100'
                : 'text-slate-600 hover:bg-slate-200/60 hover:text-slate-900 dark:text-slate-400 dark:hover:bg-slate-900 dark:hover:text-slate-100'
            "
          >
            {{ item.label }}
          </RouterLink>
        </template>
      </nav>

      <div ref="panelRef" class="relative flex shrink-0 items-center justify-end gap-2 leading-none">
        <div class="relative h-9 w-9 shrink-0">
          <button
            type="button"
            aria-label="管理员入口"
            class="absolute inset-0 m-0 inline-flex appearance-none items-center justify-center overflow-hidden rounded-full bg-white p-0 text-slate-600 ring-1 ring-inset ring-slate-200 transition hover:bg-slate-100 hover:text-slate-900 dark:bg-slate-950 dark:text-slate-300 dark:ring-slate-800 dark:hover:bg-slate-900 dark:hover:text-slate-100"
            @click="onUserAction"
          >
            <img
              v-if="sessionStore.authenticated && sessionStore.avatarUrl"
              :src="sessionStore.avatarUrl"
              alt="管理员头像"
              class="absolute inset-0 block h-full w-full rounded-full object-cover object-center"
            />
            <span
              v-else-if="sessionStore.authenticated && userButtonLabel"
              class="relative z-[1] text-xs font-semibold leading-none"
            >
              {{ userButtonLabel }}
            </span>
            <UserRound v-else class="relative z-[1] h-[18px] w-[18px]" />
          </button>
          <span
            v-if="sessionStore.authenticated && sessionStore.pendingAuditCount > 0"
            class="absolute right-[-1px] top-[-1px] h-2.5 w-2.5 rounded-full bg-rose-500 ring-2 ring-white"
          />
        </div>

        <section
          v-if="panelOpen"
          class="absolute right-0 top-[calc(100%+12px)] z-20 w-[min(320px,calc(100vw-32px))] rounded-xl border border-slate-200 bg-white p-4 shadow-sm shadow-slate-950/[0.06] dark:border-slate-800 dark:bg-slate-950 dark:shadow-none"
        >
            <div v-if="loginSuccess" class="flex min-h-[184px] flex-col items-center justify-center gap-3 text-center">
              <div class="flex h-12 w-12 items-center justify-center rounded-full bg-slate-900 text-white dark:bg-slate-100 dark:text-slate-900">
                <Check class="h-5 w-5 animate-pulse" />
              </div>
              <div>
                <p class="text-sm font-semibold text-slate-900 dark:text-slate-100">登录成功</p>
                <p class="mt-1 text-sm text-slate-500 dark:text-slate-400">再次点击右上角头像进入管理后台。</p>
              </div>
            </div>

            <template v-else>
              <div class="space-y-1">
                <p class="text-sm font-semibold text-slate-900 dark:text-slate-100">管理员登录</p>
                <p class="text-sm text-slate-500 dark:text-slate-400">输入标示ID和密码进入 OpenShare 后台。</p>
              </div>

              <form class="mt-4 space-y-3" @submit.prevent="login">
                <input v-model="username" class="field h-10" placeholder="标示ID" autocomplete="username" />
                <input
                  v-model="password"
                  type="password"
                  class="field h-10"
                  placeholder="密码"
                  autocomplete="current-password"
                />
                <button type="submit" class="btn-primary h-10 w-full" :disabled="loginLoading">
                  {{ loginLoading ? "登录中…" : "登录后台" }}
                </button>
              </form>

              <p
                v-if="loginError"
                class="mt-3 rounded-lg border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700 dark:border-rose-900 dark:bg-rose-950/50 dark:text-rose-300"
              >
                {{ loginError }}
              </p>
            </template>
        </section>
      </div>
    </div>
  </header>
</template>
