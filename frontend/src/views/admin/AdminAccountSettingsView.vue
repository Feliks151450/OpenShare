<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";

import PageHeader from "../../components/ui/PageHeader.vue";
import SurfaceCard from "../../components/ui/SurfaceCard.vue";
import { httpClient } from "../../lib/http/client";
import { readApiError } from "../../lib/http/helpers";
import { toastError, toastSuccess } from "../../lib/toast";
import { useSessionStore } from "../../stores/session";

interface AdminProfileResponse {
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

const sessionStore = useSessionStore();
const profileSaving = ref(false);
const passwordSaving = ref(false);
const error = ref("");
const success = ref("");

const profileForm = reactive({
  displayName: "",
  avatarUrl: "",
});

const avatarUrlInput = ref("");

const passwordForm = reactive({
  newPassword: "",
  confirmPassword: "",
});

const passwordDirty = computed(() => {
  return (
    passwordForm.newPassword !== "" ||
    passwordForm.confirmPassword !== ""
  );
});

const profileDirty = computed(() => {
  return (
    profileForm.displayName.trim() !== sessionStore.displayName.trim() ||
    profileForm.avatarUrl.trim() !== sessionStore.avatarUrl.trim()
  );
});

const passwordValid = computed(() => {
  return (
    passwordForm.newPassword.length >= 8 &&
    passwordForm.confirmPassword !== "" &&
    passwordForm.newPassword === passwordForm.confirmPassword
  );
});

onMounted(() => {
  resetProfileForm();
  loadApiTokens();
});

function resetProfileForm() {
  profileForm.displayName = sessionStore.displayName;
  profileForm.avatarUrl = sessionStore.avatarUrl;
}

async function onAvatarSelected(event: Event) {
  const input = event.target as HTMLInputElement;
  const file = input.files?.[0];
  if (!file) return;
  if (!file.type.startsWith("image/")) {
    toastError("头像必须是图片文件。");
    return;
  }

  const dataUrl = await readFileAsDataURL(file);
  profileForm.avatarUrl = dataUrl;
}

function clearAvatar() {
  profileForm.avatarUrl = "";
  avatarUrlInput.value = "";
}

function applyAvatarUrl() {
  const url = avatarUrlInput.value.trim();
  if (!url) return;
  if (!/^https?:\/\//.test(url)) {
    toastError("图片直链需以 https:// 或 http:// 开头。");
    return;
  }
  profileForm.avatarUrl = url;
}

async function saveProfile() {
  profileSaving.value = true;
  error.value = "";
  success.value = "";
  try {
    const response = await httpClient.request<AdminProfileResponse>("/admin/account/profile", {
      method: "PATCH",
      body: {
        display_name: profileForm.displayName,
        avatar_url: profileForm.avatarUrl,
      },
    });
    applySessionProfile(response.admin);
    resetProfileForm();
    toastSuccess("账号资料已更新。");
  } catch (err: unknown) {
    toastError(readApiError(err, "更新账号资料失败。"));
  } finally {
    profileSaving.value = false;
  }
}

async function changePassword() {
  if (!passwordValid.value) {
    toastError(passwordForm.newPassword !== passwordForm.confirmPassword ? "两次输入的新密码不一致。" : "请填写完整且有效的新密码。");
    success.value = "";
    return;
  }

  passwordSaving.value = true;
  error.value = "";
  success.value = "";
  try {
    await httpClient.post("/admin/session/change-password", {
      new_password: passwordForm.newPassword,
    });
    passwordForm.newPassword = "";
    passwordForm.confirmPassword = "";
    toastSuccess("密码已更新。");
  } catch (err: unknown) {
    toastError(readApiError(err, "修改密码失败。"));
  } finally {
    passwordSaving.value = false;
  }
}

// ── API Token 管理 ──
interface ApiTokenItem {
  id: string;
  name: string;
  last_used_at: string | null;
  created_at: string;
}
const apiTokens = ref<ApiTokenItem[]>([]);
const newTokenName = ref("");
const newTokenValue = ref("");
const tokenCreating = ref(false);
const tokenDeleting = ref<string | null>(null);

async function loadApiTokens() {
  try {
    const resp = await httpClient.get<{ items: ApiTokenItem[] }>("/admin/api-tokens");
    apiTokens.value = resp.items;
  } catch { /* ignore */ }
}

async function createApiToken() {
  const name = newTokenName.value.trim();
  if (!name || tokenCreating.value) return;
  tokenCreating.value = true;
  try {
    const resp = await httpClient.post<{ token: string; id: string; name: string }>("/admin/api-tokens", { name });
    newTokenValue.value = resp.token;
    newTokenName.value = "";
    await loadApiTokens();
  } catch {
    toastError("创建 API Token 失败。");
  } finally {
    tokenCreating.value = false;
  }
}

async function deleteApiToken(id: string) {
  tokenDeleting.value = id;
  try {
    await httpClient.request(`/admin/api-tokens/${encodeURIComponent(id)}`, { method: "DELETE" });
    apiTokens.value = apiTokens.value.filter((t) => t.id !== id);
  } catch {
    toastError("删除 API Token 失败。");
  } finally {
    tokenDeleting.value = null;
  }
}

async function copyNewToken() {
  if (!newTokenValue.value) return;
  try {
    await navigator.clipboard.writeText(newTokenValue.value);
    toastSuccess("API Token 已复制到剪贴板。");
  } catch {
    toastError("复制失败，请手动选择复制。");
  }
}

function formatDate(iso: string | null): string {
  if (!iso) return "";
  return new Date(iso).toLocaleString();
}

function applySessionProfile(admin: AdminProfileResponse["admin"]) {
  sessionStore.setAuthenticated(true, admin.display_name || admin.username, {
    username: admin.username,
    adminId: admin.id,
    avatarUrl: admin.avatar_url,
    role: admin.role,
    status: admin.status,
    permissions: admin.permissions,
  });
}

function readFileAsDataURL(file: File) {
  return new Promise<string>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result ?? ""));
    reader.onerror = () => reject(new Error("file read failed"));
    reader.readAsDataURL(file);
  });
}
</script>

<template>
  <!-- 管理员账号设置页：修改个人资料（头像、显示名）和密码 -->
  <section class="space-y-8">
    <PageHeader
      eyebrow="Account"
      title="账号设置"
    />

    <!-- 双栏布局：左侧基本资料，右侧修改密码 -->
    <section class="grid gap-6 xl:grid-cols-[minmax(0,1fr)_minmax(0,420px)]">
      <!-- 基本资料卡片：头像预览/上传/移除 + 标示ID（只读）+ 显示名编辑 -->
      <SurfaceCard>
        <div>
          <h2 class="text-lg font-semibold text-slate-900">基本资料</h2>
        </div>

        <!-- 头像区域：圆形预览 + 上传/URL/移除 -->
        <div class="mt-6 flex items-start gap-4">
          <div class="flex h-24 w-24 shrink-0 items-center justify-center overflow-hidden rounded-full border border-slate-200 bg-slate-100 text-3xl font-semibold text-slate-700">
            <img v-if="profileForm.avatarUrl" :src="profileForm.avatarUrl" alt="头像预览" class="h-full w-full object-cover" />
            <!-- 无头像时显示用户名首字母 -->
            <span v-else>{{ sessionStore.displayName.slice(0, 1).toUpperCase() || "A" }}</span>
          </div>
          <div class="flex flex-col gap-3">
            <label class="inline-flex h-10 cursor-pointer items-center rounded-xl border border-slate-200 px-4 text-sm font-medium text-slate-600 transition hover:bg-slate-50 hover:text-slate-900">
              <span>本地上传</span>
              <input type="file" accept="image/*" class="hidden" @change="onAvatarSelected" />
            </label>
            <div class="flex items-center gap-2">
              <input
                v-model="avatarUrlInput"
                type="url"
                class="field h-10 w-64 text-sm"
                placeholder="或粘贴图片直链 https://..."
                @blur="applyAvatarUrl"
              />
              <button
                type="button"
                class="inline-flex h-10 shrink-0 items-center rounded-xl border border-slate-200 px-3 text-sm font-medium text-slate-600 transition hover:bg-slate-50 hover:text-slate-900"
                @click="applyAvatarUrl"
              >
                使用
              </button>
            </div>
            <button
              type="button"
              class="inline-flex h-10 w-fit items-center rounded-xl border border-slate-200 px-4 text-sm font-medium text-slate-600 transition hover:bg-slate-50 hover:text-slate-900"
              @click="clearAvatar"
            >
              移除头像
            </button>
          </div>
        </div>

        <!-- 表单字段：标示ID（只读）、对外展示名 -->
        <div class="mt-6 grid gap-4">
          <div class="space-y-2">
            <label class="text-sm font-medium text-slate-700">标示ID</label>
            <input :value="sessionStore.username" class="field bg-slate-50" disabled />
          </div>
          <div class="space-y-2">
            <label class="text-sm font-medium text-slate-700">用户名（对外展示）</label>
            <input v-model="profileForm.displayName" class="field" placeholder="请输入用户名（对外展示）" />
          </div>
        </div>

        <div class="mt-6 flex gap-3">
          <button type="button" class="btn-primary" :disabled="profileSaving || !profileDirty" @click="saveProfile">
            {{ profileSaving ? "更新中…" : "确认更新" }}
          </button>
        </div>
      </SurfaceCard>

      <!-- API Token 管理卡片 -->
      <SurfaceCard>
        <div>
          <h2 class="text-lg font-semibold text-slate-900">API Token</h2>
          <p class="mt-1 text-sm text-slate-500">创建 Token 后通过 <code class="rounded bg-slate-100 px-1 text-xs">Authorization: Bearer &lt;token&gt;</code> 头调用管理端接口。Token 仅创建时显示一次，持久有效直至手动删除。</p>
        </div>
        <div class="mt-4 space-y-3">
          <!-- 创建表单 -->
          <form class="flex items-end gap-2" @submit.prevent="createApiToken">
            <div class="flex-1 space-y-1">
              <label class="text-xs font-medium text-slate-600">Token 名称</label>
              <input v-model="newTokenName" class="field" placeholder="例如：CLI 脚本、CI 部署" />
            </div>
            <button type="submit" class="btn-primary shrink-0" :disabled="tokenCreating || !newTokenName.trim()">
              {{ tokenCreating ? "创建中…" : "创建" }}
            </button>
          </form>
          <!-- 新创建的 Token 展示（仅一次） -->
          <div v-if="newTokenValue" class="rounded-xl border border-emerald-200 bg-emerald-50 p-3">
            <p class="text-xs font-medium text-emerald-800">Token 已创建，请立即复制保存（关闭后无法再次查看）：</p>
            <div class="mt-2 flex items-center gap-2">
              <input :value="newTokenValue" class="field flex-1 font-mono text-xs" readonly @focus="($event.target as HTMLInputElement).select()" />
              <button type="button" class="btn-secondary shrink-0" @click="copyNewToken">复制</button>
            </div>
          </div>
          <!-- 已有 Token 列表 -->
          <div v-if="apiTokens.length > 0" class="space-y-1">
            <div v-for="t in apiTokens" :key="t.id" class="flex items-center justify-between gap-2 rounded-lg border border-slate-200 px-3 py-2">
              <div class="min-w-0 flex-1">
                <p class="truncate text-sm font-medium text-slate-700">{{ t.name }}</p>
                <p class="text-xs text-slate-400">
                  创建于 {{ formatDate(t.created_at) }}
                  <template v-if="t.last_used_at"> · 最近使用 {{ formatDate(t.last_used_at) }}</template>
                  <template v-else> · 尚未使用</template>
                </p>
              </div>
              <button type="button" class="btn-secondary shrink-0 text-xs text-rose-600" :disabled="tokenDeleting === t.id" @click="deleteApiToken(t.id)">
                {{ tokenDeleting === t.id ? "…" : "删除" }}
              </button>
            </div>
          </div>
          <p v-else-if="!newTokenValue" class="text-sm text-slate-400">暂无 API Token，请创建。</p>
        </div>
      </SurfaceCard>

      <!-- 修改密码卡片 -->
      <SurfaceCard>
        <div>
          <h2 class="text-lg font-semibold text-slate-900">修改密码</h2>
          <p class="mt-1 text-sm text-slate-500">新密码至少 8 位。修改后立即对当前账号生效。</p>
        </div>

        <form class="mt-6 space-y-4" @submit.prevent="changePassword">
          <div class="space-y-2">
            <label class="text-sm font-medium text-slate-700">新密码</label>
            <input v-model="passwordForm.newPassword" type="password" class="field" placeholder="至少 8 位" />
          </div>
          <div class="space-y-2">
            <label class="text-sm font-medium text-slate-700">确认新密码</label>
            <input v-model="passwordForm.confirmPassword" type="password" class="field" placeholder="再次输入新密码" />
          </div>
          <button type="submit" class="btn-primary" :disabled="passwordSaving || !passwordValid">
            {{ passwordSaving ? "更新中…" : "确认更新" }}
          </button>
        </form>

        <!-- 密码校验提示 -->
        <p v-if="passwordDirty && !passwordValid" class="mt-4 text-sm text-slate-500">
          新密码至少 8 位，且两次输入保持一致。
        </p>
      </SurfaceCard>
    </section>

    <!-- 操作结果反馈 -->
</section>
</template>
