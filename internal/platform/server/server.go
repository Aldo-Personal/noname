// Package server owns process lifecycle and baseline HTTP behavior.
package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"
)

// Version is overridden at release build time.
var Version = "0.1.0-dev"

type Status struct {
	Service string `json:"service"`
	Version string `json:"version"`
	Mode    string `json:"mode"`
}

func JSON(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Debug("response write failed", "error", err)
	}
}

// Handler exposes only scaffold diagnostics. No product operations are enabled.
func Handler(service string, ready *atomic.Bool) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		JSON(w, http.StatusOK, map[string]string{"status": "alive"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if !ready.Load() {
			JSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
			return
		}
		JSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	if service == "api" {
		mux.HandleFunc("GET /v1/status", func(w http.ResponseWriter, r *http.Request) {
			JSON(w, http.StatusOK, Status{Service: service, Version: Version, Mode: "scaffold"})
		})
	}
	if service == "gateway" {
		mux.HandleFunc("POST /rpc", func(w http.ResponseWriter, r *http.Request) {
			JSON(w, http.StatusServiceUnavailable, map[string]string{"code": "not_configured", "message": "RPC forwarding is not enabled."})
		})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := make([]byte, 16)
		if _, err := rand.Read(id); err != nil {
			http.Error(w, "request initialization failed", 500)
			return
		}
		w.Header().Set("X-Request-ID", hex.EncodeToString(id))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		mux.ServeHTTP(w, r)
	})
}

func Address(service string) (string, error) {
	ports := map[string]string{"api": "8080", "gateway": "8081", "worker": "8082"}
	port, ok := ports[service]
	if !ok {
		return "", fmt.Errorf("unknown service %q", service)
	}
	env := os.Getenv("APP_ENV")
	if env != "" && env != "development" && env != "test" {
		return "", fmt.Errorf("scaffold is restricted to development/test; production prerequisites are in docs/operations.md")
	}
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = "127.0.0.1:" + port
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("HTTP_ADDR: %w", err)
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return "", fmt.Errorf("HTTP_ADDR must contain a port from 1 to 65535")
	}
	return addr, nil
}

func Run(service string) error {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	addr, err := Address(service)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var ready atomic.Bool
	// The worker has no durable queue adapter yet and must not advertise readiness.
	ready.Store(service != "worker")
	srv := &http.Server{Addr: addr, Handler: Handler(service, &ready), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	result := make(chan error, 1)
	go func() { result <- srv.Serve(listener) }()
	slog.Info("service started", "service", service, "address", addr, "version", Version, "mode", "scaffold")
	select {
	case err := <-result:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		ready.Store(false)
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdown); err != nil {
			_ = srv.Close()
			return fmt.Errorf("shutdown: %w", err)
		}
		err := <-result
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}
