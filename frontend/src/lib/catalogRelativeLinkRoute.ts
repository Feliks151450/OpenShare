import type { RouteLocationRaw } from "vue-router";

import { httpClient } from "./http/client";

type ResolveCatalogPathResponse = {
  type?: string;
  folder_id?: string;
  file_id?: string;
  name?: string;
};

/**
 * 以 anchorFolderID 为锚点，把 Markdown 相对链（`nc/`、`../笔记.md`）按名称解析为站内路由。
 * anchorFolderID 为空表示根目录（公告等无所属目录的内容）。
 *
 * 解析不到、目标需要访客密钥、或请求失败时一律返回 null，由调用方回退到浏览器默认行为。
 */
export async function resolveRelativeCatalogLinkRoute(
  relPath: string,
  anchorFolderID: string,
): Promise<RouteLocationRaw | null> {
  const params = new URLSearchParams({ path: relPath });
  const anchor = (anchorFolderID ?? "").trim();
  if (anchor !== "") {
    params.set("folder_id", anchor);
  }

  let resp: ResolveCatalogPathResponse;
  try {
    resp = await httpClient.get<ResolveCatalogPathResponse>(
      `/public/resolve-catalog-path?${params.toString()}`,
    );
  } catch {
    return null;
  }

  if (resp.type === "folder") {
    const folderID = (resp.folder_id ?? "").trim();
    return folderID ? { name: "public-home", query: { folder: folderID } } : null;
  }
  if (resp.type === "file") {
    const fileID = (resp.file_id ?? "").trim();
    return fileID ? { name: "public-file-detail", params: { fileID } } : null;
  }
  return null;
}
