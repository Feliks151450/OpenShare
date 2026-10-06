import { httpClient } from "./http/client";
import { safeLocalGet, safeLocalRemove, safeSessionGet, safeSessionRemove, safeSessionSet } from "./safeStorage";

const receiptCodeStorageKey = "openshare_receipt_code";
const receiptCodeCookieName = "openshare_receipt_code";

export async function ensureSessionReceiptCode() {
  const response = await httpClient.get<{ receipt_code: string }>("/public/receipt-code");
  const receiptCode = response.receipt_code?.trim() ?? "";
  if (receiptCode) {
    safeSessionSet(receiptCodeStorageKey, receiptCode);
  }
  return receiptCode;
}

export function readStoredReceiptCode() {
  // 注意：回执 cookie 已改为 httpOnly，此处的 cookie 兜底仅对旧会话有效
  const stored = safeSessionGet(receiptCodeStorageKey)
    || safeLocalGet(receiptCodeStorageKey);
  if (stored) {
    return stored;
  }

  try {
    const cookie = document.cookie
      .split("; ")
      .find((item) => item.startsWith(`${receiptCodeCookieName}=`));
    return cookie ? decodeURIComponent(cookie.split("=").slice(1).join("=")) : "";
  } catch {
    return "";
  }
}

export function clearStoredReceiptCode() {
  safeSessionRemove(receiptCodeStorageKey);
  safeLocalRemove(receiptCodeStorageKey);
  try {
    document.cookie = `${receiptCodeCookieName}=; Max-Age=0; path=/`;
  } catch {
    // ignore
  }
}
