// Command birrd serves the rates API and, optionally, the built dashboard.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/melastore/birrwatch/internal/api"
	"github.com/melastore/birrwatch/internal/store"
)

func main() {
	if err := run(); err != nil {
		slog.Error("server failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		addr   = flag.String("addr", ":8080", "listen address")
		dbPath = flag.String("db", "birrwatch.db", "path to the SQLite database")
		webDir = flag.String("web", "", "directory of built dashboard assets (optional)")
	)
	flag.Parse()

	// PORT is how most container platforms tell a process where to listen.
	if p := os.Getenv("PORT"); p != "" {
		*addr = ":" + p
	}

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	db, err := store.Open(*dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	srv := &http.Server{
		Addr:              *addr,
		Handler:           api.New(db, log, *webDir).Routes(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", *addr, "db", *dbPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	}
}
