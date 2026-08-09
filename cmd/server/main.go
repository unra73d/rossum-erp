// Command server runs the mock ERPX: vendor master data, the invoice posting
// webhook Rossum calls, and the small UI that makes both visible.
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

	"github.com/unra73d/rossum-erp/internal/api"
	"github.com/unra73d/rossum-erp/internal/store"
	"github.com/unra73d/rossum-erp/web"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	dataPath := env("ERPX_DATA", "data/erpx.json")
	st, err := store.Open(dataPath)
	if err != nil {
		log.Error("cannot open store", "path", dataPath, "error", err)
		os.Exit(1)
	}

	ui, err := web.Dist()
	if err != nil {
		// The API is still fully usable without the UI bundle, which keeps
		// `go run ./cmd/server` working before the first `npm run build`.
		log.Warn("UI bundle not embedded, serving API only", "error", err)
		ui = nil
	}

	addr := ":" + env("PORT", "8080")
	srv := &http.Server{
		Addr:              addr,
		Handler:           api.New(st, ui, log),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Info("ERPX listening", "addr", addr, "data", dataPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server stopped", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("shutdown", "error", err)
	}
	log.Info("bye")
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
