package router

import (
	"gorm.io/gorm"

	"openshare/backend/internal/config"
	"openshare/backend/internal/handler"
	"openshare/backend/internal/repository"
	"openshare/backend/internal/service"
	"openshare/backend/internal/session"
	"openshare/backend/internal/storage"
)

func buildRouteHandlers(db *gorm.DB, cfg config.Config, sessionManager *session.Manager) (*routeHandlers, *routeServices) {
	repos := buildRouteRepositories(db)
	services := buildRouteServices(db, cfg, repos, sessionManager)
	return buildHandlers(cfg, sessionManager, services, repos), services
}

func buildRouteRepositories(db *gorm.DB) *routeRepositories {
	return &routeRepositories{
		admin:              repository.NewAdminRepository(db),
		adminDashboard:     repository.NewAdminDashboardRepository(db),
		announcement:       repository.NewAnnouncementRepository(db),
		feedback:           repository.NewFeedbackRepository(db),
		fileTag:            repository.NewFileTagRepository(db),
		imports:            repository.NewImportRepository(db),
		moderation:         repository.NewModerationRepository(db),
		operationLog:       repository.NewOperationLogRepository(db),
		publicCatalog:      repository.NewPublicCatalogRepository(db),
		publicDownload:     repository.NewPublicDownloadRepository(db),
		publicSubmission:   repository.NewPublicSubmissionRepository(db),
		resourceManagement: repository.NewResourceManagementRepository(db),
		search:             repository.NewSearchRepository(db),
		siteVisit:          repository.NewSiteVisitRepository(db),
		systemSetting:      repository.NewSystemSettingRepository(db),
		upload:             repository.NewUploadRepository(db),
		receiptCode:        repository.NewReceiptCodeRepository(db),
		apiToken:           repository.NewApiTokenRepository(db),
	}
}

func buildRouteServices(
	db *gorm.DB,
	cfg config.Config,
	repos *routeRepositories,
	sessionManager *session.Manager,
) *routeServices {
	storageService := storage.NewService(cfg.Storage)
	receiptCodeService := service.NewReceiptCodeService(repos.receiptCode, cfg.Upload.ReceiptCodeLength)
	systemSettingService := service.NewSystemSettingService(repos.systemSetting, cfg)
	adminAuthService := service.NewAdminAuthService(db, repos.admin, sessionManager)
	fileTagService := service.NewFileTagService(repos.fileTag, repos.resourceManagement)
	publicDownloadService := service.NewPublicDownloadService(repos.publicDownload, storageService, fileTagService)
	searchService := service.NewSearchService(repos.search, publicDownloadService, fileTagService)

	return &routeServices{
		adminAuth:          adminAuthService,
		adminDashboard:     service.NewAdminDashboardService(repos.adminDashboard),
		announcement:       service.NewAnnouncementService(repos.announcement, repos.admin),
		adminManagement:    service.NewAdminManagementService(repos.admin),
		feedback:           service.NewFeedbackService(repos.feedback, receiptCodeService),
		fileTag:            fileTagService,
		imports:            service.NewImportService(repos.imports, storageService),
		moderation:         service.NewModerationService(repos.moderation, storageService),
		operationLog:       service.NewOperationLogService(repos.operationLog),
		publicCatalog:      service.NewPublicCatalogService(repos.publicCatalog, publicDownloadService, fileTagService),
		publicDownload:     publicDownloadService,
		publicReceipt:      receiptCodeService,
		publicSubmission:   service.NewPublicSubmissionService(repos.publicSubmission),
		publicUpload:       service.NewPublicUploadService(cfg.Upload, repos.upload, receiptCodeService, storageService, systemSettingService),
		resourceManagement: service.NewResourceManagementService(repos.resourceManagement, storageService),
		search:             searchService,
		siteVisit:          service.NewSiteVisitService(repos.siteVisit),
		systemSetting:      systemSettingService,
		apiToken:           service.NewApiTokenService(repos.apiToken),
	}
}

func buildHandlers(cfg config.Config, sessionManager *session.Manager, services *routeServices, repos *routeRepositories) *routeHandlers {
	return &routeHandlers{
		adminAuth:          handler.NewAdminAuthHandler(services.adminAuth, sessionManager),
		adminDashboard:     handler.NewAdminDashboardHandler(services.adminDashboard),
		announcement:       handler.NewAnnouncementHandler(services.announcement),
		adminManagement:    handler.NewAdminManagementHandler(services.adminManagement, services.adminAuth),
		feedback:           handler.NewFeedbackHandler(services.feedback),
		fileTag:            handler.NewFileTagHandler(services.fileTag),
		imports:            handler.NewImportHandler(services.imports, services.adminAuth),
		moderation:         handler.NewModerationHandler(services.moderation),
		operationLog:       handler.NewOperationLogHandler(services.operationLog),
		publicCatalog:      handler.NewPublicCatalogHandler(services.publicCatalog, services.systemSetting),
		publicDownload:     handler.NewPublicDownloadHandler(services.publicDownload),
		publicReceipt:      handler.NewPublicReceiptHandler(services.publicReceipt),
		publicSubmission:   handler.NewPublicSubmissionHandler(services.publicSubmission),
		publicUpload:       handler.NewPublicUploadHandler(services.publicUpload),
		resourceManagement: handler.NewResourceManagementHandler(services.resourceManagement, services.adminAuth, services.systemSetting),
		search:             handler.NewSearchHandler(services.search),
		siteVisit:          handler.NewSiteVisitHandler(services.siteVisit),
		systemSetting:      handler.NewSystemSettingHandler(services.systemSetting, repos.imports),
		export_:            handler.NewExportHandler(services.announcement, services.publicCatalog, services.systemSetting, services.fileTag, services.imports, services.publicDownload, repos.imports),
		apiToken:           handler.NewApiTokenHandler(services.apiToken),
	}
}
