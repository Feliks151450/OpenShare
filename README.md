# OpenShare

开源、自托管的**资料共享与分发平台**：把服务器磁盘上的目录一键变成带搜索、预览、下载统计与精细权限治理的文件站点，适合课程资料、团队知识库、内网共享等场景。

- **Go 1.25 + Gin + GORM + SQLite** 后端，单二进制部署，无外部数据库依赖
- **Vue 3 + Vite + TypeScript + Tailwind CSS + Pinia** 前端，可内嵌进二进制或独立部署
- 免登录访客体验 + 多级管理员治理 + CDN 分发加速

## 核心特性 🌟

### 1. 简约、现代的前端界面

- 首页与管理后台统一采用简洁、现代的界面风格
- 支持卡片 / 表格等多种展示方式
- 顶部导航栏提供**刷新按钮**，点击后清除本地缓存并重新加载页面数据

<p align="center">
  <img src="assets/1-1.png" alt="前端界面 1" width="49%">
  <img src="assets/1-2.png" alt="前端界面 2" width="49%">
</p>

### 2. 普通用户使用体验优化

- 免登录即可使用，包括 **浏览、搜索、下载、上传与反馈**
- 支持**收藏**文件和文件夹：卡片视图中点击星标按钮即可收藏，导航栏提供"我的收藏"入口，收藏数据保存在浏览器本地
- 支持 Markdown 富文本（**简介**、公告、`.md` 文件正文预览），由 Vditor 渲染：公式（KaTeX）、mermaid 图表、任务列表、`==高亮==`、表格与代码高亮均可用；图片支持宽度控制：`![描述|width=800](url)` 或 `![描述|width=80%](url)`，设置图片最大宽度（引擎与自托管资源详见下文「Markdown 渲染与编辑」一节）
- 文件与文件夹支持**封面图片**展示：卡片视图优先展示 `cover_url`，留空则提取简介中 `![cover](url)` 语法；管理员可通过拖拽上传自动转为站内链接，也支持手动输入外部直链
- 文件与文件夹另有**备注**字段：仅单行纯文本，与简介区分；**卡片视图**下卡片副标题只显示备注，**简介**在进入文件夹后的简介区或文件详情页中展示
- 首页集成 **公告、热门下载、资料上新** 等信息面板
- 支持单文件下载与批量下载
- 下载文件夹（ZIP 打包）或超过阈值的大文件时，会先弹出确认对话框，避免误触大流量下载；默认阈值为 1 GB
- 排序方式支持 **智能排序**（管理员自定义顺序）、名称、下载量、格式、修改日期等，可切换升/降序；切换排序会自动重新请求服务端，避免因文件数超过单页上限而部分文件无法显示
- 目录文件列表支持 **分页**：超出单页上限时通过底部分页控件切换页码或跳转到指定页；切换排序、修改单页大小后会自动重置到第 1 页
- **单页最大文件数** 可在 20 / 50 / 100 之间自由切换（后端上限 100），选择会持久化到浏览器本地存储
- 支持目录内搜索
- 虚拟目录下的文件通过 CDN 直链下载/播放，可配合 `staticData` 从 CDN 加载目录数据，减轻源服务器压力
- 文件详情信息完整，覆盖名称、大小、下载量、更新时间、所属目录等内容
- 文件详情页支持多种预览格式：**视频播放**（支持主/备直链回退，设置封面后自动用作视频 poster，兼容 Safari 等不自动显示首帧的浏览器）、**PDF 内嵌预览**、**NetCDF 结构化摘要**、**Markdown 渲染**（含公式与图表，可切换为所见即所得编辑正文）、**ASCII / 纯文本**等
- **视频时间戳链接**：视频详情页可复制"含时间戳的链接"（形如 `/files/<文件ID>?t=2483.8`，单位为秒），打开该链接会自动跳到对应时间点
  - 该链接也可写进**简介**里当作章节目录，如 `[不稳定性理论](/files/<文件ID>?t=2483.8)`
  - 点击时会自动判断目标文件是否就是**当前正在看的这个文件**：是则页面完全不刷新、不滚动，仅把播放器跳到该时间点并自动播放（地址栏的 `?t=` 同步更新，方便直接分享）；否则整页跳转到目标文件详情页并从该时间点开始播放
- **自定义访问路径**：管理员可为文件夹或文件设置短路径（如 `doc`），访客通过 `/doc` 即可直达，替代冗长的 UUID 链接；自定义路径支持多级（如 `doc/sub`），且原 UUID 链接仍然可用
  - **HTML 文件特殊处理**：当 HTML 文件设置了自定义路径时，访问该路径直接在浏览器中渲染网页（而非跳转到文件详情页），地址栏保持自定义路径；生产模式下后端直接返回 HTML 内容，无需加载 SPA
  - **Markdown 文件特殊处理**：当 Markdown 文件设置了自定义路径时，访问该路径以只读整页形式直接显示渲染后的正文（Vditor 预览，含公式/代码块/表格），同样不跳转到文件详情页、地址栏保持自定义路径。与 HTML 不同，Markdown 必须由浏览器渲染，因此该模式下 SPA 仍会加载；正文超过 1 MB 时退回文件详情页。整页带文档大纲：宽屏（≥1024px）为左侧固定栏并高亮当前章节，窄屏为右下角悬浮按钮唤起的抽屉；大纲由渲染结果中的标题生成，点击按滚动容器实时坐标跳转。右上角另有正文搜索按钮：在渲染结果中高亮全部命中（公式/SVG 内部跳过以免破坏排版），显示 `当前/总数` 并支持上一条/下一条与 Enter/Shift+Enter 逐条平滑跳转，Esc 清除
  - **基于文件夹层级路径的下载直链**：通过 `/dl/文件夹/子文件夹/文件名.pdf` 可直接下载文件，路径由文件夹名称逐级拼接而成，无需知道资源 UUID；文件夹路径自动打包为 ZIP 下载。`/dl/*` 默认对**任意来源开放 CORS**（`Access-Control-Allow-Origin: *`），并支持 `GET` / `HEAD` / `OPTIONS` 与 `Range` 断点续传，**不得**携带 Cookie 或 `credentials: 'include'`；管理 API 仍按 `cors.allowed_origins` 白名单访问
- 管理员可为文件设置 **预设标签**（如"教材""课件""试卷"），访客可按标签筛选文件
- 浏览器控制台提供 `window.OpenShare` API：程序化路由跳转、获取文件元数据、控制首页视图/排序偏好、`staticData` CDN 数据加载、自定义路径解析等

<p align="center">
  <img src="assets/2-1.png" alt="普通用户体验 1" width="49%">
  <img src="assets/2-2.png" alt="普通用户体验 2" width="49%">
</p>

<p align="center">
  <img src="assets/2-3.png" alt="普通用户体验 3" width="49%">
  <img src="assets/2-4.png" alt="普通用户体验 4" width="49%">
</p>

### 3. 后台治理与权限管理优化

- 管理员分为超级管理员和普通管理员，启动时会自动生成超级管理员初始密码
- 管理后台提供控制台、审核、公告、日志、账号设置等页面
- 支持修改账号信息，包括头像、用户名和密码
- 支持 **API Token**：管理员在账号设置中创建和管理 API Token，通过 `Authorization: Bearer <token>` 头调用所有管理端接口，权限与创建者一致，持久有效直至手动删除
- 资源管理中可分别维护文件的 **备注**（单行文本）与 **简介**（Markdown）；文件夹同理
- **所见即所得编辑 Markdown**：文件简介、文件夹简介、公告正文、上传时的资料简介，以及 `.md` 文件的**正文本身**，都在独占弹窗中用 Vditor 可视化编辑；图片可直接拖入或粘贴截图，自动上传并插入站内地址。文件夹工具栏另有**新建 Markdown** 按钮：弹窗内编辑后直接以 `.md` 存入当前目录（管理员会话立即发布，访客提交则进入审核队列，与普通上传同一链路）
- 可为文件和文件夹**设置封面图片**：点击工具栏"封面"按钮，可直接输入外部 URL，也可拖拽图片自动上传转为站内链接（需超级管理员预先配置封面存储目录）；视频文件支持 **"当前视频帧"** 一键截取当前画面作为封面；已选图片但未填 URL 时，点击确认自动上传并设置封面
- 文件夹和文件均可设置 **自定义访问路径**（如 `doc`、`course/math`），访客通过短链接即可直达；路径以英文字母开头，支持多级，与系统路由冲突时自动拒绝
- 支持 **自定义文件排序**：管理员可拖拽调整文件夹内文件的显示顺序，设置后作为默认「智能排序」方式展示给访客；支持拖拽间隙提示、上移/下移按钮
- 支持 **移动文件到其他文件夹**：管理员在文件夹详情页多选文件后，或在文件详情页中，可将文件移动到其他文件夹；数据库和本地磁盘同步更新，文件 ID 不变，原链接依然可用
- 支持 **替换文件内容**：保留文件 ID、元数据与下载计数，仅更新文件本体
- 可建立 **预设标签**（如"教材""课件""试卷"），为文件批量分配标签，访客首页可按标签过滤
- 支持 **托管根目录隐藏**：在访客首页根列表中隐藏特定托管树，但知道 ID 或自定义路径的访客仍可直达
- 可配置 **子文件直链前缀**：为整个目录树设置 CDN 下载前缀，其下文件无需单独配置播放/下载地址
- 支持 **服务端代理下载**：文件配置为代理模式后，下载请求由服务端从内网地址拉取再流式返回客户端，解决客户端无法直连内网存储的问题
- 支持 **虚拟目录**：不依赖物理磁盘路径，仅存数据库，子文件通过 CDN 直链提供；适用于跨站点资源聚合、外部存储对接等场景
  - 虚拟目录下的 **虚拟文件** 通过 CDN 播放/下载直链访问，由服务端代理拉取（可选）或客户端直接请求
  - 可配合 `staticData` 机制，将预导出的 JSON 文件上传至 CDN，访客直接从 CDN 加载目录数据，无需请求源服务器

以下为超级管理员特有权限

- 配置访客策略：上传大小限制、**大文件下载确认阈值**（超过该大小的单文件下载前会要求访客确认；文件夹打包下载始终确认）、速率限制（上传/搜索频率控制）、**封面图片存储目录**（设置磁盘绝对路径，首次使用自动创建为隐藏托管根目录，管理员即可拖拽上传封面）
- 导入本地目录：将服务器磁盘目录导入平台，支持目录浏览器选择和手动输入路径；**重新扫描**可检测磁盘文件变更并同步到数据库，**对任一非虚拟托管目录（含子目录）均可触发**，扫描范围包含该目录及其所有后代
- **访客密钥访问**：启用后，访客在网页端浏览被标记为"要求密钥访问"的托管目录时必须先输入密钥才能查看详情（直链/下载不受密钥保护）。管理员可在后台维护一个密钥池，每个被保护的目录可独立选择允许哪些密钥解锁；访客输入正确的密钥后，密钥会保存在浏览器本地存储（`openshare-guest-key`），后续浏览自动附带；浏览器控制台 `window.OpenShare.guestAccessKey` 也提供 `get/set/clear/has` 方法
- **CDN 模式**：启用后可配置全局及单目录 CDN 直链，访客优先从 CDN 加载数据（`staticData`），降低源站压力
- 管理员创建、停用、删除、重置密码与权限分配
- 导出系统数据：全局 JSON（公告/热门/最新/标签/策略）及单目录完整数据，上传到 CDN 后供前端 `staticData` 加载

<p align="center">
  <img src="assets/3-1.png" alt="后台治理 1" width="49%">
  <img src="assets/3-2.png" alt="后台治理 2" width="49%">
</p>

### 4. 资料共建共享

- 支持普通用户上传资料（拖拽或选择文件/文件夹）、提交反馈
- 上传/反馈均生成 **回执码**，访客可随时查询处理状态（审核通过/驳回及原因）
- 上传时可自动忽略 `.DS_Store` 等系统文件，保留原始目录结构
- 管理员在审核页面对上传和反馈进行通过/驳回操作，驳回需填写原因

<p align="center">
  <img src="assets/4-1.png" alt="资料共建共享 1" width="49%">
  <img src="assets/4-2.png" alt="资料共建共享 2" width="49%">
</p>

## Markdown 渲染与编辑 📝

### 渲染引擎：Vditor（自托管）

- 主文档面（文件 / 文件夹简介、公告正文、`.md` 文件正文预览）由 **vscode-vditor 4.0.0** 渲染：行内与块级公式（KaTeX）、mermaid 代码块图表、任务列表、`==高亮==`、表格与代码高亮，输出经 Lute 的 XSS 过滤
- 编辑面是同一引擎的 **所见即所得（wysiwyg）** 模式，编辑时看到的效果即访客看到的效果
- 仍保留 `marked` + Web Worker 通道，用于**卡片摘要、首页公告条、NetCDF 结构化摘要**等短文本

> **双轨是刻意的**：Vditor 的纯预览需要 `index.min.js`（约 3.3 MB）与 `lutePro.min.js`（约 3.5 MB）。若连卡片摘要也走 Vditor，每位访客打开首页都要先拉这约 8.5 MB；短文本用 `marked` 本地渲染即可。

### 自托管资源（部署必读）

- 位于 `frontend/public/vendor/vditor/`，`vite build` 时原样产出到 `dist/`，**不进 JS bundle**，首次用到时才懒加载
- 内容：`dist/index.min.js`、`dist/index.css`、`dist/lutePro.min.js`、`dist/js/katex/**`（含 `fonts/`）、`dist/js/mermaid/mermaid.min.js`、`dist/js/i18n/zh_CN.js`，合计约 12 MB
- ⚠️ **不要用 npm 安装替代**：npm 上的 `vscode-vditor` 最高只有 `3.8.x`，与本仓库所用 fork 的 4.0.0 在编辑模式、主题体系、浮层定位上都有差异
- 升级方式：在该 fork 仓库 `npm install && npm run build`，再把 `dist/index.min.js`、`dist/index.css`、`lutePro.min.js` 覆盖到上述目录。注意该 fork 的 `dist/` 可能**落后于** `src/`，改完必须重新构建并确认产物里真的有新 API
- 未内置 emoji 图片表（`dist/images/emoji/`），因此 `:smile:` 一类短码会降级为纯文本

### 站内图片地址 `/files/<fileID>`

简介里可以直接写 `![描述](/files/<文件ID>)` 引用站内图片。后端 `GET /files/:fileID` 在**非 HTML 请求**（即 `<img>` 加载）时 302 到 `/api/public/files/<fileID>/download`；地址栏导航（`Accept` 含 `text/html`）不受影响，仍走 SPA 的文件详情页路由。

- 开发模式由 `frontend/vite.config.ts` 里的同名中间件提供；前后端分离部署时按 `nginx.example.conf` 中的 `map $http_accept` 规则配置
- 前端渲染时也会把 `/files/<id>` 改写为完整下载地址，因此反代未配置时**访客侧预览依然正常**；该重定向主要服务于编辑器内的即时显示与第三方客户端

### 编辑入口与保存

- 文件简介、文件夹简介、公告正文、上传表单简介：走各自资源的 `PUT` 元数据接口
- `.md` 文件**正文**：复用「替换文件」接口 `POST /api/admin/resources/files/:id/replace`，文件 ID、元数据与下载计数均保持不变，后端会先备份原文件
- 简介与正文编辑都在**独占弹窗**中进行，不在多字段表单里内嵌编辑器
- 编辑器内拖入或粘贴图片走 `POST /api/admin/resources/upload-cover`；访客无该接口权限时，上传入口自动禁用并给出提示
- `.md` 正文预览为控制内存按 1 MB 上限截断读取；**被截断的文件禁止保存**，避免用不完整内容覆盖磁盘原件

### 维护提示

- 自定义 Markdown 语法（相对图址、`|width=`、外链新窗口）统一在 `frontend/src/lib/markdownVditorHtml.ts` 的 `transform` 钩子里补齐——那是 Vditor 纯预览改写 HTML 的唯一入口，编辑器侧没有对应钩子
- 使用 `MarkdownPreview.vue` 时外层根必须带 `class="vditor"`（id 随意）：主题变量定义在这一层，且 `.vditor-reset` 的 `font-size` 取 `var(--editor-font-size)` 无回退值
- 编辑器构造必须显式传 `cdn` 与 `_lutePath`（纯预览传 `lutePath`）：fork 默认按「当前页面相对路径」找 `lutePro.min.js`，SPA 路由下必然 404

## 项目结构 🧩

```text
OpenShare/
├── assets/                     README 配图与静态资源
├── backend/                    Go 后端服务
│   ├── cmd/server/             服务入口（main.go）
│   ├── configs/                config.default.json 默认配置 + config.local.json 本地覆盖
│   ├── internal/
│   │   ├── bootstrap/          启动时自动建表与初始化
│   │   ├── config/             配置加载（默认值 → 文件覆盖 → 环境变量）
│   │   ├── handler/            HTTP 处理层（public_* / admin_* / 资源管理等）
│   │   ├── middleware/         CORS、会话、鉴权、访客密钥
│   │   ├── model/              GORM 数据模型
│   │   ├── repository/         数据访问层
│   │   ├── router/             路由注册（route_groups.go 为 API 总览入口）
│   │   ├── service/            业务逻辑层（导入、下载、审核、NetCDF 等）
│   │   ├── session/            管理员会话管理
│   │   └── storage/            本地磁盘存储抽象
│   ├── pkg/                    database（SQLite 初始化）、identity 等通用包
│   └── web/                    嵌入式前端构建产物（embed / noembed 构建标记）+ 站内图片地址重定向
├── frontend/                   Vue 3 前端项目
│   ├── src/views/public/       首页、文件详情、上传、收藏
│   ├── src/views/admin/        控制台、审核、公告、标签、日志、账号
│   ├── src/components/markdown/ Vditor 封装：只读预览 + 所见即所得编辑器
│   ├── src/lib/                OpenShare 控制台桥、staticData 加载器、Markdown 工具与 Vditor 懒加载器等
│   ├── public/vendor/vditor/   Vditor 自托管资源（约 12 MB，不进 JS bundle、按需懒加载）
│   └── standalone-readonly/    免构建只读静态页（CDN 场景）
├── docker/                     Dockerfile（多阶段构建）与 entrypoint
├── docker-compose.yml          Docker Compose 一键部署
├── start.sh                    开发模式启动脚本（前后端分离 + HMR）
├── start_back.sh               开发模式后台运行
├── deploy.sh                   生产部署脚本（构建 + 运行，--daemon / --separated）
├── nginx.example.conf          Nginx 反代配置模板（前后端分离部署时参考）
├── api.md                      完整 API 文档（公开接口 + 管理端接口 + 控制台 API）
└── cmd.md                      常用开发命令备忘
```

## 快速开始 ⚡

> 完整的公开 API 与管理端 API 文档见 **[api.md](api.md)**，包含请求参数、返回值示例和字段说明。

环境要求：

- Go 1.25+
- Node.js / npm

### 开发模式

前后端分离运行，前端支持热更新（HMR），适合调试和修改代码：

```bash
./start.sh        # 前台运行，Ctrl+C 同时停止前后端
./start_back.sh   # 后台运行
```

- 前端 Vite dev server: `http://localhost:5173/`（`/api` 与 `/dl` 代理到后端 8080）
- 后端 API: `http://127.0.0.1:8080/`
- 管理后台: `http://localhost:5173/admin`
- 健康检查: `http://127.0.0.1:8080/healthz`
- 数据库与存储: `.localdata/`；日志: `.localdata/logs/`

首次启动会自动初始化数据库，并在日志中输出超级管理员初始凭据（`start.sh` 会自动捕获并打印）。

### 生产部署

构建优化后的单二进制文件（前端内嵌），适合内网穿透、VPS 等场景：

```bash
# 构建并后台运行（默认 --daemon，自动停旧进程、日志记录）
./deploy.sh
```

- 默认监听 `:5173`，可通过环境变量 `BACKEND_PORT` 修改（如 `BACKEND_PORT=8080 ./deploy.sh`）
- 数据目录 `.production/`（可用 `DATA_DIR` 修改），与开发环境隔离
- 会话密钥持久化在 `.production/session_secret`（0600），跨部署保持管理员登录态
- 日志写入 `.production/logs/server.log`，PID 记录在 `build/openshare.pid`：

```bash
# 停止后台服务（等价于 kill $(cat build/openshare.pid)，带等待与 SIGKILL 兜底）
./stop.sh --daemon
```

`--foreground` 可前台运行（Ctrl+C 停止）。

`--separated` 模式下，前端静态文件输出到 `build/static/`，Go 后端只处理 API（构建时使用 `-tags noembed` 跳过静态文件内嵌），需配合 `nginx.example.conf` 使用：

```bash
./deploy.sh --separated
```

### Docker 部署

仓库提供多阶段构建的 `docker/Dockerfile`（Node 构建前端 → Go 编译静态二进制 → Debian slim 运行时）：

```bash
# docker compose 一键启动（数据持久化在 named volume openshare-data）
docker compose up -d

# 或手动构建运行
docker build -f docker/Dockerfile -t openshare:latest .
docker run -d -p 8080:8080 -v openshare-data:/data/openshare openshare:latest
```

- 容器内数据目录为 `/data/openshare`，监听 `:8080`
- 未设置 `OPENSHARE_SESSION_SECRET` 时，entrypoint 会生成临时密钥（容器重建后会话失效）；生产环境建议显式设置
- 多架构镜像可用 `docker buildx build -f docker/Dockerfile --platform linux/amd64,linux/arm64 ...` 构建

### 前后端分离部署

1. `./deploy.sh --separated` 产出 `build/static/`（前端）与 `build/openshare`（API 后端）
2. 参考 `nginx.example.conf`：Nginx 托管静态文件并把 `/api`、`/dl` 反代到后端
3. 后端配置 `cors.allowed_origins`（或环境变量 `OPENSHARE_CORS_ALLOWED_ORIGINS`）加入静态站点的 Origin

### 数据库同步

`deploy.sh` **不会**自动同步开发库到生产库（早期的 mtime 自动覆盖会静默丢失生产数据，已移除）。需要同步时显式执行：

```bash
./deploy.sh --sync-db-from-dev
```

该操作会先停止旧生产进程，再通过 `sqlite3 .backup`（自动合并 WAL）安全导出开发库写入生产；sqlite3 不可用时连同 `-wal`/`-shm` 一起拷贝。同步会**完全覆盖**生产库，请确认生产端没有未同步的增量数据。

## 配置说明 ⚙️

配置加载顺序（后者覆盖前者）：

1. `backend/configs/config.default.json` — 默认值（必需）
2. `backend/configs/config.local.json` — 本地覆盖（可选；`start.sh` 首次运行时自动生成开发配置）
3. `OPENSHARE_*` 环境变量 — 优先级最高（Docker / CLI 场景）

| 环境变量 | 对应配置项 | 默认值 | 说明 |
|----------|-----------|--------|------|
| `OPENSHARE_SERVER_HOST` / `OPENSHARE_SERVER_PORT` | `server.host` / `server.port` | `0.0.0.0` / `8080` | 监听地址与端口 |
| `OPENSHARE_DATABASE_PATH` | `database.path` | `<storage.root>/openshare.db` | SQLite 数据库路径 |
| `OPENSHARE_DATABASE_LOG_LEVEL` | `database.log_level` | `warn` | `silent` / `error` / `warn` / `info` |
| `OPENSHARE_STORAGE_ROOT` | `storage.root` | `/data/openshare` | 应用存储根目录（数据库、上传暂存等） |
| `OPENSHARE_STORAGE_TRASH` | `storage.trash` | `trash` | 回收站目录名（挂在每个卷根目录下） |
| `OPENSHARE_STORAGE_COVER_UPLOAD_DIR` | `storage.cover_upload_dir` | 空 | 封面图片存储目录（超管在后台配置也可） |
| `OPENSHARE_UPLOAD_MAX_UPLOAD_TOTAL_BYTES` | `upload.max_upload_total_bytes` | 5 GiB | 单次上传总大小上限 |
| `OPENSHARE_UPLOAD_MAX_DESCRIPTION_LENGTH` | `upload.max_description_length` | 4000 | 简介最大长度 |
| `OPENSHARE_SESSION_SECRET` | `session.secret` | — | **必须覆盖**（默认值会导致启动失败）；生产环境务必设置固定随机值 |
| `OPENSHARE_SESSION_MAX_AGE_SECONDS` | `session.max_age_seconds` | 604800（7 天） | 会话有效期 |
| `OPENSHARE_SESSION_SECURE` / `SAME_SITE` | `session.secure` / `session.same_site` | `false` / `lax` | HTTPS 部署时建议 `secure=true`；`same_site=none` 需 `secure=true` |
| `OPENSHARE_RATE_LIMIT_UPLOAD_*` / `OPENSHARE_RATE_LIMIT_SEARCH_*` | `rate_limit.upload` / `rate_limit.search` | 10 次/分、60 次/分 | 上传与搜索限流 |
| `OPENSHARE_CORS_ALLOWED_ORIGINS` | `cors.allowed_origins` | 空 | 逗号分隔的跨域 Origin 白名单（前后端分离部署必配） |
| `OPENSHARE_IMPORT_ALLOWED_ROOTS` | `import.allowed_roots` | 空 | 冒号分隔的目录导入/浏览白名单（空 = 不限制；建议生产配置） |

完整默认值见 [`backend/configs/config.default.json`](backend/configs/config.default.json)，校验逻辑见 `backend/internal/config/config.go`。

## API 概览 🔌

完整文档见 **[api.md](api.md)**。常用接口速查：

| 分类 | 方法与路径 | 说明 |
|------|-----------|------|
| 健康检查 | `GET /healthz` | 探活（含数据库 ping） |
| 公开目录 | `GET /api/public/folders`、`GET /api/public/folders/:id/files` | 浏览目录与文件（分页 + 排序） |
| 公开详情 | `GET /api/public/files/:id` | 文件元数据 |
| 下载 | `GET /api/public/files/:id/download`、`GET /api/public/folders/:id/download` | 单文件 / ZIP 打包下载 |
| 层级直链 | `GET /dl/*path` | 按文件夹路径下载（开放 CORS、支持 Range） |
| 搜索 | `GET /api/public/search?q=` | 文件 + 文件夹统一搜索 |
| 上传 | `POST /api/public/submissions` | 访客上传（管理员带 Token 直接过审） |
| 管理端 | `POST /api/admin/session/login` 及 `/api/admin/*` | Cookie 或 `Authorization: Bearer <osk_...>` Token 鉴权 |

## 存储与资源删除 🗂️

### 配置项含义

- **`storage.root`**：应用侧存储根目录，用于数据库路径（若写在同盘）、上传**暂存区**（staging）等。可与实际资料盘分离。
- **`storage.trash`**：回收站**目录名**（默认 `trash`），含义是「挂在**每个卷 / 挂载点根目录**下的文件夹」，而不是 `storage.root` 下的子目录。
  - 例如资料在 `/SSD1/课程/笔记.pdf`，选择「移到回收站」时，文件会进入 **`/SSD1/trash/`**（若该名已存在则自动加后缀），从而与项目所在磁盘空间解耦，避免大数据盘资料把应用盘占满。

### 管理员删除资料

在删除文件或文件夹的确认框中可选择：

| 选项 | 行为 |
|------|------|
| **移动到垃圾桶** | 将对应路径移入**该资料所在磁盘**根目录下的 `trash` 文件夹（可在配置中改名）。 |
| **彻底删除** | 直接从磁盘删除，不经过回收站，不可恢复。 |

调用删除 API（`DELETE /api/admin/resources/files/:id`、`DELETE /api/admin/resources/folders/:id`）时，请求体除密码外可带 **`move_to_trash`**（布尔，默认 `true`）：`false` 表示彻底删除。

### 跨文件系统说明

当上传暂存目录与资料目录、或资料目录与回收站不在同一文件系统时，后端会以「复制后删除源」等方式处理，无需把 staging 与资料强行放在同一挂载点。

## 开发与测试 🛠️

```bash
# 后端编译检查与单元测试
cd backend
go build ./...
go test ./...

# 前端开发服务器（独立启动）
cd frontend
npm install
npm run dev

# 前端生产构建（vue-tsc 类型检查 + vite build）
npm run build

# 只读静态页样式 / 依赖（standalone-readonly）
npm run build:readonly
npm run vendor:readonly
```

后端测试覆盖路由、CORS、上传策略、目录导入等场景（`backend/internal/**/*_test.go`）；路由定义以 `backend/internal/router/route_groups.go` 为准，新增接口后请同步 `api.md`。

## 致谢 🫶

如果这个项目对你有帮助，欢迎点一个 Star ！

<div align="center">
  <p><strong>感谢LinuxDo社区的支持！</strong></p>
  <p>
    <a href="https://linux.do" target="_blank" rel="noopener noreferrer">
      <img
        alt="LinuxDo Community"
        src="https://img.shields.io/badge/COMMUNITY-LINUXDO-3E7FC1?style=for-the-badge&labelColor=5A5A5A&logoWidth=0"
      />
    </a>
  </p>
</div>
