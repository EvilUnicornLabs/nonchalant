// If you are AI: This file implements the RTMP server that accepts connections.
// The server handles handshake, command processing, and media publishing.

package rtmp

import (
	"bytes"
	"fmt"
	"github.com/vinq1911/nonchalant/internal/core/protocol/amf0"
	rtmpprotocol "github.com/vinq1911/nonchalant/internal/core/protocol/rtmp"
	"github.com/vinq1911/nonchalant/pkg/core/bus"
	"io"
	"log"
	"net"
)

// Server represents an RTMP server.
type Server struct {
	registry *bus.Registry
	listener net.Listener
	auth     *Authenticator // nil means anonymous publishing is allowed
}

// NewServer creates a new RTMP server.
// Pass a nil Authenticator (or one returned for an empty key list) to allow
// anonymous publishing; otherwise publishers must include "?key=<secret>"
// in the stream name on the publish command.
func NewServer(registry *bus.Registry, auth *Authenticator) *Server {
	return &Server{registry: registry, auth: auth}
}

// Listen starts listening on the specified address.
func (s *Server) Listen(addr string) error {
	var err error
	s.listener, err = net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return nil
}

// Accept accepts connections and handles them in goroutines.
func (s *Server) Accept() error {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return err
		}
		go s.handleConnection(conn)
	}
}

// sessionConn wraps net.Conn to implement io.ReadWriter for the session.
type sessionConn struct {
	net.Conn
}

// handleConnection handles a single RTMP connection.
// NOTE: SetChunkSize, WindowAckSize, PeerBandwidth are sent during HandleConnect
// (matching node-media-server order), not immediately after handshake.
func (s *Server) handleConnection(conn net.Conn) {
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("Error closing connection: %v", err)
		}
	}()

	sc := &sessionConn{Conn: conn}
	session := NewServiceSession(sc, s.registry, s.auth)
	defer session.Close()

	if err := session.PerformHandshake(); err != nil {
		if err.Error() == "invalid RTMP version" {
			return // Silently close non-RTMP connections
		}
		log.Printf("Handshake failed: %v", err)
		return
	}

	// Main message loop
	for {
		csID, err := session.ReadChunk()
		if err != nil {
			if err != io.EOF {
				log.Printf("Read chunk error: %v", err)
			}
			return
		}

		// ACK tracking
		bytesRead := session.Session.GetBytesReadForChunk(csID)
		if bytesRead > 0 {
			if _, err := session.Session.RecordBytesReceived(bytesRead); err != nil {
				log.Printf("Failed to send ACK: %v", err)
			}
		}

		body, msgType, timestamp, streamID, complete := session.GetCompleteMessage(csID)
		if !complete {
			continue
		}

		switch msgType {
		case rtmpprotocol.MessageTypeSetChunkSize:
			size, err := rtmpprotocol.ParseSetChunkSize(body)
			if err != nil {
				log.Printf("Failed to parse set chunk size: %v", err)
				continue
			}
			// Update READ chunk size only — this is the client's sending chunk size
			session.SetReadChunkSize(size)
			log.Printf("Client set chunk size: %d", size)

		case rtmpprotocol.MessageTypeWinAckSize:
			// Client's window ack size — acknowledge receipt, no action needed
			log.Printf("Client set window ack size")

		case rtmpprotocol.MessageTypeUserCtrl:
			// User control messages (ping, stream begin, etc.) — no response needed

		case rtmpprotocol.MessageTypeCommandAMF0:
			if err := s.handleCommand(session, body, streamID); err != nil {
				log.Printf("Command error: %v", err)
				return
			}

		case rtmpprotocol.MessageTypeAudio, rtmpprotocol.MessageTypeVideo,
			rtmpprotocol.MessageTypeDataAMF0:
			session.HandleMediaMessage(msgType, timestamp, body)

		default:
			// NOTE: Other message types (abort, ack, peer bandwidth) are ignored
		}
	}
}

// handleCommand handles AMF0 command messages.
// streamID is the stream ID from the message header.
func (s *Server) handleCommand(session *ServiceSession, body []byte, streamID uint32) error {
	command, err := amf0.DecodeCommand(bytes.NewReader(body))
	if err != nil {
		logDecodeError(body, err)
		return err
	}
	if len(command) == 0 {
		return nil
	}

	cmdName, ok := command[0].(string)
	if !ok {
		return nil
	}

	switch cmdName {
	case "connect":
		log.Printf("Command: connect")
		return session.HandleConnect(command)
	case "releaseStream":
		log.Printf("Command: releaseStream")
		return session.HandleReleaseStream(command)
	case "FCPublish":
		log.Printf("Command: FCPublish")
		return session.HandleFCPublish(command)
	case "createStream":
		log.Printf("Command: createStream")
		return session.HandleCreateStream(command)
	case "publish":
		log.Printf("Command: publish (streamID=%d)", streamID)
		return session.HandlePublish(command, streamID)
	case "deleteStream", "closeStream":
		log.Printf("Command: %s", cmdName)
		session.Close()
		return nil
	case "FCUnpublish":
		log.Printf("Command: FCUnpublish (ignored)")
		return nil
	default:
		log.Printf("Command: %s (unhandled)", cmdName)
		return nil
	}
}

// logDecodeError logs diagnostic info for failed AMF0 decoding.
func logDecodeError(body []byte, err error) {
	hex := ""
	if len(body) > 0 {
		hex = fmt.Sprintf(" first_byte=0x%02x", body[0])
		if len(body) > 15 {
			hex += fmt.Sprintf(" hex=%x", body[:16])
		}
	}
	log.Printf("Failed to decode command: %v (len=%d%s)", err, len(body), hex)
}

// Close closes the server.
func (s *Server) Close() error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}
