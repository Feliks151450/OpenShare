<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";

import PageHeader from "../../components/ui/PageHeader.vue";
import SurfaceCard from "../../components/ui/SurfaceCard.vue";
import { httpClient } from "../../lib/http/client";
import { readApiError } from "../../lib/http/helpers";
import type { FileTagDefinition } from "../../lib/publicFileTags";
import { readableTextColorForPreset } from "../../lib/publicFileTags";
import { useSessionStore } from "../../stores/session";

const sessionStore = useSessionStore();
const items = ref<FileTagDefinition[]>([]);
const loading = ref(false);
const error = ref("");
const message = ref("");

const createForm = reactive({
  name: "",
  color: "#64748b",
  sort_order: 0,
});
const creating = ref(false);

const rowDraft = reactive<Record<string, { name: string; color: string; sort_order: number }>>({});
const savingId = ref<string | null>(null);

const canAccess = () => sessionStore.hasPermission("resource_moderation");

onMounted(() => {
  if (canAccess()) {
    void load();
  }
});

async function load() {
  loading.value = true;
  error.value = "";
  message.value = "";
  try {
    const res = await httpClient.get<{ items: FileTagDefinition[] }>("/admin/file-tags");
    items.value = res.items ?? [];
    for (const row of items.value) {
      rowDraft[row.id] = {
        name: row.name,
        color: row.color,
        sort_order: row.sort_order,
      };
    }
  } catch (err: unknown) {
    error.value = readApiError(err, "加载标签失败。");
  } finally {
    loading.value = false;
  }
}

async function submitCreate() {
  if (!createForm.name.trim()) {
    error.value = "请输入标签名称。";
    return;
  }
  creating.value = true;
  error.value = "";
  message.value = "";
  try {
    await httpClient.post("/admin/file-tags", {
      name: createForm.name.trim(),
      color: createForm.color.trim(),
      sort_order: createForm.sort_order,
    });
    createForm.name = "";
    createForm.color = "#64748b";
    createForm.sort_order = 0;
    message.value = "已添加标签。";
    await load();
  } catch (err: unknown) {
    error.value = readApiError(err, "创建标签失败。");
  } finally {
    creating.value = false;
  }
}

async function saveRow(id: string) {
  const d = rowDraft[id];
  if (!d || !d.name.trim()) {
    error.value = "标签名称不能为空。";
    return;
  }
  savingId.value = id;
  error.value = "";
  message.value = "";
  try {
    await httpClient.request(`/admin/file-tags/${encodeURIComponent(id)}`, {
      method: "PATCH",
      body: {
        name: d.name.trim(),
        color: d.color.trim(),
        sort_order: d.sort_order,
      },
    });
    message.value = "已保存。";
    await load();
  } catch (err: unknown) {
    error.value = readApiError(err, "保存失败。");
  } finally {
    savingId.value = null;
  }
}

async function removeRow(id: string) {
  if (!window.confirm("确定删除该标签？所有文件上的此标签也会被移除。")) {
    return;
  }
  error.value = "";
  message.value = "";
  try {
    await httpClient.request(`/admin/file-tags/${encodeURIComponent(id)}`, {
      method: "DELETE",
    });
    message.value = "已删除。";
    delete rowDraft[id];
    await load();
  } catch (err: unknown) {
    error.value = readApiError(err, "删除失败。");
  }
}
</script>

<template>
  <!-- 文件标签管理页：管理员可创建、编辑、删除全站统一的预设标签，标签用于在资料详情页为文件分类 -->
  <div class="space-y-6">
    <PageHeader title="文件标签" description="全站统一的预设标签与颜色；访客详情页仅可选择此处配置的标签。" />

    <!-- 权限不足时的提示 -->
    <div v-if="!canAccess()" class="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900">
      当前账号没有资料管理权限，无法编辑标签。
    </div>

    <template v-else>
      <!-- 操作结果反馈消息 -->
      <p v-if="message" class="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-800">{{ message }}</p>
      <p v-if="error" class="rounded-xl border border-rose-200 bg-rose-50 px-4 py-3 text-sm text-rose-800">{{ error }}</p>

      <!-- 新建标签表单卡片 -->
      <SurfaceCard class="space-y-4 p-5">
        <h2 class="text-sm font-semibold text-slate-900">新建标签</h2>
        <div class="flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-end">
          <label class="min-w-[10rem] flex-1 space-y-1">
            <span class="text-xs font-medium text-slate-600">名称</span>
            <input v-model="createForm.name" class="field w-full" placeholder="例如：数据集" maxlength="64" />
          </label>
          <label class="w-32 space-y-1">
            <span class="text-xs font-medium text-slate-600">颜色</span>
            <input v-model="createForm.color" type="text" class="field w-full font-mono text-sm" placeholder="#2563eb" />
          </label>
          <label class="w-24 space-y-1">
            <span class="text-xs font-medium text-slate-600">排序</span>
            <input v-model.number="createForm.sort_order" type="number" class="field w-full" />
          </label>
          <button type="button" class="btn-primary h-10 shrink-0" :disabled="creating" @click="submitCreate">
            {{ creating ? "添加中…" : "添加" }}
          </button>
        </div>
        <p class="text-xs text-slate-500">颜色须为 #RGB 或 #RRGGBB。排序数字越小越靠前。</p>
      </SurfaceCard>

      <!-- 已有标签列表表格：每行可内联编辑名称/颜色/排序，并提供保存与删除操作 -->
      <SurfaceCard class="overflow-hidden">
        <div v-if="loading" class="p-8 text-center text-sm text-slate-500">加载中…</div>
        <div v-else-if="items.length === 0" class="p-8 text-center text-sm text-slate-500">暂无标签，请在上方添加。</div>
        <table v-else class="data-table table-fixed">
          <thead>
            <tr>
              <th class="text-left">预览</th>
              <th class="text-left">名称</th>
              <th class="w-[110px] text-left">颜色</th>
              <th class="w-[90px] text-left">排序</th>
              <th class="w-[150px] text-left">操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in items" :key="row.id">
              <!-- 标签预览：以当前编辑中的名称和颜色实时渲染标签外观 -->
              <td>
                <span
                  class="inline-flex rounded-xl px-3 py-3 text-s font-medium ring-1 ring-black/10"
                  :style="{
                    backgroundColor: rowDraft[row.id]?.color ?? row.color,
                    color: readableTextColorForPreset(rowDraft[row.id]?.color ?? row.color),
                  }"
                >{{ rowDraft[row.id]?.name ?? row.name }}</span>
              </td>
              <td>
                <input
                  v-if="rowDraft[row.id]"
                  v-model="rowDraft[row.id].name"
                  class="field w-full py-1.5 text-sm px-3"
                  maxlength="64"
                />
              </td>
              <td>
                <input
                  v-if="rowDraft[row.id]"
                  v-model="rowDraft[row.id].color"
                  class="field w-full py-1.5 font-mono text-xs px-3"
                />
              </td>
              <td class="text-left">
                <input
                  v-if="rowDraft[row.id]"
                  v-model.number="rowDraft[row.id].sort_order"
                  type="number"
                  class="field w-full py-1.5 text-sm text-left tabular-nums pl-3 pr-1"
                />
              </td>
              <!-- 行操作：保存修改 / 删除标签 -->
              <td class="text-left">
                <button
                  type="button"
                  class="btn-secondary mr-0.5 py-3 text-xs rounded-xl"
                  :disabled="savingId === row.id"
                  @click="saveRow(row.id)"
                >
                  {{ savingId === row.id ? "保存中" : "保存" }}
                </button>
                <button type="button" class="btn-secondary py-3 text-xs text-rose-700 rounded-xl" @click="removeRow(row.id)">
                  删除
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </SurfaceCard>
    </template>
  </div>
</template>
