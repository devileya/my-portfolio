// Command portfolio is the whole site in one binary: API, dashboard, media
// proxy and the embedded React app.
//
//	portfolio            run the server
//	portfolio seed       load seed/data.json into an empty database
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/devileya/my-portfolio/internal/auth"
	"github.com/devileya/my-portfolio/internal/config"
	"github.com/devileya/my-portfolio/internal/content"
	"github.com/devileya/my-portfolio/internal/db"
	"github.com/devileya/my-portfolio/internal/seed"
	"github.com/devileya/my-portfolio/internal/server"
	"github.com/devileya/my-portfolio/internal/storage"
	"github.com/devileya/my-portfolio/web"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := db.Migrate(ctx, pool); err != nil {
		return err
	}

	store, err := storage.New(cfg)
	if err != nil {
		return err
	}
	if store == nil {
		slog.Warn("object storage not configured; uploads and images are disabled")
	}
	repo := content.NewRepo(pool)

	if len(os.Args) > 1 && os.Args[1] == "seed" {
		fs := flag.NewFlagSet("seed", flag.ExitOnError)
		dir := fs.String("dir", "seed", "directory with data.json and images/")
		cv := fs.String("cv", "", "PDF to upload as the CV")
		force := fs.Bool("force", false, "replace existing projects, experience and skills")
		_ = fs.Parse(os.Args[2:])
		return seed.Run(ctx, pool, repo, store, seed.Options{Dir: *dir, CVPath: *cv, Force: *force})
	}

	authSvc := auth.New(pool, cfg.JWTSecret, cfg.SessionTTL, cfg.Production())
	if created, err := authSvc.Bootstrap(ctx, cfg.AdminEmail, cfg.AdminPassword); err != nil {
		return err
	} else if created {
		slog.Info("created admin account", "email", cfg.AdminEmail)
	}

	srv, err := server.New(cfg, repo, authSvc, store, web.FS())
	if err != nil {
		return err
	}
	httpSrv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       2 * time.Minute,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}

	errc := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", httpSrv.Addr, "dashboard", cfg.AdminPath)
		errc <- httpSrv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http server: %w", err)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpSrv.Shutdown(shutdownCtx)
	}
	return nil
}
