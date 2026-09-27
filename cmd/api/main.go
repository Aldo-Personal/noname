package main

import (
	"infra.local/platform/internal/platform/server"
	"log/slog"
	"os"
)

func main() {
	if err := server.Run("api"); err != nil {
		slog.Error("service stopped", "error", err)
		os.Exit(1)
	}
}
