package router

import (
	"context"

	"gorm.io/gorm"

	"openshare/backend/internal/config"
	"openshare/backend/internal/handler"
	"openshare/backend/internal/repository"
	"openshare/backend/internal/service"
	"openshare/backend/internal/session"
	"openshare/backend/internal/storage"
)

type routeHandlers struct {
	adminAuth          *handler.AdminAuthHandler
	adminDashboard     *handler.AdminDashboardHandler
	announcement       *handler.AnnouncementHandler
	adminManagement    *handler.AdminManagementHandler
	imports            *handler.ImportHandler
	moderation         *handler.ModerationHandler
	operationLog       *handler.OperationLogHandler
	publicCatalog      *handler.PublicCatalogHandler
	publicDownload     *handler.PublicDownloadHandler
	publicReceipt      *handler.PublicReceiptHandler
	publicSubmission   *handler.PublicSubmissionHandler
	publicUpload       *handler.PublicUploadHandler
	report             *handler.ReportHandler
	resourceManagement *handler.ResourceManagementHandler
	search             *handler.SearchHandler
	siteVisit          *handler.SiteVisitHandler
	systemSetting      *handler.SystemSettingHandler
}

func buildRouteHandlers(db *gorm.DB, cfg config.Config, sessionManager *session.Manager) *routeHandlers {
	storageService := storage.NewService(cfg.Storage)
	receiptCodeService := service.NewReceiptCodeService(
		repository.NewReceiptCodeRepository(db),
		cfg.Upload.ReceiptCodeLength,
	)

	adminRepo := repository.NewAdminRepository(db)
	systemSettingService := service.NewSystemSettingService(repository.NewSystemSettingRepository(db), cfg)
	adminAuthService := service.NewAdminAuthService(db, adminRepo, sessionManager)

	searchRepo := repository.NewSearchRepository(db)
	searchService := service.NewSearchService(searchRepo, systemSettingService)
	_ = searchService.RebuildAllIndexes(context.Background())

	return &routeHandlers{
		adminAuth: handler.NewAdminAuthHandler(adminAuthService, sessionManager),
		adminDashboard: handler.NewAdminDashboardHandler(
			service.NewAdminDashboardService(repository.NewAdminDashboardRepository(db)),
		),
		announcement: handler.NewAnnouncementHandler(
			service.NewAnnouncementService(repository.NewAnnouncementRepository(db), adminRepo),
		),
		adminManagement: handler.NewAdminManagementHandler(
			service.NewAdminManagementService(adminRepo),
		),
		imports: handler.NewImportHandler(
			service.NewImportService(repository.NewImportRepository(db), storageService, searchService),
			adminAuthService,
		),
		moderation: handler.NewModerationHandler(
			service.NewModerationService(repository.NewModerationRepository(db), storageService, searchService),
		),
		operationLog: handler.NewOperationLogHandler(
			service.NewOperationLogService(repository.NewOperationLogRepository(db)),
		),
		publicCatalog: handler.NewPublicCatalogHandler(
			service.NewPublicCatalogService(repository.NewPublicCatalogRepository(db)),
		),
		publicDownload: handler.NewPublicDownloadHandler(
			service.NewPublicDownloadService(repository.NewPublicDownloadRepository(db), storageService),
		),
		publicReceipt: handler.NewPublicReceiptHandler(receiptCodeService),
		publicSubmission: handler.NewPublicSubmissionHandler(
			service.NewPublicSubmissionService(repository.NewPublicSubmissionRepository(db)),
		),
		publicUpload: handler.NewPublicUploadHandler(
			service.NewPublicUploadService(
				cfg.Upload,
				repository.NewUploadRepository(db),
				receiptCodeService,
				storageService,
				systemSettingService,
			),
			systemSettingService,
			cfg.Upload.MaxBatchTotalSizeBytes+(1<<20),
		),
		report: handler.NewReportHandler(
			service.NewReportService(repository.NewReportRepository(db), receiptCodeService, searchService, storageService),
		),
		resourceManagement: handler.NewResourceManagementHandler(
			service.NewResourceManagementServiceWithSettings(
				repository.NewResourceManagementRepository(db),
				storageService,
				systemSettingService,
				searchService,
			),
			adminAuthService,
		),
		search: handler.NewSearchHandler(searchService),
		siteVisit: handler.NewSiteVisitHandler(
			service.NewSiteVisitService(repository.NewSiteVisitRepository(db)),
		),
		systemSetting: handler.NewSystemSettingHandler(systemSettingService),
	}
}
