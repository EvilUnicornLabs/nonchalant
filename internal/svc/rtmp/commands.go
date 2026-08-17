// If you are AI: This file handles RTMP command messages after connect.
// Implements releaseStream, FCPublish, createStream, publish, deleteStream.

package rtmp

import (
	"fmt"
	"log"
	"github.com/vinq1911/nonchalant/internal/core/bus"
	"github.com/vinq1911/nonchalant/internal/core/protocol/amf0"
	rtmpprotocol "github.com/vinq1911/nonchalant/internal/core/protocol/rtmp"
)

// HandleReleaseStream handles the releaseStream command.
// FFmpeg sends this before createStream; respond with _result for the transaction ID.
// Reference: go2rtc responds with _result; node-media-server ignores it.
func (s *ServiceSession) HandleReleaseStream(command amf0.Array) error {
	if len(command) < 2 {
		return nil // Silently ignore malformed command
	}
	transID := toFloat64(command[1])
	body, err := amf0.EncodeCommand(amf0.Array{"_result", transID, nil})
	if err != nil {
		return err
	}
	return s.WriteMessage(3, rtmpprotocol.MessageTypeCommandAMF0, 0, 0, body)
}

// HandleFCPublish handles the FCPublish command.
// FFmpeg sends this before createStream. Most servers (including go2rtc) do not respond.
// We send onFCPublish for compatibility with stricter clients.
func (s *ServiceSession) HandleFCPublish(command amf0.Array) error {
	// NOTE: go2rtc sends no response for FCPublish; node-media-server also ignores.
	// Some clients may expect onFCPublish, so we send it for safety.
	if len(command) < 2 {
		return nil
	}
	transID := toFloat64(command[1])
	body, err := amf0.EncodeCommand(amf0.Array{"_result", transID, nil})
	if err != nil {
		return err
	}
	return s.WriteMessage(3, rtmpprotocol.MessageTypeCommandAMF0, 0, 0, body)
}

// HandleCreateStream handles the createStream command.
// Returns _result with a new stream ID.
func (s *ServiceSession) HandleCreateStream(command amf0.Array) error {
	if len(command) < 2 {
		return fmt.Errorf("invalid createStream command")
	}
	streamID := s.nextStreamID
	s.nextStreamID++

	transID := toFloat64(command[1])
	body, err := amf0.EncodeCommand(amf0.Array{"_result", transID, nil, float64(streamID)})
	if err != nil {
		return err
	}

	return s.WriteMessage(3, rtmpprotocol.MessageTypeCommandAMF0, 0, 0, body)
}

// HandlePublish handles the publish command.
// streamID is the stream ID from the message header where the publish command was received.
// Sends StreamBegin + onStatus NetStream.Publish.Start on success.
// If publish-key authentication is configured, the stream name must include
// "?key=<secret>"; otherwise the publish is rejected with NetStream.Publish.Failed.
func (s *ServiceSession) HandlePublish(command amf0.Array, streamID uint32) error {
	// publish format: ["publish", txnID, null, streamName, publishType]
	rawName := extractStreamName(command)
	if rawName == "" {
		return fmt.Errorf("stream name not found in publish command")
	}

	streamName, key := ParseStreamName(rawName)
	if streamName == "" {
		return fmt.Errorf("empty stream name")
	}

	if !s.auth.Allow(key) {
		log.Printf("Publish rejected: invalid or missing auth key for %s", streamName)
		// Notify client via onStatus, then return error to close the session.
		_ = s.sendOnStatus(streamID, "error",
			"NetStream.Publish.Failed", "Authentication failed")
		return fmt.Errorf("auth failed for stream %q", streamName)
	}

	app := s.GetApp()
	if app == "" {
		return fmt.Errorf("app not set")
	}

	streamKey := bus.NewStreamKey(app, streamName)
	stream, created := s.registry.GetOrCreate(streamKey)
	if !created {
		log.Printf("Stream %s already exists", streamKey)
	}

	// FIXME: If publisher already exists, reject deterministically
	publisherID := uint64(1)
	if !stream.AttachPublisher(publisherID) {
		return fmt.Errorf("stream already has a publisher")
	}

	s.publisher = NewPublisher(s.Session, stream, publisherID)
	s.SetStreamName(streamName)
	s.SetState(rtmpprotocol.StatePublishing)

	// Send StreamBegin user control message for this stream ID
	if err := s.WriteMessage(2, rtmpprotocol.MessageTypeUserCtrl, 0, 0,
		rtmpprotocol.CreateStreamBegin(streamID)); err != nil {
		log.Printf("Failed to send StreamBegin: %v", err)
		// Non-fatal, continue
	}

	// Send onStatus NetStream.Publish.Start (critical — clients wait for this)
	return s.sendOnStatus(streamID, "status", "NetStream.Publish.Start", "Start publishing")
}

// sendOnStatus sends an onStatus message on the given stream ID.
// Used for publish start/stop and play start/stop notifications.
func (s *ServiceSession) sendOnStatus(streamID uint32, level, code, description string) error {
	status := amf0.Object{
		"level":       level,
		"code":        code,
		"description": description,
	}
	body, err := amf0.EncodeCommand(amf0.Array{"onStatus", float64(0), nil, status})
	if err != nil {
		return err
	}
	// Send on chunk stream 5 with the publish stream ID
	return s.WriteMessage(5, rtmpprotocol.MessageTypeCommandAMF0, 0, streamID, body)
}

// extractStreamName extracts the stream name from a publish command.
// publish format: ["publish", txnID, null, streamName, publishType]
func extractStreamName(command amf0.Array) string {
	// Try index 3 first (standard position after null command object)
	if len(command) >= 4 {
		if name, ok := command[3].(string); ok {
			return name
		}
	}
	// Fallback: try index 2 (some clients omit the null)
	if len(command) >= 3 {
		if name, ok := command[2].(string); ok {
			return name
		}
	}
	return ""
}
