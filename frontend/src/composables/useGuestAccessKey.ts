/**
 * 访客密钥访问 composable
 * - 密钥明文保存在 localStorage（同源浏览器内有效）；
 * - 通过 httpClient request interceptor 自动在浏览类公共请求上附带 X-OpenShare-Guest-Key 头；
 * - 提供 set / clear / 校验失败 hint 提示。
 */

import { ref } from "vue";

import { httpClient } from "../lib/http/client";

const STORAGE_KEY = "openshare-guest-key";

/** 全局共享的当前密钥响应式状态（仅在浏览器内有效）。 */
const currentKey = ref<string>(loadFromStorage());

interface ValidateSuccess {
  ok: true;
  unlockedFolderIds: string[];
}

interface ValidateFailure {
  ok: false;
  hint: string;
}

export type ValidateResult = ValidateSuccess | ValidateFailure;

function loadFromStorage(): string {
  try {
    return localStorage.getItem(STORAGE_KEY) ?? "";
  } catch {
    return "";
  }
}

function saveToStorage(value: string) {
  try {
    if (value) {
      localStorage.setItem(STORAGE_KEY, value);
    } else {
      localStorage.removeItem(STORAGE_KEY);
    }
  } catch {
    // 忽略 localStorage 异常（隐私模式 / 禁用）
  }
}

/**
 * 供浏览器命中受保护目录时调用：提交密钥值给后端 validate 端点校验。
 * 成功时保存密钥并返回解锁的目录 ID；失败时返回通用 hint 提示。
 */
async function setKey(value: string): Promise<ValidateResult> {
  const trimmed = (value ?? "").trim();
  if (!trimmed) {
    return { ok: false, hint: "" };
  }
  try {
    const response = await httpClient.post<{
      valid: boolean;
      key_id?: string;
      unlocked_folder_ids?: string[];
      hint?: string;
    }>("/public/guest-access/validate", { value: trimmed });
    if (response.valid) {
      currentKey.value = trimmed;
      saveToStorage(trimmed);
      return { ok: true, unlockedFolderIds: response.unlocked_folder_ids ?? [] };
    }
    return { ok: false, hint: response.hint ?? "" };
  } catch {
    return { ok: false, hint: "" };
  }
}

/** 清空已保存的密钥。 */
function clearKey() {
  currentKey.value = "";
  saveToStorage("");
}

export function useGuestAccessKey() {
  return {
    currentKey,
    setKey,
    clearKey,
  };
}

/** window.OpenShare 与 HTTP 拦截器需要的非响应式读取入口。 */
export function readGuestAccessKey(): string {
  return currentKey.value;
}
