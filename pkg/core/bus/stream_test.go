// If you are AI: This file contains unit tests for stream lifecycle and publisher exclusivity.

package bus

import (
	"testing"
)

func TestStreamKey(t *testing.T) {
	key := NewStreamKey("live", "mystream")
	if key.App != "live" {
		t.Errorf("Expected app 'live', got '%s'", key.App)
	}
	if key.Name != "mystream" {
		t.Errorf("Expected name 'mystream', got '%s'", key.Name)
	}

	str := key.String()
	expected := "live/mystream"
	if str != expected {
		t.Errorf("Expected string '%s', got '%s'", expected, str)
	}
}

func TestStreamLifecycle(t *testing.T) {
	key := NewStreamKey("live", "test")
	stream := NewStream(key)

	if stream.Key() != key {
		t.Error("Stream key mismatch")
	}

	if stream.HasPublisher() {
		t.Error("New stream should not have publisher")
	}

	if stream.SubscriberCount() != 0 {
		t.Error("New stream should have no subscribers")
	}

	if !stream.IsEmpty() {
		t.Error("New stream should be empty")
	}
}

func TestPublisherExclusivity(t *testing.T) {
	key := NewStreamKey("live", "test")
	stream := NewStream(key)

	// First publisher should attach
	if !stream.AttachPublisher(1) {
		t.Error("First publisher should attach successfully")
	}

	if !stream.HasPublisher() {
		t.Error("Stream should have publisher after attach")
	}

	// Second publisher should fail
	if stream.AttachPublisher(2) {
		t.Error("Second publisher should not attach")
	}

	// Detach publisher
	stream.DetachPublisher()
	if stream.HasPublisher() {
		t.Error("Stream should not have publisher after detach")
	}

	// After detach, new publisher should attach
	if !stream.AttachPublisher(3) {
		t.Error("Publisher should attach after previous detach")
	}
}

func TestSubscriberAttachDetach(t *testing.T) {
	key := NewStreamKey("live", "test")
	stream := NewStream(key)

	// Attach first subscriber
	sub1, id1 := stream.AttachSubscriber(100, BackpressureDropOldest)
	if sub1 == nil {
		t.Error("Subscriber should be created")
	}
	if id1 == 0 {
		t.Error("Subscriber ID should be non-zero")
	}
	if stream.SubscriberCount() != 1 {
		t.Errorf("Expected 1 subscriber, got %d", stream.SubscriberCount())
	}

	// Attach second subscriber
	_, id2 := stream.AttachSubscriber(100, BackpressureDropOldest)
	if id2 == id1 {
		t.Error("Subscriber IDs should be unique")
	}
	if stream.SubscriberCount() != 2 {
		t.Errorf("Expected 2 subscribers, got %d", stream.SubscriberCount())
	}

	// Detach first subscriber
	stream.DetachSubscriber(id1)
	if stream.SubscriberCount() != 1 {
		t.Errorf("Expected 1 subscriber after detach, got %d", stream.SubscriberCount())
	}

	// Detach second subscriber
	stream.DetachSubscriber(id2)
	if stream.SubscriberCount() != 0 {
		t.Errorf("Expected 0 subscribers, got %d", stream.SubscriberCount())
	}

	if !stream.IsEmpty() {
		t.Error("Stream should be empty after removing all subscribers")
	}
}

func TestPublishFanout(t *testing.T) {
	key := NewStreamKey("live", "test")
	stream := NewStream(key)

	// Attach two subscribers
	sub1, id1 := stream.AttachSubscriber(10, BackpressureDropOldest)
	sub2, id2 := stream.AttachSubscriber(10, BackpressureDropOldest)
	_ = id1
	_ = id2

	// Create and publish a message
	msg := AcquireMessage()
	msg.Type = MessageTypeVideo
	msg.Timestamp = 1000
	msg.SetPayload([]byte("test data"))

	stream.Publish(msg)

	// Both subscribers should receive the message
	read1, ok1 := sub1.Read()
	if !ok1 {
		t.Error("Subscriber 1 should receive message")
	}
	if read1.Type != MessageTypeVideo {
		t.Error("Message type mismatch for subscriber 1")
	}

	read2, ok2 := sub2.Read()
	if !ok2 {
		t.Error("Subscriber 2 should receive message")
	}
	if read2 != nil && read2.Type != MessageTypeVideo {
		t.Error("Message type mismatch for subscriber 2")
	}
	_ = read2

	// Cleanup
	ReleaseMessage(msg)
}

func TestStreamWithPublisherAndSubscribers(t *testing.T) {
	key := NewStreamKey("live", "test")
	stream := NewStream(key)

	// Attach publisher
	stream.AttachPublisher(1)

	// Attach subscribers
	stream.AttachSubscriber(10, BackpressureDropOldest)
	stream.AttachSubscriber(10, BackpressureDropOldest)

	// Stream should not be empty
	if stream.IsEmpty() {
		t.Error("Stream with publisher and subscribers should not be empty")
	}

	// Detach publisher
	stream.DetachPublisher()

	// Stream should still not be empty (has subscribers)
	if stream.IsEmpty() {
		t.Error("Stream with subscribers should not be empty")
	}
}
