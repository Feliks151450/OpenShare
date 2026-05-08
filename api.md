# OpenShare 浏览器控制台 API（`window.OpenShare`）

在 **公开访客页面**（无管理端权限要求）加载完成后，开发者工具控制台可通过全局对象 `window.OpenShare` 做程序化 **路由跳转**、按需 **请求公开文件元数据**，以及首页 **列表展示偏好**。  
不入库、不发起登录逻辑；不涉及上传、管理员接口或删除资源。

实现位置：

- **SPA**：`frontend` 打包的主站，`main.ts` 挂载后可用。
- **只读静态页**：`frontend/standalone-readonly/readonly.js`（hash 路由，便于静态／`file://` 部署）。

两端的 **方法签名与语义尽量一致**；下文先写共有约定，再标出差异。

---

## 顶层字段

| 字段 | 类型 | 含义 |
|------|------|------|
| `version` | `string` | 当前文档对应实现版本：**`1.0`**。 |
| `runtime` | `'spa' \| 'readonly'` | **`spa`**：Vue Router + History；**`readonly`**：hash 路由。 |
| `nav` | object | 页面导航相关。 |
| `home` | object | 首页目录列表视图／排序偏好（不涉及搜索关键词等临时状态）。 |
| `staticData` | `StaticDataLoader` | CDN 静态数据加载器，可配置预导出 JSON 直链以替代部分公开 API 请求。 |

---

## `OpenShare.nav`

### `getRoute()`

返回当前路由的只读快照（控制台查看或脚本分支用）。

**SPA**

- **返回**：`{ name, path, fullPath, params, query }`
  - `name`：`vue-router` 路由名字符串，例如 `public-home`、`public-file-detail`。
  - `path` / `fullPath`：与 Vue Router 一致。
  - `params` / `query`：字典，值均已规范为 **`string`**（多值查询取第一项）。

**只读静态页**

- **返回**：`parseHashRoute()` 得到的对象，并附加 **`hash`**（当前完整的 `location.hash`）。
  - `view`: `'home' | 'file'`
  - 首页：`folder`、`root`（`'1'` 表示根视图）、`fileId`、`t` 在文件视图中使用。
  - 文件页：`fileId`、`t`（时间戳查询字符串，可为空字符串）。

首页 hash 形如：`#/`、`#/?folder=<id>`、`#/?root=1`。  
文件详情：`#/files/<fileId>?t=<秒>`（`t` 可选）。

---

### `goHome(opts?)`

跳到「公开首页」对应路由。

**参数 `opts`**（可选，均为可选字段）：

| 字段 | 类型 | 说明 |
|------|------|------|
| `folder` | `string` | 指定目录 **`folder` id**，进入该目录视图。与非空 **`root` 不同时建议只传其一；若二者都出现，当前实现 **`folder` 优先**。 |
| `root` | `boolean` | 为真时等价于站内「返回根视图」：`?root=1`（只读：`root=1`）。 |
| `replace` | `boolean` | 为真时用 **替换** 当前历史条目（不产生新的「后退」步）。 |

**不传 `folder` 且 `root` 不为真**：进入无前缀首页（无 `folder` / `root` 查询）。

**返回**：`Promise<void>`（便于在控制台里写 `await`）。

**SPA**：内部使用路由名 **`public-home`** 与对应 `query`。

**只读**：改写 `location.hash`；若 `replace: true`，则 `history.replaceState` 后触发内部 **`bootstrapRoute()`**（不会在 `readonly` 下重复触发两次 `hashchange`）。

---

### `goFile(fileID, opts?)`

打开公开 **文件详情**。

**`fileID`**：资源 id 字符串（空串会报错或 `reject`，见下文）。

**`opts`**：

| 字段 | 类型 | 说明 |
|------|------|------|
| `t` | `string \| number` | 播放时间戳，对应详情页 **`?t=`**（秒，可为小数；与站内行为一致）。空值不传该查询参数。 |
| `replace` | `boolean` | 同 `goHome`。 |

**返回**：`Promise<void>`；**无效 `fileID`** 时 **SPA `throw`**，**只读 `Promise.reject(Error)`**。

**SPA**：路由名 **`public-file-detail`**，参数 **`fileID`**。

**只读**：`#/files/<fileId>?t=...`。

---

### `goUpload(opts?)`

进入 **上传／回执** 访客页对应的 SPA 路径 **`/upload`**。

**参数**：`opts?: { replace?: boolean }`。  
**返回**：`Promise<void>`。

**只读静态页**：**不提供** `/upload`。方法返回 **`Promise.resolve(false)`**；并在 **首次调用**时在控制台 **`console.warn`** 说明静态页不包含该路由。**无 `opts`** 重载兼容。

---

### `back()`

调用 **`history.back()`**，与站内「后退」按钮一致性取决于用户实际历史栈。

**返回**：`void`。

---

### `getFileInfo(fileID)`

按需 **`GET /api/public/files/<fileID>`**，返回控制台友好的 **camelCase** 摘要对象（不写路由、不写 localStorage）。

**参数**

| 参数 | 类型 | 说明 |
|------|------|------|
| `fileID` | `string` | 必填，公开文件 id |

**返回**：`Promise<OpenSharePublicFileInfo>`

典型字段：

| 字段 | 说明 |
|------|------|
| `name` | 文件名 |
| `extension` | 扩展名字符串 |
| `sizeBytes` | 字节数 |
| `uploadedAt` | 服务端 `uploaded_at` 的 ISO 时间字符串 |
| `folderId`、`path`、`mimeType`、`description`、`remark`、`downloadCount` | 与接口语义一致 |
| `downloadAllowed` | 若为 `false` 表示站点策略不允许「下载」（部分类型仍可走内嵌播放） |
| `playbackUrl`、`playbackFallbackUrl`、`folderDirectDownloadUrl` | 直链／回退／目录前缀导出；无则为 `null` |
| `effectiveDownloadHref` | 与站内播放器／「复制下载直链」一致：**playback > 目录前缀直链 > 本站 `/download`** |
| `effectiveDownloadAbsoluteUrl` | `effectiveDownloadHref` 若为相对路径则用当前页的 `location.origin` 拼成绝对 URL |
| `siteDownloadHref` | 固定形如 **`/api/public/files/<id>/download`** 的本站路径（便于识别是否最终会走站内统计）；**实际可用的下载入口以 `effectiveDownloadHref` 为准** |

错误：非法 `fileID` 时 **SPA 抛错**，只读 **`Promise.reject(Error)`**。HTTP **4xx／5xx** 时由各端 **`fetch`/httpClient** 报错（只读：`HttpError`，可在捕获后读 `payload.error`）。

**只读**：`credentials: omit`；使用当前配置的 **`apiUrl`**。

TypeScript 类型见 **`frontend/src/lib/openSharePublicFileInfo.ts`**（`OpenSharePublicFileInfo`）。

---

### `leaveFileTowardFolder(opts?)`

等价于站内文件详情页的 **「返回到所属目录首页」**：尽量进入 **包含该文件的目录**，否则退回 **公共首页根部**。

**参数**：`opts?: { replace?: boolean }`。

**SPA**

1. 若当前路由名 **不是** `public-file-detail`：**返回 **`Promise.resolve(false)`**，不改变路由。
2. 若是详情页：先 **`GET /api/public/files/<fileID>`**，读取 **`folder_id`**：
   - 有 `folder_id`：`goHome({ folder: folder_id, replace })`
   - 无或请求失败：**`goHome({ replace })`**
3. **返回 **`Promise.resolve(true)`**`。

**注意**：会使用项目内 **`httpClient`**（带 **`credentials: 'include'`**），仅读取公开详情；若需在无痕／跨域环境下自动化，注意 Cookie 与同源策略。

**只读静态页**

1. 若当前 **`parseHashRoute().view !== 'file'`**：**返回 **`Promise.resolve(false)`**。
2. 否则调用与 UI **「详情返回」** 相同逻辑：**优先用内存中已加载的 `state.fileDetail.folder_id`** 跳目录；若没有目录信息则回 **`#/`**。
3. **返回 **`Promise.resolve(true)`**。

「未载入完详情就调用」时，只读场景下可能仍会回到 **`#/`**，与 SPA 会先请求接口的行为略有差别。

---

## `OpenShare.home`

用于 **首页列表**的 **视图模式**（卡片／表格）与 **排序**。  
不涉及搜索框内容、勾选批量下载等资源操作。

以下 **localStorage 键名**与主站 **`Home.vue`** 一致（便于同一浏览器在 SPA 与只读页之间共享偏好）：

| 偏好 | Key |
|------|-----|
| 列表视图 | `public-home-view-mode`：`cards` \| `table` |
| 排序字段 | `public-home-sort-mode`：`name` \| `download` \| `format` \| `modified` |
| 排序方向 | `public-home-sort-direction`：`asc` \| `desc` |

### `setListView(mode)`

- **参数**：`mode`：`"cards"` \| `"table"`。
- **返回**：成功为 **`true`**；非法取值 **`false`** 并 **`console.warn`**。

**SPA**：写入上述 localStorage；若 **首页组件已挂载**（`Home.vue` 注册了桥接回调），则 **同步更新界面**（与点击工具栏一致）。若在其它路由（例如详情），仅持久化偏好，下次进入首页生效。

**只读**：更新内存中的 `state`、写 **同一 key**，若当前已在 **首页视图** 则 **`render()`**。

### `setSortMode(mode)`

- **参数**：`"name"` \| `"download"` \| `"format"` \| `"modified"`。
- **返回**：同上。

语义与站内排序菜单一致：**名称／下载量／格式／修改时间**。

### `setSortDirection(direction)`

- **参数**：`"asc"` \| `"desc"`。
- **返回**：同上。

**只读** 下会顺带关闭 **`state.sortMenuOpen`** 并可能触发 **`render()`**。

---

## 使用示例（控制台）

```js
OpenShare.version;
OpenShare.runtime;

OpenShare.nav.getRoute();

await OpenShare.nav.goHome({ folder: "YOUR_FOLDER_ID" });
await OpenShare.nav.goHome({ root: true });
await OpenShare.nav.goHome();

await OpenShare.nav.goFile("YOUR_FILE_ID", { t: 120.5 });
await OpenShare.nav.goFile("YOUR_FILE_ID", { replace: true });

await OpenShare.nav.goUpload(); // SPA 可用；readonly 中为 no-op + 首次 warn

await OpenShare.nav.leaveFileTowardFolder();

const info = await OpenShare.nav.getFileInfo("YOUR_FILE_ID");
console.log(info.name, info.sizeBytes, info.uploadedAt, info.effectiveDownloadHref);

OpenShare.nav.back();

OpenShare.home.setListView("table");
OpenShare.home.setSortMode("download");
OpenShare.home.setSortDirection("desc");
```

---

## 详情／目录简介里的 Markdown 站内链接

在 **公开文件详情页**（简介、Markdown 文件预览、NetCDF 结构化摘要预览）以及 **首页当前文件夹简介**（含站内编辑简介时的预览区域）中，正文为简单 Markdown；其中 **站内超链接**在用户 **普通左键单击**时会 **走站内路由**，避免整页刷新；**按住 Ctrl／Cmd（新标签）、中键、`mailto:`、`tel:`、`http(s):` 外链**等保持浏览器默认。

链接目标先按 **`new URL(链接, 当前页的 location.href)`** 解析为绝对地址，再在 **与同页同源**（含 **`file://` 静态页与同源规则**）前提下匹配下文路径；语义实现见 **`frontend/src/lib/publicMarkdownLinks.ts`**（SPA）与只读 **`readonly.js`** 内 **`tryMarkdownHrefToReadonlyHashRoute`**（需与 SPA 对齐时一并改两处）。

### 可识别为站内跳转的写法（示例）

| 语义 | SPA 路径（解析结果） | Markdown 写法示例 |
|------|---------------------|-------------------|
| 另一文件详情 | **`/files/<fileID>`**，可选 **`?t=秒`** | `[标签](/files/<fileID>?t=120)`、`[标签](./<同目录其它 fileID>)`（当前在 **`/files/…`** 时 `./` 会落在同一 **`/files/…`** 段下） |
| 首页 — 指定目录 | **`/`** + 查询 **`folder=<folderID>`** | `[目录](/?folder=<folderID>)`、`[目录](../?folder=<folderID>)`（从 **`/files/…`** 进首页带 query） |
| 首页 — 根视图 | **`/`** + **`root=1`** | `[根目录](/?root=1)` |
| 首页 — 无前缀首页 | **`/`**，无额外 query | `[首页](/)` |
| 上传访客页（仅 SPA） | **`/upload`** | **`[上传](/upload)`** |

首页查询串 **仅识别** **`folder`** 与 **`root`**；若还带其它参数，当前实现 **不按站内路由拦截**（走默认 `<a>` 行为）。

**只读**：上述绝对路径在 **`file://`** 或带 **hash** 的真实 URL 下同样先经 **`URL` 解析**；站内命中后改为 **`#/files/…`、`#/?folder=…`** 等。只读页 **没有** `/upload`，指向 **`/upload`** 的 Markdown 链接 **不会**被改成 hash 跳转。

**控制台**：若在自动化里需要程序化跳转，请继续用 **`OpenShare.nav.goFile` / `goHome`**；Markdown 出站链与本文所述行为一致，等价于与用户点击对齐的浏览器侧 **`URL`** 解析（含相对路径）。

---

## 公开目录「隐藏托管根」与相关 API

管理员可将 **托管根目录**（`parent_id` 为空）标记为 **不在公开目录中展示**（字段 `hide_public_catalog`）。该设置影响的是 **发现与聚合类** 公开接口，**不会**拦掉 **已知 id 的直达访问**（与站内直链、书签、`OpenShare.nav.getFileInfo` / `goFile` 等场景一致）。

**会排除**「隐藏托管根」及其 **整棵子目录** 下资源的接口示例：

| 接口 | 说明 |
|------|------|
| `GET /api/public/folders` | 仅当 **根视图**（无 `parent_id` 或等价「列根」）时，不返回被隐藏的托管根；进入某 **已列出的** 子目录后，列子项行为与此前一致。 |
| `GET /api/public/search` | 搜索结果中不出现落在隐藏托管根子树内的文件与目录。 |
| `GET /api/public/files/hot` | 热门列表不包含上述子树内的文件。 |
| `GET /api/public/files/latest` | 上新／最近列表不包含上述子树内的文件。 |

**不做上述「目录发现过滤」** 的典型情况（仍可 404 等已由业务定义的校验）：

- 按 **`fileID`** 读取元数据：**`GET /api/public/files/<fileID>`**（即控制台 **`getFileInfo`** 所用）。
- 按约定路径的 **本站下载**：**`GET /api/public/files/<fileID>/download`** 等（以实际路由为准）。
- 按 **`folderID`** 拉取 **目录详情**、列目录内文件等 **已知 id** 的公开接口：服务端不会因「根被隐藏」而单独拒绝；访客若拿不到 id，通常仍无法从未过滤的首页／搜索／热门进入该树。

SPA 与只读静态页共用同一套后端行为，控制台脚本无需分叉处理。

---

## `OpenShare.staticData`

CDN 静态数据加载器，可配置预导出的 JSON 文件直链，命中时无需请求源服务器 API。未命中时调用方可降级为实时 HTTP 请求。

**类型**：`StaticDataLoader`（单例），定义于 `frontend/src/lib/staticDataLoader.ts`。

### `staticData.configure(config)`

配置 CDN 直链地址。

**`config`**：

| 字段 | 类型 | 说明 |
|------|------|------|
| `globalUrl` | `string` | 全局数据 JSON 文件的 CDN 直链。 |
| `directoryBaseUrl` | `string` | 目录 JSON 文件的基础 URL，文件名格式为 `{目录名}.json`。 |

```js
OpenShare.staticData.configure({
  globalUrl: "https://cdn.example.com/openshare-global-2026-05-09.json",
  directoryBaseUrl: "https://cdn.example.com/directories",
});
```

### `staticData.loadGlobal(url?)`

从 CDN 加载全局数据文件，返回 `Promise<boolean>`（成功 `true`）。

### `staticData.loadDirectory(urlOrName, url?)`

从 CDN 加载单个托管目录的数据文件，返回 `Promise<boolean>`。两种用法：

**直接传完整 URL（无需配置 `directoryBaseUrl`）**：

```js
await OpenShare.staticData.loadDirectory("https://cdn.example.com/my-folder.json");
const view = OpenShare.staticData.getDirectoryView("folder-id-xxx");
```

**传文件名 + 已配置 `directoryBaseUrl`**（加载多个目录时更简洁）：

```js
OpenShare.staticData.configure({
  directoryBaseUrl: "https://cdn.example.com/directories",
});
// 自动拼接为 https://cdn.example.com/directories/my-folder.json
await OpenShare.staticData.loadDirectory("my-folder");
await OpenShare.staticData.loadDirectory("another-folder");
```

### 数据访问器

所有访问器在数据未加载时返回 `null`。

| 访问器 | 返回值 | 对应 API |
|--------|--------|----------|
| `staticData.hasGlobal` | `boolean` | — |
| `staticData.globalLoading` | `boolean` | — |
| `staticData.globalError` | `string \| null` | — |
| `staticData.globalExportedAt` | `string \| null` | — |
| `staticData.announcements` | `ExportAnnouncement[] \| null` | `GET /public/announcements` |
| `staticData.hotFiles` | `ExportHotFiles \| null` | `GET /public/files/hot` |
| `staticData.latestFiles` | `ExportLatestFiles \| null` | `GET /public/files/latest` |
| `staticData.rootFolders` | `ExportPublicFolderItem[] \| null` | `GET /public/folders` |
| `staticData.downloadPolicy` | `ExportDownloadPolicy \| null` | `GET /public/download-policy` |
| `staticData.fileTags` | `ExportFileTag[] \| null` | `GET /public/file-tags` |

### 目录数据访问器

| 访问器 | 说明 |
|--------|------|
| `staticData.hasDirectory(id)` | 该托管根目录数据是否已加载 |
| `staticData.getManagedRoot(id)` | 获取托管根元信息 |
| `staticData.getDirectoryView(folderId)` | 获取某个文件夹的视图数据（详情 + 子文件夹 + 文件） |
| `staticData.findDirectoryView(folderId)` | 在所有已加载目录数据中查找某个文件夹 ID |
| `staticData.getFolderIdsForManagedRoot(managedRootId)` | 获取托管树下所有已缓存的文件夹 ID |

### 使用示例

```js
// ── 加载全局数据 ──
OpenShare.staticData.configure({
  globalUrl: "https://cdn.example.com/openshare-global.json",
});
await OpenShare.staticData.loadGlobal();

// 直接读取，无需请求服务器
const folders = OpenShare.staticData.rootFolders;
const tags = OpenShare.staticData.fileTags;

// ── 加载单个托管目录（直接传 URL，无需配置 baseUrl）──
await OpenShare.staticData.loadDirectory("https://cdn.example.com/my-folder.json");
const view = OpenShare.staticData.getDirectoryView("folder-id-xxx");
if (view) {
  console.log(view.detail, view.folders, view.files);
}

// ── 加载多个托管目录（先配置 baseUrl，再传文件名）──
OpenShare.staticData.configure({
  directoryBaseUrl: "https://cdn.example.com/directories",
});
await OpenShare.staticData.loadDirectory("dir-a");  // → .../directories/dir-a.json
await OpenShare.staticData.loadDirectory("dir-b");  // → .../directories/dir-b.json
```

---

## 管理端导出接口

管理员可在后台"系统配置 → 当前已托管文件目录"区域导出静态 JSON 文件，上传至 CDN 后供前端 `staticData` 加载。

所有导出接口需 **admin 登录态**（`credentials: include`）。

### `GET /api/admin/export/global`

导出全局公开数据，包含 announcements、hot_files、latest_files、root_folders、download_policy、file_tags。

**响应**：

```json
{
  "version": 1,
  "exported_at": "2026-05-09T12:00:00Z",
  "announcements": [...],
  "hot_files": { "items": [...] },
  "latest_files": { "items": [...] },
  "root_folders": [...],
  "download_policy": {
    "large_download_confirm_bytes": 1073741824,
    "wide_layout_extensions": "md,markdown"
  },
  "file_tags": [...]
}
```

### `GET /api/admin/export/directory/:folderID`

导出指定托管目录的完整数据，包含该目录树下每个子目录的详情、子文件夹和全部文件（自动分页拉全）。

**路径参数**：`folderID` — 托管根目录的 ID。

**响应**：

```json
{
  "version": 1,
  "exported_at": "2026-05-09T12:00:00Z",
  "managed_root": {
    "id": "...",
    "name": "...",
    "source_path": "/data/...",
    "file_count": 42,
    "download_count": 1000,
    "total_size": 123456789
  },
  "directories": {
    "<folderId>": {
      "detail": { ... },
      "folders": [ ... ],
      "files": [ ... ]
    }
  }
}
```

`directories` 为 `folderId → { detail, folders, files }` 的映射，detail 为 `GET /public/folders/:id` 响应体，folders 为 `GET /public/folders?parent_id=:id` 的 items 数组，files 为该目录下全部文件（`GET /public/folders/:id/files` 的分页聚合）。

### 使用流程

1. 管理员在后台点击"导出全局数据"按钮，下载 `openshare-global-YYYY-MM-DD.json`
2. 管理员点击各托管目录行的"导出数据"按钮，下载 `{目录名}.json`
3. 将 JSON 文件上传至 CDN
4. 前端在 `main.ts` 或初始化脚本中调用 `staticData.configure()` + `loadGlobal()` / `loadDirectory()`
5. 后续 API 数据优先从 `staticData` 读取，未命中时降级为真实 HTTP 请求

---

## 兼容性说明摘要

| 能力 | SPA | 只读 |
|------|-----|------|
| 路由基础 | `/`、`/upload`、`/files/:id`，History API | **`#/`**、 **`#/files/:id`** |
| `goUpload` | 支持 | 忽略并首次 warn |
| `leaveFileTowardFolder` | 详情页：`GET /public/files/:id` | 用语义内 **`state.fileDetail`**；无则用 `#/` |
| `getFileInfo` | `GET`，`credentials: include`，同源 `/api` | `GET`，`credentials: omit`，走 **`apiUrl(...)`** |
| `replace` | `router.replace` | `replaceState` + `bootstrapRoute` |
| 路由重复（同目标） | 忽略 `NavigationDuplicated`，其它 `console.warn` | 由 hash／replace 实现决定 |
| 简介／预览区 Markdown 站内链（左键） | `publicMarkdownLinks` → **`router.push`** | `tryMarkdownHrefToReadonlyHashRoute` → **`setHashRoute`**；`/upload` 不拦截 |

若你在扩展更多控制台能力，请同时更新 **`frontend/src/lib/openShareConsole.ts`**、**`frontend/src/lib/openSharePublicFileInfo.ts`** 与 **`frontend/standalone-readonly/readonly.js`**，并同步本文档。
