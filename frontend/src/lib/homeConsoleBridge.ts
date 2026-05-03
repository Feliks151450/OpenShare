export type HomeListSortDirection = "asc" | "desc";
export type HomeListSortMode = "name" | "download" | "format" | "modified";
export type HomeListViewMode = "cards" | "table";

export type HomeConsoleHooks = {
  setListView(mode: HomeListViewMode): void;
  setListSort(mode: HomeListSortMode): void;
  setListSortDirection(direction: HomeListSortDirection): void;
};

let homeConsoleHooks: HomeConsoleHooks | null = null;

export function registerHomeConsoleHooks(next: HomeConsoleHooks): void {
  homeConsoleHooks = next;
}

export function unregisterHomeConsoleHooks(): void {
  homeConsoleHooks = null;
}

export function getHomeConsoleHooks(): HomeConsoleHooks | null {
  return homeConsoleHooks;
}
