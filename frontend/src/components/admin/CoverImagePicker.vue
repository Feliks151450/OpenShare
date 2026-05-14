<script setup lang="ts">
/**
 * 封面图片选择器：支持手动输入 URL 或拖拽/选择图片自动上传到站内封面存储目录。
 * 确认后通过 emit 将站内链接（如 /files/<uuid>）或外部 URL 返回给父组件。
 */
import { ref, watch } from "vue";
import { httpClient } from "../../lib/http/client";
import { readApiError } from "../../lib/http/helpers";
import { resolveMarkdownImageUrlToHref } from "../../lib/markdown";

const props = defineProps<{
  modelValue: string;  // 当前封面 URL
  open: boolean;       // 弹窗是否打开
}>();

const emit = defineEmits<{
  (e: "update:open", value: boolean): void;
  (e: "confirm", url: string): void;
}>();

const urlDraft = ref("");
const dragOver = ref(false);
const uploadPending = ref(false);
const uploadError = ref("");
const previewUrl = ref("");
const selectedFile = ref<File | null>(null);

// 弹窗打开时用当前值初始化草稿
watch(
  () => [props.open, props.modelValue] as const,
  ([open, val]) => {
    if (open) {
      urlDraft.value = val ?? "";
      uploadError.value = "";
      previewUrl.value = "";
      selectedFile.value = null;
      // 如果已有值，解析为可显示的 URL 后预览
      if (val && (val.startsWith("/files/") || val.startsWith("http"))) {
        previewUrl.value = resolveMarkdownImageUrlToHref(val);
      }
    }
  },
  { immediate: true },
);

function close() {
  emit("update:open", false);
}

function confirm() {
  emit("confirm", urlDraft.value.trim());
  close();
}

function handleDragOver(e: DragEvent) {
  e.preventDefault();
  dragOver.value = true;
}

function handleDragLeave() {
  dragOver.value = false;
}

function handleDrop(e: DragEvent) {
  e.preventDefault();
  dragOver.value = false;
  const file = e.dataTransfer?.files?.[0];
  if (file) {
    handleFile(file);
  }
}

function handleFileInput(e: Event) {
  const target = e.target as HTMLInputElement;
  const file = target.files?.[0];
  if (file) {
    handleFile(file);
  }
}

function handleFile(file: File) {
  uploadError.value = "";
  // 仅允许图片格式
  const allowed = ["image/png", "image/jpeg", "image/gif", "image/webp", "image/svg+xml", "image/bmp"];
  if (!allowed.includes(file.type) && !file.name.match(/\.(png|jpe?g|jfif|gif|webp|svg|bmp)$/i)) {
    uploadError.value = "仅支持 PNG、JPG、GIF、WebP、SVG、BMP 格式的图片";
    return;
  }
  if (file.size > 10 * 1024 * 1024) {
    uploadError.value = "图片大小不能超过 10 MB";
    return;
  }
  selectedFile.value = file;
  previewUrl.value = URL.createObjectURL(file);
}

async function uploadToServer() {
  if (!selectedFile.value) return;
  uploadPending.value = true;
  uploadError.value = "";
  try {
    const formData = new FormData();
    formData.append("file", selectedFile.value);
    const resp = await httpClient.request<{ url: string }>("/admin/resources/upload-cover", {
      method: "POST",
      body: formData,
    });
    // 将站内链接填入输入框，并解析为可显示 URL 用于预览
    const url = resp.url ?? "";
    urlDraft.value = url;
    previewUrl.value = resolveMarkdownImageUrlToHref(url);
    selectedFile.value = null;
  } catch (err: unknown) {
    uploadError.value = readApiError(err, "上传封面失败，请检查封面存储目录是否已配置");
  } finally {
    uploadPending.value = false;
  }
}

// 判断当前草稿值是否为站内链接
const isInternalUrl = () => urlDraft.value.trim().startsWith("/files/");
</script>

<template>
  <div
    v-if="open"
    class="fixed inset-0 z-[100] flex items-center justify-center bg-black/30 backdrop-blur-sm"
    @click.self="close"
    @keydown.escape="close"
  >
    <div class="modal-card mx-4 w-full max-w-lg rounded-2xl bg-white p-6 shadow-2xl dark:bg-slate-800">
      <h3 class="text-lg font-semibold text-slate-900 dark:text-slate-100">设置封面图片</h3>

      <!-- 手动输入 URL -->
      <label class="mt-5 block space-y-2">
        <span class="text-sm font-medium text-slate-700 dark:text-slate-300">封面图地址</span>
        <input
          v-model="urlDraft"
          type="text"
          class="field w-full"
          placeholder="https://cdn.example.com/cover.jpg 或 /files/<uuid>"
          autocomplete="off"
        />
        <p class="text-xs text-slate-500 dark:text-slate-400">
          {{ isInternalUrl() ? "站内链接，无需额外处理" : "可填写外部直链或通过下方拖拽上传自动生成站内链接" }}
        </p>
      </label>

      <!-- 拖拽上传区域 -->
      <div
        class="mt-4 rounded-xl border-2 border-dashed p-6 text-center transition-colors"
        :class="
          dragOver
            ? 'border-indigo-400 bg-indigo-50 dark:border-indigo-500 dark:bg-indigo-900/20'
            : 'border-slate-300 bg-slate-50/50 dark:border-slate-600 dark:bg-slate-700/30'
        "
        @dragover="handleDragOver"
        @dragleave="handleDragLeave"
        @drop="handleDrop"
      >
        <div v-if="!previewUrl" class="space-y-3">
          <p class="text-sm text-slate-500 dark:text-slate-400">
            拖拽图片到此处，或点击下方按钮选择文件
          </p>
          <label class="btn-secondary inline-block cursor-pointer">
            选择图片文件
            <input type="file" accept="image/*" class="hidden" @change="handleFileInput" />
          </label>
        </div>
        <div v-else class="space-y-3">
          <img
            :src="previewUrl"
            alt="封面预览"
            class="mx-auto max-h-48 max-w-full rounded-lg object-contain shadow"
          />
          <div class="flex items-center justify-center gap-3">
            <label class="btn-secondary cursor-pointer text-sm">
              重新选择
              <input type="file" accept="image/*" class="hidden" @change="handleFileInput" />
            </label>
            <button
              v-if="selectedFile"
              type="button"
              class="btn-primary text-sm"
              :disabled="uploadPending"
              @click="uploadToServer"
            >
              {{ uploadPending ? "上传中…" : "转为站内链接" }}
            </button>
          </div>
        </div>
      </div>

      <p v-if="uploadError" class="mt-3 text-sm text-red-600 dark:text-red-400">{{ uploadError }}</p>

      <!-- 操作按钮 -->
      <div class="mt-6 flex items-center justify-end gap-3">
        <button type="button" class="btn-secondary" @click="close">取消</button>
        <button
          type="button"
          class="btn-primary"
          :disabled="!urlDraft.trim()"
          @click="confirm"
        >
          确认
        </button>
      </div>
    </div>
  </div>
</template>
