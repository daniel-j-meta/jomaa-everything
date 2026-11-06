// Command hub runs the home hub: it loads the device layout, connects to the
// Matter server, and serves the HTTP API.
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

	"github.com/jomaa-daniel/jomaa-everything/jomaa-home-hub/internal/api"
	"github.com/jomaa-daniel/jomaa-everything/jomaa-home-hub/internal/config"
	"github.com/jomaa-daniel/jomaa-everything/jomaa-home-hub/internal/govee"
	"github.com/jomaa-daniel/jomaa-everything/jomaa-home-hub/internal/hub"
	"github.com/jomaa-daniel/jomaa-everything/jomaa-home-hub/internal/matter"
)

func main() {
	configPath := flag.String("config", "config.json", "path to the device and room layout")
	flag.Parse()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(*configPath, log); err != nil {
		log.Error("hub stopped", "err", err)
		os.Exit(1)
	}
}

func run(configPath string, log *slog.Logger) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	password := os.Getenv("HUB_PASSWORD")
	if password == "" {
		return errors.New("HUB_PASSWORD is not set")
	}
	goveeKey := os.Getenv("GOVEE_API_KEY")
	if cfg.UsesDriver(config.DriverGovee) && goveeKey == "" {
		return errors.New("config has Govee devices but GOVEE_API_KEY is not set")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	mc := matter.NewClient(cfg.MatterServerURL, log.With("component", "matter"))
	go mc.Run(ctx)

	gc := govee.NewClient(goveeKey)
	if u := os.Getenv("GOVEE_BASE_URL"); u != "" {
		gc.BaseURL = u // for tests against a fake Govee API
	}
	h, err := hub.Build(cfg, gc, mc)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           api.New(h, mc, password, log).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Info("hub listening", "addr", cfg.Listen, "devices", len(cfg.Devices), "rooms", len(cfg.Rooms))

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
