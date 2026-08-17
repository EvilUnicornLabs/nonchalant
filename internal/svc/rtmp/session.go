// If you are AI: This file manages RTMP service session handling.
// Handles connect lifecycle and session state.

package rtmp

import (
	"fmt"
	"log"
	"github.com/vinq1911/nonchalant/internal/core/bus"
	"github.com/vinq1911/nonchalant/internal/core/protocol/amf0"
	rtmpprotocol "github.com/vinq1911/nonchalant/internal/core/protocol/rtmp"
)

// ServiceSession wraps RTMP protocol session with service logic.
type ServiceSession struct {
	*rtmpprotocol.Session
	registry     *bus.Registry
	auth         *Authenticator
	publisher    *Publisher
	nextStreamID uint32
}

// NewServiceSession creates a new service session.
// auth may be nil to allow anonymous publishing (backward compatible).
func NewServiceSession(conn *sessionConn, registry *bus.Registry, auth *Authenticator) *ServiceSession {
	return &ServiceSession{
		Session:      rtmpprotocol.NewSession(conn),
		registry:     registry,
		auth:         auth,
		nextStreamID: 1,
	}
}

// HandleConnect handles the connect command and sends all required control messages.
// Reference order (matching node-media-server):
//  1. Window Acknowledgement Size (type 5)
//  2. Set Peer Bandwidth (type 6)
//  3. Set Chunk Size (type 1)
//  4. _result command (type 20)
func (s *ServiceSession) HandleConnect(command amf0.Array) error {
	if len(command) < 2 {
		return fmt.Errorf("invalid connect command: need at least 2 elements")
	}

	app := "live"
	objectEncoding := float64(0)

	// Extract app and objectEncoding from command object if present
	if len(command) >= 3 && command[2] != nil {
		cmdObj := toObject(command[2])
		if cmdObj != nil {
			if appVal, ok := cmdObj["app"].(string); ok {
				app = appVal
			}
			if encVal, ok := cmdObj["objectEncoding"].(float64); ok {
				objectEncoding = encVal
			}
		}
	}

	s.SetApp(app)

	// 1. Window Acknowledgement Size
	ackWindowSize := uint32(5000000)
	if err := s.WriteMessage(2, rtmpprotocol.MessageTypeWinAckSize, 0, 0,
		rtmpprotocol.CreateWindowAckSize(ackWindowSize)); err != nil {
		return fmt.Errorf("send window ack size: %w", err)
	}
	s.Session.SetAckSize(ackWindowSize)

	// 2. Set Peer Bandwidth (dynamic limit type = 2)
	if err := s.WriteMessage(2, rtmpprotocol.MessageTypeSetPeerBandwidth, 0, 0,
		rtmpprotocol.CreateSetPeerBandwidth(5000000, 2)); err != nil {
		return fmt.Errorf("send peer bandwidth: %w", err)
	}

	// 3. Set Chunk Size
	if err := s.WriteMessage(2, rtmpprotocol.MessageTypeSetChunkSize, 0, 0,
		rtmpprotocol.CreateSetChunkSize(4096)); err != nil {
		return fmt.Errorf("send chunk size: %w", err)
	}
	s.SetWriteChunkSize(4096)

	// 4. _result response
	if err := s.sendConnectResult(command[1], objectEncoding); err != nil {
		return fmt.Errorf("send connect result: %w", err)
	}

	log.Printf("HandleConnect: app=%s objectEncoding=%.0f", app, objectEncoding)
	return nil
}

// sendConnectResult sends the connect _result response.
// Format: _result, transID, cmdObj{fmsVer, capabilities}, info{level, code, description, objectEncoding}
func (s *ServiceSession) sendConnectResult(transID interface{}, objectEncoding float64) error {
	transIDFloat := toFloat64(transID)

	cmdObj := amf0.Object{
		"fmsVer":       "FMS/3,0,1,123",
		"capabilities": float64(31),
	}
	info := amf0.Object{
		"level":          "status",
		"code":           "NetConnection.Connect.Success",
		"description":    "Connection succeeded.",
		"objectEncoding": objectEncoding,
	}

	body, err := amf0.EncodeCommand(amf0.Array{"_result", transIDFloat, cmdObj, info})
	if err != nil {
		return err
	}

	return s.WriteMessage(3, rtmpprotocol.MessageTypeCommandAMF0, 0, 0, body)
}

// HandleMediaMessage handles audio/video/data messages.
func (s *ServiceSession) HandleMediaMessage(msgType byte, timestamp uint32, body []byte) {
	if s.publisher == nil {
		return // Not publishing
	}

	switch msgType {
	case rtmpprotocol.MessageTypeAudio:
		s.publisher.PublishAudio(timestamp, body)
	case rtmpprotocol.MessageTypeVideo:
		s.publisher.PublishVideo(timestamp, body)
	case rtmpprotocol.MessageTypeDataAMF0:
		s.publisher.PublishMetadata(timestamp, body)
	default:
		// NOTE: Other message types are ignored
	}
}

// Close closes the session and detaches publisher.
func (s *ServiceSession) Close() {
	if s.publisher != nil {
		s.publisher.Detach()
		if s.publisher.stream != nil {
			streamKey := s.publisher.StreamKey()
			s.registry.RemoveIfEmpty(streamKey)
		}
	}
	s.Session.Close()
}

// toObject converts interface{} to amf0.Object.
func toObject(v interface{}) amf0.Object {
	switch obj := v.(type) {
	case amf0.Object:
		return obj
	case map[string]interface{}:
		result := make(amf0.Object)
		for k, val := range obj {
			result[k] = val
		}
		return result
	default:
		return nil
	}
}

// toFloat64 converts interface{} to float64 with fallback.
func toFloat64(v interface{}) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 1.0
	}
}
