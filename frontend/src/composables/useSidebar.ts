import { ref } from "vue";

const STORAGE_KEY = "global-sidebar-open";

const expanded = ref(true);

function loadStoredState() {
  try {
    const stored = window.localStorage.getItem(STORAGE_KEY);
    if (stored === "false") {
      expanded.value = false;
    }
  } catch {
    // ignore
  }
}

function persistState() {
  try {
    window.localStorage.setItem(STORAGE_KEY, String(expanded.value));
  } catch {
    // ignore
  }
}

export function useSidebar() {
  function toggle() {
    expanded.value = !expanded.value;
    persistState();
  }

  function close() {
    expanded.value = false;
    persistState();
  }

  return { expanded, toggle, close, loadStoredState };
}
