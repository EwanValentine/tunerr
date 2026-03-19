package main

// Step 3: Move tidied album folders into the output music library.
//
// Source (COMPLETE_DIR):  Artist - Album (YYYY)/
// Destination layout:     OUTPUT_MUSIC_DIR/<Artist>/<YYYY> - <Album>/
//
// e.g. "Pink Floyd - The Dark Side of the Moon (1973)"
//   -> /music/Pink Floyd/1973 - The Dark Side of the Moon/

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

func moveToMusicDir(cfg *Config, stats *RunStats, log *slog.Logger) error {
	entries, err := os.ReadDir(cfg.CompleteDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading complete dir: %w", err)
	}

	if err := ensureDir(cfg.OutputMusicDir, cfg.DryRun); err != nil {
		return fmt.Errorf("creating output music dir: %w", err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "_") {
			continue
		}

		artist, album, year, ok := parseAlbumFolderName(name)
		if !ok {
			log.Debug("folder not yet well-named, skipping move", "name", name)
			continue
		}

		src := filepath.Join(cfg.CompleteDir, name)
		destDir := filepath.Join(cfg.OutputMusicDir, sanitizeName(artist))
		destAlbumName := sanitizeName(fmt.Sprintf("%s - %s", year, album))
		dest := filepath.Join(destDir, destAlbumName)

		if err := ensureDir(destDir, cfg.DryRun); err != nil {
			log.Warn("error creating artist dir", "dir", destDir, "err", err)
			stats.incError()
			continue
		}

		destInfo, statErr := os.Stat(dest)
		if statErr == nil && destInfo.IsDir() {
			// Destination exists — merge.
			log.Info("destination exists, merging", "src", src, "dest", dest)
			if err := mergeAlbumDir(src, dest, cfg.DryRun, stats, log); err != nil {
				log.Warn("error merging into output", "src", src, "err", err)
				stats.incError()
				continue
			}
		} else {
			log.Info("moving album to music library", "src", src, "dest", dest)
			if !cfg.DryRun {
				// Try a fast same-filesystem rename first; mergeAlbumDir handles
				// cross-device moves transparently via moveFile.
				if err := os.Rename(src, dest); err != nil {
					if !isCrossDevice(err) {
						log.Warn("error moving album", "src", src, "err", err)
						stats.incError()
						continue
					}
					if err := mergeAlbumDir(src, dest, cfg.DryRun, stats, log); err != nil {
						log.Warn("error moving album (cross-device)", "src", src, "err", err)
						stats.incError()
						continue
					}
				}
			}
			stats.incMoved()
		}

		// Remove empty source dir (no-op if rename already moved it).
		if err := removeEmptyDirs(src, cfg.CompleteDir, cfg.DryRun, log); err != nil {
			log.Warn("error cleaning source after move", "src", src, "err", err)
		}
	}
	return nil
}

