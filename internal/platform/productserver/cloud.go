package productserver

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nathanxiangang-web/panta/internal/acquisition"
	"github.com/nathanxiangang-web/panta/internal/app"
	"github.com/nathanxiangang-web/panta/internal/platform/bootstrap"
	"github.com/nathanxiangang-web/panta/internal/platform/config"
	"github.com/nathanxiangang-web/panta/internal/product"
	"github.com/nathanxiangang-web/panta/internal/producthttp"
	"github.com/nathanxiangang-web/panta/internal/runtime"
	"github.com/nathanxiangang-web/panta/internal/storage"
	"github.com/nathanxiangang-web/panta/internal/store/postgres"
)

const CloudAction = "serve-product-acquisition"
const CloudPreflightAction = "preflight-product-acquisition"

type CloudDownloadConfig struct {
	Target        producthttp.CloudDownloadTarget `json:"target"`
	BootstrapFile string                          `json:"bootstrap_file"`
}

func (c CloudDownloadConfig) Valid() bool {
	return c.Target.Valid() && c.BootstrapFile != "" && strings.TrimSpace(c.BootstrapFile) == c.BootstrapFile
}

// Composition reuses the existing graph. Construction probes no provider and
// never claims a Job; live login and IndexCore health are deployment checks.
func buildCloud(ctx context.Context, cfg Config, policy config.AcquisitionWorker, pool *pgxpool.Pool) (*runtime.Runtime, producthttp.WorkspaceOption, error) {
	if cfg.CloudDownload == nil || !cfg.CloudDownload.Valid() || !policy.Enabled || policy.Validate() != nil {
		return nil, nil, ErrConfig
	}
	topology, _ := postgres.NewStorageRepository(pool)
	binding, err := topology.GetBinding(ctx, cfg.CloudDownload.Target.BindingID)
	if err != nil || binding.Status != storage.BindingStatusActive || binding.ProviderScope == nil || strings.TrimSpace(*binding.ProviderScope) == "" || storage.ValidateProviderScope(binding.ProviderScope) != nil || binding.IndexCoreRootID == "" {
		return nil, nil, ErrConfig
	}
	deps, err := bootstrap.Load(ctx, cfg.CloudDownload.BootstrapFile)
	if err != nil {
		return nil, nil, ErrConfig
	}
	registered := false
	for _, session := range deps.Sessions {
		if session.ConnectionID == binding.ConnectionID {
			registered = true
		}
	}
	if !registered {
		return nil, nil, ErrConfig
	}
	deps.WorkerOptions = append(deps.WorkerOptions, app.WithWorkerEventSink(func(event app.WorkerEvent) {
		if event.Kind == app.WorkerTransientError || event.Kind == app.WorkerFatalRecoveryDebt || event.DiagnosticCategory != "" {
			log.Printf("cloud worker event=%s job=%s stage=%s category=%s parser=%s http=%d", event.Kind, event.JobID, event.Stage, event.DiagnosticCategory, event.DiagnosticStage, event.DiagnosticHTTPStatus)
		}
	}))
	// Bind both owned pools to the already validated database configuration. A
	// protected-file rotation between two reads must not split admission/Worker.
	worker, err := runtime.NewRuntime(ctx, config.Config{Environment: "product", DatabaseURL: pool.Config().ConnString(), IndexCoreBaseURL: cfg.IndexCoreBaseURL, AcquisitionWorker: policy}, deps)
	if err != nil {
		return nil, nil, startupError(err)
	}
	catalog, _ := postgres.NewCatalogRepository(pool)
	manifests, _ := postgres.NewAcquisitionManifestRepository(pool)
	jobs, _ := postgres.NewJobRepository(pool)
	creator, _ := acquisition.NewService(topology, catalog, manifests)
	activationRepo, _ := postgres.NewAcquisitionActivationRepository(pool)
	activation, _ := acquisition.NewActivationService(activationRepo)
	submitter, _ := acquisition.NewSubmissionService(creator, manifests, activation, jobs)
	store, _ := postgres.NewProductAdmissionRepository(pool)
	service, err := product.NewAdmissionService(store, submitter, []storage.BindingID{cfg.CloudDownload.Target.BindingID})
	if err != nil {
		worker.Close()
		return nil, nil, ErrStartup
	}
	return worker, producthttp.WithCloudDownload(cfg.CloudDownload.Target, service, store), nil
}

type managedWorker interface{ Run(context.Context) error }

// Listener is already acquired before Worker.Run can claim anything. Either
// side exiting cancels and joins the other before database pools are closed.
func serveCloud(ctx context.Context, listener net.Listener, handler http.Handler, worker managedWorker) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	httpDone, workerDone := make(chan error, 1), make(chan error, 1)
	go func() { httpDone <- serve(runCtx, listener, handler) }()
	go func() { workerDone <- worker.Run(runCtx) }()
	var err error
	select {
	case err = <-workerDone:
		unexpected := ctx.Err() == nil
		cancel()
		httpErr := <-httpDone
		if unexpected || err != nil && !errors.Is(err, context.Canceled) || httpErr != nil {
			return ErrStartup
		}
	case err = <-httpDone:
		cancel()
		workerErr := <-workerDone
		if err != nil || workerErr != nil && !errors.Is(workerErr, context.Canceled) {
			return ErrStartup
		}
	}
	return nil
}
