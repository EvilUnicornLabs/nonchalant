// If you are AI: This file defines the configuration structure for nonchalant.
// It uses strict YAML decoding and explicit defaults.

package config

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds the complete server configuration.
// All fields must have explicit defaults or be required.
type Config struct {
	Server    ServerConfig     `yaml:"server"`
	Auth      AuthConfig       `yaml:"auth,omitempty"`
	HLS       HLSConfig        `yaml:"hls,omitempty"`
	Relays    []RelayConfig    `yaml:"relays,omitempty"`
	Transcode *TranscodeConfig `yaml:"transcode,omitempty"`
}

// HLSConfig tunes the native HLS / DASH packager.
// LowLatency switches to 1-second fMP4 segments with the program-date-time
// flag and independent segments, suitable for LL-HLS-aware players.
// Ladder enables adaptive-bitrate (ABR) packaging: ffmpeg encodes one rendition
// per rung and emits a master playlist (.m3u8) or multi-AdaptationSet MPD.
// When the ladder is empty the packager runs in stream-copy single-rendition
// mode and is essentially free; when present it transcodes, which is CPU-heavy.
type HLSConfig struct {
	LowLatency bool         `yaml:"low_latency,omitempty"`
	Ladder     []LadderRung `yaml:"ladder,omitempty"`
}

// LadderRung describes a single ABR rendition. Width / Height are the
// target frame size; VideoBitrate is in kbit/s. Name is used as the URL
// segment ("v0", "v1", ...) and as the rendition tag in the master playlist.
// When AudioOnly is true the rung produces audio only — useful for pure
// audio fallbacks on very lossy networks.
type LadderRung struct {
	Name         string `yaml:"name"`
	Width        int    `yaml:"width,omitempty"`
	Height       int    `yaml:"height,omitempty"`
	VideoBitrate int    `yaml:"video_bitrate,omitempty"` // kbit/s
	AudioBitrate int    `yaml:"audio_bitrate,omitempty"` // kbit/s
	AudioOnly    bool   `yaml:"audio_only,omitempty"`
}

// AuthConfig configures publish- and play-side authentication.
// PublishKeys is a list of pre-shared secrets a publisher must present in the
// RTMP stream name as "?key=<secret>". When empty, publishing is anonymous.
// PlayKeys is the same idea for HTTP-FLV / WS-FLV / HLS / DASH consumers,
// who pass the secret as a "?key=<secret>" query parameter on the playback URL.
// When empty, playback is anonymous.
type AuthConfig struct {
	PublishKeys []string `yaml:"publish_keys,omitempty"`
	PlayKeys    []string `yaml:"play_keys,omitempty"`
}

// ServerConfig defines HTTP server settings.
type ServerConfig struct {
	HealthPort int `yaml:"health_port"` // Port for health endpoint
	HTTPPort   int `yaml:"http_port"`   // Port for future HTTP services
	RTMPPort   int `yaml:"rtmp_port"`   // Port for future RTMP service
}

// RelayConfig defines a relay task configuration.
type RelayConfig struct {
	App       string `yaml:"app"`                 // Application name
	Name      string `yaml:"name"`                // Stream name
	Mode      string `yaml:"mode"`                // "pull" or "push"
	RemoteURL string `yaml:"remote_url"`          // Remote RTMP URL
	Reconnect bool   `yaml:"reconnect,omitempty"` // Enable reconnect on failure
}

// TranscodeConfig defines transcoding configuration.
// Only used when built with -tags ffmpeg.
type TranscodeConfig struct {
	Enabled  bool               `yaml:"enabled"`            // Enable transcoding
	Profiles []TranscodeProfile `yaml:"profiles,omitempty"` // Transcoding profiles
}

// TranscodeProfile defines a transcoding profile.
type TranscodeProfile struct {
	Name      string `yaml:"name"`       // Profile name
	App       string `yaml:"app"`        // Source application
	Stream    string `yaml:"stream"`     // Source stream name
	Format    string `yaml:"format"`     // Output format (hls, dash, etc.)
	OutputURL string `yaml:"output_url"` // Output URL
}

// New returns a Config populated with sane defaults (see setDefaults), for
// consumers that want to build a configuration programmatically instead of
// loading it from a YAML file — e.g. an embedding application that assigns
// its own (possibly ephemeral) ports. The returned Config can be mutated
// freely before being passed to server.New; call Validate afterwards to
// confirm the result is well-formed.
func New() *Config {
	cfg := &Config{}
	cfg.setDefaults()
	return cfg
}

// Load reads configuration from a YAML file.
// Returns an error if the file cannot be read or decoded.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var cfg Config
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true) // Reject unknown fields

	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	// Apply defaults
	cfg.setDefaults()

	return &cfg, nil
}

// setDefaults applies explicit default values to unset fields.
func (c *Config) setDefaults() {
	if c.Server.HealthPort == 0 {
		c.Server.HealthPort = 8080
	}
	if c.Server.HTTPPort == 0 {
		c.Server.HTTPPort = 8081
	}
	if c.Server.RTMPPort == 0 {
		c.Server.RTMPPort = 1935
	}
}
