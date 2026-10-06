/**
 * localStorage / sessionStorage 安全访问封装。
 *
 * Safari「阻止所有 Cookie」等隐私模式下，访问 `window.localStorage` 属性本身
 * 就会抛 SecurityError，未捕获的异常会中断调用方（例如 Home 的 onMounted，
 * 曾导致首页永久卡在加载中）。所有读写都通过本模块进行，异常静默降级。
 */

function tryGet(storage: Storage | undefined, key: string): string | null {
  try {
    return storage?.getItem(key) ?? null;
  } catch {
    return null;
  }
}

function trySet(storage: Storage | undefined, key: string, value: string): void {
  try {
    storage?.setItem(key, value);
  } catch {
    // 配额满或隐私模式：写入失败不影响主流程
  }
}

function tryRemove(storage: Storage | undefined, key: string): void {
  try {
    storage?.removeItem(key);
  } catch {
    // ignore
  }
}

function local(): Storage | undefined {
  try {
    return window.localStorage;
  } catch {
    return undefined;
  }
}

function session(): Storage | undefined {
  try {
    return window.sessionStorage;
  } catch {
    return undefined;
  }
}

export function safeLocalGet(key: string): string | null {
  return tryGet(local(), key);
}

export function safeLocalSet(key: string, value: string): void {
  trySet(local(), key, value);
}

export function safeLocalRemove(key: string): void {
  tryRemove(local(), key);
}

export function safeSessionGet(key: string): string | null {
  return tryGet(session(), key);
}

export function safeSessionSet(key: string, value: string): void {
  trySet(session(), key, value);
}

export function safeSessionRemove(key: string): void {
  tryRemove(session(), key);
}
