// Web Worker: offloads CPU-intensive marked.parse() from the main thread.
// DOMPurify stays on the main thread (requires DOM API).
import { marked, type Renderer, type Tokens } from "marked";

function escapeHtml(value: string) {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

function isSafeImageUrlForSrc(url: string): boolean {
  const u = url.trim().toLowerCase();
  if (!u) return false;
  if (u.startsWith("javascript:") || u.startsWith("data:") || u.startsWith("vbscript:")) {
    return false;
  }
  return true;
}

// Reuse the same marked config across invocations
marked.use({
  gfm: true,
  breaks: true,
  renderer: {
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
      const src = escapeHtml(rawHref);
      const alt = escapeHtml(altPlain);
      const title =
        token.title != null && String(token.title).trim() !== ""
          ? ` title="${escapeHtml(String(token.title))}"`
          : "";
      return `<img src="${src}" alt="${alt}" class="markdown-img" loading="lazy" decoding="async"${maxWidthStyle}${title} />`;
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
