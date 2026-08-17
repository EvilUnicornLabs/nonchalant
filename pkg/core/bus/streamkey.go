// If you are AI: This file defines StreamKey for uniquely identifying streams.
// StreamKey is used as a map key in the registry.

package bus

import (
	"fmt"
)

// StreamKey uniquely identifies a stream by application and stream name.
// It is comparable and can be used as a map key.
type StreamKey struct {
	App  string // Application name (e.g., "live")
	Name string // Stream name (e.g., "mystream")
}

// String returns a stable, deterministic string representation of the stream key.
// Format: "app/name"
func (k StreamKey) String() string {
	return fmt.Sprintf("%s/%s", k.App, k.Name)
}

// NewStreamKey creates a new StreamKey from app and name.
func NewStreamKey(app, name string) StreamKey {
	return StreamKey{
		App:  app,
		Name: name,
	}
}
