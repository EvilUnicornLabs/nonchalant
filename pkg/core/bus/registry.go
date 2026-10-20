// If you are AI: This file implements the Registry for managing stream lifecycle.
// The registry maps StreamKey to Stream instances and handles creation/teardown.

package bus

import (
	"sync"
)

// Registry manages the lifecycle of streams.
// It maps StreamKey to Stream instances and handles creation and teardown.
// Lock expectations: Mutex-protected for concurrent access.
// Allocation: Map pre-allocated, stream creation allocates once per stream.
type Registry struct {
	mu      sync.RWMutex
	streams map[StreamKey]*Stream
}

// NewRegistry creates a new stream registry.
func NewRegistry() *Registry {
	return &Registry{
		streams: make(map[StreamKey]*Stream),
	}
}

// GetOrCreate retrieves an existing stream or creates a new one.
// Returns the stream and true if it was newly created, false if it already existed.
func (r *Registry) GetOrCreate(key StreamKey) (*Stream, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if stream, exists := r.streams[key]; exists {
		return stream, false
	}

	stream := NewStream(key)
	r.streams[key] = stream
	return stream, true
}

// Get retrieves a stream by key, returning nil if not found.
func (r *Registry) Get(key StreamKey) *Stream {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.streams[key]
}

// Remove removes a stream from the registry.
// The stream should be empty (no publisher, no subscribers) before removal.
func (r *Registry) Remove(key StreamKey) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	stream, exists := r.streams[key]
	if !exists {
		return false
	}

	// Only remove if stream is empty
	if !stream.IsEmpty() {
		return false
	}

	delete(r.streams, key)
	return true
}

// RemoveIfEmpty removes a stream only if it has no publisher and no subscribers.
// This is a convenience method for cleanup.
func (r *Registry) RemoveIfEmpty(key StreamKey) bool {
	return r.Remove(key)
}

// Count returns the number of active streams in the registry.
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.streams)
}

// List returns all stream keys in the registry.
func (r *Registry) List() []StreamKey {
	r.mu.RLock()
	defer r.mu.RUnlock()

	keys := make([]StreamKey, 0, len(r.streams))
	for key := range r.streams {
		keys = append(keys, key)
	}
	return keys
}
