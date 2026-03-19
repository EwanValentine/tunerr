package main

// Optional Lidarr integration: trigger a RescanFolders command after a run
// where at least one file was moved.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type lidarrCommand struct {
	Name string `json:"name"`
}

type lidarrClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func newLidarrClient(baseURL, apiKey string) *lidarrClient {
	return &lidarrClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *lidarrClient) triggerRescan(log *slog.Logger) error {
	body, err := json.Marshal(lidarrCommand{Name: "RescanFolders"})
	if err != nil {
		return fmt.Errorf("marshalling lidarr command: %w", err)
	}

	url := c.baseURL + "/api/v1/command"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating lidarr request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("calling lidarr: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("lidarr returned status %d", resp.StatusCode)
	}

	log.Info("lidarr RescanFolders triggered", "status", resp.StatusCode)
	return nil
}

func maybeTriggerLidarr(cfg *Config, stats *RunStats, log *slog.Logger) {
	if !cfg.LidarrRescan || cfg.LidarrURL == "" || cfg.LidarrAPIKey == "" {
		return
	}
	if !stats.anyFilesMoved() {
		log.Debug("lidarr rescan skipped: no files moved this run")
		return
	}
	client := newLidarrClient(cfg.LidarrURL, cfg.LidarrAPIKey)
	if err := client.triggerRescan(log); err != nil {
		log.Warn("lidarr rescan failed", "err", err)
	}
}
