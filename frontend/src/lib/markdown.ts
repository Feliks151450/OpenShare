import DOMPurify from "dompurify";
import { marked, type Renderer, type Tokens } from "marked";

function escapeHtml(value: string) {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

/** 从简介 Markdown 中取封面图：仅当 alt 为 `cover`（不区分大小写）时，取首次出现的图片 URL */
export function extractCoverImageUrlFromMarkdown(source: string): string | null {
  const normalized = source.replace(/\r\n/g, "\n");
  const re = /!\[([^\]]*)\]\(([^)]+)\)/g;
  let m: RegExpExecArray | null;
  while ((m = re.exec(normalized)) !== null) {
    if (m[1].trim().toLowerCase() !== "cover") {
      continue;
    }
    return m[2].trim();
  }
  return null;
}

/** 去掉 `![cover](...)`，避免卡片摘要里出现 Markdown 源码 */
export function stripCoverImageMarkdown(source: string): string {
  return source
    .replace(/\r\n/g, "\n")
    .replace(/!\[cover\]\([^)]*\)/gi, "")
    .replace(/\n{3,}/g, "\n\n")
    .trim();
}

function isSafeImageUrlForSrc(url: string): boolean {
  const u = url.trim().toLowerCase();
  if (!u) {
    return false;
  }
  if (u.startsWith("javascript:") || u.startsWith("data:") || u.startsWith("vbscript:")) {
    return false;
  }
  return true;
}

const internalFileCoverRe = /^\/files\/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$/i;

function internalFileCoverHref(path: string): string | null {
  const m = path.trim().match(internalFileCoverRe);
  if (!m) {
    return null;
  }
  return `/api/public/files/${m[1]}/download`;
}

/** 将 Markdown 中的图片地址转为可在当前页展示的绝对 URL */
export function resolveMarkdownImageUrlToHref(raw: string): string {
  const u = raw.trim();
  if (!u) {
    return "";
  }
  if (typeof window === "undefined") {
    return u;
  }
  if (/^https?:\/\//i.test(u)) {
    return u;
  }
  const internal = internalFileCoverHref(u);
  if (internal) {
    return internal;
  }
  try {
    return new URL(u, window.location.href).href;
  } catch {
    return u;
  }
}

export function coverImageHrefFromDescription(description: string): string | null {
  const raw = extractCoverImageUrlFromMarkdown(description);
  if (!raw || !isSafeImageUrlForSrc(raw)) {
    return null;
  }
  const href = resolveMarkdownImageUrlToHref(raw);
  return href || null;
}

/** 列表封面：优先使用后台填写的 `cover_url`，否则使用简介中 `![cover](...)` */
export function fileCoverImageHrefFromFields(coverUrlField: string | undefined, description: string): string | null {
  const direct = (coverUrlField ?? "").trim();
  if (direct) {
    if (!isSafeImageUrlForSrc(direct)) {
      return null;
    }
    const href = resolveMarkdownImageUrlToHref(direct);
    return href || null;
  }
  return coverImageHrefFromDescription(description);
}

function encodeHrefLikeMarked(href: string): string | null {
  const h = href.trim();
  if (!h) {
    return null;
  }
  try {
    return encodeURI(decodeURI(h));
  } catch {
    try {
      return encodeURI(h);
    } catch {
      return null;
    }
  }
}

function markdownFencedCodeHtml(token: Tokens.Code): string {
  const langToken = (token.lang ?? "").trim().match(/^\S+/)?.[0] ?? "";
  const langClass = langToken ? ` class="language-${escapeHtml(langToken)}"` : "";
  const langLabel = langToken ? escapeHtml(langToken) : "";
  const text = token.text.replace(/\n$/, "") + "\n";
  const inner = token.escaped ? text : escapeHtml(text);
  return (
    `<div class="markdown-code-wrap">` +
    `<div class="markdown-code-toolbar">` +
    `<span class="markdown-code-lang">${langLabel}</span>` +
    `<button type="button" class="markdown-code-copy" aria-label="复制代码块">复制</button>` +
    `</div>` +
    `<pre><code${langClass}>${inner}</code></pre>` +
    `</div>`
  );
}

marked.use({
  gfm: true,
  breaks: false,
  renderer: {
    code(this: Renderer, token: Tokens.Code): string {
      return markdownFencedCodeHtml(token);
    },
    image(this: Renderer, token: Tokens.Image): string {
      let altPlain = token.text ?? "";
      if (token.tokens?.length) {
        altPlain = this.parser.parseInline(token.tokens, this.parser.textRenderer);
      }
      /* 支持图片宽度控制: ![描述|width=800](url) 或 ![描述|width=80%](url)，设置图片最大宽度 */
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
      const resolved = resolveMarkdownImageUrlToHref(rawHref);
      const src = escapeHtml(resolved);
      const alt = escapeHtml(altPlain);
      const title =
        token.title != null && String(token.title).trim() !== ""
          ? ` title="${escapeHtml(String(token.title))}"`
          : "";
      return `<img src="${src}" alt="${alt}" class="markdown-img" loading="lazy" decoding="async"${maxWidthStyle}${title} />`;
    },
    link(this: Renderer, token: Tokens.Link): string {
      const inner = this.parser.parseInline(token.tokens);
      const encoded = encodeHrefLikeMarked(String(token.href ?? ""));
      if (encoded === null) {
        return inner;
      }
      const title =
        token.title != null && String(token.title).trim() !== ""
          ? ` title="${escapeHtml(String(token.title))}"`
          : "";
      const hrefAttr = escapeHtml(encoded);
      if (/^https?:\/\//i.test(encoded)) {
        return `<a href="${hrefAttr}" target="_blank" rel="noopener noreferrer"${title}>${inner}</a>`;
      }
      return `<a href="${hrefAttr}"${title}>${inner}</a>`;
    },
  },
});

export function renderSimpleMarkdown(source: string): string {
  const normalized = source.replace(/\r\n/g, "\n");
  if (!normalized.trim()) {
    return "";
  }
  try {
    const html = marked.parse(normalized, { async: false }) as string;
    return DOMPurify.sanitize(html, {
      ADD_ATTR: ["target", "rel", "loading", "decoding", "align", "start", "open", "style"],
      ADD_TAGS: ["input", "details", "summary", "section", "header"],
    });
  } catch {
    return escapeHtml(normalized);
  }
}
