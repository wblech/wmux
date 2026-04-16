// Package protocol defines the binary wire format and message types for wmux IPC.
package protocol

import "errors"

// Protocol version and frame size constants.
const (
	ProtocolVersion byte = 1
	HeaderSize           = 6               // version:1 + type:1 + length:4
	MaxPayloadSize       = 4 * 1024 * 1024 // 4MB
)

// MessageType identifies the kind of message in a frame.
type MessageType byte

// Message type constants for all supported wmux protocol messages.
const (
	MsgData                     MessageType = 0x01
	MsgCreate                   MessageType = 0x02
	MsgAttach                   MessageType = 0x03
	MsgDetach                   MessageType = 0x04
	MsgKill                     MessageType = 0x05
	MsgResize                   MessageType = 0x06
	MsgList                     MessageType = 0x07
	MsgInfo                     MessageType = 0x08
	MsgInput                    MessageType = 0x09
	MsgEvent                    MessageType = 0x0A
	MsgHeartbeat                MessageType = 0x0B
	MsgHeartbeatAck             MessageType = 0x0C
	MsgError                    MessageType = 0x0D
	MsgOK                       MessageType = 0x0E
	MsgShutdown                 MessageType = 0x0F
	MsgAuth                     MessageType = 0x10
	MsgStatus                   MessageType = 0x11
	MsgMetaSet                  MessageType = 0x12
	MsgMetaGet                  MessageType = 0x13
	MsgEnvForward               MessageType = 0x14
	MsgExec                     MessageType = 0x15
	MsgExecSync                 MessageType = 0x16
	MsgWait                     MessageType = 0x17
	MsgRecord                   MessageType = 0x18
	MsgHistory                  MessageType = 0x19
	MsgHistoryEnd               MessageType = 0x1A
	MsgKillPrefix               MessageType = 0x1B
	MsgUpdateEmulatorScrollback MessageType = 0x1C
)

// String returns the lowercase name of the message type.
func (m MessageType) String() string {
	switch m {
	case MsgData:
		return "data"
	case MsgCreate:
		return "create"
	case MsgAttach:
		return "attach"
	case MsgDetach:
		return "detach"
	case MsgKill:
		return "kill"
	case MsgResize:
		return "resize"
	case MsgList:
		return "list"
	case MsgInfo:
		return "info"
	case MsgInput:
		return "input"
	case MsgEvent:
		return "event"
	case MsgHeartbeat:
		return "heartbeat"
	case MsgHeartbeatAck:
		return "heartbeat_ack"
	case MsgError:
		return "error"
	case MsgOK:
		return "ok"
	case MsgShutdown:
		return "shutdown"
	case MsgAuth:
		return "auth"
	case MsgStatus:
		return "status"
	case MsgMetaSet:
		return "meta_set"
	case MsgMetaGet:
		return "meta_get"
	case MsgEnvForward:
		return "env_forward"
	case MsgExec:
		return "exec"
	case MsgExecSync:
		return "exec_sync"
	case MsgWait:
		return "wait"
	case MsgRecord:
		return "record"
	case MsgHistory:
		return "history"
	case MsgHistoryEnd:
		return "history_end"
	case MsgKillPrefix:
		return "kill_prefix"
	case MsgUpdateEmulatorScrollback:
		return "update_emulator_scrollback"
	default:
		return "unknown"
	}
}

// Frame is a single unit of data exchanged over the wire.
type Frame struct {
	Version byte
	Type    MessageType
	Payload []byte
}

// Sentinel errors returned by Codec operations.
var (
	ErrVersionMismatch = errors.New("protocol: version mismatch")
	ErrPayloadTooLarge = errors.New("protocol: payload too large")
	ErrInvalidFrame    = errors.New("protocol: invalid frame")
	ErrShortRead       = errors.New("protocol: short read")
)
