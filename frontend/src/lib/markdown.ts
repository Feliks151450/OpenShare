import DOMPurify from "dompurify";
import { marked, type Renderer, type Tokens } from "marked";
import { escapeHtml, isSafeImageUrlForSrc, resolveMarkdownImageUrlToHref, markdownFencedCodeHtml } from "./markdown-shared";
export { resolveMarkdownImageUrlToHref };

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

marked.use({
  gfm: true,
  breaks: false,
  renderer: {
    code(this: Renderer, token: Tokens.Code): string {
      return markdownFencedCodeHtml(token.lang ?? "", token.text, token.escaped);
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
