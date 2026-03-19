package main

// Step 1: Park non-audio folders from complete into complete/_non_audio/.

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const nonAudioSubdir = "_non_audio"

func parkNonAudio(cfg *Config, stats *RunStats, log *slog.Logger) error {
	entries, err := os.ReadDir(cfg.CompleteDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading complete dir: %w", err)
	}

	nonAudioDest := filepath.Join(cfg.CompleteDir, nonAudioSubdir)

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "_") {
			continue
		}

		folderPath := filepath.Join(cfg.CompleteDir, name)
		hasAudio, err := containsAudio(folderPath)
		if err != nil {
			log.Warn("error checking audio in folder", "folder", folderPath, "err", err)
			stats.incError()
			continue
		}
		if hasAudio {
			continue
		}

		dest := uniqueDest(nonAudioDest, name)
		log.Info("parking non-audio folder", "src", folderPath, "dest", dest)
		stats.incNonAudio()

		if !cfg.DryRun {
			if err := ensureDir(nonAudioDest, cfg.DryRun); err != nil {
				return fmt.Errorf("creating _non_audio dir: %w", err)
			}
			if err := os.Rename(folderPath, dest); err != nil {
				log.Warn("error moving non-audio folder", "src", folderPath, "err", err)
				stats.incError()
			}
		}
	}
	return nil
}
