<script setup lang="ts">
import { computed, onMounted } from "vue";
import { RouterView, useRoute, useRouter } from "vue-router";

import GlobalSidebar from "../components/layout/GlobalSidebar.vue";
import Navbar from "../components/layout/Navbar.vue";
import { httpClient } from "../lib/http/client";
import { staticDataLoader } from "../lib/staticDataLoader";
import { useNavActions, type PanelName } from "../composables/useNavActions";
import { useSidebar } from "../composables/useSidebar";

const route = useRoute();
const router = useRouter();
const { openPanel } = useNavActions();
const { expanded: sidebarExpanded } = useSidebar();

const showPublicNavbar = computed(() => route.name !== "public-file-detail");

const showSidebar = computed(() => showPublicNavbar.value);

const mainMarginClass = computed(() => {
  if (!showSidebar.value || !sidebarExpanded.value) return "";
  return "xl:ml-56";
});

const viewKey = computed(() => {
  if (route.name === "public-file-detail") {
    return `file:${String(route.params.fileID ?? "")}`;
  }
  return String(route.name ?? route.path);
});

function triggerPanel(name: PanelName) {
  if (route.name === "public-home") {
    openPanel(name);
  } else {
    router.push({ path: "/", query: { panel: name } });
  }
}

const links = [
  { to: "/", label: "首页" },
  { to: "/upload", label: "回执查询" },
  {
    to: "",
    label: "公告栏",
    action: () => triggerPanel("announcements"),
  },
  {
    to: "",
    label: "资料上新",
    action: () => triggerPanel("latestItems"),
  },
  {
    to: "",
    label: "热门下载",
    action: () => triggerPanel("hotDownloads"),
  },
];

onMounted(() => {
  void trackVisit();
  void prefetchDownloadPolicy();
});

async function prefetchDownloadPolicy() {
  if (staticDataLoader.policyApplied || staticDataLoader.policyPromise) return;
  let taskResolve!: () => void;
  staticDataLoader.setPolicyPromise(new Promise<void>((r) => { taskResolve = r; }));
  try {
    const resp = await httpClient.get<Record<string, unknown>>("/public/download-policy");
    if ((resp as any).directory_cdn_urls) staticDataLoader.setCdnUrlMapFromObject((resp as any).directory_cdn_urls);
    if ((resp as any).global_cdn_url) staticDataLoader.setGlobalCdnUrl((resp as any).global_cdn_url);
    staticDataLoader.setLivePolicy(resp);
    staticDataLoader.markPolicyApplied();
  } catch {
    /* 忽略 */
  } finally {
    taskResolve();
    staticDataLoader.setPolicyPromise(null);
  }
}

async function trackVisit() {
  try {
    await httpClient.request("/visits", {
      method: "POST",
      body: {
        scope: "public",
        path: route.path,
      },
    });
  } catch {
    // Ignore analytics failures.
  }
}
</script>

<template>
  <div class="app-shell">
    <Navbar v-if="showPublicNavbar" :items="links" :current-path="route.path" />
    <GlobalSidebar v-if="showSidebar" />

    <main :class="[showPublicNavbar ? 'pt-16' : 'pt-0', mainMarginClass]">
      <RouterView :key="viewKey" />
    </main>
  </div>
</template>
