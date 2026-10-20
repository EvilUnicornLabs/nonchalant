// If you are AI: This file implements the Stream type that manages publisher and subscribers.
// A stream allows exactly one publisher and many subscribers via a shared log:
// the publisher writes one slot per message, all subscribers read from the
// same log via per-subscriber cursors. Publish is therefore O(1) regardless
// of subscriber count.

package bus

import (
	"sync"
	"sync/atomic"
)

// defaultLogSize is the per-stream shared-log capacity. Sized to be large
// enough that a slow subscriber gets a few seconds of grace at typical live
// rates (~30 Hz video + ~50 Hz audio + occasional metadata = ~100 msg/s)
// before the producer overwrites their unread slots.
const defaultLogSize = 1024

// defaultArenaSlots is the per-stream payload-arena slot count. Sized as
// 4× the SharedLog so even a heavy backlog of subscribers can finish
// processing the oldest slot before its underlying memory is reused.
const defaultArenaSlots = 4096

// defaultArenaSlotSize covers most live-stream frames (audio + delta video
// at 1080p ≤ 4 Mbps comfortably fits). Frames larger than this — typically
// keyframes at high bitrates — fall back to a one-shot heap allocation,
// which is rare relative to delta frames.
const defaultArenaSlotSize = 16 * 1024

// Stream represents a live media stream instance.
// It manages one publisher and multiple subscribers via a per-stream
// SharedLog. Init messages (AVC/AAC sequence headers) are cached so
// late-joining subscribers receive codec configuration before live frames.
// Lock expectations: Mutex protects publisher / subscriber registry and
// init cache. The Publish hot path takes no Stream-level lock.
// Allocation: No allocations in steady state.
type Stream struct {
	key       StreamKey
	mu        sync.RWMutex
	publisher *Publisher

	// Subscribers are tracked for accounting (counts, drop totals, IsEmpty).
	// They do NOT need to be visited on Publish — the shared log makes
	// fanout cursor-based.
	subscribers map[uint64]*Subscriber
	nextSubID   uint64

	// Cached init messages (codec sequence headers + onMetaData). New
	// subscribers receive a snapshot of these in their `pending` queue
	// at attach time so they can decode before consulting the log.
	initVideo *MediaMessage
	initAudio *MediaMessage
	initMeta  *MediaMessage

	// Shared message log. Single producer (the publisher's goroutine),
	// many readers (one cursor per subscriber).
	log *SharedLog

	// Per-stream payload arena. Replaces the broken global sync.Pool that
	// was supposed to recycle publisher payload buffers but never received
	// Releases — every Acquire used to allocate a fresh 64 KB buffer. With
	// the arena, a steady-rate publisher allocates zero per frame.
	arena *Arena

	// Pre-allocated message-struct slab + bump cursor. Same wraparound
	// trick as the payload arena: the publisher takes the next slot,
	// resets it, and uses it. No GC pressure on the struct itself.
	msgs   []MediaMessage
	msgCur atomic.Uint64

	// Wake-on-publish: subscribers waiting on an empty log park on the
	// channel returned by WaitChan. Each Publish atomically swaps in a
	// fresh channel and closes the old one, broadcasting "data ready" to
	// every parked subscriber at once. Idle subscribers cost zero CPU.
	notify atomic.Pointer[chan struct{}]
}

// NewStream creates a new stream with default capacities suitable for live
// 1080p video at typical bitrates: 1024-slot log, 4096-slot × 16 KB arena.
func NewStream(key StreamKey) *Stream {
	return NewStreamWithCapacity(key, defaultLogSize, defaultArenaSlots, defaultArenaSlotSize)
}

// NewStreamWithCapacity is a knob for callers (notably benchmarks) that
// want to pick the log + arena sizes explicitly. Production code should
// stick with NewStream.
func NewStreamWithCapacity(key StreamKey, logSize, arenaSlots, arenaSlotSize int) *Stream {
	s := &Stream{
		key:         key,
		subscribers: make(map[uint64]*Subscriber),
		nextSubID:   1,
		log:         NewSharedLog(uint32(logSize)),
		arena:       NewArena(arenaSlots, arenaSlotSize),
		msgs:        make([]MediaMessage, arenaSlots),
	}
	initial := make(chan struct{})
	s.notify.Store(&initial)
	return s
}

// WaitChan returns the current "data ready" channel. Subscribers that
// find the log empty park on this channel; the next Publish closes it,
// waking every parked subscriber simultaneously. Callers must re-call
// WaitChan after each wakeup to obtain the freshly-rotated channel.
func (s *Stream) WaitChan() <-chan struct{} {
	return *s.notify.Load()
}

// broadcastReady rotates the notify channel — closes the old one (waking
// every parked subscriber) and stores a fresh one for the next round.
// Called from Publish on the hot path; must stay allocation-light.
func (s *Stream) broadcastReady() {
	fresh := make(chan struct{})
	old := s.notify.Swap(&fresh)
	if old != nil {
		close(*old)
	}
}

// AcquirePayload returns a zero-length, arena-backed slice with capacity
// large enough for `size` bytes when size <= the arena slot size. For
// larger payloads it falls back to a heap allocation. Publishers fill the
// returned slice via append before calling Publish.
func (s *Stream) AcquirePayload(size int) []byte {
	return s.arena.Acquire(size)
}

// AcquireMessage returns a pointer to a stream-owned MediaMessage slot.
// The slot is reset before return, so the caller sees zero-valued fields.
// Slots wrap-recycle every defaultArenaSlots calls; subscribers must
// finish processing a message before the slot is reused (the SharedLog's
// own wrap timing already provides this guarantee at typical frame rates).
func (s *Stream) AcquireMessage() *MediaMessage {
	n := s.msgCur.Add(1) - 1
	msg := &s.msgs[int(n)%len(s.msgs)]
	msg.Type = 0
	msg.Timestamp = 0
	msg.Payload = nil
	msg.IsInit = false
	return msg
}

// Key returns the stream's key.
func (s *Stream) Key() StreamKey { return s.key }

// AttachSubscriber attaches a new subscriber to the stream.
// The capacity and strategy arguments are accepted for API stability but
// the storage layout is now per-stream (the shared log). The subscriber's
// initial cursor is at the current end-of-log so it sees only new messages,
// preceded by a one-shot replay of cached init messages.
func (s *Stream) AttachSubscriber(_ uint32, _ BackpressureStrategy) (*Subscriber, uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := s.nextSubID
	s.nextSubID++

	sub := newSubscriber(id, s)
	// Snapshot init messages into the subscriber's pending queue so it
	// drains those before consulting the live log.
	if s.initMeta != nil {
		sub.pending = append(sub.pending, s.initMeta)
	}
	if s.initVideo != nil {
		sub.pending = append(sub.pending, s.initVideo)
	}
	if s.initAudio != nil {
		sub.pending = append(sub.pending, s.initAudio)
	}

	s.subscribers[id] = sub
	return sub, id
}

// DetachSubscriber detaches a subscriber from the stream.
func (s *Stream) DetachSubscriber(id uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.subscribers, id)
}

// Publish delivers a message to every subscriber via the shared log.
// Hot path: O(1) — one atomic.Add (sequence number) + one atomic.Store
// (slot pointer) + a chan-rotate broadcast that wakes any parked subscriber.
// Subscriber count does not affect publish cost.
// Allocation: one fresh chan struct{} per publish for the wake broadcast
// (~96 B). Init messages additionally allocate a clone (rare).
func (s *Stream) Publish(msg *MediaMessage) {
	if msg == nil {
		return
	}

	if msg.IsInit {
		s.cacheInitMessage(msg)
	}

	s.log.Publish(msg)
	s.broadcastReady()
}

// cacheInitMessage stores a clone of an init message for late-joining subscribers.
// Only called for messages with IsInit=true (codec sequence headers).
func (s *Stream) cacheInitMessage(msg *MediaMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch msg.Type {
	case MessageTypeVideo:
		s.initVideo = msg.Clone()
	case MessageTypeAudio:
		s.initAudio = msg.Clone()
	case MessageTypeMetadata:
		s.initMeta = msg.Clone()
	}
}

// SubscriberCount returns the number of active subscribers.
func (s *Stream) SubscriberCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.subscribers)
}

// IsEmpty returns true if the stream has no publisher and no subscribers.
func (s *Stream) IsEmpty() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.publisher == nil && len(s.subscribers) == 0
}

// MessagesPublished returns the cumulative number of messages routed through Publish.
// Equal to the highest sequence emitted on the shared log.
func (s *Stream) MessagesPublished() uint64 {
	return s.log.LatestSeq()
}

// HasAudioInit reports whether the publisher has produced an AAC sequence
// header on this stream. Subscribers use this to set the FLV header's
// has-audio flag correctly — claiming audio when none is present makes
// ffmpeg's analyzer hang in find_stream_info.
func (s *Stream) HasAudioInit() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.initAudio != nil
}

// HasVideoInit reports whether an AVC sequence header has been cached.
func (s *Stream) HasVideoInit() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.initVideo != nil
}

// TotalDropped returns the sum of dropped-message counts across all current subscribers.
// Subscribers that have already disconnected are not included; this metric is for live
// pressure, not a permanent total.
func (s *Stream) TotalDropped() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var total uint64
	for _, sub := range s.subscribers {
		total += sub.Dropped()
	}
	return total
}
