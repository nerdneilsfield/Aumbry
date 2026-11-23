// Command aumbry-server runs the Aumbry HTTP server and maintenance subcommands.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/nerdneilsfield/aumbry/internal/api"
	"github.com/nerdneilsfield/aumbry/internal/auth"
	"github.com/nerdneilsfield/aumbry/internal/config"
	"github.com/nerdneilsfield/aumbry/internal/db"
	"github.com/nerdneilsfield/aumbry/internal/storage"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cmd := newRootCmd(logger)
	if err := cmd.ExecuteContext(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd(logger *slog.Logger) *cobra.Command {
	var configPath string
	root := &cobra.Command{
		Use:           "aumbry-server",
		Short:         "Run and maintain the Aumbry server",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServer(cmd.Context(), configPath, logger)
		},
	}
	root.PersistentFlags().StringVar(&configPath, "config", "", "path to TOML configuration file")
	root.AddCommand(
		newCheckConfigCmd(&configPath),
		newBackupCmd(&configPath),
		newVerifyStorageCmd(&configPath),
		newGCCmd(&configPath),
		newReindexCmd(&configPath),
	)
	return root
}

func newCheckConfigCmd(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "check-config",
		Short: "Validate server configuration and print masked effective settings",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := config.Load(*configPath)
			if err != nil {
				return err
			}
			for _, w := range res.Warnings {
				if _, err = fmt.Fprintln(cmd.ErrOrStderr(), "warning:", w); err != nil {
					return err
				}
			}
			out, err := res.Config.FormatMasked()
			if err != nil {
				return err
			}
			_, err = io.WriteString(cmd.OutOrStdout(), out)
			return err
		},
	}
}

func runServer(ctx context.Context, configPath string, logger *slog.Logger) error {
	res, err := config.Load(configPath)
	if err != nil {
		return err
	}
	cfg := res.Config
	if res.FilePath != "" {
		logger.Info("loaded configuration", "source", res.FilePath)
	} else {
		logger.Info("loaded configuration", "source", "environment")
	}
	for _, w := range res.Warnings {
		logger.Warn("configuration warning", "warning", w)
	}

	sessions, err := auth.New(auth.Settings{
		AdminUser:         cfg.Admin.User,
		AdminPasswordHash: cfg.Admin.PasswordHash,
		APITokens:         cfg.APITokens,
		CookieSecret:      cfg.CookieSecret,
	})
	if err != nil {
		return err
	}
	store, err := openStore(cfg, true)
	if err != nil {
		return fmt.Errorf("configure %s storage: %w", cfg.Storage, err)
	}
	lock, err := db.TryLockDataDir(cfg.DataDir)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := lock.Close(); closeErr != nil {
			logger.Error("release data directory lock", "error", closeErr)
		}
	}()
	database, err := db.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := database.Close(); closeErr != nil {
			logger.Error("close database", "error", closeErr)
		}
	}()

	ready := api.NewReadinessChecker(database, store, logger, 0, 0)
	if rep := ready.Check(ctx); rep.Ready {
		logger.Info("startup dependency check passed", "sqlite", rep.Checks["sqlite"], "storage", rep.Checks["storage"])
	} else {
		logger.Warn("startup dependency check degraded", "sqlite", rep.Checks["sqlite"], "storage", rep.Checks["storage"])
	}

	router, err := api.NewRouter(database, store, sessions, logger, api.RouterOptions{
		TrustedProxies:      cfg.TrustedProxyPrefixes,
		PublicURL:           cfg.PublicURL,
		MaxRevisionsPerItem: cfg.Revisions.MaxPerItem,
		MaxIngestBodyBytes:  cfg.ResolvedMaxIngestBodyBytes(),
	})
	if err != nil {
		return err
	}
	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

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
		return server.Shutdown(shutdownCtx)
	}
}

func openStore(cfg config.Config, createLocalRoot bool) (storage.Store, error) {
	switch cfg.Storage {
	case "local":
		return storage.NewLocalWithOptions(cfg.ResolvedLocalPath(), storage.LocalOptions{CreateRoot: createLocalRoot})
	case "webdav":
		return storage.NewWebDAV(cfg.WebDAV.URL, cfg.WebDAV.User, cfg.WebDAV.Password)
	case "s3", "r2":
		s3cfg := cfg.ResolvedS3()
		return storage.NewS3(storage.S3Options{
			Endpoint:        s3cfg.Endpoint,
			Region:          s3cfg.Region,
			Bucket:          s3cfg.Bucket,
			AccessKeyID:     s3cfg.AccessKeyID,
			SecretAccessKey: s3cfg.SecretAccessKey,
			PathStyle:       s3cfg.PathStyle,
		})
	default:
		return nil, fmt.Errorf("unsupported storage backend %q", cfg.Storage)
	}
}
