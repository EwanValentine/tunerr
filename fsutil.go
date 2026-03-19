package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

var audioExtensions = map[string]bool{
	".flac": true,
	".mp3":  true,
	".m4a":  true,
	".ogg":  true,
	".opus": true,
	".wav":  true,
	".aiff": true,
	".alac": true,
}

// isAudio reports whether name has an audio extension.
func isAudio(name string) bool {
	return audioExtensions[strings.ToLower(filepath.Ext(name))]
}

// containsAudio returns true if any file under root has an audio extension.
func containsAudio(root string) (bool, error) {
	found := false
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && isAudio(d.Name()) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found, err
}

// firstAudioFile returns the path of the first audio file found under root.
func firstAudioFile(root string) (string, error) {
	var result string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && isAudio(d.Name()) {
			result = path
			return filepath.SkipAll
		}
		return nil
	})
	return result, err
}

// sanitizeName replaces filesystem-unsafe characters and collapses whitespace.
func sanitizeName(s string) string {
	replacer := strings.NewReplacer(
		`\`, "-",
		"/", "-",
		":", "-",
		"*", "-",
		"?", "-",
		`"`, "-",
		"<", "-",
		">", "-",
		"|", "-",
	)
	s = replacer.Replace(s)
	return strings.Join(strings.Fields(s), " ")
}

// isDirEmpty reports whether a directory has no entries.
func isDirEmpty(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	entries, err := f.Readdirnames(1)
	if err != nil && err != io.EOF {
		return false, err
	}
	return len(entries) == 0, nil
}

// removeEmptyDirs removes dir and any empty ancestor dirs up to (but not
// including) stopAt.
func removeEmptyDirs(dir, stopAt string, dryRun bool, log *slog.Logger) error {
	for dir != stopAt && strings.HasPrefix(dir, stopAt) {
		empty, err := isDirEmpty(dir)
		if err != nil || !empty {
			return err
		}
		log.Info("removing empty directory", "dir", dir)
		if !dryRun {
			if err := os.Remove(dir); err != nil {
				return err
			}
		}
		dir = filepath.Dir(dir)
	}
	return nil
}

// uniqueDest returns a path that does not yet exist, by appending .2, .3, …
func uniqueDest(dir, name string) string {
	candidate := filepath.Join(dir, name)
	if _, err := os.Stat(candidate); os.IsNotExist(err) {
		return candidate
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 2; ; i++ {
		candidate = filepath.Join(dir, fmt.Sprintf("%s.%d%s", base, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

// ensureDir creates dir (and parents) if it does not exist.
func ensureDir(path string, dryRun bool) error {
	if dryRun {
		return nil
	}
	return os.MkdirAll(path, 0o755)
}

func isCrossDevice(err error) bool {
	return err != nil && strings.Contains(err.Error(), "invalid cross-device link")
}

// moveFile moves src to dest. If they are on different filesystems it falls
// back to copy+delete automatically.
func moveFile(src, dest string) error {
	if err := os.Rename(src, dest); err == nil {
		return nil
	} else if !isCrossDevice(err) {
		return err
	}
	// Cross-device: copy then remove source.
	if err := copyFile(src, dest); err != nil {
		return err
	}
	return os.Remove(src)
}

// copyFile copies src to dest, preserving permissions.
func copyFile(src, dest string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
