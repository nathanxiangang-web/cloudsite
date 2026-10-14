package productserver

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nathanxiangang-web/panta/internal/auth"
	"github.com/nathanxiangang-web/panta/internal/integrations/indexcore"
	"github.com/nathanxiangang-web/panta/internal/platform/bootstrap"
	"github.com/nathanxiangang-web/panta/internal/product"
	"github.com/nathanxiangang-web/panta/internal/producthttp"
	"github.com/nathanxiangang-web/panta/internal/resourceview"
	"github.com/nathanxiangang-web/panta/internal/storage"
	"github.com/nathanxiangang-web/panta/internal/store/postgres"
)

func startupError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return ErrStartup
}
func openDatabase(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	secret, err := bootstrap.ReadProtectedFile(cfg.DatabaseURLFile, 4096)
	if err != nil {
		return nil, ErrConfig
	}
	defer clear(secret)
	pool, err := postgres.Open(ctx, strings.TrimSpace(string(secret)))
	if err != nil {
		return nil, startupError(err)
	}
	migrator, err := postgres.NewMigrator(pool)
	if err != nil {
		pool.Close()
		return nil, ErrStartup
	}
	status, err := migrator.Status(ctx)
	if err != nil || !status.Compatible || status.CurrentVersion != postgres.CurrentSchemaVersion || status.LatestVersion != postgres.CurrentSchemaVersion {
		pool.Close()
		return nil, ErrStartup
	}
	return pool, nil
}

// Run is explicit and owns exactly one pool. It never migrates a database.
func Run(ctx context.Context, action, configFile, userFile string) error {
	if action != ServeAction && action != ProvisionAction {
		return ErrConfig
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	startupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	cfg, err := LoadConfig(startupCtx, configFile)
	if err != nil {
		cancel()
		return err
	}
	var input provisionRequest
	if action == ProvisionAction {
		input, err = loadProvision(startupCtx, userFile)
		if err != nil {
			cancel()
			return err
		}
		defer func() { input.Password = "" }()
	}
	pool, err := openDatabase(startupCtx, cfg)
	if err != nil {
		cancel()
		return err
	}
	defer pool.Close()
	repo, err := postgres.NewAuthRepository(pool)
	if err != nil {
		cancel()
		return ErrStartup
	}
	accounts, err := auth.NewService(repo)
	if err != nil {
		cancel()
		return ErrStartup
	}
	if action == ProvisionAction {
		_, err = accounts.Provision(startupCtx, input.Login, input.Password, input.Role)
		cancel()
		if errors.Is(err, auth.ErrConflict) {
			return auth.ErrConflict
		}
		if err != nil {
			return startupError(err)
		}
		return nil
	}
	topology, err := postgres.NewStorageRepository(pool)
	if err != nil {
		cancel()
		return ErrStartup
	}
	bindings := make([]storage.BindingID, len(cfg.Bindings))
	for i, display := range cfg.Bindings {
		binding, err := topology.GetBinding(startupCtx, display.ID)
		if err != nil || binding.ID != display.ID || binding.IndexCoreRootID == "" || !binding.Status.Valid() {
			cancel()
			return ErrStartup
		}
		bindings[i] = display.ID
	}
	catalog, err := postgres.NewCatalogRepository(pool)
	if err != nil {
		cancel()
		return ErrStartup
	}
	tasks, err := postgres.NewProductReadRepository(pool)
	if err != nil {
		cancel()
		return ErrStartup
	}
	index, err := indexcore.NewClient(cfg.IndexCoreBaseURL)
	if err != nil {
		cancel()
		return ErrStartup
	}
	physical, err := resourceview.NewPhysicalService(index, catalog)
	if err != nil {
		cancel()
		return ErrStartup
	}
	reads, err := product.NewReadService(tasks, topology, index, physical, catalog, bindings)
	if err != nil {
		cancel()
		return ErrStartup
	}
	handler, err := producthttp.NewWorkspaceHandler(accounts, cfg.Origin, reads, cfg.Bindings, postgres.CurrentSchemaVersion)
	if err != nil {
		cancel()
		return ErrStartup
	}
	cancel()
	if err = ctx.Err(); err != nil {
		return err
	}
	listener, err := net.Listen("tcp", cfg.ListenAddress)
	if err != nil {
		return ErrStartup
	}
	// No URL/credentials are logged. Connection errors cannot include backend
	// responses or sources through the server's default logger.
	return serve(ctx, listener, handler)
}
func serve(ctx context.Context, listener net.Listener, handler http.Handler) error {
	requestCtx, stopRequests := context.WithCancel(context.Background())
	defer stopRequests()
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10, ErrorLog: log.New(io.Discard, "", 0), BaseContext: func(net.Listener) context.Context { return requestCtx }}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case err := <-done:
		stopRequests()
		_ = server.Close()
		if active, ok := handler.(interface{ Wait() }); ok {
			active.Wait()
		}
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return ErrStartup
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := server.Shutdown(shutdownCtx)
		cancel()
		if err != nil {
			stopRequests()
			_ = server.Close()
		}
		<-done
		// Every request has <=15s application lifetime. Join outstanding handlers
		// before the caller closes the pool, including a forced connection close.
		if active, ok := handler.(interface{ Wait() }); ok {
			active.Wait()
		}
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return ErrStartup
		}
		return nil
	}
}
