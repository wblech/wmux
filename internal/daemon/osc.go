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
	// OSCType133 indicates an OSC 133 shell-integration marker
	// (A = prompt, B = user input, C = command, D = exit).
	OSCType133
)

// OSCResult holds a parsed OSC sequence.
type OSCResult struct {
	Type  OSCType
	Value string
	// Offset is the byte position of the ESC ] introducer in the original
	// data slice. Used by scanOSC for offset-aware row tracking.
	Offset int
}

// oscIntroducer is the byte pair that starts an OSC sequence: ESC ].
var oscIntroducer = []byte{0x1b, ']'}

// oscTerminatorST is the ANSI string terminator: ESC \.
var oscTerminatorST = []byte{0x1b, '\\'}

// ParseOSC scans data for OSC sequences (7, 9, 99, 133, 777) and returns
// parsed results in source order. Each result includes the byte offset of
// its ESC ] introducer in data, enabling callers to compute positions
// (e.g. row counts) relative to each match.
//
// Allocation: returns nil when no OSC sequences are found. The
// no-match path is gated by BenchmarkParseOSC_NoOSC at 0 allocs/op.
func ParseOSC(data []byte) []OSCResult {
	var results []OSCResult
	cursor := 0

	for {
		idx := bytes.Index(data[cursor:], oscIntroducer)
		if idx < 0 {
			break
		}
		matchStart := cursor + idx
		cursor = matchStart + 2

		endST := bytes.Index(data[cursor:], oscTerminatorST)
		endBEL := bytes.IndexByte(data[cursor:], 0x07)
		end := pickOSCEnd(endST, endBEL)
		if end < 0 {
			break
		}
		bodyEnd := cursor + end
		body := data[cursor:bodyEnd]
		cursor = bodyEnd + 1

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
				results = append(results, OSCResult{
					Type:   OSCTypeCwd,
					Value:  parsed.Path,
					Offset: matchStart,
				})
			}
		case "9":
			results = append(results, OSCResult{
				Type:   OSCTypeNotification,
				Value:  oscValue,
				Offset: matchStart,
			})
		case "99":
			parts := strings.SplitN(oscValue, ";", 2)
			val := oscValue
			if len(parts) == 2 {
				val = parts[1]
			}
			results = append(results, OSCResult{
				Type:   OSCTypeNotification,
				Value:  val,
				Offset: matchStart,
			})
		case "133":
			results = append(results, OSCResult{
				Type:   OSCType133,
				Value:  oscValue,
				Offset: matchStart,
			})
		case "777":
			parts := strings.SplitN(oscValue, ";", 3)
			if len(parts) >= 2 && parts[0] == "wmux" && parts[1] == "shell-ready" {
				results = append(results, OSCResult{
					Type:   OSCTypeShellReady,
					Value:  "shell-ready",
					Offset: matchStart,
				})
				continue
			}
			val := oscValue
			if len(parts) == 3 {
				val = parts[2]
			} else if len(parts) == 2 {
				val = parts[1]
			}
			results = append(results, OSCResult{
				Type:   OSCTypeNotification,
				Value:  val,
				Offset: matchStart,
			})
		}
	}

	return results
}

// pickOSCEnd selects the earlier non-negative terminator index.
// Refactored from the inline switch in ParseOSC.
func pickOSCEnd(endST, endBEL int) int {
	switch {
	case endST >= 0 && endBEL >= 0 && endST < endBEL:
		return endST
	case endST >= 0 && endBEL >= 0:
		return endBEL
	case endST >= 0:
		return endST
	case endBEL >= 0:
		return endBEL
	}
	return -1
}
