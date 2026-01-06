package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"tags-worker/internal"
)

func mustAbs(path, name string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		log.Fatalf("Failed to resolve %s directory: %v", name, err)
	}
	return abs
}

func main() {
	// Configure logger to not include timestamps
	log.SetFlags(0)

	if len(os.Args) != 3 {
		log.Fatal("Usage: tags-worker <input-dir> <output-dir>")
	}

	inputDir := os.Args[1]
	outputDir := os.Args[2]

	cfg := internal.LoadConfig()

	absInput := mustAbs(inputDir, "input")
	absOutput := mustAbs(outputDir, "output")

	watcher, err := internal.NewWatcher(cfg, absInput, absOutput)
	if err != nil {
		log.Fatalf("Failed to create webp watcher: %v", err)
	}
	defer watcher.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Handle signals
	go func() {
		sig := <-sigChan
		log.Printf("Received signal: %v, shutting down...", sig)
		cancel()
	}()

	// Start watcher (this blocks until context is cancelled)
	watcher.Start(ctx)

	// Ensure clean shutdown
	log.Printf("Shutdown complete")
}
