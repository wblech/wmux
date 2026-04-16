package charmvt

import (
	"bytes"
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
)

// renderScrollback converts the scrollback buffer cells into ANSI byte output.
func renderScrollback(term *vt.Emulator, cols int) []byte {
	sbLen := term.ScrollbackLen()
	if sbLen == 0 {
		return nil
	}

	var buf bytes.Buffer
	// Pre-allocate: rough estimate of cols * lines bytes.
	buf.Grow(sbLen * cols)

	var prevStyle uv.Style

	for y := range sbLen {
		if y > 0 {
			buf.WriteString("\r\n")
		}

		// Reset style at the start of each line for robustness.
		if !prevStyle.IsZero() {
			buf.WriteString("\x1b[m")
			prevStyle = uv.Style{}
		}

		// Build the line content, tracking the last non-space column for trimming.
		var lineBuf bytes.Buffer
		trailingSpaces := 0

		for x := range cols {
			cell := term.ScrollbackCellAt(x, y)
			if cell == nil {
				trailingSpaces++
				continue
			}

			content := cell.Content
			if content == "" || content == " " {
				// Check if the cell has a non-default style (e.g., background color).
				if cell.Style.IsZero() {
					trailingSpaces++
					continue
				}
			}

			// Flush any accumulated spaces before this non-space cell.
			if trailingSpaces > 0 {
				lineBuf.WriteString(strings.Repeat(" ", trailingSpaces))
				trailingSpaces = 0
			}

			// Emit style change if needed.
			if !cell.Style.Equal(&prevStyle) {
				sgr := sgrForStyle(&cell.Style)
				lineBuf.WriteString(sgr)
				prevStyle = cell.Style
			}

			if content == "" {
				lineBuf.WriteByte(' ')
			} else {
				lineBuf.WriteString(content)
			}
		}

		// Write line content (trailing spaces are already trimmed by not flushing them).
		buf.Write(lineBuf.Bytes())
	}

	// Reset style at the end if needed.
	if !prevStyle.IsZero() {
		buf.WriteString("\x1b[m")
	}

	return buf.Bytes()
}

// sgrForStyle returns the ANSI SGR sequence to set the given style.
// Uses reset + reapply strategy for simplicity and correctness.
func sgrForStyle(s *uv.Style) string {
	if s.IsZero() {
		return "\x1b[m"
	}
	// Use the Style.String() method from ultraviolet which already
	// generates the proper ANSI SGR sequence.
	return s.String()
}
