package main

// Step 0: Sweep failed_imports back into complete.
//
// Layout expected under FAILED_IMPORTS_DIR:
//
//	complete/
//	complete_1/
//	complete_2/
//	    <album-folder>/
//	        <files>
//
// Folders starting with "incomplete" are left untouched.

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

func sweepFailedImports(cfg *Config, stats *RunStats, log *slog.Logger) error {
	entries, err := os.ReadDir(cfg.FailedImportsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading failed_imports dir: %w", err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		// Only process "complete", "complete_N" dirs; skip incomplete_* etc.
		if name != "complete" && !strings.HasPrefix(name, "complete_") {
			continue
		}

		srcBatch := filepath.Join(cfg.FailedImportsDir, name)
		if err := sweepBatch(srcBatch, cfg.CompleteDir, cfg.DryRun, stats, log); err != nil {
			log.Warn("error sweeping batch", "batch", srcBatch, "err", err)
			stats.incError()
		}

		// Remove the batch directory if now empty.
		if err := removeEmptyDirs(srcBatch, cfg.FailedImportsDir, cfg.DryRun, log); err != nil {
			log.Warn("error removing empty batch dir", "dir", srcBatch, "err", err)
		}
	}
	return nil
}

// sweepBatch merges all album folders inside srcBatch into destCompleteDir.
func sweepBatch(srcBatch, destCompleteDir string, dryRun bool, stats *RunStats, log *slog.Logger) error {
	albums, err := os.ReadDir(srcBatch)
	if err != nil {
		return fmt.Errorf("reading batch dir %s: %w", srcBatch, err)
	}

	for _, a := range albums {
		if !a.IsDir() {
			continue
		}
		srcAlbum := filepath.Join(srcBatch, a.Name())
		destAlbum := filepath.Join(destCompleteDir, a.Name())

		log.Info("sweeping album back to complete", "src", srcAlbum, "dest", destAlbum)

		if err := mergeAlbumDir(srcAlbum, destAlbum, dryRun, stats, log); err != nil {
			log.Warn("error merging album", "src", srcAlbum, "err", err)
			stats.incError()
			continue
		}
		stats.incSwept()

		// Clean up empty source album dir.
		if err := removeEmptyDirs(srcAlbum, srcBatch, dryRun, log); err != nil {
			log.Warn("error removing source album dir", "dir", srcAlbum, "err", err)
		}
	}
	return nil
}

// mergeAlbumDir merges all files from src into dest, handling conflicts.
// It is shared by the sweep and move steps.
func mergeAlbumDir(src, dest string, dryRun bool, stats *RunStats, log *slog.Logger) error {
	if err := ensureDir(dest, dryRun); err != nil {
		return fmt.Errorf("creating dest dir %s: %w", dest, err)
	}

	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		destPath := filepath.Join(dest, rel)

		if d.IsDir() {
			return ensureDir(destPath, dryRun)
		}

		destInfo, statErr := os.Stat(destPath)
		if statErr == nil {
			// Destination file already exists.
			srcInfo, _ := d.Info()
			if destInfo.Size() == srcInfo.Size() {
				// Exact duplicate — remove source.
				log.Info("duplicate (same size): removing source copy",
					"src", path, "dest", destPath)
				stats.incDuplicate()
				if !dryRun {
					if err := os.Remove(path); err != nil {
						log.Warn("could not remove duplicate source", "path", path, "err", err)
					}
				}
				return nil
			}

			// Different size — park in _conflicts/.
			conflictsDir := filepath.Join(dest, "_conflicts")
			if err := ensureDir(conflictsDir, dryRun); err != nil {
				return fmt.Errorf("creating conflicts dir: %w", err)
			}
			conflictDest := uniqueDest(conflictsDir, d.Name())
			log.Warn("conflict: dest exists with different size, moving to _conflicts",
				"src", path, "conflict_dest", conflictDest)
			stats.incConflict()
			if !dryRun {
				if err := moveFile(path, conflictDest); err != nil {
					return fmt.Errorf("moving conflict file: %w", err)
				}
			}
			return nil
		}

		// Destination does not exist — move (with cross-device fallback).
		log.Info("moving file", "src", path, "dest", destPath)
		stats.incMovedFile()
		if !dryRun {
			if err := moveFile(path, destPath); err != nil {
				return fmt.Errorf("moving %s -> %s: %w", path, destPath, err)
			}
		}
		return nil
	})
}
