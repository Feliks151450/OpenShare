package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"openshare/backend/internal/config"
	"openshare/backend/internal/middleware"
	"openshare/backend/internal/session"
	webui "openshare/backend/web"
)

func New(db *gorm.DB, cfg config.Config, sessionManager *session.Manager) *gin.Engine {
	engine := gin.New()
	engine.Use(gin.Logger(), gin.Recovery())
	// /dl/* 下载直链使用公开 CORS 策略，必须注册在白名单 CORS 之前，
	// 否则白名单 CORS 会先以"未命中"或"裸 204"截断 OPTIONS 请求。
	engine.Use(middleware.PublicDownloadCORS())
	engine.Use(middleware.CORS(cfg.CORS.AllowedOrigins))

	handlers, services := buildRouteHandlers(db, cfg, sessionManager)
	engine.Use(middleware.SessionLoader(sessionManager, services.apiToken))

	registerHealthRoutes(engine, func(ctx *gin.Context) {
		sqlDB, err := db.DB()
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"status": "error",
				"error":  "database handle is unavailable",
			})
			return
		}

		if err := sqlDB.Ping(); err != nil {
			ctx.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "error",
				"error":  "database ping failed",
			})
			return
		}

		ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	api := engine.Group("/api")
	registerPublicRoutes(api, handlers)
	// 基于文件夹层级路径的下载直链，注册在顶层以获得更短的 /dl/*path 路径
	// 同时支持 HEAD 探测：媒体客户端在发起 Range GET 前常用 HEAD 获取 Content-Length/Accept-Ranges。
	engine.GET("/dl/*path", handlers.publicDownload.DownloadByPath)
	engine.HEAD("/dl/*path", handlers.publicDownload.DownloadByPath)
	registerAdminRoutes(api, handlers)
	webui.Register(engine, newCustomPathHTMLHandler(services.publicCatalog, services.publicDownload))

	return engine
}
