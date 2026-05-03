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

## 兼容性说明摘要

| 能力 | SPA | 只读 |
|------|-----|------|
| 路由基础 | `/`、`/upload`、`/files/:id`，History API | **`#/`**、 **`#/files/:id`** |
| `goUpload` | 支持 | 忽略并首次 warn |
| `leaveFileTowardFolder` | 详情页：`GET /public/files/:id` | 用语义内 **`state.fileDetail`**；无则用 `#/` |
| `getFileInfo` | `GET`，`credentials: include`，同源 `/api` | `GET`，`credentials: omit`，走 **`apiUrl(...)`** |
| `replace` | `router.replace` | `replaceState` + `bootstrapRoute` |
| 路由重复（同目标） | 忽略 `NavigationDuplicated`，其它 `console.warn` | 由 hash／replace 实现决定 |

若你在扩展更多控制台能力，请同时更新 **`frontend/src/lib/openShareConsole.ts`**、**`frontend/src/lib/openSharePublicFileInfo.ts`** 与 **`frontend/standalone-readonly/readonly.js`**，并同步本文档。
