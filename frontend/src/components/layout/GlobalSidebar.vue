<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { Folder, Home } from "lucide-vue-next";

import { httpClient } from "../../lib/http/client";
import type { PublicFolderItem } from "../../lib/publicHomeDirectoryCache";
import { useSidebar } from "../../composables/useSidebar";

const route = useRoute();
const router = useRouter();
const { expanded, loadStoredState, close } = useSidebar();

const folders = ref<PublicFolderItem[]>([]);
const loading = ref(false);

const activeFolderId = computed(() => {
  const raw = route.query.folder;
  if (typeof raw === "string" && raw.trim()) return raw.trim();
  return null;
});

const isRoot = computed(() => route.query.root === "1");

async function loadRootFolders() {
  loading.value = true;
  try {
    const response = await httpClient.get<{ items: PublicFolderItem[] }>("/public/folders");
    folders.value = response.items ?? [];
  } catch {
    folders.value = [];
  } finally {
    loading.value = false;
  }
}

function goHome() {
  router.push({ name: "public-home", query: { root: "1" } });
  close();
}

function goFolder(id: string) {
  router.push({ name: "public-home", query: { folder: id } });
  close();
}

onMounted(() => {
  loadStoredState();
  loadRootFolders();
});

watch(() => route.name, (name) => {
  if (name === "public-home") {
    loadRootFolders();
  }
});
</script>

<template>
  <!-- Backdrop — only on screens below xl when expanded -->
  <Teleport to="body">
    <Transition name="fade">
      <div
        v-if="expanded"
        class="fixed inset-0 z-[55] bg-slate-950/30 xl:hidden"
        @click="close"
      />
    </Transition>
  </Teleport>

  <aside
    class="fixed bottom-0 left-0 top-16 z-[56] flex shrink-0 flex-col border-r border-slate-200 bg-white transition-all duration-200 dark:border-slate-800 dark:bg-slate-950 xl:z-50"
    :class="
      expanded
        ? 'w-56 xl:w-56'
        : '-translate-x-full xl:translate-x-0 xl:w-11'
    "
  >
    <!-- Home button -->
    <div class="border-b border-slate-100 px-2 py-1.5 dark:border-slate-800">
      <button
        type="button"
        class="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-sm transition"
        :class="
          isRoot
            ? 'bg-slate-200/70 font-medium text-slate-900 dark:bg-slate-800 dark:text-slate-100'
            : 'text-slate-600 hover:bg-slate-100 hover:text-slate-900 dark:text-slate-400 dark:hover:bg-slate-800 dark:hover:text-slate-100'
        "
        :title="expanded ? '' : '主页'"
        @click="goHome"
      >
        <Home class="h-4 w-4 shrink-0" />
        <span v-if="expanded" class="truncate">主页</span>
      </button>
    </div>

    <!-- Folder list -->
    <div class="flex-1 overflow-y-auto px-2 py-1.5">
      <p
        v-if="!loading && folders.length === 0"
        class="px-2 py-4 text-center text-xs text-slate-400"
      >
        暂无目录
      </p>
      <div v-if="loading" class="px-2 py-4 text-center text-xs text-slate-400">
        加载中…
      </div>
      <div class="space-y-0.5">
        <button
          v-for="folder in folders"
          :key="folder.id"
          type="button"
          class="flex w-full items-center gap-2 rounded-lg px-2 py-1.5 text-sm transition"
          :class="
            activeFolderId === folder.id
              ? 'bg-slate-200/70 font-medium text-slate-900 dark:bg-slate-800 dark:text-slate-100'
              : 'text-slate-600 hover:bg-slate-100 hover:text-slate-900 dark:text-slate-400 dark:hover:bg-slate-800 dark:hover:text-slate-100'
          "
          :title="folder.name"
          @click="goFolder(folder.id)"
        >
          <Folder class="h-4 w-4 shrink-0" />
          <span class="truncate">{{ folder.name }}</span>
        </button>
      </div>
    </div>
  </aside>
</template>

<style scoped>
.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.2s ease;
}
.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
