// If you are AI: This file validates configuration values and returns descriptive errors.

package config

import (
	"fmt"
)

// Validate checks that all configuration values are within acceptable ranges.
// Returns an error describing the first validation failure found.
func (c *Config) Validate() error {
	if err := c.Server.Validate(); err != nil {
		return fmt.Errorf("server config: %w", err)
	}
	if err := c.HLS.Validate(); err != nil {
		return fmt.Errorf("hls config: %w", err)
	}
	return nil
}

// Validate checks HLS configuration values, including the ABR ladder.
func (h *HLSConfig) Validate() error {
	seen := make(map[string]struct{}, len(h.Ladder))
	for i, r := range h.Ladder {
		if r.Name == "" {
			return fmt.Errorf("ladder[%d]: name is required", i)
		}
		if !isLadderName(r.Name) {
			return fmt.Errorf("ladder[%d]: name %q must be alphanumeric (no slashes / dots)", i, r.Name)
		}
		if _, dup := seen[r.Name]; dup {
			return fmt.Errorf("ladder[%d]: duplicate name %q", i, r.Name)
		}
		seen[r.Name] = struct{}{}
		if r.AudioOnly {
			continue
		}
		if r.Width <= 0 || r.Height <= 0 {
			return fmt.Errorf("ladder[%d] %q: width and height required for video rungs", i, r.Name)
		}
		if r.VideoBitrate <= 0 {
			return fmt.Errorf("ladder[%d] %q: video_bitrate must be positive (kbit/s)", i, r.Name)
		}
	}
	return nil
}

// isLadderName checks that a rung name is a safe URL path segment.
// We never want a "v0/.." or "v0/index.m3u8" rung name reaching the filesystem.
func isLadderName(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '_' || r == '-':
		default:
			return false
		}
	}
	return true
}

// Validate checks server configuration values.
func (s *ServerConfig) Validate() error {
	if s.HealthPort <= 0 || s.HealthPort > 65535 {
		return fmt.Errorf("health_port must be between 1 and 65535, got %d", s.HealthPort)
	}
	if s.HTTPPort <= 0 || s.HTTPPort > 65535 {
		return fmt.Errorf("http_port must be between 1 and 65535, got %d", s.HTTPPort)
	}
	if s.RTMPPort <= 0 || s.RTMPPort > 65535 {
		return fmt.Errorf("rtmp_port must be between 1 and 65535, got %d", s.RTMPPort)
	}
	if s.HealthPort == s.HTTPPort {
		return fmt.Errorf("health_port and http_port must be different, both are %d", s.HealthPort)
	}
	if s.HealthPort == s.RTMPPort {
		return fmt.Errorf("health_port and rtmp_port must be different, both are %d", s.HealthPort)
	}
	if s.HTTPPort == s.RTMPPort {
		return fmt.Errorf("http_port and rtmp_port must be different, both are %d", s.HTTPPort)
	}
	return nil
}
