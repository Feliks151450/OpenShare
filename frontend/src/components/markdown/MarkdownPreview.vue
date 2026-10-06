<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from "vue";

import { renderSimpleMarkdown } from "../../lib/markdown";
import { rewriteVditorPreviewHtml } from "../../lib/markdownVditorHtml";
import { VDITOR_ASSET_ROOT, VDITOR_LUTE_URL, loadVditor, type VditorGlobal } from "../../lib/vditorLoader";

const props = withDefaults(
  defineProps<{
    source: string;
    emptyText?: string;
    /** 编辑面板旁的实时预览需要去抖，静态文档可传 0 */
    debounceMs?: number;
  }>(),
  {
    emptyText: "",
    debounceMs: 120,
  },
);

/** 渲染是异步的，宿主若要按内容高度做截断/测量，需监听这个事件重算 */
const emit = defineEmits<{
  rendered: [];
}>();

const contentRef = ref<HTMLElement | null>(null);
const fallbackHTML = ref("");
const usingFallback = ref(false);
const isEmpty = ref(false);
const isRendering = ref(false);
const hasRendered = ref(false);

/** 递增序号：并发渲染时只有最后一次结果可以落 DOM */
let serial = 0;
let timer: ReturnType<typeof setTimeout> | null = null;
let vditorGlobal: VditorGlobal | null = null;

async function renderNow(markdown: string) {
  // immediate watch 在 DOM 挂载前就会跑一次，这里必须等一帧再取容器
  await nextTick();
  const target = contentRef.value;
  if (!target) return;
  const mySerial = ++serial;

  if (!markdown) {
    isEmpty.value = true;
    isRendering.value = false;
    target.innerHTML = "";
    emit("rendered");
    return;
  }
  isEmpty.value = false;
  isRendering.value = true;

  try {
    vditorGlobal ??= await loadVditor();
    if (mySerial !== serial) return;
    await vditorGlobal.preview(target, markdown, {
      cdn: VDITOR_ASSET_ROOT,
      // fork 的纯预览默认走 ${cdn}/dist/js/lute/lute.min.js，这里统一到定制版 lutePro
      lutePath: VDITOR_LUTE_URL,
      mode: "light",
      lang: "zh_CN",
      math: { engine: "KaTeX", inlineDigit: true },
      markdown: { mark: true, footnotes: true },
      transform: rewriteVditorPreviewHtml,
    });
    if (mySerial !== serial) return;
    usingFallback.value = false;
    hasRendered.value = true;
  } catch {
    if (mySerial !== serial) return;
    // 静态资源不可用时退回 marked，正文不能静默消失
    fallbackHTML.value = renderSimpleMarkdown(markdown);
    usingFallback.value = true;
    hasRendered.value = true;
  } finally {
    if (mySerial === serial) {
      isRendering.value = false;
      emit("rendered");
    }
  }
}

function scheduleRender() {
  if (timer) clearTimeout(timer);
  const markdown = props.source.replace(/\r\n/g, "\n").trim();
  if (props.debounceMs > 0 && markdown) {
    timer = setTimeout(() => { void renderNow(markdown); }, props.debounceMs);
    return;
  }
  timer = null;
  void renderNow(markdown);
}

watch(() => props.source, scheduleRender, { immediate: true });

onBeforeUnmount(() => {
  if (timer) clearTimeout(timer);
});
</script>

<template>
  <div class="vditor" style="border: 0; border-radius: 0">
    <div ref="contentRef" class="markdown-content" style="padding: 0" :hidden="usingFallback" />
    <div v-if="usingFallback" class="markdown-content" v-html="fallbackHTML" />
    <p v-if="isEmpty" class="text-sm text-slate-400">{{ emptyText || "暂无内容" }}</p>
    <p v-else-if="isRendering && !hasRendered" class="text-sm text-slate-400">正在渲染…</p>
  </div>
</template>
