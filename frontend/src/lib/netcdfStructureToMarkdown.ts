/** 与后端 `NetCDFDump*` JSON 对齐，用于生成可渲染的 Markdown。 */

export interface NetCDFDumpAttr {
  key: string;
  value: string;
}

export interface NetCDFDumpDim {
  name: string;
  size: number;
}

export interface NetCDFDumpVar {
  name: string;
  type: string;
  dimensions?: string[];
  shape?: number[];
  attributes?: NetCDFDumpAttr[];
  unreadable?: boolean;
  values?: string[];
  values_truncated?: boolean;
}

export interface NetCDFDumpGroup {
  path: string;
  global_attributes?: NetCDFDumpAttr[];
  dimensions?: NetCDFDumpDim[];
  variables?: NetCDFDumpVar[];
  subgroups?: NetCDFDumpGroup[];
}

function escapeHtmlCell(s: string): string {
  return s
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

/** 属性表折叠块（details 内为 HTML 表格）。summaryInnerHtml 必须为已转义/可信片段。 */
function attributesDisclosureHtml(attrs: NetCDFDumpAttr[], summaryInnerHtml: string): string {
  const rows = attrs
    .map(
      (a) =>
        `<tr><td>${escapeHtmlCell(a.key)}</td><td>${escapeHtmlCell(a.value)}</td></tr>`,
    )
    .join("");
  return (
    `<details class="netcdf-attrs-disclosure">\n` +
    `<summary class="netcdf-attrs-disclosure-summary">${summaryInnerHtml}</summary>\n` +
    `<div class="netcdf-attrs-disclosure-body">\n` +
    `<table>\n<thead><tr><th>名称</th><th>值</th></tr></thead>\n` +
    `<tbody>${rows}</tbody>\n</table>\n` +
    `</div>\n` +
    `</details>`
  );
}

function globalAttributesDisclosureHtml(attrs: NetCDFDumpAttr[]): string {
  const n = attrs.length;
  return attributesDisclosureHtml(
    attrs,
    `全局属性 <span class="netcdf-attrs-disclosure-count">（${n} 项）</span>`,
  );
}

function variableAttributesDisclosureHtml(v: NetCDFDumpVar): string {
  const attrs = v.attributes ?? [];
  const n = attrs.length;
  return attributesDisclosureHtml(
    attrs,
    `属性 <span class="netcdf-attrs-disclosure-varname">${escapeHtmlCell(v.name)}</span> <span class="netcdf-attrs-disclosure-count">（${n} 项）</span>`,
  );
}

/** 一维变量值折叠块，以可读形式展示数值。 */
function variableValuesDisclosureHtml(v: NetCDFDumpVar): string {
  const vals = v.values ?? [];
  if (vals.length === 0) return "";
  const n = vals.length;
  const truncatedNote = v.values_truncated
    ? ` <span class="netcdf-var-values-truncated-note">（仅展示前 ${n} 项）</span>`
    : "";
  const summary = `变量值 <span class="netcdf-attrs-disclosure-count">（${n} 项）</span>${truncatedNote}`;
  const valuesList = vals.map((val) => escapeHtmlCell(val)).join(", ");
  return (
    `<details class="netcdf-attrs-disclosure netcdf-var-values-disclosure">\n` +
    `<summary class="netcdf-attrs-disclosure-summary">${summary}</summary>\n` +
    `<div class="netcdf-var-values-body">${valuesList}</div>\n` +
    `</details>`
  );
}

function variableCardHtml(v: NetCDFDumpVar): string {
  const vDims = v.dimensions ?? [];
  const sh = v.shape ?? [];
  const vAttr = v.attributes ?? [];

  const metaParts: string[] = [];
  if (vDims.length > 0) {
    const dimText = vDims.map((d) => escapeHtmlCell(d)).join("，");
    metaParts.push(
      `<div class="netcdf-var-card-meta-row"><span class="netcdf-var-card-meta-label">维度</span><span class="netcdf-var-card-meta-value">${dimText}</span></div>`,
    );
  }
  if (sh.length > 0) {
    metaParts.push(
      `<div class="netcdf-var-card-meta-row"><span class="netcdf-var-card-meta-label">形状</span><span class="netcdf-var-card-meta-value">${escapeHtmlCell(shapeLabel(sh))}</span></div>`,
    );
  }
  const metaHtml =
    metaParts.length > 0 ? `<div class="netcdf-var-card-meta">${metaParts.join("")}</div>` : "";

  const attrsHtml = vAttr.length > 0 ? variableAttributesDisclosureHtml(v) : "";
  const valuesHtml = variableValuesDisclosureHtml(v);
  const bodyInner = [valuesHtml, metaHtml, attrsHtml].filter(Boolean).join("\n");

  return (
    `<section class="netcdf-var-card">\n` +
    `<header class="netcdf-var-card-head">\n` +
    `<span class="netcdf-var-card-name">${escapeHtmlCell(v.name)}</span>\n` +
    `<span class="netcdf-var-card-type">${escapeHtmlCell(v.type)}</span>\n` +
    `</header>\n` +
    `<div class="netcdf-var-card-body">\n` +
    bodyInner +
    `</div>\n` +
    `</section>`
  );
}

function variableUnreadableCardHtml(v: NetCDFDumpVar): string {
  return (
    `<section class="netcdf-var-card netcdf-var-card--unreadable">\n` +
    `<header class="netcdf-var-card-head">\n` +
    `<span class="netcdf-var-card-name">${escapeHtmlCell(v.name)}</span>\n` +
    `<span class="netcdf-var-card-type netcdf-var-card-type--muted">无法读取</span>\n` +
    `</header>\n` +
    `<div class="netcdf-var-card-body">\n` +
    `<p class="netcdf-var-card-unreadable-msg">无法读取该变量的结构与属性。</p>\n` +
    `</div>\n` +
    `</section>`
  );
}

function mdInlineCode(s: string): string {
  const t = s.replace(/`/g, "'");
  return `\`${t}\``;
}

function mdTableCell(s: string): string {
  return s.replace(/\|/g, "\\|").replace(/\n/g, " ");
}

function shapeLabel(shape: number[]): string {
  if (!shape.length) {
    return "—";
  }
  return shape.map((n) => String(n)).join(" × ");
}

function appendGroup(lines: string[], g: NetCDFDumpGroup, depth: number): void {
  const hGroup = Math.min(2 + depth, 6);
  const pathTitle =
    g.path === "/" || g.path === ""
      ? "根组（`/`）"
      : `组 ${mdInlineCode(g.path)}`;
  lines.push(`${"#".repeat(hGroup)} ${pathTitle}`, "");

  const hSec = Math.min(hGroup + 1, 6);

  const attrs = g.global_attributes ?? [];
  if (attrs.length > 0) {
    lines.push(globalAttributesDisclosureHtml(attrs), "");
  }

  const dims = g.dimensions ?? [];
  if (dims.length > 0) {
    lines.push(`${"#".repeat(hSec)} 维度`, "");
    lines.push("| 名称 | 长度 |", "| --- | --- |");
    for (const d of dims) {
      lines.push(`| ${mdTableCell(d.name)} | ${d.size} |`);
    }
    lines.push("");
  }

  const vars = g.variables ?? [];
  if (vars.length > 0) {
    lines.push(`${"#".repeat(hSec)} 变量`, "");
    for (const v of vars) {
      if (v.unreadable) {
        lines.push(variableUnreadableCardHtml(v), "");
        continue;
      }
      lines.push(variableCardHtml(v), "");
    }
  }

  const subs = g.subgroups ?? [];
  for (const sub of subs) {
    appendGroup(lines, sub, depth + 1);
  }
}

/** 将 API 返回的 `structure` 转为 Markdown，供 `renderSimpleMarkdown` 使用。 */
export function netcdfStructureToMarkdown(root: NetCDFDumpGroup): string {
  const lines: string[] = [];
  appendGroup(lines, root, 0);
  let out = lines.join("\n");
  out = out.replace(/\n{3,}/g, "\n\n");
  return out.trimEnd() + "\n";
}
