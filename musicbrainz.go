package main

// MusicBrainz YearLookup implementation.
//
// Uses the MusicBrainz release-group search API to find the first release
// year for an (artist, album) pair. Results are cached in-memory for the
// lifetime of the process so each album is only looked up once.
//
// Rate limit: MusicBrainz allows 1 request/second for non-commercial use.
// This client enforces that limit via a ticker.
//
// Required env vars when MB_ENABLED=true:
//   MB_USER_AGENT — e.g. "myapp/1.0 ( you@example.com )"
//                   MusicBrainz requires a meaningful User-Agent; requests
//                   without one will be blocked.

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const mbBaseURL = "https://musicbrainz.org/ws/2"

// mbReleaseGroupResponse is the minimal shape of the MB release-group search.
type mbReleaseGroupResponse struct {
	ReleaseGroups []mbReleaseGroup `json:"release-groups"`
}

type mbReleaseGroup struct {
	Title            string `json:"title"`
	FirstReleaseDate string `json:"first-release-date"` // "YYYY", "YYYY-MM", or "YYYY-MM-DD"
	Score            int    `json:"score"`
	PrimaryType      string `json:"primary-type"`
}

// mbClient implements YearLookup using the MusicBrainz web service.
type mbClient struct {
	userAgent string
	http      *http.Client
	log       *slog.Logger

	// rate limiter: one token per second
	rateTicker <-chan time.Time

	// in-process cache: "artist\x00album" -> year (0 = not found)
	mu    sync.Mutex
	cache map[string]int
}

func newMBClient(userAgent string, log *slog.Logger) *mbClient {
	return &mbClient{
		userAgent:  userAgent,
		http:       &http.Client{Timeout: 10 * time.Second},
		log:        log,
		rateTicker: time.NewTicker(time.Second).C,
		cache:      make(map[string]int),
	}
}

// LookupYear implements YearLookup.
func (c *mbClient) LookupYear(artist, album string) (int, error) {
	key := artist + "\x00" + album

	c.mu.Lock()
	if y, hit := c.cache[key]; hit {
		c.mu.Unlock()
		c.log.Debug("musicbrainz cache hit", "artist", artist, "album", album, "year", y)
		return y, nil
	}
	c.mu.Unlock()

	// Respect the 1 req/s rate limit.
	<-c.rateTicker

	year, err := c.fetchYear(artist, album)
	if err != nil {
		return 0, err
	}

	c.mu.Lock()
	c.cache[key] = year
	c.mu.Unlock()

	return year, nil
}

func (c *mbClient) fetchYear(artist, album string) (int, error) {
	// Build a Lucene query: releasegroup:"Album" AND artist:"Artist"
	query := fmt.Sprintf(`releasegroup:"%s" AND artist:"%s"`,
		escapeMBQuery(album), escapeMBQuery(artist))

	reqURL := fmt.Sprintf("%s/release-group?query=%s&fmt=json&limit=5",
		mbBaseURL, url.QueryEscape(query))

	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return 0, fmt.Errorf("building mb request: %w", err)
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("mb request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusServiceUnavailable {
		return 0, fmt.Errorf("musicbrainz rate-limited (503)")
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("musicbrainz returned %d", resp.StatusCode)
	}

	var result mbReleaseGroupResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("decoding mb response: %w", err)
	}

	year := pickBestYear(result.ReleaseGroups, album)
	c.log.Debug("musicbrainz lookup", "artist", artist, "album", album, "year", year)
	return year, nil
}

// pickBestYear selects the year from the highest-score Album result whose
// title matches (case-insensitive). Falls back to first result if no exact
// match is found.
func pickBestYear(groups []mbReleaseGroup, wantTitle string) int {
	want := strings.ToLower(wantTitle)

	// Try an exact (case-insensitive) title match first.
	for _, g := range groups {
		if strings.ToLower(g.Title) == want && g.Score >= 90 {
			return parseYearFromDate(g.FirstReleaseDate)
		}
	}
	// Fall back to the first result (highest score from MB).
	if len(groups) > 0 {
		return parseYearFromDate(groups[0].FirstReleaseDate)
	}
	return 0
}

// parseYearFromDate extracts the year from "YYYY", "YYYY-MM", or "YYYY-MM-DD".
func parseYearFromDate(date string) int {
	if len(date) < 4 {
		return 0
	}
	y, err := strconv.Atoi(date[:4])
	if err != nil {
		return 0
	}
	return y
}

// escapeMBQuery escapes double-quotes inside a Lucene phrase value.
func escapeMBQuery(s string) string {
	return strings.ReplaceAll(s, `"`, `\"`)
}
