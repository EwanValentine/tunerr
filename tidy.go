package main

// Step 2: Tidy / rename album folders in complete.
//
// Target format: "Artist - Album (Year)"
// e.g. "Pink Floyd - The Dark Side of the Moon (1973)"
//
// Extensibility: YearLookup is an interface that can be satisfied by a
// MusicBrainz client in the future without changing the core logic.

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dhowden/tag"
)

// albumPattern matches "Artist - Album (YYYY)" or "Artist - Album (????)"
var albumPattern = regexp.MustCompile(`^(.+) - (.+) \((\d{4}|\?\?\?\?)\)$`)

// YearLookup is a hook for resolving the original release year from an
// external source (e.g. MusicBrainz). Return 0 if unknown.
// The tag-based path falls through to this hook only when m.Year() == 0,
// so external lookups are purely additive and never override real tag data.
type YearLookup interface {
	LookupYear(artist, album string) (int, error)
}

// noopYearLookup is the default: always defers to tag data / "????".
type noopYearLookup struct{}

func (noopYearLookup) LookupYear(_, _ string) (int, error) { return 0, nil }

// isNoop reports whether the lookup will never return a real year.
func isNoop(y YearLookup) bool {
	_, ok := y.(noopYearLookup)
	return ok
}

// tidyAlbumFoldersWithLookup is the pipeline entry point for step 2.
func tidyAlbumFoldersWithLookup(cfg *Config, stats *RunStats, log *slog.Logger, yearLookup YearLookup) error {
	entries, err := os.ReadDir(cfg.CompleteDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading complete dir: %w", err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "_") {
			continue
		}

		if albumPattern.MatchString(name) {
			// If the folder has an unknown year and we have a real lookup,
			// try to resolve it. Otherwise skip.
			if !strings.HasSuffix(name, "(????)") || isNoop(yearLookup) {
				log.Debug("folder already well-named, skipping rename", "name", name)
				continue
			}
			log.Debug("folder has unknown year, attempting MB lookup", "name", name)
		}

		folderPath := filepath.Join(cfg.CompleteDir, name)
		meta, err := inferAlbumMeta(folderPath, yearLookup)
		if err != nil {
			// Cannot determine metadata — leave folder untouched.
			log.Info("skipping rename: could not read tags", "folder", name, "reason", err.Error())
			continue
		}

		newName := buildFolderName(meta)
		if newName == name {
			continue
		}

		newPath := filepath.Join(cfg.CompleteDir, newName)
		if _, err := os.Stat(newPath); err == nil {
			log.Warn("rename target already exists, skipping",
				"src", name, "target", newName)
			continue
		}

		log.Info("renaming album folder", "from", name, "to", newName)
		if !cfg.DryRun {
			if err := os.Rename(folderPath, newPath); err != nil {
				log.Warn("error renaming folder", "src", folderPath, "err", err)
				stats.incError()
			}
		}
	}
	return nil
}

// albumMeta holds the normalised metadata extracted from an album folder.
type albumMeta struct {
	Artist string
	Album  string
	Year   int // 0 = unknown
}

// inferAlbumMeta reads the first audio file's tags.
// Returns an error if no audio file exists or tags are insufficient —
// the caller should skip renaming in that case.
func inferAlbumMeta(folderPath string, yearLookup YearLookup) (*albumMeta, error) {
	audioFile, err := firstAudioFile(folderPath)
	if err != nil {
		return nil, fmt.Errorf("walking folder: %w", err)
	}
	if audioFile == "" {
		return nil, fmt.Errorf("no audio file found under %s", folderPath)
	}

	f, err := os.Open(audioFile)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", audioFile, err)
	}
	defer f.Close()

	m, err := tag.ReadFrom(f)
	if err != nil {
		return nil, fmt.Errorf("reading tags from %s: %w", audioFile, err)
	}

	artist := coalesce(m.AlbumArtist(), m.Artist())
	album := m.Album()

	if artist == "" || album == "" {
		return nil, fmt.Errorf("tags missing artist or album in %s", audioFile)
	}

	year := m.Year()
	if year == 0 && yearLookup != nil {
		// Extension point: MusicBrainz or other lookup.
		if y, err := yearLookup.LookupYear(artist, album); err == nil && y > 0 {
			year = y
		}
	}

	return &albumMeta{Artist: artist, Album: album, Year: year}, nil
}

// buildFolderName produces the sanitised "Artist - Album (Year)" string.
func buildFolderName(m *albumMeta) string {
	year := "????"
	if m.Year > 0 {
		year = fmt.Sprintf("%d", m.Year)
	}
	return sanitizeName(fmt.Sprintf("%s - %s (%s)", m.Artist, m.Album, year))
}

// parseAlbumFolderName splits "Artist - Album (YYYY)" into its components.
// Returns ok=false if the name does not match the pattern.
func parseAlbumFolderName(name string) (artist, album, year string, ok bool) {
	m := albumPattern.FindStringSubmatch(name)
	if m == nil {
		return "", "", "", false
	}
	return m[1], m[2], m[3], true
}

func coalesce(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
