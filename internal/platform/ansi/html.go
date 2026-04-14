package ansi

import (
	"fmt"
	"html"
	"strconv"
	"strings"
)

// basic16 maps ANSI color indices 0-7 (normal) and 8-15 (bright) to CSS hex colors.
var basic16 = [16]string{
	"#000000", "#aa0000", "#00aa00", "#aa5500",
	"#0000aa", "#aa00aa", "#00aaaa", "#aaaaaa",
	"#555555", "#ff5555", "#55ff55", "#ffff55",
	"#5555ff", "#ff55ff", "#55ffff", "#ffffff",
}

// sgrState tracks the current ANSI SGR attributes.
type sgrState struct {
	bold      bool
	italic    bool
	underline bool
	fg        string // CSS color or empty
	bg        string // CSS color or empty
}

// reset returns the state to default (no styling).
func (s *sgrState) reset() {
	s.bold = false
	s.italic = false
	s.underline = false
	s.fg = ""
	s.bg = ""
}

// style returns a CSS style string for the current state, or empty if default.
func (s *sgrState) style() string {
	var parts []string
	if s.bold {
		parts = append(parts, "font-weight:bold")
	}
	if s.italic {
		parts = append(parts, "font-style:italic")
	}
	if s.underline {
		parts = append(parts, "text-decoration:underline")
	}
	if s.fg != "" {
		parts = append(parts, "color:"+s.fg)
	}
	if s.bg != "" {
		parts = append(parts, "background-color:"+s.bg)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ";")
}

// color256 maps a 256-color palette index to a CSS color string.
func color256(idx int) string {
	if idx < 0 || idx > 255 {
		return ""
	}
	if idx < 16 {
		return basic16[idx]
	}
	if idx < 232 {
		idx -= 16
		r := (idx / 36) * 51
		g := ((idx / 6) % 6) * 51
		b := (idx % 6) * 51
		return fmt.Sprintf("rgb(%d,%d,%d)", r, g, b)
	}
	v := (idx-232)*10 + 8
	return fmt.Sprintf("rgb(%d,%d,%d)", v, v, v)
}

// ToHTML converts ANSI-escaped terminal output to styled HTML wrapped in
// a <pre> tag. Supports SGR attributes (bold, italic, underline), basic
// 16 colors, 256-color palette, and 24-bit truecolor for both foreground
// and background. Non-SGR escape sequences are stripped.
func ToHTML(data []byte) string {
	var b strings.Builder
	b.WriteString(`<pre style="font-family:monospace;background:#000;color:#aaa;padding:8px">`)

	if len(data) == 0 {
		b.WriteString("</pre>")
		return b.String()
	}

	var state sgrState
	spanOpen := false
	i := 0

	flushSpan := func() {
		if spanOpen {
			b.WriteString("</span>")
			spanOpen = false
		}
	}

	openSpan := func() {
		style := state.style()
		if style != "" {
			b.WriteString(`<span style="`)
			b.WriteString(style)
			b.WriteString(`">`)
			spanOpen = true
		}
	}

	for i < len(data) {
		if data[i] == 0x1b {
			i++
			if i >= len(data) {
				break
			}
			switch data[i] {
			case '[': // CSI
				i++
				paramStart := i
				for i < len(data) && data[i] >= 0x30 && data[i] <= 0x3F {
					i++
				}
				for i < len(data) && data[i] >= 0x20 && data[i] <= 0x2F {
					i++
				}
				if i >= len(data) {
					break
				}
				finalByte := data[i]
				i++

				if finalByte == 'm' { // SGR
					flushSpan()
					paramStr := string(data[paramStart : i-1])
					applySGR(&state, paramStr)
					openSpan()
				}
			case ']': // OSC -- strip
				i++
				for i < len(data) {
					if data[i] == 0x07 {
						i++
						break
					}
					if data[i] == 0x1b && i+1 < len(data) && data[i+1] == '\\' {
						i += 2
						break
					}
					i++
				}
			default:
				i++ // two-byte escape
			}
		} else {
			b.WriteString(html.EscapeString(string(data[i : i+1])))
			i++
		}
	}

	flushSpan()
	b.WriteString("</pre>")
	return b.String()
}

// applySGR updates the state based on a semicolon-separated SGR parameter string.
func applySGR(s *sgrState, params string) {
	if params == "" {
		s.reset()
		return
	}

	codes := strings.Split(params, ";")
	ci := 0
	for ci < len(codes) {
		n, err := strconv.Atoi(codes[ci])
		if err != nil {
			ci++
			continue
		}

		switch {
		case n == 0:
			s.reset()
		case n == 1:
			s.bold = true
		case n == 3:
			s.italic = true
		case n == 4:
			s.underline = true
		case n == 22:
			s.bold = false
		case n == 23:
			s.italic = false
		case n == 24:
			s.underline = false
		case n >= 30 && n <= 37:
			s.fg = basic16[n-30]
		case n == 38: // extended fg
			ci++
			if ci < len(codes) {
				mode, _ := strconv.Atoi(codes[ci])
				if mode == 5 && ci+1 < len(codes) { // 256 color
					ci++
					idx, _ := strconv.Atoi(codes[ci])
					s.fg = color256(idx)
				} else if mode == 2 && ci+3 < len(codes) { // truecolor
					r, _ := strconv.Atoi(codes[ci+1])
					g, _ := strconv.Atoi(codes[ci+2])
					bv, _ := strconv.Atoi(codes[ci+3])
					s.fg = fmt.Sprintf("rgb(%d,%d,%d)", r, g, bv)
					ci += 3
				}
			}
		case n == 39:
			s.fg = ""
		case n >= 40 && n <= 47:
			s.bg = basic16[n-40]
		case n == 48: // extended bg
			ci++
			if ci < len(codes) {
				mode, _ := strconv.Atoi(codes[ci])
				if mode == 5 && ci+1 < len(codes) { // 256 color
					ci++
					idx, _ := strconv.Atoi(codes[ci])
					s.bg = color256(idx)
				} else if mode == 2 && ci+3 < len(codes) { // truecolor
					r, _ := strconv.Atoi(codes[ci+1])
					g, _ := strconv.Atoi(codes[ci+2])
					bv, _ := strconv.Atoi(codes[ci+3])
					s.bg = fmt.Sprintf("rgb(%d,%d,%d)", r, g, bv)
					ci += 3
				}
			}
		case n == 49:
			s.bg = ""
		case n >= 90 && n <= 97:
			s.fg = basic16[n-90+8]
		case n >= 100 && n <= 107:
			s.bg = basic16[n-100+8]
		}

		ci++
	}
}
