package daemon

import (
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
)

// OSCResult holds a parsed OSC sequence.
type OSCResult struct {
	Type  OSCType
	Value string
}

// ParseOSC scans data for OSC sequences (7, 9, 99, 777) and returns parsed results.
// This is a passive scanner — it does not modify the data.
func ParseOSC(data []byte) []OSCResult {
	var results []OSCResult
	s := string(data)

	for {
		idx := strings.Index(s, "\x1b]")
		if idx < 0 {
			break
		}
		s = s[idx+2:]

		endST := strings.Index(s, "\x1b\\")
		endBEL := strings.IndexByte(s, 0x07)

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

		semicolon := strings.IndexByte(body, ';')
		if semicolon < 0 {
			continue
		}
		oscNum := body[:semicolon]
		oscValue := body[semicolon+1:]

		switch oscNum {
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
