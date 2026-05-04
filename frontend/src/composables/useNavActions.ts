import { ref } from "vue";

export type PanelName = "announcements" | "hotDownloads" | "latestItems";

const activePanel = ref<PanelName | null>(null);

export function useNavActions() {
  function openPanel(name: PanelName) {
    activePanel.value = name;
  }

  function closePanel() {
    activePanel.value = null;
  }

  return { activePanel, openPanel, closePanel };
}
