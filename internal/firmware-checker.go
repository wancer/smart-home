package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	tasmotaLatestReleaseUrl = "https://api.github.com/repos/arendst/Tasmota/releases/latest"
	firmwareCheckPeriod     = 24 * time.Hour
	firmwareRetryPeriod     = 1 * time.Hour
	firmwareCheckTimeout    = 30 * time.Second
)

// FirmwareChecker keeps the latest released Tasmota version, refreshed on start and then daily.
type FirmwareChecker struct {
	client *http.Client

	mu     sync.RWMutex
	latest *string
}

func NewFirmwareChecker() *FirmwareChecker {
	return &FirmwareChecker{client: &http.Client{Timeout: firmwareCheckTimeout}}
}

// Latest returns the latest released version (e.g. "16.0.0"), or nil if no check has succeeded yet.
func (c *FirmwareChecker) Latest() *string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.latest
}

func (c *FirmwareChecker) Run() chan struct{} {
	quit := make(chan struct{})
	go func() {
		for {
			next := firmwareCheckPeriod
			if err := c.check(); err != nil {
				slog.Warn("[firmware] latest version check failed", "err", err)
				next = firmwareRetryPeriod
			}

			select {
			case <-time.After(next):
			case <-quit:
				return
			}
		}
	}()

	return quit
}

func (c *FirmwareChecker) check() error {
	ctx, cancel := context.WithTimeout(context.Background(), firmwareCheckTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tasmotaLatestReleaseUrl, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "smart-home-backend") // GitHub rejects requests without one

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return err
	}

	version := strings.TrimPrefix(release.TagName, "v")
	if _, ok := parseFirmwareVersion(version); !ok {
		return fmt.Errorf("unexpected release tag: %q", release.TagName)
	}

	c.mu.Lock()
	c.latest = &version
	c.mu.Unlock()

	slog.Info("[firmware] latest version", "version", version)
	return nil
}

// IsFirmwareOutdated reports whether current is older than latest.
// Unparsable versions are never reported as outdated.
func IsFirmwareOutdated(current string, latest string) bool {
	currentParts, ok := parseFirmwareVersion(current)
	if !ok {
		return false
	}
	latestParts, ok := parseFirmwareVersion(latest)
	if !ok {
		return false
	}

	return slices.Compare(currentParts, latestParts) < 0
}

// parseFirmwareVersion accepts the Tasmota format "14.3.0.2(sensors)"; the variant suffix is ignored.
func parseFirmwareVersion(version string) ([]int, bool) {
	number, _, _ := strings.Cut(version, "(")

	fields := strings.Split(number, ".")
	parts := make([]int, 0, len(fields))
	for _, field := range fields {
		part, err := strconv.Atoi(field)
		if err != nil {
			return nil, false
		}
		parts = append(parts, part)
	}

	return parts, true
}
