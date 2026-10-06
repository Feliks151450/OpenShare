import { isSafeImageUrlForSrc, resolveMarkdownImageUrlToHref } from "./markdown-shared";

const imageWidthRe = /^(.*?)\|width=(\d+%?)\s*$/;

/**
 * Lute 的纯预览输出不带本站的 Markdown 约定（相对图址、`![alt|width=800]`、外链新窗口），
 * 这里在写入 DOM 之前补齐规则，使 vditor 预览与原 marked 渲染口径一致。
 */
export function rewriteVditorPreviewHtml(html: string): string {
  const template = document.createElement("template");
  template.innerHTML = html;

  template.content.querySelectorAll("img").forEach((img) => {
    const rawSrc = img.getAttribute("src") ?? "";
    const rawAlt = img.getAttribute("alt") ?? "";

    // Lute 会把 :smile: 之类的短码转成 ${cdn}/dist/images/emoji/*.png，
    // 自托管资源里没有这套图，退回 alt 文本，避免整页碎图。
    if (rawSrc.includes("/dist/images/emoji/")) {
      img.replaceWith(document.createTextNode(rawAlt || rawSrc));
      return;
    }

    const widthMatch = rawAlt.match(imageWidthRe);
    const altText = widthMatch ? widthMatch[1].trimEnd() : rawAlt;

    if (!isSafeImageUrlForSrc(rawSrc)) {
      img.replaceWith(document.createTextNode(`![${altText}](${rawSrc})`));
      return;
    }

    // 先改 src 再补属性，避免浏览器带着相对地址先发一次必定 404 的请求
    img.setAttribute("src", resolveMarkdownImageUrlToHref(rawSrc));
    img.setAttribute("alt", altText);
    img.setAttribute("loading", "lazy");
    img.setAttribute("decoding", "async");
    img.classList.add("markdown-img");
    if (widthMatch) {
      img.style.maxWidth = widthMatch[2].endsWith("%") ? widthMatch[2] : `${widthMatch[2]}px`;
    }
  });

  template.content.querySelectorAll("a[href]").forEach((anchor) => {
    const href = anchor.getAttribute("href") ?? "";
    if (/^https?:\/\//i.test(href)) {
      anchor.setAttribute("target", "_blank");
      anchor.setAttribute("rel", "noopener noreferrer");
    }
  });

  return template.innerHTML;
}
