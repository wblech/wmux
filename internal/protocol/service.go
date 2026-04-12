package protocol

import (
	"encoding/binary"
	"io"
)

// Codec encodes and decodes frames to/from the binary wire format.
// Wire format: [version:1][type:1][length:4][payload:N] — all big-endian.
type Codec struct{}

// Encode writes f to w using the binary wire format.
// Returns ErrPayloadTooLarge if len(f.Payload) > MaxPayloadSize.
func (c Codec) Encode(w io.Writer, f Frame) error {
	if len(f.Payload) > MaxPayloadSize {
		return ErrPayloadTooLarge
	}

	header := [HeaderSize]byte{
		f.Version,
		byte(f.Type),
	}
	binary.BigEndian.PutUint32(header[2:], uint32(len(f.Payload)))

	if _, err := w.Write(header[:]); err != nil {
		return err
	}

	if len(f.Payload) > 0 {
		if _, err := w.Write(f.Payload); err != nil {
			return err
		}
	}

	return nil
}

// Decode reads one frame from r.
// Returns ErrShortRead if the header cannot be read fully.
// Returns ErrVersionMismatch if the frame version != ProtocolVersion.
// Returns ErrPayloadTooLarge if the declared payload length > MaxPayloadSize.
func (c Codec) Decode(r io.Reader) (Frame, error) {
	var header [HeaderSize]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		if err == io.ErrUnexpectedEOF || err == io.EOF {
			return Frame{}, ErrShortRead
		}
		return Frame{}, err
	}

	version := header[0]
	if version != ProtocolVersion {
		return Frame{}, ErrVersionMismatch
	}

	msgType := MessageType(header[1])
	payloadLen := binary.BigEndian.Uint32(header[2:])

	if payloadLen > MaxPayloadSize {
		return Frame{}, ErrPayloadTooLarge
	}

	var payload []byte
	if payloadLen > 0 {
		payload = make([]byte, payloadLen)
		if _, err := io.ReadFull(r, payload); err != nil {
			if err == io.ErrUnexpectedEOF || err == io.EOF {
				return Frame{}, ErrShortRead
			}
			return Frame{}, err
		}
	}

	return Frame{
		Version: version,
		Type:    msgType,
		Payload: payload,
	}, nil
}
