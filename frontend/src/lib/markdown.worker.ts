// Web Worker: offloads CPU-intensive marked.parse() from the main thread.
// DOMPurify stays on the main thread (requires DOM API).
import { marked, type Renderer, type Tokens } from "marked";
import {
  escapeHtml,
  isSafeImageUrlForSrc,
  resolveMarkdownImageUrlToHref,
  markdownFencedCodeHtml,
  markdownLinkHtml,
} from "./markdown-shared";
// Reuse the same marked config across invocations.
// breaks 必须与主线程 markdown.ts 保持一致（false），否则同一段简介在
// 正文与编辑器预览中换行表现不同。
marked.use({
  gfm: true,
  breaks: false,
  renderer: {
    code(this: Renderer, token: Tokens.Code): string {
      return markdownFencedCodeHtml(token.lang ?? "", token.text, token.escaped ?? false);
    },
    /* 支持图片宽度控制: ![描述|width=800](url) 或 ![描述|width=80%](url)，设置图片最大宽度 */
    image(this: Renderer, token: Tokens.Image): string {
      let altPlain = token.text ?? "";
      if (token.tokens?.length) {
        altPlain = this.parser.parseInline(token.tokens, this.parser.textRenderer);
      }
      let maxWidthStyle = "";
      const widthMatch = altPlain.match(/^(.*?)\|width=(\d+%?)\s*$/);
      if (widthMatch) {
        altPlain = widthMatch[1].trimEnd();
        const widthVal = widthMatch[2];
        const cssVal = widthVal.endsWith("%") ? widthVal : `${widthVal}px`;
        maxWidthStyle = ` style="max-width:${cssVal}"`;
      }
      const rawHref = String(token.href ?? "").trim();
      if (!isSafeImageUrlForSrc(rawHref)) {
        return escapeHtml(token.raw ?? altPlain);
      }
      const src = escapeHtml(resolveMarkdownImageUrlToHref(rawHref));
      const alt = escapeHtml(altPlain);
      const title =
        token.title != null && String(token.title).trim() !== ""
          ? ` title="${escapeHtml(String(token.title))}"`
          : "";
      return `<img src="${src}" alt="${alt}" class="markdown-img" loading="lazy" decoding="async"${maxWidthStyle}${title} />`;
    },
    link(this: Renderer, token: Tokens.Link): string {
      const inner = this.parser.parseInline(token.tokens);
      return markdownLinkHtml(String(token.href ?? ""), token.title, inner);
    },
  },
});

interface WorkerRequest {
  id: number;
  source: string;
}

interface WorkerResponse {
  id: number;
  html: string;
  error?: string;
}

self.onmessage = (e: MessageEvent<WorkerRequest>) => {
  const { id, source } = e.data;
  const normalized = source.replace(/\r\n/g, "\n");
  if (!normalized.trim()) {
    self.postMessage({ id, html: "" } satisfies WorkerResponse);
    return;
  }
  try {
    const html = marked.parse(normalized, { async: false }) as string;
    self.postMessage({ id, html } satisfies WorkerResponse);
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : "markdown parse error";
    self.postMessage({ id, html: "", error: message } satisfies WorkerResponse);
  }
};
