package main

import (
	"log/slog"
	"os"
	"testing"
)

func TestParseYearFromDate(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"1997-05-21", 1997},
		{"1973-03", 1973},
		{"2001", 2001},
		{"", 0},
		{"???", 0},
	}
	for _, c := range cases {
		got := parseYearFromDate(c.in)
		if got != c.want {
			t.Errorf("parseYearFromDate(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestPickBestYear(t *testing.T) {
	groups := []mbReleaseGroup{
		{Title: "OK Computer", FirstReleaseDate: "1997-05-21", Score: 100},
		{Title: "OK Computer OKNOTOK", FirstReleaseDate: "2017-06-23", Score: 80},
	}
	got := pickBestYear(groups, "OK Computer")
	if got != 1997 {
		t.Errorf("expected 1997, got %d", got)
	}
}

func TestPickBestYear_FallbackToFirst(t *testing.T) {
	groups := []mbReleaseGroup{
		{Title: "Something Else", FirstReleaseDate: "2005-01-01", Score: 60},
	}
	got := pickBestYear(groups, "My Album")
	if got != 2005 {
		t.Errorf("expected 2005, got %d", got)
	}
}

func TestPickBestYear_Empty(t *testing.T) {
	got := pickBestYear(nil, "anything")
	if got != 0 {
		t.Errorf("expected 0, got %d", got)
	}
}

// TestMBClientLive hits the real MusicBrainz API.
// Run with: go test -run TestMBClientLive -v
func TestMBClientLive(t *testing.T) {
	if os.Getenv("MB_LIVE") != "1" {
		t.Skip("set MB_LIVE=1 to run live MusicBrainz tests")
	}
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	c := newMBClient("tunerr-test/1.0 ( test )", log)

	cases := []struct {
		artist, album string
		wantYear      int
	}{
		{"Radiohead", "OK Computer", 1997},
		{"Pink Floyd", "The Dark Side of the Moon", 1973},
		{"Portishead", "Dummy", 1994},
	}

	for _, tc := range cases {
		year, err := c.LookupYear(tc.artist, tc.album)
		if err != nil {
			t.Errorf("%s / %s: error: %v", tc.artist, tc.album, err)
			continue
		}
		if year != tc.wantYear {
			t.Errorf("%s / %s: got %d, want %d", tc.artist, tc.album, year, tc.wantYear)
		}
	}
}
