package daemon

import (
	"bytes"
	"net/url"
	"strings"
)

// OSCType identifies the kind of OSC sequence detected.
type OSCType int

const (
	_ OSCType = iota
	// OSCTypeCwd indicates an OSC 7 working directory change.
	OSCTypeCwd
	// OSCTypeNotification indicates an OSC 9/99/777 notification.
	OSCTypeNotification
	// OSCTypeShellReady indicates a wmux shell-ready marker (OSC 777;wmux;shell-ready).
	OSCTypeShellReady
)

// OSCResult holds a parsed OSC sequence.
type OSCResult struct {
	Type  OSCType
	Value string
}

// oscIntroducer is the byte pair that starts an OSC sequence: ESC ].
var oscIntroducer = []byte{0x1b, ']'}

// oscTerminatorST is the ANSI string terminator: ESC \.
var oscTerminatorST = []byte{0x1b, '\\'}

// ParseOSC scans data for OSC sequences (7, 9, 99, 777) and returns parsed results.
// This is a passive scanner — it does not modify the data.
//
// Operates directly on []byte without converting to string; for the common
// case of PTY output without any OSC sequences this avoids allocating a
// copy of the entire chunk just to scan over it. Only the OSC body content
// is converted to string when a match is found, since OSCResult.Value is
// a string.
func ParseOSC(data []byte) []OSCResult {
	var results []OSCResult
	s := data

	for {
		idx := bytes.Index(s, oscIntroducer)
		if idx < 0 {
			break
		}
		s = s[idx+2:]

		endST := bytes.Index(s, oscTerminatorST)
		endBEL := bytes.IndexByte(s, 0x07)

		end := -1
		switch {
		case endST >= 0 && endBEL >= 0 && endST < endBEL:
			end = endST
		case endST >= 0 && endBEL >= 0:
			end = endBEL
		case endST >= 0:
			end = endST
		case endBEL >= 0:
			end = endBEL
		}

		if end < 0 {
			break
		}

		body := s[:end]
		s = s[end+1:]

		semicolon := bytes.IndexByte(body, ';')
		if semicolon < 0 {
			continue
		}
		oscNum := body[:semicolon]
		oscValue := string(body[semicolon+1:])

		switch string(oscNum) {
		case "7":
			parsed, err := url.Parse(oscValue)
			if err == nil && parsed.Path != "" {
				results = append(results, OSCResult{Type: OSCTypeCwd, Value: parsed.Path})
			}
		case "9":
			results = append(results, OSCResult{Type: OSCTypeNotification, Value: oscValue})
		case "99":
			parts := strings.SplitN(oscValue, ";", 2)
			val := oscValue
			if len(parts) == 2 {
				val = parts[1]
			}
			results = append(results, OSCResult{Type: OSCTypeNotification, Value: val})
		case "777":
			parts := strings.SplitN(oscValue, ";", 3)
			// Detect wmux shell-ready marker: 777;wmux;shell-ready
			if len(parts) >= 2 && parts[0] == "wmux" && parts[1] == "shell-ready" {
				results = append(results, OSCResult{Type: OSCTypeShellReady, Value: "shell-ready"})
				continue
			}
			val := oscValue
			if len(parts) == 3 {
				val = parts[2]
			} else if len(parts) == 2 {
				val = parts[1]
			}
			results = append(results, OSCResult{Type: OSCTypeNotification, Value: val})
		}
	}

	return results
}
