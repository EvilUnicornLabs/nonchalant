// If you are AI: This file contains unit tests for the registry.

package bus

import (
	"testing"
)

func TestRegistryGetOrCreate(t *testing.T) {
	reg := NewRegistry()

	key := NewStreamKey("live", "test")

	// Create new stream
	stream1, created := reg.GetOrCreate(key)
	if !created {
		t.Error("First GetOrCreate should create new stream")
	}
	if stream1 == nil {
		t.Error("Stream should not be nil")
	}

	// Get existing stream
	stream2, created := reg.GetOrCreate(key)
	if created {
		t.Error("Second GetOrCreate should not create new stream")
	}
	if stream1 != stream2 {
		t.Error("GetOrCreate should return same stream instance")
	}

	if reg.Count() != 1 {
		t.Errorf("Expected 1 stream, got %d", reg.Count())
	}
}

func TestRegistryGet(t *testing.T) {
	reg := NewRegistry()

	key := NewStreamKey("live", "test")

	// Get non-existent stream
	stream := reg.Get(key)
	if stream != nil {
		t.Error("Get should return nil for non-existent stream")
	}

	// Create stream
	reg.GetOrCreate(key)

	// Get existing stream
	stream = reg.Get(key)
	if stream == nil {
		t.Error("Get should return stream after creation")
	}
}

func TestRegistryRemove(t *testing.T) {
	reg := NewRegistry()

	key := NewStreamKey("live", "test")

	// Remove non-existent stream
	if reg.Remove(key) {
		t.Error("Remove should return false for non-existent stream")
	}

	// Create empty stream
	reg.GetOrCreate(key)

	// Remove empty stream
	if !reg.Remove(key) {
		t.Error("Remove should succeed for empty stream")
	}

	if reg.Count() != 0 {
		t.Errorf("Expected 0 streams, got %d", reg.Count())
	}
}

func TestRegistryRemoveNonEmpty(t *testing.T) {
	reg := NewRegistry()

	key := NewStreamKey("live", "test")
	stream, _ := reg.GetOrCreate(key)

	// Attach publisher
	stream.AttachPublisher(1)

	// Remove should fail (stream not empty)
	if reg.Remove(key) {
		t.Error("Remove should fail for non-empty stream")
	}

	if reg.Count() != 1 {
		t.Errorf("Expected 1 stream, got %d", reg.Count())
	}

	// Detach publisher
	stream.DetachPublisher()

	// Now remove should succeed
	if !reg.Remove(key) {
		t.Error("Remove should succeed after stream is empty")
	}
}

func TestRegistryList(t *testing.T) {
	reg := NewRegistry()

	key1 := NewStreamKey("live", "stream1")
	key2 := NewStreamKey("live", "stream2")

	reg.GetOrCreate(key1)
	reg.GetOrCreate(key2)

	keys := reg.List()
	if len(keys) != 2 {
		t.Errorf("Expected 2 streams, got %d", len(keys))
	}

	// Verify both keys are present
	found1 := false
	found2 := false
	for _, k := range keys {
		if k == key1 {
			found1 = true
		}
		if k == key2 {
			found2 = true
		}
	}

	if !found1 || !found2 {
		t.Error("List should contain both streams")
	}
}
