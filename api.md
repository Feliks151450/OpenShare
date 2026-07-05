# OpenShare 浏览器控制台 API（`window.OpenShare`）

在 **公开访客页面**（无管理端权限要求）加载完成后，开发者工具控制台可通过全局对象 `window.OpenShare` 做程序化 **路由跳转**、按需 **请求公开文件元数据**，以及首页 **列表展示偏好**。  
不入库、不发起登录逻辑；不涉及上传、管理员接口或删除资源。

实现位置：

- **SPA**：`frontend` 打包的主站，`main.ts` 挂载后可用。

---

## 顶层字段

| 字段 | 类型 | 含义 |
|------|------|------|
| `version` | `string` | 当前文档对应实现版本：**`1.0`**。 |
| `runtime` | `'spa'` | **`spa`**：Vue Router + History |
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

## `OpenShare.favorites`

基于 **localStorage** 的收藏管理，浏览器本地存储，不涉及后端 API。

**存储 key**：`openShareFavorites`  
**数据格式**：`[{ id: string, kind: "file" | "folder" }]`

### `list()`

获取所有收藏项。

- **返回**：`FavoriteItem[]`，每项包含 `id`（资源 ID）和 `kind`（`"file"` 或 `"folder"`）。

### `has(id)`

判断某个资源是否已收藏。

- **参数**：`id`：资源 ID 字符串。
- **返回**：`boolean`。

### `add(id, kind)`

添加收藏。

- **参数**：
  - `id`：资源 ID 字符串（必填）。
  - `kind`：`"file"` 或 `"folder"`（必填）。
- **返回**：成功为 `true`；参数无效返回 `false` 并 `console.warn`；已存在视为成功。

### `remove(id)`

移除收藏。

- **参数**：`id`：资源 ID 字符串。
- **返回**：成功移除返回 `true`；不存在返回 `false`。

### `toggle(id, kind)`

切换收藏状态：已收藏则取消，未收藏则添加。

- **参数**：同 `add`。
- **返回**：`boolean`，操作后的新状态（`true` 表示已收藏）。

### `set(items)`

一次性设置整个收藏列表（覆盖现有内容）。

- **参数**：`items`：`FavoriteItem[]`，每项包含 `id`（资源 ID）和 `kind`（`"file"` 或 `"folder"`）。
- **返回**：无。

**示例**：

```js
// 批量设置收藏
OpenShare.favorites.set([
  { id: "file-1", kind: "file" },
  { id: "folder-1", kind: "folder" },
  { id: "file-2", kind: "file" }
]);

// 覆盖为单个收藏
OpenShare.favorites.set([{ id: "file-1", kind: "file" }]);
```

### `clear()`

清空所有收藏。无返回值。

### `count()`

获取收藏数量。

- **返回**：`number`。

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

// 收藏管理
OpenShare.favorites.list();                    // 获取所有收藏
OpenShare.favorites.has("YOUR_FILE_ID");       // 判断是否已收藏
OpenShare.favorites.add("YOUR_FILE_ID", "file");      // 收藏文件
OpenShare.favorites.add("YOUR_FOLDER_ID", "folder");  // 收藏文件夹
OpenShare.favorites.remove("YOUR_FILE_ID");    // 取消收藏
OpenShare.favorites.toggle("YOUR_FILE_ID", "file");   // 切换收藏状态
OpenShare.favorites.count();                   // 收藏数量
OpenShare.favorites.clear();                   // 清空所有收藏

// 批量设置收藏（覆盖现有内容）
OpenShare.favorites.set([
  { id: "YOUR_FILE_ID", kind: "file" },
  { id: "YOUR_FOLDER_ID", kind: "folder" }
]);
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

# 管理端导出接口

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

---

# 公开 API


### 目录与文件

#### `GET /api/public/folders`

列出根目录或指定父目录下的子文件夹。

| 参数 | 类型 | 说明 |
|------|------|------|
| `parent_id` | `string` | 可选，父文件夹 ID。为空时列根目录 |

**响应**：
```json
{
  "items": [{
    "id": "uuid",
    "name": "文件夹名",
    "description": "...",
    "remark": "...",
    "cover_url": "...",
    "is_virtual": false,
    "updated_at": "ISO8601",
    "file_count": 42,
    "download_count": 100,
    "total_size": 123456
  }],
  "download_policy": { ... }
}
```

#### `GET /api/public/folders/:folderID`

获取文件夹详情（含面包屑导航、下载策略等）。

**响应**：
```json
{
  "id": "uuid",
  "name": "...",
  "description": "...",
  "remark": "...",
  "cover_url": "...",
  "parent_id": "uuid|null",
  "file_count": 42,
  "download_count": 100,
  "total_size": 123456,
  "updated_at": "ISO8601",
  "direct_link_prefix": "...",
  "download_allowed": true,
  "download_policy": "allow|deny|inherit",
  "is_virtual": false,
  "hide_public_catalog": true,
  "custom_path": "doc",
  "hide_file_extension": "inherit|hide|show",
  "hide_file_extension_resolved": false,
  "breadcrumbs": [{ "id": "uuid", "name": "..." }]
}
```
> `hide_file_extension` 为原始设置值；`hide_file_extension_resolved` 为沿祖先链解析后的最终值（`true` 表示隐藏后缀，默认 `false`）。

#### `GET /api/public/folders/:folderID/files`

列出文件夹内的文件（分页）。

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `page` | `int` | 1 | 页码 |
| `page_size` | `int` | 20 | 每页条数（上限 **100**） |
| `sort` | `string` | `created_at_desc` | `name_asc` / `download_count_desc` / `created_at_desc` |

**响应**：
```json
{
  "items": [{
    "id": "uuid",
    "name": "文件名",
    "description": "...",
    "remark": "...",
    "extension": ".pdf",
    "cover_url": "...",
    "playback_url": "...",
    "proxy_download": false,
    "proxy_source_url": "...",
    "folder_direct_download_url": "...",
    "download_allowed": true,
    "uploaded_at": "ISO8601",
    "download_count": 10,
    "size": 123456,
    "tags": [{ "id": "uuid", "name": "教材", "color": "#..." }]
  }],
  "page": 1,
  "page_size": 20,
  "total": 42
}
```

#### `GET /api/public/files/:fileID`

获取文件详情。

**响应**：
```json
{
  "id": "uuid",
  "name": "文件名",
  "extension": ".pdf",
  "folder_id": "uuid",
  "path": "目录/子目录",
  "description": "...",
  "remark": "...",
  "mime_type": "application/pdf",
  "playback_url": "...",
  "playback_fallback_url": "...",
  "proxy_download": false,
  "proxy_source_url": "...",
  "cover_url": "...",
  "folder_direct_download_url": "...",
  "download_allowed": true,
  "download_policy": "inherit|allow|deny",
  "custom_path": "doc/report",
  "download_path": "cdn/icon",
  "size": 123456,
  "uploaded_at": "ISO8601",
  "download_count": 10,
  "tags": [...]
}
```

> `path` 为展示用路径（段间带空格，如 `"cdn / icon"`）；`download_path` 为下载直链拼接用路径（无空格，如 `"cdn/icon"`），对应 `GET /dl/*path` 接口。

#### `GET /api/public/files/:fileID/download`

下载文件（302 重定向到直链或流式返回）。以下文件类型支持 `?inline=1` 在浏览器中内嵌预览：视频/音频、图片、PDF、HTML、纯文本、Markdown、CSV 等。

#### `GET /api/public/folders/:folderID/download`

打包下载整个文件夹（ZIP），返回流式响应。

#### `GET /api/public/files/hot`

热门下载排行（近七天），可选 `?limit=N`（默认 20）。

#### `GET /api/public/files/latest`

最新上传文件，可选 `?limit=N`（默认 20）。

#### `POST /api/public/files/batch-download`

批量下载，请求体为 `{ "file_ids": ["uuid1", "uuid2"] }`。

#### `POST /api/public/resources/batch-download`

批量下载（含文件夹），请求体为 `{ "file_ids": [...], "folder_ids": [...] }`。

### 搜索

#### `GET /api/public/search`

| 参数 | 类型 | 说明 |
|------|------|------|
| `q` | `string` | 搜索关键词 |
| `page` | `int` | 页码（默认 1） |
| `page_size` | `int` | 每页条数 |

**响应**：`{ "files": [...], "folders": [...], "total_files": N, "total_folders": N }`

### 自定义路径

#### `GET /api/public/resolve-custom-path?path=xxx`

根据自定义路径解析到文件夹或文件。

**响应**：
```json
{
  "type": "folder|file",
  "folder_id": "uuid",
  "file_id": "uuid",
  "name": "...",
  "extension": ".html"
}
```

`extension` 仅在 `type=file` 时返回，如 `.html`、`.pdf`。

> **HTML 文件特殊处理**：当解析到 HTML 文件（`extension` 为 `.html` 或 `.htm`）时，前端不会跳转到文件详情页，而是直接获取文件内容并替换当前页面，使浏览器直接渲染网页。生产模式下后端在 SPA 路由前拦截，直接返回 HTML 内容，自定义路径保持在地址栏中。

404：未找到对应路径。

### 基于文件夹层级路径的下载直链

#### `GET /dl/*path`

通过文件夹层级路径直接下载文件或打包下载文件夹，无需知道资源的 UUID。路径由文件夹名称逐级拼接，最后一段为文件名或文件夹名。

**路径参数**：`*path` — 文件夹层级路径，如 `课程/数学/笔记.pdf`。

**查询参数**：

| 参数 | 类型 | 说明 |
|------|------|------|
| `inline` | `string` | 可选，设为 `1` 时对支持的文件类型（视频/图片/PDF/文本等）以内嵌方式返回（`Content-Disposition: inline`） |

**行为**：

- 路径段按 `/` 分割，前 N-1 段逐级匹配子文件夹名称，最后一段优先匹配文件名，其次匹配文件夹名
- **文件路径**：复用 `/api/public/files/:fileID/download` 的完整下载逻辑（CDN 重定向 / 服务端代理 / 本地流式），支持 `?inline=1`
- **文件夹路径**：打包为 ZIP 下载（与 `GET /api/public/folders/:folderID/download` 行为一致）
- 路径中的中文、空格等字符需 URL 编码
- 文件夹名称匹配区分大小写
- 下载计数正常记录
- 隐藏的托管根目录不会出现在路径解析中

**示例**：

```bash
# 下载文件（文件夹 "课程/数学" 下的 "笔记.pdf"）
curl -O https://example.com/dl/%E8%AF%BE%E7%A8%8B/%E6%95%B0%E5%AD%A6/%E7%AC%94%E8%AE%B0.pdf

# 英文路径
curl -O https://example.com/dl/docs/report.pdf

# 内嵌预览
curl "https://example.com/dl/docs/report.pdf?inline=1"

# 打包下载文件夹
curl -O https://example.com/dl/docs/templates
```

**错误**：

| 状态码 | 含义 |
|--------|------|
| 400 | 路径为空或编码无效 |
| 404 | 路径不存在（文件夹或文件名不匹配） |
| 403 | 下载权限被禁止 |
| 410 | 文件不可用 |

### 公告

#### `GET /api/public/announcements`

获取已发布公告列表。响应：`{ "items": [...] }`

### 文件标签

#### `GET /api/public/file-tags`

获取所有预设文件标签定义。响应：`[{ "id": "uuid", "name": "教材", "color": "#..." }]`

### 下载策略

#### `GET /api/public/download-policy`

返回大文件确认阈值和宽布局扩展名。响应：
```json
{
  "large_download_confirm_bytes": 1073741824,
  "wide_layout_extensions": "md,markdown",
  "cdn_mode": false
}
```

### 上传与反馈

#### `POST /api/public/submissions`

用户上传资料。请求体为 `multipart/form-data`，字段：

| 字段 | 类型 | 说明 |
|------|------|------|
| `files` | file[] | 上传文件列表 |
| `folder_id` | string | 目标目录 ID |
| `description` | string | 资料简介（可选） |
| `manifest` | string (JSON) | 文件路径清单，格式 `[{ "relative_path": "subdir/file.txt", "id": "custom-id" }]` |
| `overwrite` | string | 设为 `"1"` 时覆盖同名文件（仅管理员直接上传生效，不产生新 DB 记录） |
| `overwrite_id` | string | 设为 `"1"` 时允许覆盖指定 `file_id` 的现有文件（仅管理员直接上传生效） |

> **指定 file_id**：manifest 中可为每个文件指定 `id` 字段，上传后该文件的 `file_id` 将使用指定值。若指定的 `id` 已存在，需同时设置 `overwrite_id=1` 才能覆盖，否则返回 409 错误。

管理员（具有 `submission_moderation` 权限或超管）上传时直接通过审核，`status` 返回 `"approved"`；访客上传进入审核池。

> **外部 API 调用**：需携带 `Authorization: Bearer <token>` 头（token 从 `POST /api/admin/session/login` 获取）。校验通过后获得管理员身份，上传直接通过审核。

**响应**：`{ "receipt_code": "ABCD-1234", "status": "approved|pending", "item_count": 2, "names": [...], "uploaded_at": "..." }`

#### `GET /api/public/submissions/:receiptCode`

通过回执码查询上传处理状态。响应：`{ "status": "pending|approved|rejected", "reason": "..." }`

#### `POST /api/public/feedback`

提交反馈。请求体：`{ "file_id": "...", "folder_id": "...", "description": "问题说明" }`

**响应**：`{ "receipt_code": "ABCD-1234" }`

#### `GET /api/public/feedback/:receiptCode`

查询反馈处理状态。

#### `GET /api/public/receipt-code`

获取或创建当前会话的回执码。

---

# 管理端 API

所有管理端接口需 **admin 登录态**（Cookie / Session），部分接口需特定权限。

### 认证与会话

所有管理端接口均支持两种鉴权方式：

| 方式 | 适用场景 | 说明 |
|------|---------|------|
| Cookie | 浏览器访问管理后台 | 登录后自动设置，无需手动处理 |
| Bearer Token | 外部 API 调用（curl/脚本/第三方服务） | 在账号设置中创建 API Token，放入 `Authorization: Bearer <osk_...>` 头 |

API Token 持久有效，仅手动删除时失效。退出登录不会使 Token 失效。所有 `RequireAdminAuth` / `RequireAdminPermission` / `RequireSuperAdmin` 保护的接口均可通过 Bearer Token 调用。

### 多语言调用示例

以下示例均使用 API Token 进行鉴权。`<BASE_URL>` 替换为实际部署地址，`<TOKEN>` 替换为创建的 API Token。

#### curl

```bash
# 获取当前管理员信息
curl -s https://<BASE_URL>/api/admin/me \
  -H "Authorization: Bearer <TOKEN>"

# 上传文件（管理员直接通过审核）
curl -X POST https://<BASE_URL>/api/public/submissions \
  -H "Authorization: Bearer <TOKEN>" \
  -F "folder_id=<FOLDER_ID>" \
  -F "files=@test.pdf" \
  -F 'manifest=[{"relative_path":"test.pdf"}]'

# 批量上传
curl -X POST https://<BASE_URL>/api/public/submissions \
  -H "Authorization: Bearer <TOKEN>" \
  -F "folder_id=<FOLDER_ID>" \
  -F "files=@a.pdf" \
  -F "files=@b.pdf" \
  -F 'manifest=[{"relative_path":"a.pdf"},{"relative_path":"b.pdf"}]'

# 覆盖同名文件
curl -X POST https://<BASE_URL>/api/public/submissions \
  -H "Authorization: Bearer <TOKEN>" \
  -F "folder_id=<FOLDER_ID>" \
  -F "files=@test.pdf" \
  -F "overwrite=1"

# 指定 file_id 上传
curl -X POST https://<BASE_URL>/api/public/submissions \
  -H "Authorization: Bearer <TOKEN>" \
  -F "folder_id=<FOLDER_ID>" \
  -F "files=@test.pdf" \
  -F 'manifest=[{"relative_path":"test.pdf","id":"custom-file-id-001"}]'

# 覆盖指定 file_id 的现有文件
curl -X POST https://<BASE_URL>/api/public/submissions \
  -H "Authorization: Bearer <TOKEN>" \
  -F "folder_id=<FOLDER_ID>" \
  -F "files=@test.pdf" \
  -F 'manifest=[{"relative_path":"test.pdf","id":"existing-file-id"}]' \
  -F "overwrite_id=1"

# 列出所有托管文件
curl -s https://<BASE_URL>/api/admin/resources/files \
  -H "Authorization: Bearer <TOKEN>"

# 更新文件元数据
curl -s -X PUT https://<BASE_URL>/api/admin/resources/files/<FILE_ID> \
  -H "Authorization: Bearer <TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"name":"新文件名","description":"简介","remark":"备注"}'

# 删除文件
curl -s -X DELETE https://<BASE_URL>/api/admin/resources/files/<FILE_ID> \
  -H "Authorization: Bearer <TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"password":"<ADMIN_PASSWORD>","move_to_trash":true}'

# 审核通过提交
curl -s -X POST https://<BASE_URL>/api/admin/submissions/<SUBMISSION_ID>/approve \
  -H "Authorization: Bearer <TOKEN>"
```

#### Python

```python
import requests

BASE = "https://<BASE_URL>/api"
HEADERS = {"Authorization": "Bearer <TOKEN>"}

# 获取管理员信息
r = requests.get(f"{BASE}/admin/me", headers=HEADERS)
print(r.json())

# 上传文件
with open("test.pdf", "rb") as f:
    r = requests.post(
        f"{BASE}/public/submissions",
        headers=HEADERS,
        files={"files": ("test.pdf", f, "application/pdf")},
        data={
            "folder_id": "<FOLDER_ID>",
            "manifest": '[{"relative_path":"test.pdf"}]',
        },
    )
    print(r.json())

# 列出文件
r = requests.get(f"{BASE}/admin/resources/files", headers=HEADERS)
for item in r.json()["items"]:
    print(item["id"], item["name"])

# 更新文件
r = requests.put(
    f"{BASE}/admin/resources/files/<FILE_ID>",
    headers={**HEADERS, "Content-Type": "application/json"},
    json={"name": "新名称", "description": "简介"},
)
print(r.status_code)
```

#### JavaScript (fetch)

```js
const BASE = "https://<BASE_URL>/api";
const HEADERS = { Authorization: "Bearer <TOKEN>" };

// 获取管理员信息
const me = await fetch(`${BASE}/admin/me`, { headers: HEADERS }).then(r => r.json());
console.log(me.admin.display_name);

// 上传文件
const form = new FormData();
form.set("folder_id", "<FOLDER_ID>");
form.set("manifest", JSON.stringify([{ relative_path: "test.pdf" }]));
form.append("files", fileBlob, "test.pdf");  // fileBlob 为 File 或 Blob 对象

const upload = await fetch(`${BASE}/public/submissions`, {
  method: "POST",
  headers: HEADERS,  // 注意：不要手动设置 Content-Type，浏览器会自动带 boundary
  body: form,
}).then(r => r.json());
console.log(upload.status, upload.item_count);

// 查询下载策略
const policy = await fetch(`${BASE}/public/download-policy`).then(r => r.json());
console.log(policy.large_download_confirm_bytes);
```

#### Go

```go
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io"
    "mime/multipart"
    "net/http"
    "os"
)

const baseURL = "https://<BASE_URL>/api"
const token = "<TOKEN>"

func main() {
    // 获取管理员信息
    req, _ := http.NewRequest("GET", baseURL+"/admin/me", nil)
    req.Header.Set("Authorization", "Bearer "+token)
    resp, _ := http.DefaultClient.Do(req)
    body, _ := io.ReadAll(resp.Body)
    resp.Body.Close()
    fmt.Println(string(body))

    // 上传文件
    var buf bytes.Buffer
    w := multipart.NewWriter(&buf)
    w.WriteField("folder_id", "<FOLDER_ID>")
    w.WriteField("manifest", `[{"relative_path":"test.pdf"}]`)
    fw, _ := w.CreateFormFile("files", "test.pdf")
    file, _ := os.Open("test.pdf")
    io.Copy(fw, file)
    file.Close()
    w.Close()

    req, _ = http.NewRequest("POST", baseURL+"/public/submissions", &buf)
    req.Header.Set("Authorization", "Bearer "+token)
    req.Header.Set("Content-Type", w.FormDataContentType())
    resp, _ = http.DefaultClient.Do(req)
    body, _ = io.ReadAll(resp.Body)
    resp.Body.Close()
    fmt.Println(string(body))
}
```

#### `POST /api/admin/session/login`

请求体：`{ "username": "...", "password": "..." }`

**响应**：`{ "admin": { "id": "...", "username": "...", "display_name": "...", "role": "super_admin|admin", "permissions": [...] } }`

#### `POST /api/admin/session/logout`

#### `GET /api/admin/me`

获取当前管理员信息。响应为：

```json
{
  "admin": {
    "id": "uuid",
    "username": "superadmin",
    "display_name": "xxx",
    "avatar_url": "https://...",
    "role": "super_admin|admin",
    "status": "active|disabled",
    "permissions": ["submission_moderation", "resource_moderation"]
  }
}
```

> **注意**：`role === "super_admin"` 时 `permissions` 为 `null`，但超管天然具有所有权限。普通管理员的 `permissions` 为字符串数组，逐一列出已分配的权限。前端判断权限时应同时检查 `role === "super_admin" || permissions.includes("目标权限")`。

### API Token 管理

用于创建和管理外部 API 调用的 Bearer Token。

#### `GET /api/admin/api-tokens`

列出当前管理员的所有 API Token。响应：`{ "items": [{ "id", "name", "last_used_at", "created_at" }] }`

#### `POST /api/admin/api-tokens`

创建新的 API Token。请求体：`{ "name": "CLI 脚本" }`。响应：`{ "id", "name", "token": "osk_xxx", "created_at" }`

> `token` 仅在创建时返回一次，之后无法再次查看。Token 前缀 `osk_`，持久有效直至手动删除。

#### `DELETE /api/admin/api-tokens/:tokenID`

删除指定 API Token。

**API Token 管理示例**（通过 Cookie 登录后在浏览器中操作，或使用已有 Token 管理）：

```bash
# 创建 Token
curl -s -X POST https://<BASE_URL>/api/admin/api-tokens \
  -H "Authorization: Bearer <TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{"name":"CI 部署脚本"}'
# → {"id":"...","name":"CI 部署脚本","token":"osk_xxxx","created_at":"..."}

# 列出所有 Token
curl -s https://<BASE_URL>/api/admin/api-tokens \
  -H "Authorization: Bearer <TOKEN>"
# → {"items":[{"id":"...","name":"CI 部署脚本","last_used_at":"...","created_at":"..."}]}

# 删除 Token
curl -s -X DELETE https://<BASE_URL>/api/admin/api-tokens/<TOKEN_ID> \
  -H "Authorization: Bearer <TOKEN>"
# → 204 No Content
```

```python
# 创建 Token
r = requests.post(
    f"{BASE}/admin/api-tokens",
    headers={**HEADERS, "Content-Type": "application/json"},
    json={"name": "Python 脚本"},
)
token = r.json()["token"]  # 仅此一次可获取
print(f"新 Token: {token}")

# 列出 Token
r = requests.get(f"{BASE}/admin/api-tokens", headers=HEADERS)
for t in r.json()["items"]:
    print(t["id"], t["name"], t["last_used_at"])

# 删除 Token
r = requests.delete(f"{BASE}/admin/api-tokens/<TOKEN_ID>", headers=HEADERS)
print(r.status_code)  # 204
```

#### `POST /api/admin/session/change-password`

请求体：`{ "old_password": "...", "new_password": "..." }`

#### `PATCH /api/admin/account/profile`

更新账号资料。请求体：`{ "display_name": "...", "avatar_url": "..." }`

### 资源管理（文件/文件夹）

以下接口需要 `resource_moderation` 权限，部分需要 `manage_system` 或超管权限。

#### `GET /api/admin/resources/folders`

列出所有非虚拟文件夹（不受 `hide_public_catalog` 影响，供管理员文件夹选择器使用）。返回：`{ "items": [{ "id", "name", "parent_id" }] }`

#### `GET /api/admin/resources/files`

列出所有托管文件（支持搜索）。参数：`?q=关键词`

#### `PUT /api/admin/resources/files/:fileID`

更新文件元数据。请求体：
```json
{
  "name": "文件名",
  "description": "简介（Markdown）",
  "remark": "备注（单行）",
  "playback_url": "https://...",
  "playback_fallback_url": "https://...",
  "proxy_source_url": "https://...",
  "cover_url": "https://... 或留空",
  "custom_path": "doc/report",
  "download_policy": "inherit|allow|deny"
}
```
> `cover_url` 为 `*string` 类型，不传或传空不覆盖数据库已有值；其余字段必传。

#### `DELETE /api/admin/resources/files/:fileID`

删除文件。请求体：`{ "password": "...", "move_to_trash": true }`

#### `PUT /api/admin/resources/files/:fileID/move`

将文件移动到指定目标文件夹，同步更新数据库和本地磁盘。文件 ID 不变，原链接依然可用。请求体：
```json
{
  "target_folder_id": "uuid"
}
```
> 目标文件夹存在同名文件时返回错误。虚拟目录/文件仅更新 DB 记录。

#### `POST /api/admin/resources/folders`

创建子文件夹。请求体：
```json
{
  "parent_id": "uuid",
  "name": "文件夹名",
  "custom_path": "optional-path"
}
```

#### `PUT /api/admin/resources/folders/:folderID`

更新文件夹元数据。请求体：
```json
{
  "name": "名称",
  "description": "简介（Markdown）",
  "remark": "备注（单行）",
  "cover_url": "https://... 或留空",
  "direct_link_prefix": "https://cdn.example.com/",
  "custom_path": "doc",
  "download_policy": "inherit|allow|deny",
  "hide_file_extension": "inherit|hide|show"
}
```
> `cover_url` 为 `*string` 类型，不传不覆盖已有值。
> `hide_file_extension`：`inherit` 继承上层文件夹设置，`hide` 隐藏子文件后缀，`show` 显示子文件后缀。未设置时默认显示后缀。

#### `DELETE /api/admin/resources/folders/:folderID`

删除文件夹（含子文件和子目录）。请求体：`{ "password": "...", "move_to_trash": true }`

#### `PUT /api/admin/resources/folders/:folderID/catalog-visibility`

设置托管根目录在公开列表中的可见性。请求体：`{ "hide_public_catalog": true }`

#### `PATCH /api/admin/resources/folders/:folderID/cdn-url`

更新目录的 CDN 直链地址。请求体：`{ "cdn_url": "https://..." }`

#### `PUT /api/admin/resources/folders/:folderID/file-order`

自定义文件排序。请求体：
```json
{
  "orders": [
    { "file_id": "uuid1", "sort_order": 1 },
    { "file_id": "uuid2", "sort_order": 2 }
  ]
}
```
> `sort_order` 为 1-based 连续序号。保存时先清空该目录下所有文件的排序，再写入新的。

#### `POST /api/admin/resources/virtual-folders`

创建虚拟目录（无物理磁盘路径）。请求体同 `POST /api/admin/resources/folders`。

#### `POST /api/admin/resources/virtual-files`

在虚拟目录下添加虚拟文件。请求体：
```json
{
  "folder_id": "uuid",
  "name": "文件名",
  "description": "...",
  "remark": "...",
  "playback_url": "https://cdn.example.com/file.mp4",
  "proxy_source_url": "https://lan-ip/file.mp4",
  "cover_url": "...",
  "custom_path": "...",
  "size": 123456
}
```

#### `POST /api/admin/probe-url`

探测远程 URL 的可达性和文件大小。请求体：`{ "url": "https://..." }`

**响应**：`{ "ok": true, "size": 123456, "content_type": "video/mp4", "file_name": "video.mp4" }`

#### `POST /api/admin/resources/upload-cover`

上传封面图片（multipart）。字段：`file`（图片文件，格式 PNG/JPG/GIF/WebP/SVG/BMP，最大 10 MB）。

**响应**：`{ "url": "/files/cover-uuid" }`

#### `GET /api/admin/resources/check-custom-path?path=xxx`

检查指定自定义路径是否可用（格式校验 + 是否已被占用），不暴露已用路径列表。

**参数**：`?path=doc/report`

**响应（可用）**：
```json
{
  "path": "doc/report",
  "available": true
}
```

**响应（不可用 - 已被占用）**：
```json
{
  "path": "doc/report",
  "available": false
}
```

**响应（不可用 - 格式/保留冲突）**：
```json
{
  "path": "api/test",
  "available": false,
  "reason": "custom path is invalid"
}
```

**鉴权**：`AdminAuth`（Cookie 或 `Authorization: Bearer <token>`）。

```bash
# curl 示例
curl -H "Authorization: Bearer $TOKEN" \
  "$ORIGIN/api/admin/resources/check-custom-path?path=doc%2Freport"
```

### 文件标签

#### `GET /api/admin/file-tags`

获取所有预设标签定义。

#### `POST /api/admin/file-tags`

创建标签。请求体：`{ "name": "教材", "color": "#3b82f6" }`

#### `PATCH /api/admin/file-tags/:tagID`

更新标签。请求体形同创建。

#### `DELETE /api/admin/file-tags/:tagID`

删除标签。

#### `PUT /api/admin/resources/files/:fileID/tags`

替换文件的标签。请求体：`{ "tag_ids": ["uuid1", "uuid2"] }`

### 公告管理

需要 `announcements` 权限。

#### `GET /api/admin/announcements`

列表（含草稿和隐藏）。

#### `POST /api/admin/announcements`

创建。请求体：`{ "title": "...", "content_md": "...", "status": "draft|published|hidden", "pinned": false }`

#### `PUT /api/admin/announcements/:announcementID`

更新。

#### `DELETE /api/admin/announcements/:announcementID`

删除。

### 审核管理

#### `GET /api/admin/submissions/pending`

待审核的提交列表（需要 `submission_moderation` 权限）。

#### `POST /api/admin/submissions/:submissionID/approve`

通过提交。请求体：`{ "target_folder_id": "uuid" }`（目标文件夹）

#### `POST /api/admin/submissions/:submissionID/reject`

驳回提交。请求体：`{ "reason": "驳回原因" }`

#### `GET /api/admin/feedback`

反馈列表（需要 `resource_moderation` 权限）。

#### `POST /api/admin/feedback/:feedbackID/approve`

处理反馈（标记已处理）。

#### `POST /api/admin/feedback/:feedbackID/reject`

驳回反馈。请求体：`{ "reason": "驳回说明" }`

### 目录导入

需要 `manage_system` 权限，取消托管需要超管权限。

#### `POST /api/admin/imports/local`

导入本地目录。请求体：`{ "path": "/data/documents" }`

#### `GET /api/admin/imports/directories`

列出已托管目录列表。

#### `DELETE /api/admin/imports/local/:folderID`

取消托管目录。请求体：`{ "password": "超管密码" }`

#### `POST /api/admin/imports/local/:folderID/rescan`

重新扫描托管目录，同步磁盘变更。需要 `resource_moderation` 权限。

#### `GET /api/admin/folders/tree`

获取全部文件夹树结构（用于目录选择器）。

### 系统配置

需要超管权限。

#### `GET /api/admin/system/settings`

获取系统策略配置（上传限制、下载阈值、CDN 模式等）。

**响应**：
```json
{
  "upload": { "max_upload_total_bytes": 5368709120 },
  "download": {
    "large_download_confirm_bytes": 1073741824,
    "wide_layout_extensions": "md,markdown",
    "cdn_mode": false,
    "global_cdn_url": "",
    "directory_cdn_urls": { "folder-id": "https://cdn.example.com/dir.json" }
  }
}
```

#### `PUT /api/admin/system/settings`

更新系统策略配置。请求体形同 GET 响应。

### 导出

需要 admin 登录态。`global` 无需额外权限，`directory` 为 admin 可用。

#### `GET /api/admin/export/global`

导出全局公开数据（公告/热门/最新/标签/策略），详情见上文"管理端导出接口"章节。

#### `GET /api/admin/export/directory/:folderID`

导出指定托管目录完整数据，详情见上文。

### 管理员管理

需要 `manage_admins` 权限。

#### `GET /api/admin/admins`

管理员列表。

#### `POST /api/admin/admins`

创建管理员。请求体：`{ "username": "...", "password": "...", "display_name": "...", "role": "admin", "permissions": [...] }`

#### `PUT /api/admin/admins/:adminID`

更新管理员信息（角色、权限、状态）。

#### `POST /api/admin/admins/:adminID/reset-password`

重置管理员密码。请求体：`{ "new_password": "..." }`

#### `DELETE /api/admin/admins/:adminID`

删除管理员。

### 操作日志

#### `GET /api/admin/operation-logs`

操作日志列表。支持 `?page=&page_size=&q=` 分页和搜索。

### 控制台

#### `GET /api/admin/dashboard/stats`

控制台统计数据。响应：`{ "pending_audit_count": 3 }`
