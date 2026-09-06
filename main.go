package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	printServerBanner()
	startupStarted := time.Now()

	cfgMgr := NewConfigManager("config.yml")
	cfg, err := cfgMgr.Load()
	if err != nil {
		logError("Failed to load server config: %v", err)
		os.Exit(1)
	}

	server := NewServer(cfgMgr)
	if err := server.Start(); err != nil {
		logError("Failed to start server on port %d: %v", cfg.Port, err)
		os.Exit(1)
	}

	printServerBoot(cfg, time.Since(startupStarted))

	stopChan := make(chan struct{})

	go runConsole(server, stopChan)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case <-stopChan:
	case <-sigChan:
		logInfo("[CyuCore] Received interrupt signal. Stopping server...")
		server.Stop()
		logInfo("[CyuCore] Server stopped.")
	}
}
