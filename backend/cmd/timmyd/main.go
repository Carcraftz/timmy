package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"tailscale.com/tsnet"

	"timmy/backend/internal/auth"
	"timmy/backend/internal/config"
	"timmy/backend/internal/httpapi"
	"timmy/backend/internal/store"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	if err := os.MkdirAll(cfg.StateDir, 0o700); err != nil {
		log.Fatalf("create state directory: %v", err)
	}

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	defer pool.Close()

	if err := store.ApplyMigrations(ctx, pool); err != nil {
		log.Fatalf("apply migrations: %v", err)
	}

	st := store.NewPostgresStore(pool)
	tailnetID, err := st.EnsureTailnet(ctx, cfg.TailnetName)
	if err != nil {
		log.Fatalf("ensure tailnet: %v", err)
	}

	ts := &tsnet.Server{
		Hostname:      cfg.Hostname,
		Dir:           cfg.StateDir,
		AuthKey:       cfg.AuthKey,
		ClientSecret:  cfg.ClientSecret,
		AdvertiseTags: cfg.AdvertiseTags,
	}
	defer ts.Close()

	localClient, err := ts.LocalClient()
	if err != nil {
		log.Fatalf("start tsnet local client: %v", err)
	}

	handler := httpapi.NewHandler(
		st,
		tailnetID,
		cfg.TailnetName,
		auth.NewLocalIdentityResolver(localClient),
	)

	server := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	var (
		listener    net.Listener
		listenerErr error
	)
	if cfg.UseTLS {
		listener, listenerErr = ts.ListenTLS("tcp", cfg.ListenAddr)
	} else {
		listener, listenerErr = ts.Listen("tcp", cfg.ListenAddr)
	}
	if listenerErr != nil {
		log.Fatalf("listen: %v", listenerErr)
	}
	defer listener.Close()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	log.Printf("timmyd listening on %s://%s%s", scheme(cfg.UseTLS), cfg.Hostname, cfg.ListenAddr)

	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("serve: %v", err)
	}
}

func scheme(useTLS bool) string {
	if useTLS {
		return "https"
	}
	return "http"
}
