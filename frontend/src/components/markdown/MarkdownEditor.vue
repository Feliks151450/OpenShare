<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from "vue";

import { httpClient, HttpError } from "../../lib/http/client";
import { VDITOR_ASSET_ROOT, VDITOR_LUTE_URL, loadVditor, nextVditorMountId, type VditorInstance } from "../../lib/vditorLoader";

const props = withDefaults(
  defineProps<{
    modelValue: string;
    placeholder?: string;
    /** 编辑器整体高度（px）。vditor 直接写在自己的根节点上，不依赖宿主的高度链 */
    height?: string | number;
    /** 访客无 /admin/resources/upload-cover 权限时关闭图片上传 */
    uploadEnabled?: boolean;
  }>(),
  {
    placeholder: "",
    height: 320,
    uploadEnabled: true,
  },
);

const emit = defineEmits<{
  "update:modelValue": [value: string];
}>();

const mountId = nextVditorMountId();

const isReady = ref(false);
const loadError = ref("");

let editor: VditorInstance | null = null;

async function uploadImages(files: File[]): Promise<string | null> {
  if (!editor) return "编辑器尚未就绪，请稍后重试。";
  let lastError = "";
  for (const file of files) {
    try {
      const formData = new FormData();
      formData.append("file", file);
      const resp = await httpClient.request<{ url: string }>("/admin/resources/upload-cover", {
        method: "POST",
        body: formData,
      });
      const url = (resp.url ?? "").trim();
      if (!url) {
        lastError = "上传图片失败：未获取到链接。";
        continue;
      }
      const alt = file.name.replace(/[[\]()|]/g, "").replace(/\.[^.]+$/, "");
      editor.insertMarkdown(`![${alt}](${url})`);
    } catch (err: unknown) {
      lastError = err instanceof HttpError
        ? `上传图片失败：${err.message}`
        : "上传图片失败，请检查封面存储目录是否已配置。";
    }
  }
  return lastError || null;
}

async function mount() {
  const Vditor = await loadVditor().catch((err: unknown) => {
    loadError.value = err instanceof Error ? err.message : "编辑器加载失败";
    return null;
  });
  if (!Vditor) return;

  editor = new Vditor(mountId, {
    value: props.modelValue,
    // fork 的编辑器默认按「当前页面相对路径」找 lutePro.min.js，SPA 路由下必坏，必须显式给
    cdn: VDITOR_ASSET_ROOT,
    _lutePath: VDITOR_LUTE_URL,
    height: props.height,
    mode: "wysiwyg",
    lang: "zh_CN",
    tab: "\t",
    placeholder: props.placeholder,
    cache: { enable: false },
    // 紧凑表单里不需要大纲侧栏
    outline: { enable: false },
    toolbarConfig: { pin: true },
    editorTheme: "Light",
    codeMirrorTheme: "Github",
    mermaidTheme: "Light",
    toolbar: [
      "edit-mode",
      "headings",
      "bold",
      "italic",
      "strike",
      "link",
      "|",
      "list",
      "ordered-list",
      "check",
      "|",
      "quote",
      "line",
      "code",
      "inline-code",
      "table",
      "|",
      "upload",
      "|",
      "undo",
      "redo",
      "|",
      "find",
      "settings",
    ],
    preview: {
      markdown: { mark: true, footnotes: true },
      math: { engine: "KaTeX", inlineDigit: true },
    },
    upload: {
      accept: "image/*",
      multiple: false,
      // 配置 handler 后 fork 会短路其余 upload 选项，插入动作要自己完成
      handler: (files: File[]) =>
        props.uploadEnabled
          ? uploadImages(files)
          : Promise.resolve("访客不能在这里上传图片：请先用下方文件区上传，再在正文中引用。"),
    },
    input: (value: string) => emit("update:modelValue", value),
    after: () => {
      isReady.value = true;
      // fork 的设置面板等内部面板从全局取公开方法（INTEGRATION.md 16 节第 7 条）
      if (editor) window.vditor = editor;
    },
  });
}

watch(
  () => props.modelValue,
  (next) => {
    if (!editor) return;
    // setValue 会把编辑区整体重建，内容没变就绝不能调
    if (editor.getValue() === next) return;
    editor.setValue(next ?? "");
  },
);

onMounted(mount);

onBeforeUnmount(() => {
  if (window.vditor === editor) delete window.vditor;
  editor?.destroy();
  editor = null;
});
</script>

<template>
  <div class="os-vditor-editor w-full">
    <p v-if="loadError" class="rounded-xl bg-rose-50 px-3 py-2 text-sm text-rose-700">{{ loadError }}</p>
    <template v-else>
      <div :id="mountId"></div>
      <p v-if="!isReady" class="py-1 text-sm text-slate-400">编辑器加载中…</p>
    </template>
  </div>
</template>

<style scoped>
/* 大纲相关的显隐规则在 vditor 产物里挂在 `:is(#vditor,.vditor)[data-editor-theme]` 下，但
   `outline.enable:false` 并不隐藏移动端 trigger（上游 mobileOutlineMenu.ts:164 无条件加
   --mobile-ready，而隐藏规则恰好 :not(--mobile-ready)）。紧凑字段里大纲本就关掉，直接隐藏。 */
.os-vditor-editor :deep(.vditor-mobile-outline-trigger),
.os-vditor-editor :deep(.vditor-outline),
.os-vditor-editor :deep(.vditor-outline__backdrop) {
  display: none !important;
}
</style>
