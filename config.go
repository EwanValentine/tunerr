package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	DownloadRoot    string
	CompleteDir     string
	FailedImportsDir string
	OutputMusicDir  string
	IntervalSeconds int
	DryRun          bool

	// Logging
	LogPath string

	// Lidarr
	LidarrURL    string
	LidarrAPIKey string
	LidarrRescan bool
}

func loadConfig() (*Config, error) {
	c := &Config{}

	c.DownloadRoot = requireEnv("DOWNLOAD_ROOT")
	c.OutputMusicDir = requireEnv("OUTPUT_MUSIC_DIR")

	c.CompleteDir = envOr("COMPLETE_DIR", c.DownloadRoot+"/complete")
	c.FailedImportsDir = envOr("FAILED_IMPORTS_DIR", c.DownloadRoot+"/failed_imports")

	interval, err := strconv.Atoi(envOr("INTERVAL_SECONDS", "300"))
	if err != nil {
		return nil, fmt.Errorf("invalid INTERVAL_SECONDS: %w", err)
	}
	c.IntervalSeconds = interval

	c.DryRun = strings.ToLower(envOr("DRY_RUN", "false")) == "true"
	c.LogPath = os.Getenv("LOG_PATH")

	c.LidarrURL = os.Getenv("LIDARR_URL")
	c.LidarrAPIKey = os.Getenv("LIDARR_API_KEY")
	c.LidarrRescan = strings.ToLower(envOr("LIDARR_RESCAN", "false")) == "true"

	return c, nil
}

func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "required environment variable %s is not set\n", key)
		os.Exit(1)
	}
	return v
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
