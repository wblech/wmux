package daemon

import "bytes"

// DAType identifies the kind of device attribute request.
type DAType int

const (
	DATypeDA1 DAType = iota + 1
	DATypeDA2
)

// String returns the human-readable name of the DA type.
func (d DAType) String() string {
	switch d {
	case DATypeDA1:
		return "DA1"
	case DATypeDA2:
		return "DA2"
	default:
		return "unknown"
	}
}

// DARequest represents a single device attribute request detected in a byte buffer.
type DARequest struct {
	Type DAType
}

// DA sequences (longer/more-specific sequences must be checked first).
var (
	da2Seq0 = []byte("\x1b[>0c") // ESC[>0c — DA2 with explicit 0
	da2Seq  = []byte("\x1b[>c")  // ESC[>c  — DA2
	da1Seq0 = []byte("\x1b[0c")  // ESC[0c  — DA1 with explicit 0
	da1Seq  = []byte("\x1b[c")   // ESC[c   — DA1
)

// ParseDA scans data for DA1 and DA2 device attribute request sequences and
// returns all matches in order of appearance. It handles multiple sequences in
// a single buffer as well as sequences embedded in other output. Returns nil
// when no sequences are found.
func ParseDA(data []byte) []DARequest {
	var requests []DARequest

	for i := 0; i < len(data); {
		idx := bytes.IndexByte(data[i:], 0x1b) // ESC
		if idx < 0 {
			break
		}

		pos := i + idx
		tail := data[pos:]

		switch {
		case bytes.HasPrefix(tail, da2Seq0):
			requests = append(requests, DARequest{Type: DATypeDA2})
			i = pos + len(da2Seq0)
		case bytes.HasPrefix(tail, da2Seq):
			requests = append(requests, DARequest{Type: DATypeDA2})
			i = pos + len(da2Seq)
		case bytes.HasPrefix(tail, da1Seq0):
			requests = append(requests, DARequest{Type: DATypeDA1})
			i = pos + len(da1Seq0)
		case bytes.HasPrefix(tail, da1Seq):
			requests = append(requests, DARequest{Type: DATypeDA1})
			i = pos + len(da1Seq)
		default:
			// ESC byte not part of a known DA sequence — advance past it.
			i = pos + 1
		}
	}

	return requests
}

// DA1Response returns the VT220 primary device attribute response
// (VT220 with ANSI color capability).
func DA1Response() []byte {
	return []byte("\x1b[?62;22c")
}

// DA2Response returns the secondary device attribute response.
func DA2Response() []byte {
	return []byte("\x1b[>1;0;0c")
}
