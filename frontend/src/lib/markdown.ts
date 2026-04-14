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

function renderInlineMarkdown(value: string) {
  const escaped = escapeHtml(value);
  return escaped
    .replace(/`([^`]+)`/g, "<code>$1</code>")
    .replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>")
    .replace(/\*([^*]+)\*/g, "<em>$1</em>")
    .replace(/!\[([^\]]*)\]\(([^)]+)\)/g, (_, alt, url) => {
      const rawUrl = String(url).trim();
      if (!isSafeImageUrlForSrc(rawUrl)) {
        return `![${alt}](${url})`;
      }
      const src = escapeHtml(resolveMarkdownImageUrlToHref(rawUrl));
      return `<img src="${src}" alt="${escapeHtml(String(alt))}" class="markdown-img" loading="lazy" decoding="async" />`;
    })
    .replace(/\[([^\]]+)\]\((https?:\/\/[^\s)]+)\)/g, '<a href="$2" target="_blank" rel="noopener noreferrer">$1</a>');
}

export function renderSimpleMarkdown(source: string) {
  const normalized = source.replace(/\r\n/g, "\n").trim();
  if (!normalized) {
    return "";
  }

  const lines = normalized.split("\n");
  const html: string[] = [];
  let paragraph: string[] = [];
  let listItems: string[] = [];

  function flushParagraph() {
    if (paragraph.length === 0) {
      return;
    }
    html.push(`<p>${paragraph.map((line) => renderInlineMarkdown(line)).join("<br />")}</p>`);
    paragraph = [];
  }

  function flushList() {
    if (listItems.length === 0) {
      return;
    }
    html.push(`<ul>${listItems.map((item) => `<li>${renderInlineMarkdown(item)}</li>`).join("")}</ul>`);
    listItems = [];
  }

  for (const rawLine of lines) {
    const line = rawLine.trimEnd();
    const trimmed = line.trim();

    if (!trimmed) {
      flushParagraph();
      flushList();
      continue;
    }

    const heading = trimmed.match(/^(#{1,3})\s+(.+)$/);
    if (heading) {
      flushParagraph();
      flushList();
      const level = heading[1].length;
      html.push(`<h${level}>${renderInlineMarkdown(heading[2].trim())}</h${level}>`);
      continue;
    }

    if (trimmed.startsWith("- ")) {
      flushParagraph();
      listItems.push(trimmed.slice(2).trim());
      continue;
    }

    flushList();
    paragraph.push(trimmed);
  }

  flushParagraph();
  flushList();

  return html.join("");
}
