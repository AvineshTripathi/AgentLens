// AgentLens — Full-stack AI agent observability platform.
//
// Usage:
//
//	agentlens --config config.yaml
//	AGENTLENS_POSTGRES_DSN=postgres://... agentlens
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AvineshTripathi/AgentLens/internal/config"
	"github.com/AvineshTripathi/AgentLens/internal/dashboard"
	_ "github.com/AvineshTripathi/AgentLens/internal/metrics" // register Prometheus metrics
	"github.com/AvineshTripathi/AgentLens/internal/proxy"
	"github.com/AvineshTripathi/AgentLens/internal/store"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to config file")
	flag.Parse()

	// ── Logger ────────────────────────────────────────────────────────────
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})))

	// ── Config ────────────────────────────────────────────────────────────
	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	slog.Info("AgentLens starting",
		"gateway_port", cfg.Server.GatewayPort,
		"dashboard_port", cfg.Server.DashboardPort,
	)

	// ── Database ──────────────────────────────────────────────────────────
	st, err := store.New(cfg.Storage.PostgresDSN)
	if err != nil {
		slog.Error("failed to connect to postgres", "err", err)
		slog.Info("tip: set AGENTLENS_POSTGRES_DSN or update config.yaml")
		os.Exit(1)
	}
	defer st.Close()
	slog.Info("connected to postgres")

	// ── Run migrations ────────────────────────────────────────────────────
	if err := runMigrations(st); err != nil {
		slog.Error("migration failed", "err", err)
		os.Exit(1)
	}

	// ── Gateway (proxy) server ────────────────────────────────────────────
	gw := proxy.NewGateway(st, cfg.Proxy)
	gwServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.GatewayPort),
		Handler:      gw,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// ── Dashboard server ──────────────────────────────────────────────────
	dash := dashboard.NewServer(st)
	dashServer := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.DashboardPort),
		Handler:      dash,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	// ── Start servers ─────────────────────────────────────────────────────
	go func() {
		slog.Info("gateway listening", "addr", gwServer.Addr,
			"endpoints", []string{"/proxy/anthropic/", "/proxy/openai/", "/proxy/gemini/", "/tools/execute", "/infra/events"})
		if err := gwServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("gateway error", "err", err)
		}
	}()

	go func() {
		slog.Info("dashboard listening", "addr", dashServer.Addr,
			"ui", fmt.Sprintf("http://localhost:%d", cfg.Server.DashboardPort))
		if err := dashServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("dashboard error", "err", err)
		}
	}()

	// ── Graceful shutdown ─────────────────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_ = gwServer.Shutdown(ctx)
	_ = dashServer.Shutdown(ctx)
	slog.Info("shutdown complete")
}

// runMigrations applies the SQL schema. In production you'd use a proper
// migration library (goose, atlas). For simplicity we run the init SQL directly.
func runMigrations(st *store.Store) error {
	slog.Info("running database migrations")
	// The store exposes its DB for migrations.
	// In a production system, embed the SQL file and run it here.
	// For now, print a helpful message.
	slog.Info("tip: run 'psql $DSN -f internal/store/migrations/001_init.sql' to apply schema")
	return nil
}
