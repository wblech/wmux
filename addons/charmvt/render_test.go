package charmvt

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRenderScrollback_Empty verifies that a fresh emulator (no overflow) produces a replay
// with only the clear prefix and cursor-restore CUP — no scrollback content.
func TestRenderScrollback_Empty(t *testing.T) {
	cfg := defaultConfig()
	em := newEmulator("render-empty", 80, 24, cfg)

	snap := em.Snapshot()
	require.True(t, strings.HasPrefix(string(snap.Replay), "\x1b[2J\x1b[H\x1b[3J"),
		"replay must begin with clear prefix")
	tail := string(snap.Replay[len("\x1b[2J\x1b[H\x1b[3J"):])
	assert.Regexp(t, `^\x1b\[\d+;\d+H$`, tail,
		"empty emulator replay should contain only the clear prefix and final CUP")
}

// TestRenderScrollback_PlainText verifies that plain-text lines pushed past a 3-row viewport appear
// in scrollback.
func TestRenderScrollback_PlainText(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000

	// 3-row viewport: writing 10 lines forces the first several into scrollback.
	em := newEmulator("render-plain", 80, 3, cfg)

	var lines []string
	for i := range 10 {
		lines = append(lines, fmt.Sprintf("plain line %02d", i))
	}
	em.Process([]byte(strings.Join(lines, "\r\n")))

	snap := em.Snapshot()
	require.NotNil(t, snap.Replay, "scrollback must not be nil after overflow")

	sb := string(snap.Replay)

	// At least one of the early lines must appear in scrollback.
	found := false
	for i := range 7 {
		if strings.Contains(sb, fmt.Sprintf("plain line %02d", i)) {
			found = true
			break
		}
	}
	assert.True(t, found, "scrollback should contain text from early plain lines")
}

// TestRenderScrollback_ColoredText verifies that red-colored lines in scrollback contain both SGR
// sequences and the original text.
func TestRenderScrollback_ColoredText(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000

	em := newEmulator("render-colored", 80, 3, cfg)

	for i := range 10 {
		em.Process([]byte(fmt.Sprintf("\033[31mred line %02d\033[0m\r\n", i)))
	}

	snap := em.Snapshot()
	require.NotNil(t, snap.Replay, "scrollback must not be nil after overflow")

	sb := string(snap.Replay)

	// SGR sequence must be present.
	assert.Contains(t, sb, "\033[")

	// At least one early line's text must appear.
	found := false
	for i := range 7 {
		if strings.Contains(sb, fmt.Sprintf("red line %02d", i)) {
			found = true
			break
		}
	}
	assert.True(t, found, "scrollback should contain colored line text")
}

// TestRenderScrollback_BoldAndColor verifies that bold+green styled lines appear in scrollback.
func TestRenderScrollback_BoldAndColor(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000

	em := newEmulator("render-bold-color", 80, 3, cfg)

	for i := range 10 {
		em.Process([]byte(fmt.Sprintf("\033[1;32mbold green %02d\033[0m\r\n", i)))
	}

	snap := em.Snapshot()
	require.NotNil(t, snap.Replay, "scrollback must not be nil after overflow")

	sb := string(snap.Replay)

	// At least one early line's text must appear in scrollback.
	found := false
	for i := range 7 {
		if strings.Contains(sb, fmt.Sprintf("bold green %02d", i)) {
			found = true
			break
		}
	}
	assert.True(t, found, "scrollback should contain bold+green line text")
}

// TestRenderScrollback_TrailingSpacesTrimmed verifies that scrollback lines do not contain trailing
// spaces when short text is written to a wide terminal.
func TestRenderScrollback_TrailingSpacesTrimmed(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000

	// Wide terminal (160 cols), 3-row viewport.
	em := newEmulator("render-trim", 160, 3, cfg)

	for i := range 10 {
		// Short content — far less than 160 columns.
		em.Process([]byte(fmt.Sprintf("short %02d\r\n", i)))
	}

	snap := em.Snapshot()
	require.NotNil(t, snap.Replay, "scrollback must not be nil after overflow")

	sb := string(snap.Replay)
	lines := strings.Split(sb, "\r\n")

	for _, line := range lines {
		// Strip any ANSI sequences before checking trailing spaces so that reset
		// sequences at end of line are not counted as trailing spaces.
		plain := stripANSI(line)
		assert.Equal(t, strings.TrimRight(plain, " "), plain,
			"scrollback line should have no trailing spaces: %q", plain)
	}
}

// TestRenderScrollback_TerminalLineEndings verifies that scrollback uses \r\n line endings.
func TestRenderScrollback_TerminalLineEndings(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000

	em := newEmulator("render-crlf", 80, 3, cfg)

	for i := range 10 {
		em.Process([]byte(fmt.Sprintf("line %02d\r\n", i)))
	}

	snap := em.Snapshot()
	require.NotNil(t, snap.Replay, "scrollback must not be nil after overflow")

	sb := string(snap.Replay)

	// Must contain \r\n between lines.
	assert.Contains(t, sb, "\r\n", "scrollback should use \\r\\n line endings")

	// Must not contain bare \n (not preceded by \r).
	cleaned := strings.ReplaceAll(sb, "\r\n", "")
	assert.NotContains(t, cleaned, "\n",
		"scrollback should not contain bare \\n outside of \\r\\n pairs")
}

// stripANSI removes ANSI escape sequences from a string for comparison purposes.
func stripANSI(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\033' && i+1 < len(s) && s[i+1] == '[' {
			// Skip until the final byte of the CSI sequence (letter).
			i += 2
			for i < len(s) && (s[i] < 0x40 || s[i] > 0x7E) {
				i++
			}
			i++ // skip the final byte
			continue
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}
