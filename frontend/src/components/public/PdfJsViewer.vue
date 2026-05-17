<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from "vue";
import { ChevronDown, ChevronUp, Maximize, Minimize, ZoomIn, ZoomOut } from "lucide-vue-next";
import pdfjsWorkerUrl from "pdfjs-dist/build/pdf.worker.min.mjs?url";

const props = defineProps<{
  src: string;
  fileId: string;
}>();

const outerRef = ref<HTMLDivElement | null>(null);
const scrollRef = ref<HTMLDivElement | null>(null);
const isFullscreen = ref(false);
const loading = ref(false);
const error = ref("");
const totalPages = ref(0);
const scale = ref(1.25);
const visiblePage = ref(1);
/** 已创建容器的最大页码 */
const pageWatermark = ref(0);

let pdfDoc: any = null;
let destroyed = false;
const pageCanvases = new Map<number, HTMLCanvasElement>();
let renderBusy = false;
let reRendering = false;

async function loadPdf() {
  destroyed = false;
  loading.value = true;
  error.value = "";
  totalPages.value = 0;
  visiblePage.value = 1;
  pageWatermark.value = 0;
  pageCanvases.forEach((c) => c.remove());
  pageCanvases.clear();

  try {
    const pdfjsLib = await import("pdfjs-dist");
    pdfjsLib.GlobalWorkerOptions.workerSrc = pdfjsWorkerUrl;

    const doc = await pdfjsLib.getDocument({
      url: props.src,
      disableRange: false,
      disableStream: true,
      disableAutoFetch: true,
      withCredentials: true,
    }).promise;

    pdfDoc = doc;
    totalPages.value = doc.numPages;

    if (!destroyed) {
      loading.value = false;
      await nextTick();
      // 初始只创建第 1 页容器并渲染
      appendOneContainer(1);
      await nextTick();
      renderPage(1);
      scrollRef.value?.addEventListener("scroll", onScroll, { passive: true });
    }
  } catch (e: any) {
    if (!destroyed) error.value = e?.message || "PDF 加载失败";
    loading.value = false;
  }
}

/** 创建单页占位容器 */
function appendOneContainer(num: number) {
  if (!scrollRef.value) return;
  const wrapper = document.createElement("div");
  wrapper.className = "flex justify-center py-2";
  wrapper.dataset.page = String(num);
  wrapper.style.minHeight = "200px";
  scrollRef.value.appendChild(wrapper);
  pageWatermark.value = num;
}

/** 根据容器宽度和页面原始宽度计算适合的缩放比例 */
function effectiveScale(page: any): number {
  if (!scrollRef.value) return scale.value;
  const containerWidth = scrollRef.value.clientWidth - 16; // 预留内边距
  if (containerWidth <= 0) return scale.value;
  const baseViewport = page.getViewport({ scale: 1 });
  const fitScale = containerWidth / baseViewport.width;
  return Math.min(scale.value, fitScale);
}

async function renderPage(num: number) {
  if (!pdfDoc || destroyed || renderBusy) return;
  const wrapper = scrollRef.value?.querySelector(`[data-page="${num}"]`) as HTMLDivElement | null;
  if (!wrapper || pageCanvases.has(num)) return;

  renderBusy = true;
  try {
    const page = await pdfDoc.getPage(num);
    const renderScale = effectiveScale(page);
    const viewport = page.getViewport({ scale: renderScale });

    const canvas = document.createElement("canvas");
    canvas.className = "shadow-lg";
    canvas.width = Math.floor(viewport.width * devicePixelRatio);
    canvas.height = Math.floor(viewport.height * devicePixelRatio);
    canvas.style.width = `${Math.floor(viewport.width)}px`;
    canvas.style.height = `${Math.floor(viewport.height)}px`;

    const ctx = canvas.getContext("2d")!;
    ctx.setTransform(devicePixelRatio, 0, 0, devicePixelRatio, 0, 0);

    wrapper.textContent = "";
    wrapper.appendChild(canvas);
    wrapper.style.minHeight = "";
    pageCanvases.set(num, canvas);

    await page.render({ canvasContext: ctx, viewport }).promise;
  } catch (e: any) {
    if (e?.name === "RenderingCancelledException") return;
  } finally {
    renderBusy = false;
    if (!reRendering) scheduleNext();
  }
}

/** 滚动事件：更新可见页码，需要时创建并渲染下一页 */
function onScroll() {
  if (!scrollRef.value || destroyed) return;
  updateVisiblePage();
  scheduleNext();
}

/** 如果滚动到底部附近且还有未创建的页面，创建下一页 */
function scheduleNext() {
  if (!scrollRef.value || destroyed || renderBusy) return;
  const container = scrollRef.value;
  const distToBottom = container.scrollHeight - container.scrollTop - container.clientHeight;

  // 滚动到距离底部 300px 内时，创建下一页
  if (distToBottom < 300 && pageWatermark.value < totalPages.value) {
    const next = pageWatermark.value + 1;
    appendOneContainer(next);
  }

  // 渲染当前视口内可见但未渲染的页面（优先渲染中心页）
  const centerY = container.scrollTop + container.clientHeight / 2;
  let bestNum = 0;
  let bestDist = Infinity;
  for (const child of container.children) {
    const pageNum = Number((child as HTMLElement).dataset.page);
    if (!pageNum || pageCanvases.has(pageNum)) continue;
    const elTop = (child as HTMLElement).offsetTop;
    const elBottom = elTop + (child as HTMLElement).offsetHeight;
    if (elTop > container.scrollTop + container.clientHeight || elBottom < container.scrollTop) continue;
    const dist = Math.abs((elTop + elBottom) / 2 - centerY);
    if (dist < bestDist) { bestDist = dist; bestNum = pageNum; }
  }
  if (bestNum) renderPage(bestNum);
}

function updateVisiblePage() {
  if (!scrollRef.value) return;
  const container = scrollRef.value;
  const centerY = container.scrollTop + container.clientHeight / 2;
  for (const child of container.children) {
    const rect = child.getBoundingClientRect();
    const containerRect = container.getBoundingClientRect();
    const childTop = rect.top - containerRect.top + container.scrollTop;
    const childBottom = childTop + rect.height;
    if (childTop <= centerY && childBottom >= centerY) {
      const p = Number((child as HTMLElement).dataset.page);
      if (p) visiblePage.value = p;
      break;
    }
  }
}

async function reRenderAll() {
  if (!pdfDoc || destroyed) return;
  reRendering = true;
  try {
    const rendered = [...pageCanvases.keys()];
    pageCanvases.clear();
    if (scrollRef.value) {
      for (const child of scrollRef.value.children) {
        if ((child as HTMLElement).dataset.page) {
          child.textContent = "";
          (child as HTMLElement).style.minHeight = "200px";
        }
      }
    }
    for (const num of rendered) {
      if (destroyed) break;
      await renderPage(num);
    }
  } finally {
    reRendering = false;
    scheduleNext();
  }
}

function zoomIn() { scale.value = Math.min(4, scale.value + 0.25); reRenderAll(); }
function zoomOut() { scale.value = Math.max(0.5, scale.value - 0.25); reRenderAll(); }
function fitWidth() { scale.value = 1.0; reRenderAll(); }

// 全屏切换后重渲染并自动补页填满视口
watch(isFullscreen, async (val) => {
  if (!val) { reRenderAll(); return; }
  await nextTick();
  reRenderAll();
  // 等待渲染完成后再补页
  await nextTick();
  fillViewport();
});

/** 如果视口底部还有空白，持续创建新页直到填满 */
function fillViewport() {
  if (!scrollRef.value || destroyed) return;
  const container = scrollRef.value;
  // 内容还没填满可视区域，且还有未创建页面时，追加并渲染
  if (container.scrollHeight <= container.clientHeight + 50 && pageWatermark.value < totalPages.value) {
    const next = pageWatermark.value + 1;
    appendOneContainer(next);
    nextTick(() => {
      scheduleNext();
      // 如果还没填满，继续
      nextTick(() => fillViewport());
    });
  } else {
    scheduleNext();
  }
}

function toggleFullscreen() {
  isFullscreen.value = !isFullscreen.value;
}

function scrollToPage(num: number) {
  const clamped = Math.max(1, Math.min(num, totalPages.value));
  // 确保目标页容器存在
  while (pageWatermark.value < clamped) {
    appendOneContainer(pageWatermark.value + 1);
  }
  nextTick(() => {
    const wrapper = scrollRef.value?.querySelector(`[data-page="${clamped}"]`);
    if (wrapper) wrapper.scrollIntoView({ block: "start", behavior: "smooth" });
  });
}

watch(() => props.src, () => { if (props.src) { cleanup(); loadPdf(); } }, { immediate: true });

function cleanup() {
  destroyed = true;
  pageCanvases.clear();
  pdfDoc?.destroy();
  pdfDoc = null;
  renderBusy = false;
  if (scrollRef.value) scrollRef.value.textContent = "";
}

onBeforeUnmount(() => { cleanup(); });
</script>

<template>
  <Teleport to="body" :disabled="!isFullscreen">
    <!-- 全屏遮罩层 -->
    <div v-if="isFullscreen" class="fixed inset-0 z-[130] bg-slate-100" />
    <div
      ref="outerRef"
      :class="[
        'flex flex-col bg-white',
        isFullscreen
          ? 'fixed inset-0 z-[131] rounded-none'
          : 'rounded-2xl border border-slate-200',
      ]"
      :style="isFullscreen ? {} : { maxHeight: 'min(75vh, 760px)' }"
    >
    <div
      v-if="totalPages > 0 && !error"
      class="relative z-10 flex shrink-0 flex-wrap items-center justify-center gap-2 border-b border-slate-100 bg-white px-3 py-2 sm:gap-3"
    >
      <button type="button" class="inline-flex h-8 w-8 items-center justify-center rounded-lg border border-slate-200 bg-white text-slate-600 transition hover:bg-slate-50 disabled:opacity-30"
        :disabled="visiblePage <= 1" title="上一页" @click="scrollToPage(visiblePage - 1)"><ChevronUp class="h-4 w-4" /></button>
      <span class="min-w-[4rem] text-center text-sm tabular-nums text-slate-700">{{ visiblePage }} / {{ totalPages }}</span>
      <button type="button" class="inline-flex h-8 w-8 items-center justify-center rounded-lg border border-slate-200 bg-white text-slate-600 transition hover:bg-slate-50 disabled:opacity-30"
        :disabled="visiblePage >= totalPages" title="下一页" @click="scrollToPage(visiblePage + 1)"><ChevronDown class="h-4 w-4" /></button>
      <span class="mx-1 h-5 w-px bg-slate-200" />
      <button type="button" class="inline-flex h-8 w-8 items-center justify-center rounded-lg border border-slate-200 bg-white text-slate-600 transition hover:bg-slate-50"
        title="缩小" @click="zoomOut"><ZoomOut class="h-4 w-4" /></button>
      <button type="button" class="inline-flex h-8 items-center rounded-lg border border-slate-200 bg-white px-2 text-xs font-medium text-slate-600 transition hover:bg-slate-50"
        @click="fitWidth">{{ Math.round(scale * 100) }}%</button>
      <button type="button" class="inline-flex h-8 w-8 items-center justify-center rounded-lg border border-slate-200 bg-white text-slate-600 transition hover:bg-slate-50"
        title="放大" @click="zoomIn"><ZoomIn class="h-4 w-4" /></button>
      <button type="button" class="inline-flex h-8 w-8 items-center justify-center rounded-lg border border-slate-200 bg-white text-slate-600 transition hover:bg-slate-50"
        :title="isFullscreen ? '退出全屏' : '全屏'" @click="toggleFullscreen">
        <Minimize v-if="isFullscreen" class="h-4 w-4" />
        <Maximize v-else class="h-4 w-4" />
      </button>
    </div>

    <div v-if="loading" class="flex items-center justify-center py-16">
      <span class="inline-block h-5 w-5 animate-spin rounded-full border-2 border-slate-300 border-t-sky-500" />
      <span class="ml-3 text-sm text-slate-500">加载中…</span>
    </div>
    <div v-else-if="error" class="px-4 py-12 text-center">
      <p class="text-sm text-rose-600">{{ error }}</p>
      <p class="mt-2 text-xs text-slate-400">请尝试下载文件或在新标签页中打开</p>
    </div>

    <div ref="scrollRef" v-show="!loading && !error" class="min-h-0 flex-1 overflow-y-auto bg-slate-100" />
  </div>
  </Teleport>
</template>
