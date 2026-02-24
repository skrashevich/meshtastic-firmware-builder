package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"meshtastic-firmware-builder/backend/internal/config"
	"meshtastic-firmware-builder/backend/internal/httpapi"
	"meshtastic-firmware-builder/backend/internal/jobs"
)

func main() {
	logger := log.New(os.Stdout, "", log.LstdFlags|log.LUTC)

	cfg, err := config.Load()
	if err != nil {
		logger.Fatalf("load config: %v", err)
	}

	manager := jobs.NewManager(cfg, logger)
	defer manager.Close()

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           httpapi.NewServer(cfg, manager, logger),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Printf("backend listening on %s", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("http server failed: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Printf("graceful shutdown failed: %v", err)
	}
}
