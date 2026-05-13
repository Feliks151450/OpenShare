import { Notyf } from "notyf";
import "notyf/notyf.min.css";

/* 内联 SVG 图标，与 lucide-vue-next 风格对齐 */
const icons = {
  success: `<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>`,
  error: `<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/></svg>`,
  warning: `<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><path d="M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z"/><line x1="12" y1="9" x2="12" y2="13"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>`,
  info: `<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>`,
};

/**
 * 全局 Notyf 实例，通知位置为顶部中间。
 * UI 对齐站内设计语言：圆角、阴影、系统字体、内联 SVG 图标。
 */
const notyf = new Notyf({
  position: { x: "center", y: "top" },
  duration: 3200,
  dismissible: false,
  ripple: true,
  types: [
    {
      type: "success",
      background: "#00d6a4",
      className: "toast-custom",
      icon: icons.success,
    },
    {
      type: "error",
      background: "#dc2626",
      className: "toast-custom",
      icon: icons.error,
    },
    {
      type: "info",
      background: "#0284c7",
      className: "toast-custom",
      icon: icons.info,
    },
    {
      type: "warning",
      background: "#d97706",
      className: "toast-custom",
      icon: icons.warning,
    },
  ],
});

/** 注入自定义样式，对齐站内 rounded-xl / shadow / 字体等设计 token */
function injectToastStyles() {
  if (typeof document === "undefined") return;
  const id = "notyf-custom-styles";
  if (document.getElementById(id)) return;
  const style = document.createElement("style");
  style.id = id;
  style.textContent = `
    .notyf__toast.toast-custom {
      border-radius: 0.9rem;
      box-shadow: 0 10px 25px -5px rgba(0,0,0,0.12), 0 0 0 1px rgba(0,0,0,0.04);
      padding: 5px 18px;
      font-family: ui-sans-serif, system-ui, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
      font-size: 1rem;
      line-height: 1.5;
      max-width: 420px;
      backdrop-filter: blur(8px);
    }
    .notyf__toast.toast-custom .notyf__icon {
      margin-right: 5px;
      opacity: 0.9;
    }
    .notyf__toast.toast-custom .notyf__message {
      font-weight: 500;
      letter-spacing: 0.01em;
    }
    .notyf__wrapper {
      padding-top: 14px;
    }
    /* 移动端适配 */
    @media (max-width: 640px) {
      .notyf__toast.toast-custom {
        margin: 0 12px;
        max-width: none;
        border-radius: 0.75rem;
      }
    }
  `;
  document.head.appendChild(style);
}

// 模块加载时注入样式（仅在浏览器环境）
if (typeof window !== "undefined") {
  injectToastStyles();
}

/** 成功通知（绿色） */
export function toastSuccess(message: string) {
  notyf.success(message);
}

/** 错误通知（红色） */
export function toastError(message: string) {
  notyf.error(message);
}

/** 信息提示（蓝色） */
export function toastInfo(message: string) {
  notyf.open({ type: "info", message });
}

/** 警告通知（橙色） */
export function toastWarning(message: string) {
  notyf.open({ type: "warning", message });
}
