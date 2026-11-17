package app

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/acme/autocert"

	"github.com/docpocket/docpocket/internal/api"
	"github.com/docpocket/docpocket/internal/auth"
	"github.com/docpocket/docpocket/internal/files"
	"github.com/docpocket/docpocket/internal/query"
	"github.com/docpocket/docpocket/internal/realtime"
	"github.com/docpocket/docpocket/internal/rules"
	"github.com/docpocket/docpocket/internal/storage"
	"github.com/docpocket/docpocket/ui"
)

// App owns every service and the HTTP server.
type App struct {
	Config Config
	Store  *storage.BoltStore
	Auth   *auth.Service
	Rules  *rules.Engine
	Hub    *realtime.Hub
	Files  *files.Store
	Engine *query.Engine
	API    *api.Server
	HTTP   *http.Server
	Logger *slog.Logger

	tlsConfig *tls.Config
}

// New opens the data directory and wires the application.
func New(cfg Config) (*App, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	logger := newLogger(cfg.LogLevel)
	store, err := storage.Open(cfg.Dir, storage.Options{
		ReadOnly:   cfg.ReadOnly,
		NoSync:     cfg.NoSync,
		MaxDocSize: cfg.MaxDocSize,
	})
	if err != nil {
		return nil, err
	}
	closeOnError := func(err error) (*App, error) {
		_ = store.Close()
		return nil, err
	}

	authSvc, err := auth.New(store, auth.Config{
		AccessTTL:  cfg.AccessTTL,
		RefreshTTL: cfg.RefreshTTL,
	})
	if err != nil {
		return closeOnError(err)
	}
	ruleEngine, err := rules.New()
	if err != nil {
		return closeOnError(err)
	}
	fileStore, err := files.New(filepath.Join(cfg.Dir, "files"), cfg.MaxUploadBytes)
	if err != nil {
		return closeOnError(err)
	}
	hub := realtime.NewHub(1024, 128)
	engine := query.NewEngine(store, cfg.QueryLimits)
	apiServer, err := api.New(api.Deps{
		Config: api.Config{
			MaxBodyBytes:   cfg.MaxBodyBytes,
			MaxUploadBytes: cfg.MaxUploadBytes,
			CORSOrigins:    cfg.CORSOrigins,
			StrictIndexes:  cfg.StrictIndexes,
			QueryLimits:    cfg.QueryLimits,
			TrustProxy:     cfg.TrustProxy,
		},
		Store:   store,
		Auth:    authSvc,
		Rules:   ruleEngine,
		Hub:     hub,
		Files:   fileStore,
		Engine:  engine,
		UI:      ui.FS(),
		Logger:  logger,
		Hooks:   cfg.Hooks,
		Version: cfg.Version,
	})
	if err != nil {
		hub.Close()
		return closeOnError(err)
	}

	addr := cfg.HTTPAddr
	if cfg.HTTPSDomain != "" {
		addr = cfg.HTTPSAddr
	}
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           apiServer.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		// WriteTimeout must stay zero: SSE streams are long-lived.
		IdleTimeout:    120 * time.Second,
		MaxHeaderBytes: 1 << 20,
		ErrorLog:       slog.NewLogLogger(logger.Handler(), slog.LevelError),
	}
	a := &App{
		Config: cfg,
		Store:  store,
		Auth:   authSvc,
		Rules:  ruleEngine,
		Hub:    hub,
		Files:  fileStore,
		Engine: engine,
		API:    apiServer,
		HTTP:   httpServer,
		Logger: logger,
	}
	if cfg.HTTPSDomain != "" {
		cacheDir := cfg.AutocertDir
		if cacheDir == "" {
			cacheDir = filepath.Join(cfg.Dir, "autocert")
		}
		manager := &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			HostPolicy: autocert.HostWhitelist(cfg.HTTPSDomain),
			Cache:      autocert.DirCache(cacheDir),
		}
		a.tlsConfig = manager.TLSConfig()
	}
	return a, nil
}

// Handler exposes the HTTP handler for tests and integrations.
func (a *App) Handler() http.Handler { return a.HTTP.Handler }

// Run listens until the context is canceled or the server fails.
func (a *App) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	errCh := make(chan error, 1)
	go func() {
		var err error
		switch {
		case a.tlsConfig != nil:
			a.HTTP.TLSConfig = a.tlsConfig
			err = a.HTTP.ListenAndServeTLS("", "")
		case a.Config.TLSCert != "":
			err = a.HTTP.ListenAndServeTLS(a.Config.TLSCert, a.Config.TLSKey)
		default:
			err = a.HTTP.ListenAndServe()
		}
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errCh <- err
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := a.HTTP.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("http shutdown: %w", err)
		}
		return nil
	}
}

// Close stops realtime delivery and closes the database.
func (a *App) Close() error {
	if a == nil {
		return nil
	}
	if a.API != nil {
		a.API.Close()
	}
	if a.Store != nil {
		return a.Store.Close()
	}
	return nil
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}
