package internal

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
)

const (
	colorReset   = "\033[0m"
	colorRed     = "\033[0;31m"
	colorMagenta = "\033[0;35m"
	colorCyan    = "\033[0;36m"
)

func colorize(color, text string) string {
	return color + text + colorReset
}

type fileJob struct {
	path string
	info os.FileInfo
}

type Watcher struct {
	config      *Config
	inputDir    string
	outputDir   string
	db          *DB
	fileWatcher *fsnotify.Watcher
	jobQueue    chan fileJob
	wg          sync.WaitGroup
}

func NewWatcher(cfg *Config, inputDir, outputDir string) (*Watcher, error) {
	// Initialize database
	database, err := NewDB(cfg.DBFile)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// Create file watcher
	fileWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	// Create job queue with buffer
	queueSize := cfg.Workers * 2

	w := &Watcher{
		config:      cfg,
		inputDir:    inputDir,
		outputDir:   outputDir,
		db:          database,
		fileWatcher: fileWatcher,
		jobQueue:    make(chan fileJob, queueSize),
	}

	return w, nil
}

func (w *Watcher) Start(ctx context.Context) {
	defer w.shutdown()

	// Start worker pool
	log.Printf("Starting %d worker(s) for webp generation", w.config.Workers)
	for i := 0; i < w.config.Workers; i++ {
		w.wg.Add(1)
		go w.worker(ctx)
	}

	// Initial scan of all files (also sets up watching) - cancellable
	log.Printf("Starting initial scan of %s", w.inputDir)
	scanDone := make(chan struct{})
	go func() {
		w.scanDirectory(w.inputDir)
		close(scanDone)
	}()

	// Wait for scan to complete or context cancellation
	select {
	case <-ctx.Done():
		return
	case <-scanDone:
		log.Printf("Watching %s for new files...", w.inputDir)
	}

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-w.fileWatcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Create|fsnotify.Write) != 0 {
				w.handleFileEvent(event.Name)
			}
		case err, ok := <-w.fileWatcher.Errors:
			if !ok {
				return
			}
			log.Printf("File watcher error: %v", err)
		}
	}
}

func (w *Watcher) shutdown() {
	// Close file watcher first to unblock the select loop
	if w.fileWatcher != nil {
		w.fileWatcher.Close()
	}
	// Close job queue to signal workers to stop
	close(w.jobQueue)
	// Wait for all workers to finish
	w.wg.Wait()
}

func (w *Watcher) scanDirectory(dir string) {
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("Error walking %s: %v", path, err)
			return nil // Continue walking despite errors
		}
		if info.IsDir() {
			// Also watch directories during scan
			w.watchDirectory(path)
		} else {
			// Early filter: only queue processable files
			if w.isProcessable(path) {
				w.jobQueue <- fileJob{path: path, info: info}
			}
		}
		return nil
	})
}

func (w *Watcher) handleFileEvent(filePath string) {
	info, err := os.Stat(filePath)
	if err != nil {
		// File might have been deleted, skip
		return
	}

	if info.IsDir() {
		// Watch new subdirectories
		w.watchDirectory(filePath)
		return
	}

	// Early filter: only queue processable files
	if !w.isProcessable(filePath) {
		return
	}

	// Queue file for processing (buffered channel, no need for goroutine)
	w.jobQueue <- fileJob{path: filePath, info: info}
}

var (
	processableExts = map[string]bool{
		".webp": true,
		".webm": true,
		".svg":  true,
	}
)

func (w *Watcher) worker(ctx context.Context) {
	defer w.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-w.jobQueue:
			if !ok {
				return
			}
			w.processFile(job.path, job.info)
		}
	}
}

func (w *Watcher) processFile(filePath string, info os.FileInfo) {
	basename := filepath.Base(filePath)

	// Check if file needs processing
	needsProcessing, err := w.db.NeedsProcessing(filePath, info)
	if err != nil {
		log.Printf("%s Error checking db: %v", colorize(colorRed, "[e]"), err)
		return
	}

	// Determine file type and output path
	ext := filepath.Ext(filePath)
	isWebm := ext == ".webm"
	isSvg := ext == ".svg"
	outputPath := w.getOutputPath(filePath, ext)

	// Only check output file if database says we don't need processing
	// This avoids redundant stat calls when we know we need to process
	if !needsProcessing {
		if targetFile, err := os.Stat(outputPath); err == nil && targetFile.Size() > 0 {
			// File exists and is up to date, skip processing
			return
		}
	}

	// Ensure output directory exists
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		log.Printf("%s %s: failed to create output directory: %v", colorize(colorRed, "[e]"), basename, err)
		return
	}

	if isSvg {
		// Copy SVG files as-is
		if err := w.copyFile(filePath, outputPath); err != nil {
			log.Printf("%s %s", colorize(colorRed, "[e]"), basename)
			return
		}
	} else {
		// Generate webp (from webp or webm)
		if err := w.generateThumb(filePath, outputPath, isWebm); err != nil {
			log.Printf("%s %s", colorize(colorRed, "[e]"), basename)
			return
		}
	}

	// Update database with current source file status after successful processing
	if err := w.db.UpdateStatus(filePath, info); err != nil {
		log.Printf("%s %s", colorize(colorRed, "[e]"), basename)
		return
	}

	// Log completion
	if isSvg {
		log.Printf("%s %s", colorize(colorCyan, "[c]"), basename)
	} else if isWebm {
		log.Printf("%s %s", colorize(colorMagenta, "[v]"), basename)
	} else {
		log.Printf("%s %s", colorize(colorCyan, "[i]"), basename)
	}
}

func (w *Watcher) getOutputPath(inputPath string, ext string) string {
	relPath, err := filepath.Rel(w.inputDir, inputPath)
	if err != nil {
		relPath = filepath.Base(inputPath)
	}

	outputPath := filepath.Join(w.outputDir, relPath)

	// Only webm files get .webp appended, others keep their extension
	if ext == ".webm" {
		outputPath += ".webp"
	}

	return outputPath
}

func (w *Watcher) isProcessable(filePath string) bool {
	return processableExts[filepath.Ext(filePath)]
}

func (w *Watcher) watchDirectory(path string) {
	if err := w.fileWatcher.Add(path); err != nil {
		log.Printf("Warning: failed to watch %s: %v", path, err)
	}
}

func (w *Watcher) generateThumb(inputPath, outputPath string, isWebm bool) error {
	var cmd *exec.Cmd
	if isWebm {
		// Use ffmpeg to generate webp thumbnail from webm video
		cmd = exec.Command("ffmpeg",
			"-i", inputPath,
			"-ss", "1",
			"-vframes", "1",
			"-y",
			outputPath,
		)
	} else {
		// Use cwebp to convert webp image
		cmd = exec.Command("cwebp",
			"-q", fmt.Sprintf("%d", w.config.Quality),
			inputPath,
			"-o", outputPath,
		)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error: %w, output: %s", err, string(output))
	}

	return nil
}

func (w *Watcher) copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func (w *Watcher) Close() error {
	if w.fileWatcher != nil {
		w.fileWatcher.Close()
	}
	if w.db != nil {
		w.db.Close()
	}
	return nil
}
