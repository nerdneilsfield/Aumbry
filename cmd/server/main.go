package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nerdneilsfield/aumbry/internal/api"
	"github.com/nerdneilsfield/aumbry/internal/auth"
	"github.com/nerdneilsfield/aumbry/internal/config"
	"github.com/nerdneilsfield/aumbry/internal/db"
	"github.com/nerdneilsfield/aumbry/internal/storage"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	if err := run(logger); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	sessions, err := auth.New(auth.Settings{
		AdminUser:         cfg.AdminUser,
		AdminPasswordHash: cfg.AdminPasswordHash,
		APITokens:         cfg.APITokens,
		CookieSecret:      cfg.CookieSecret,
	})
	if err != nil {
		return err
	}
	store, err := storage.NewWebDAV(cfg.WebDAVURL, cfg.WebDAVUser, cfg.WebDAVPassword)
	if err != nil {
		return fmt.Errorf("configure WebDAV storage: %w", err)
	}
	database, err := db.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer func() {
		if err := database.Close(); err != nil {
			logger.Error("close database", "error", err)
		}
	}()

	router, err := api.NewRouter(database, store, sessions, logger)
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("server listening", "address", cfg.ListenAddr)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return nil
	}
}
