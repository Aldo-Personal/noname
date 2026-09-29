// usage-maintenance runs one bounded reconciliation and retention pass.
package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"infra.local/platform/internal/platform/database"
	"infra.local/platform/internal/platform/server"
	"infra.local/platform/internal/usage"
)

func main() {
	if err := run(); err != nil {
		slog.Error("usage maintenance failed")
		os.Exit(1)
	}
}
func run() error {
	if _, err := server.Address("worker"); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := database.Open(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := database.Check(ctx, pool); err != nil {
		return err
	}
	count, err := (&usage.Store{Pool: pool}).Maintain(ctx)
	if err == nil {
		slog.Info("usage maintenance completed", "reconciliation_candidates", count)
	}
	return err
}
