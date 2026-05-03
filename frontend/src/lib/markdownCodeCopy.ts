import { copyPlainTextToClipboard } from "./clipboard";

let markdownCodeCopyDelegationInstalled = false;

/** 为 `.markdown-code-copy` 按钮挂载一次性文档委托（v-html 动态插入也生效） */
export function initMarkdownCodeCopyDelegation(): void {
  if (markdownCodeCopyDelegationInstalled || typeof document === "undefined") {
    return;
  }
  markdownCodeCopyDelegationInstalled = true;

  document.body.addEventListener("click", (event) => {
    const target = event.target;
    if (!(target instanceof Element)) {
      return;
    }
    const button = target.closest(".markdown-code-copy");
    if (!(button instanceof HTMLButtonElement)) {
      return;
    }
    const wrap = button.closest(".markdown-code-wrap");
    if (!wrap) {
      return;
    }
    const codeEl = wrap.querySelector("pre code");
    const text = codeEl?.textContent ?? "";
    if (!text) {
      return;
    }

    void (async () => {
      const ok = await copyPlainTextToClipboard(text);
      if (!ok) {
        return;
      }
      const prev = button.textContent ?? "复制";
      button.textContent = "已复制";
      button.disabled = true;
      window.setTimeout(() => {
        button.textContent = prev;
        button.disabled = false;
      }, 1600);
    })();
  });
}
