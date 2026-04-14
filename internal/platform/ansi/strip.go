// Package ansi provides utilities for processing ANSI escape sequences,
// including stripping and conversion to HTML.
package ansi

// Strip removes all ANSI escape sequences from the input and returns
// the resulting plain text as a string. Handles CSI (ESC [), OSC (ESC ]),
// and simple two-byte escape sequences (ESC followed by a single char).
func Strip(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	out := make([]byte, 0, len(data))
	i := 0

	for i < len(data) {
		if data[i] == 0x1b { // ESC
			i++
			if i >= len(data) {
				break
			}
			switch data[i] {
			case '[': // CSI sequence: consume until 0x40-0x7E
				i++
				for i < len(data) {
					b := data[i]
					i++
					if b >= 0x40 && b <= 0x7E {
						break
					}
				}
			case ']': // OSC sequence: consume until BEL or ST
				i++
				for i < len(data) {
					if data[i] == 0x07 { // BEL
						i++
						break
					}
					if data[i] == 0x1b && i+1 < len(data) && data[i+1] == '\\' { // ST
						i += 2
						break
					}
					i++
				}
			default:
				// Two-byte escape (e.g., ESC M, ESC 7, ESC 8).
				i++
			}
		} else {
			out = append(out, data[i])
			i++
		}
	}

	return string(out)
}
