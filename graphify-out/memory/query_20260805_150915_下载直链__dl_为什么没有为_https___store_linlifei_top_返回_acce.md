---
type: "query"
date: "2026-08-05T15:09:15.631573+00:00"
question: "下载直链 /dl 为什么没有为 https://store.linlifei.top 返回 Access-Control-Allow-Origin？"
contributor: "graphify"
source_nodes: ["CORS()", "CORSConfig", "normalizeCORSOrigins", "registerPublicRoutes()"]
---

# Q: 下载直链 /dl 为什么没有为 https://store.linlifei.top 返回 Access-Control-Allow-Origin？

## Answer

线上验证显示 https://qiniu.feliks.top 请求 /dl 时返回 Access-Control-Allow-Origin，而 https://store.linlifei.top 对同一资源不返回；代码中的 CORS 中间件使用 AllowedOrigins 精确匹配，未命中时静默继续。当前默认配置只允许 qiniu.feliks.top，因此根因是 store.linlifei.top 未加入部署环境白名单。推荐设置 OPENSHARE_CORS_ALLOWED_ORIGINS=https://qiniu.feliks.top,https://store.linlifei.top 并重启服务；若 fetch 使用 credentials: include，还需增加 Access-Control-Allow-Credentials: true。

## Source Nodes

- CORS()
- CORSConfig
- normalizeCORSOrigins
- registerPublicRoutes()