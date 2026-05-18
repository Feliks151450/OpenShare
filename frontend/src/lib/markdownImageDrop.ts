import { ref, type Ref } from "vue";
import { httpClient, HttpError } from "./http/client";
import { toastError } from "./toast";

const imageExtensions = new Set([
  ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".bmp",
]);

function isImageFile(file: File): boolean {
  if (file.type.startsWith("image/")) return true;
  const name = file.name.toLowerCase();
  for (const ext of imageExtensions) {
    if (name.endsWith(ext)) return true;
  }
  return false;
}

export interface MarkdownImageDropOptions {
  /** textarea 的 v-model ref，用于插入 Markdown 并触发响应式更新 */
  modelRef: Ref<string>;
}

export function useMarkdownImageDrop(options: MarkdownImageDropOptions) {
  const { modelRef } = options;

  /** 拖入高亮 */
  const isDragOver = ref(false);
  /** 上传中 */
  const isUploading = ref(false);

  let dragCounter = 0;

  function onDragEnter(e: DragEvent) {
    e.preventDefault();
    e.stopPropagation();
    dragCounter++;
    if (e.dataTransfer?.types.includes("Files")) {
      isDragOver.value = true;
    }
  }

  function onDragOver(e: DragEvent) {
    e.preventDefault();
    e.stopPropagation();
    if (e.dataTransfer) {
      e.dataTransfer.dropEffect = "copy";
    }
  }

  function onDragLeave(e: DragEvent) {
    e.preventDefault();
    e.stopPropagation();
    dragCounter--;
    if (dragCounter <= 0) {
      dragCounter = 0;
      isDragOver.value = false;
    }
  }

  async function onDrop(e: DragEvent, textarea: HTMLTextAreaElement | null) {
    e.preventDefault();
    e.stopPropagation();
    isDragOver.value = false;
    dragCounter = 0;

    const file = e.dataTransfer?.files?.[0];
    if (!file || !isImageFile(file)) return;
    if (!textarea) return;

    // 上传图片
    isUploading.value = true;
    try {
      const formData = new FormData();
      formData.append("file", file);
      const resp = await httpClient.request<{ url: string }>("/admin/resources/upload-cover", {
        method: "POST",
        body: formData,
      });
      const url = resp.url ?? "";
      if (!url) {
        toastError("上传图片失败：未获取到链接。");
        return;
      }

      // 在光标处插入 Markdown 图片语法
      const alt = file.name.replace(/[\[\]()|]/g, "").replace(/\.[^.]+$/, "");
      const insertText = `![${alt}](${url})`;
      const start = textarea.selectionStart;
      const end = textarea.selectionEnd;
      const before = modelRef.value.substring(0, start);
      const after = modelRef.value.substring(end);
      modelRef.value = before + insertText + after;

      // 光标放到插入文本之后，并聚焦
      void textarea.offsetHeight; // 等待 Vue 渲染
      textarea.focus();
      const newPos = start + insertText.length;
      textarea.setSelectionRange(newPos, newPos);
    } catch (err: unknown) {
      const message = err instanceof HttpError
        ? `上传图片失败：${err.message}`
        : "上传图片失败，请检查封面存储目录是否已配置。";
      toastError(message);
    } finally {
      isUploading.value = false;
    }
  }

  /** 将 drop 事件与 template-ref 桥接，组件内直接传给 @drop.prevent 等事件 */
  function handleDrop(e: DragEvent, textarea: HTMLTextAreaElement | null) {
    void onDrop(e, textarea);
  }

  return {
    isDragOver,
    isUploading,
    onDragEnter,
    onDragOver,
    onDragLeave,
    handleDrop,
  };
}
