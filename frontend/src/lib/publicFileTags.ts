import { httpClient } from "./http/client";

/** 与后端 `PublicFileTag` / 预设标签 API 对齐 */

export interface PublicFileTag {
  id: string;
  name: string;
  color: string;
}

export interface FileTagDefinition extends PublicFileTag {
  sort_order: number;
}

export async function fetchPublicFileTagDefinitions(): Promise<FileTagDefinition[]> {
  const res = await httpClient.get<{ items: FileTagDefinition[] }>("/public/file-tags");
  return res.items ?? [];
}

/** #rgb / #rrggbb，返回适合置于该底色上的前景色 */
export function readableTextColorForPreset(hex: string): "#0f172a" | "#f8fafc" {
  const raw = hex.trim().replace(/^#/, "");
  if (raw.length !== 3 && raw.length !== 6) {
    return "#0f172a";
  }
  const expand = raw.length === 3 ? raw.split("").map((c) => c + c).join("") : raw;
  const n = parseInt(expand, 16);
  if (!Number.isFinite(n)) {
    return "#0f172a";
  }
  const r = (n >> 16) & 255;
  const g = (n >> 8) & 255;
  const b = n & 255;
  const luminance = (0.299 * r + 0.587 * g + 0.114 * b) / 255;
  return luminance > 0.62 ? "#0f172a" : "#f8fafc";
}
