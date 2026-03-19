package main

import (
	"io"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

func main() {
	cfg, err := loadConfig()
	if err != nil {
		panic(err)
	}

	log := buildLogger(cfg)

	if cfg.DryRun {
		log.Warn("DRY_RUN=true — no files will be moved or deleted")
	}

	log.Info("tunerr started",
		"download_root", cfg.DownloadRoot,
		"complete_dir", cfg.CompleteDir,
		"failed_imports_dir", cfg.FailedImportsDir,
		"output_music_dir", cfg.OutputMusicDir,
		"interval_seconds", cfg.IntervalSeconds,
		"dry_run", cfg.DryRun,
	)

	// runMu prevents overlapping pipeline runs (e.g. if a run takes longer
	// than the configured interval).
	var runMu sync.Mutex

	run := func() {
		if !runMu.TryLock() {
			log.Warn("previous pipeline run still in progress, skipping this tick")
			return
		}
		defer runMu.Unlock()
		runPipeline(cfg, log)
	}

	// Run immediately on startup.
	run()

	ticker := time.NewTicker(time.Duration(cfg.IntervalSeconds) * time.Second)
	defer ticker.Stop()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-ticker.C:
			run()
		case <-stop:
			log.Info("shutting down")
			return
		}
	}
}

func runPipeline(cfg *Config, log *slog.Logger) {
	stats := &RunStats{}
	start := time.Now()

	log.Info("pipeline run started")

	if err := sweepFailedImports(cfg, stats, log); err != nil {
		log.Error("step 0 (sweep failed_imports) error", "err", err)
		stats.incError()
	}

	if err := parkNonAudio(cfg, stats, log); err != nil {
		log.Error("step 1 (park non-audio) error", "err", err)
		stats.incError()
	}

	if err := tidyAlbumFolders(cfg, stats, log); err != nil {
		log.Error("step 2 (tidy album folders) error", "err", err)
		stats.incError()
	}

	if err := moveToMusicDir(cfg, stats, log); err != nil {
		log.Error("step 3 (move to music dir) error", "err", err)
		stats.incError()
	}

	maybeTriggerLidarr(cfg, stats, log)

	log.Info("SUMMARY",
		"duration_ms", time.Since(start).Milliseconds(),
		"movedFolders", stats.MovedFolders.Load(),
		"movedFiles", stats.MovedFiles.Load(),
		"duplicateFiles", stats.DuplicateFiles.Load(),
		"conflictFiles", stats.ConflictFiles.Load(),
		"nonAudioMoved", stats.NonAudioMoved.Load(),
		"failedImportsSwept", stats.FailedImportsSwept.Load(),
		"errors", stats.Errors.Load(),
	)
}

func buildLogger(cfg *Config) *slog.Logger {
	var w io.Writer = os.Stdout
	if cfg.LogPath != "" {
		f, err := os.OpenFile(cfg.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			slog.Error("could not open log file, falling back to stdout", "path", cfg.LogPath, "err", err)
		} else {
			w = f
		}
	}
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
}
