import { httpClient } from "./http/client";

const receiptCodeStorageKey = "openshare_receipt_code";
const receiptCodeCookieName = "openshare_receipt_code";

export async function ensureSessionReceiptCode() {
  const response = await httpClient.get<{ receipt_code: string }>("/public/receipt-code");
  const receiptCode = response.receipt_code?.trim() ?? "";
  if (receiptCode) {
    window.sessionStorage.setItem(receiptCodeStorageKey, receiptCode);
  }
  return receiptCode;
}

export function readStoredReceiptCode() {
  const stored = window.sessionStorage.getItem(receiptCodeStorageKey)
    || window.localStorage.getItem(receiptCodeStorageKey);
  if (stored) {
    return stored;
  }

  const cookie = document.cookie
    .split("; ")
    .find((item) => item.startsWith(`${receiptCodeCookieName}=`));
  return cookie ? decodeURIComponent(cookie.split("=").slice(1).join("=")) : "";
}

export function clearStoredReceiptCode() {
  window.sessionStorage.removeItem(receiptCodeStorageKey);
  window.localStorage.removeItem(receiptCodeStorageKey);
  document.cookie = `${receiptCodeCookieName}=; Max-Age=0; path=/`;
}
