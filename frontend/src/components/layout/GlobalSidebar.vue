<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { Folder, Home } from "lucide-vue-next";

import { httpClient } from "../../lib/http/client";
import { sharedRootFolders, type PublicFolderItem } from "../../lib/publicHomeDirectoryCache";
import { staticDataLoader } from "../../lib/staticDataLoader";
import { useSidebar } from "../../composables/useSidebar";

const route = useRoute();
const router = useRouter();
const { expanded, loadStoredState, close } = useSidebar();

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
    sharedRootFolders.value = response.items ?? [];
  } catch {
    sharedRootFolders.value = [];
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
  // 优先用 staticDataLoader 预加载的数据
  if (sharedRootFolders.value.length === 0 && staticDataLoader.rootFolders) {
    sharedRootFolders.value = [...(staticDataLoader.rootFolders as unknown as PublicFolderItem[])];
  }
  // Home.vue already fetches root folders on the home page;
  // only fetch independently when mounting on a non-home route.
  if (route.name !== "public-home" && sharedRootFolders.value.length === 0) {
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
        ? 'w-56'
        : '-translate-x-full'
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
        v-if="!loading && sharedRootFolders.length === 0"
        class="px-2 py-4 text-center text-xs text-slate-400"
      >
        暂无目录
      </p>
      <div v-if="loading" class="px-2 py-4 text-center text-xs text-slate-400">
        加载中…
      </div>
      <div class="space-y-0.5">
        <button
          v-for="folder in sharedRootFolders"
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
