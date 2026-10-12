package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"example.com/pengumpulan_sampah/internal/config"
	"example.com/pengumpulan_sampah/internal/handler"
	"example.com/pengumpulan_sampah/internal/repository"
	"example.com/pengumpulan_sampah/internal/service"
	"example.com/pengumpulan_sampah/internal/storage"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("application stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	m, err := migrate.New("file://migrations", cfg.DatabaseURL)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		_, _ = m.Close()
		return err
	}
	if sourceErr, dbErr := m.Close(); sourceErr != nil {
		logger.Warn("migration source close", "error", sourceErr)
	} else if dbErr != nil {
		logger.Warn("migration database close", "error", dbErr)
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return err
	}
	files, err := storage.NewLocal(cfg.UploadDir)
	if err != nil {
		return err
	}
	repo := repository.NewPostgres(pool)
	router := handler.NewRouter(service.NewHousehold(repo), service.NewPickup(repo),
		service.NewPayment(repo), service.NewReport(repo), files, cfg.UploadDir, cfg.MaxUploadMB, logger)
	server := &http.Server{
		Addr:              ":" + cfg.AppPort,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serverErr := make(chan error, 1)
	go func() {
		logger.Info("server listening", "address", server.Addr)
		serverErr <- server.ListenAndServe()
	}()
	select {
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Shutdown)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
	}
	return nil
}
