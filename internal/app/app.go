package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	apidocmodule "admin/internal/apidoc"
	apihttp "admin/internal/apimetadata/adapters/http"
	apiapplication "admin/internal/apimetadata/application"
	"admin/internal/dictionary"
	identityhttp "admin/internal/identity/adapters/http"
	identityapplication "admin/internal/identity/application"
	navigationmodule "admin/internal/navigation"
	platformconfig "admin/internal/platform/config"
	platformdatabase "admin/internal/platform/database"
	"admin/internal/routecatalog"
	"github.com/gin-gonic/gin"
	"github.com/minio/minio-go/v7"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Resources struct {
	DB     *gorm.DB
	Redis  *redis.Client
	MinIO  *minio.Client
	Logger *zap.Logger
}

type SeedFunc func(context.Context, platformconfig.Config, []routecatalog.Descriptor) error

type BackgroundJob struct {
	Name string
	Run  func(context.Context)
}

type Options struct {
	Descriptors    []routecatalog.Descriptor
	Middleware     HTTPMiddleware
	TechnicalHTTP  TechnicalHTTP
	Seed           SeedFunc
	BackgroundJobs []BackgroundJob
}

type Application struct {
	resources   Resources
	catalog     *routecatalog.Catalog
	engine      *gin.Engine
	address     string
	navigation  *navigationmodule.Service
	apiMetadata *apiapplication.Service
	identity    *identityapplication.Service
	jobs        []BackgroundJob
	closers     []func() error

	startOnce sync.Once
	closeOnce sync.Once
	cancel    context.CancelFunc
	wait      sync.WaitGroup
	closeErr  error
}

func New(ctx context.Context, conf platformconfig.Config, resources Resources, options Options) (*Application, error) {
	if err := validateResources(resources); err != nil {
		return nil, err
	}
	filesModule, err := newFilesComposition(resources, conf)
	if err != nil {
		return nil, fmt.Errorf("build Files module: %w", err)
	}
	dictionaryRoutes := dictionaryDescriptors(resources)
	identityCore := newIdentityCore(resources)
	authorizationApplication := newAuthorizationService(resources, identityCore.directory)
	routeSource := &catalogRouteSource{}
	modules := newNavigationComposition(resources, authorizationApplication, routeSource, identityCore.directory)
	organizationRoutes := organizationDescriptorsWithVisibility(resources, authorizationOrganizationVisibility{authorization: authorizationApplication}, identityCore.directory)
	identityModule := newIdentityComposition(resources, conf, authorizationApplication, identityNavigation{navigation: modules.Navigation}, identityCore)
	auditModule := newAuditComposition(resources, conf, identityModule.repository)
	navigationRoutes := navigationmodule.Routes(modules.Navigation)
	apiRoutes := apihttp.Routes(modules.APIService)
	existingRoutes := make([]routecatalog.Descriptor, 0, len(dictionaryRoutes)+len(organizationRoutes)+len(navigationRoutes)+len(apiRoutes)+len(options.Descriptors))
	existingRoutes = append(existingRoutes, dictionaryRoutes...)
	existingRoutes = append(existingRoutes, organizationRoutes...)
	existingRoutes = append(existingRoutes, navigationRoutes...)
	existingRoutes = append(existingRoutes, apiRoutes...)
	existingRoutes = append(existingRoutes, options.Descriptors...)
	identityRoutes := identityDescriptors(identityModule, conf)
	authorizationCatalogInputs := make([]routecatalog.Descriptor, 0, len(existingRoutes)+len(identityRoutes)+len(filesModule.routes)+len(auditModule.routes))
	authorizationCatalogInputs = append(authorizationCatalogInputs, existingRoutes...)
	authorizationCatalogInputs = append(authorizationCatalogInputs, identityRoutes...)
	authorizationCatalogInputs = append(authorizationCatalogInputs, filesModule.routes...)
	authorizationCatalogInputs = append(authorizationCatalogInputs, auditModule.routes...)
	authorizationRoutes := authorizationDescriptors(authorizationApplication, authorizationCatalogInputs)
	descriptors := make([]routecatalog.Descriptor, 0, len(existingRoutes)+len(authorizationRoutes)+len(identityRoutes)+len(filesModule.routes)+len(auditModule.routes))
	descriptors = append(descriptors, existingRoutes...)
	descriptors = append(descriptors, authorizationRoutes...)
	descriptors = append(descriptors, identityRoutes...)
	descriptors = append(descriptors, filesModule.routes...)
	descriptors = append(descriptors, auditModule.routes...)
	catalog, err := routecatalog.New(descriptors)
	if err != nil {
		return nil, fmt.Errorf("build Route Catalog: %w", err)
	}
	routeSource.Set(catalog.Snapshot())
	if err := Migrate(resources.DB); err != nil {
		return nil, fmt.Errorf("migrate database: %w", err)
	}

	snapshot := catalog.Snapshot()
	seedFunction := options.Seed
	if seedFunction == nil {
		seedFunction = func(ctx context.Context, conf platformconfig.Config, descriptors []routecatalog.Descriptor) error {
			return seedDescriptors(ctx, conf, resources.DB, resources.Logger, descriptors)
		}
	}
	if err := seedFunction(ctx, conf, snapshot); err != nil {
		return nil, fmt.Errorf("seed database: %w", err)
	}

	technicalHTTP := options.TechnicalHTTP
	if conf.APIDocs.Enabled {
		documentation := apidocmodule.New(catalog, apiDocMetadataSource{core: modules.APICore}, apidocmodule.Config{Title: conf.APIDocs.Title, Version: conf.APIDocs.Version, Description: conf.APIDocs.Description})
		technicalHTTP.APIDocsEnabled = true
		if technicalHTTP.Docs == nil {
			technicalHTTP.Docs = documentation.Index
		}
		if technicalHTTP.OpenAPI == nil {
			technicalHTTP.OpenAPI = documentation.OpenAPI
		}
	}
	engine := gin.New()
	engine.Use(gin.Recovery())
	httpMiddleware := options.Middleware
	if httpMiddleware.API == nil {
		httpMiddleware.API = auditMiddleware(auditModule.recorder)
	}
	if httpMiddleware.Authenticated == nil {
		httpMiddleware.Authenticated = identityhttp.AuthMiddleware(identityModule.service)
	}
	if httpMiddleware.PermissionControlled == nil {
		httpMiddleware.PermissionControlled = apihttp.PermissionMiddleware(modules.APICore)
	}
	RegisterHTTP(engine, catalog, httpMiddleware)
	RegisterTechnicalHTTP(engine, technicalHTTP)
	application := &Application{
		resources: resources, catalog: catalog, engine: engine,
		address: fmt.Sprintf(":%d", conf.Server.Port), identity: identityModule.service,
		navigation: modules.Navigation, apiMetadata: modules.APIService,
		jobs: append(append(append([]BackgroundJob(nil), options.BackgroundJobs...), filesModule.jobs...), auditModule.jobs...),
	}
	return application, nil
}

func dictionaryDescriptors(resources Resources) []routecatalog.Descriptor {
	repository := dictionary.NewGORMRepository(resources.DB)
	service := dictionary.NewService(repository, platformdatabase.NewTransactionRunner(resources.DB))
	return dictionary.Routes(service)
}

func (application *Application) Handler() http.Handler {
	return application.engine
}

func (application *Application) Run(ctx context.Context) error {
	application.StartBackgroundJobs(ctx)
	application.resources.Logger.Info("server started", zap.String("addr", "http://localhost"+application.address))
	return application.engine.Run(application.address)
}

func (application *Application) Catalog() *routecatalog.Catalog {
	return application.catalog
}

func (application *Application) StartBackgroundJobs(ctx context.Context) {
	application.startOnce.Do(func() {
		jobContext, cancel := context.WithCancel(ctx)
		application.cancel = cancel
		for _, job := range application.jobs {
			if job.Run == nil {
				continue
			}
			application.wait.Add(1)
			go func(job BackgroundJob) {
				defer application.wait.Done()
				job.Run(jobContext)
			}(job)
		}
	})
}

func (application *Application) Close() error {
	application.closeOnce.Do(func() {
		if application.cancel != nil {
			application.cancel()
		}
		application.wait.Wait()
		for index := len(application.closers) - 1; index >= 0; index-- {
			application.closeErr = errors.Join(application.closeErr, application.closers[index]())
		}
	})
	return application.closeErr
}

func validateResources(resources Resources) error {
	switch {
	case resources.DB == nil:
		return errors.New("App database resource is required")
	case resources.Redis == nil:
		return errors.New("App Redis resource is required")
	case resources.MinIO == nil:
		return errors.New("App MinIO resource is required")
	case resources.Logger == nil:
		return errors.New("App logger resource is required")
	default:
		return nil
	}
}
