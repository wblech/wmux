package charmvt

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Unicode evidence tests: isolate where Nerd Font/emoji corruption occurs.
// Each test checks viewport (charmbracelet Render) and scrollback (our renderScrollback)
// independently to pinpoint the failing layer.

// --- Viewport tests (charmbracelet's Render path) ---

func TestViewport_NerdFont_GitBranch(t *testing.T) {
	cfg := defaultConfig()
	em := newEmulator("unicode-vp-nerdfont", 80, 24, cfg)

	// U+E0A0 = Nerd Font git branch icon, UTF-8: 0xEE 0x82 0xA0
	icon := "\uE0A0"
	em.Process([]byte("branch: " + icon + " main"))

	snap := em.Snapshot()
	vp := string(snap.Viewport)

	assert.Contains(t, vp, icon,
		"viewport should contain Nerd Font git branch icon U+E0A0")
	// Verify the icon is present as a proper UTF-8 sequence, not individual bytes.
	// U+E0A0 in UTF-8 = 0xEE 0x82 0xA0 (3 bytes forming one rune).
	assert.True(t, strings.ContainsRune(vp, '\uE0A0'),
		"viewport should contain U+E0A0 as a valid rune")
}

func TestViewport_NerdFont_DevIcons(t *testing.T) {
	cfg := defaultConfig()
	em := newEmulator("unicode-vp-devicons", 80, 24, cfg)

	icons := map[string]string{
		"folder":   "\uF07B", // U+F07B
		"git":      "\uE0A0", // U+E0A0
		"golang":   "\uE626", // U+E626
		"python":   "\uE73C", // U+E73C
		"docker":   "\uF308", // U+F308
		"readonly": "\uF023", // U+F023 (lock icon)
	}

	for name, icon := range icons {
		em.Process([]byte(fmt.Sprintf("%s=%s ", name, icon)))
	}

	snap := em.Snapshot()
	vp := string(snap.Viewport)

	for name, icon := range icons {
		assert.Contains(t, vp, icon,
			"viewport should contain %s icon U+%04X", name, []rune(icon)[0])
	}
}

func TestViewport_Emoji(t *testing.T) {
	cfg := defaultConfig()
	em := newEmulator("unicode-vp-emoji", 80, 24, cfg)

	// Regular emoji that reportedly work fine
	em.Process([]byte("crab: 🦀 rocket: 🚀 check: ✅"))

	snap := em.Snapshot()
	vp := string(snap.Viewport)

	assert.Contains(t, vp, "🦀", "viewport should contain crab emoji")
	assert.Contains(t, vp, "🚀", "viewport should contain rocket emoji")
	assert.Contains(t, vp, "✅", "viewport should contain check emoji")
}

func TestViewport_MixedUnicode(t *testing.T) {
	cfg := defaultConfig()
	em := newEmulator("unicode-vp-mixed", 80, 24, cfg)

	// Simulates a starship-like prompt with Nerd Font + regular text
	prompt := "\uE0A0 main \uF07B ~/code 🦀 v1.75"
	em.Process([]byte(prompt))

	snap := em.Snapshot()
	vp := string(snap.Viewport)

	assert.Contains(t, vp, "\uE0A0", "viewport should contain git branch icon")
	assert.Contains(t, vp, "\uF07B", "viewport should contain folder icon")
	assert.Contains(t, vp, "🦀", "viewport should contain crab emoji")
	assert.Contains(t, vp, "main", "viewport should contain plain text")
	assert.Contains(t, vp, "v1.75", "viewport should contain version text")
}

// --- Scrollback tests (our renderScrollback path) ---

func TestScrollback_NerdFont_GitBranch(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000

	// Small viewport to push content into scrollback quickly
	em := newEmulator("unicode-sb-nerdfont", 80, 3, cfg)

	icon := "\uE0A0"
	// Write enough lines to push the icon line into scrollback
	em.Process([]byte("branch: " + icon + " main\r\n"))
	for i := range 10 {
		em.Process([]byte(fmt.Sprintf("filler line %02d\r\n", i)))
	}

	snap := em.Snapshot()
	require.NotNil(t, snap.Scrollback, "scrollback should not be nil after overflow")
	sb := string(snap.Scrollback)

	assert.Contains(t, sb, icon,
		"scrollback should contain Nerd Font git branch icon U+E0A0")
	assert.NotContains(t, sb, "î",
		"scrollback should not contain î (Latin-1 corruption of 0xEE)")
}

func TestScrollback_NerdFont_DevIcons(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000
	em := newEmulator("unicode-sb-devicons", 80, 3, cfg)

	icons := map[string]string{
		"folder": "\uF07B",
		"git":    "\uE0A0",
		"golang": "\uE626",
		"python": "\uE73C",
	}

	for name, icon := range icons {
		em.Process([]byte(fmt.Sprintf("%s=%s\r\n", name, icon)))
	}
	// Push all icon lines into scrollback
	for i := range 10 {
		em.Process([]byte(fmt.Sprintf("filler %02d\r\n", i)))
	}

	snap := em.Snapshot()
	require.NotNil(t, snap.Scrollback)
	sb := string(snap.Scrollback)

	for name, icon := range icons {
		assert.Contains(t, sb, icon,
			"scrollback should contain %s icon U+%04X", name, []rune(icon)[0])
	}
}

func TestScrollback_Emoji(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000
	em := newEmulator("unicode-sb-emoji", 80, 3, cfg)

	em.Process([]byte("crab: 🦀 rocket: 🚀\r\n"))
	for i := range 10 {
		em.Process([]byte(fmt.Sprintf("filler %02d\r\n", i)))
	}

	snap := em.Snapshot()
	require.NotNil(t, snap.Scrollback)
	sb := string(snap.Scrollback)

	assert.Contains(t, sb, "🦀", "scrollback should contain crab emoji")
	assert.Contains(t, sb, "🚀", "scrollback should contain rocket emoji")
}

func TestScrollback_MixedUnicode(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000
	em := newEmulator("unicode-sb-mixed", 80, 3, cfg)

	prompt := "\uE0A0 main \uF07B ~/code 🦀 v1.75"
	em.Process([]byte(prompt + "\r\n"))
	for i := range 10 {
		em.Process([]byte(fmt.Sprintf("filler %02d\r\n", i)))
	}

	snap := em.Snapshot()
	require.NotNil(t, snap.Scrollback)
	sb := string(snap.Scrollback)

	assert.Contains(t, sb, "\uE0A0", "scrollback should contain git branch icon")
	assert.Contains(t, sb, "\uF07B", "scrollback should contain folder icon")
	assert.Contains(t, sb, "🦀", "scrollback should contain crab emoji")
	assert.Contains(t, sb, "main", "scrollback should contain plain text")
}

// --- Byte-level evidence test ---

func TestViewport_NerdFont_ByteLevel(t *testing.T) {
	cfg := defaultConfig()
	em := newEmulator("unicode-vp-bytes", 80, 24, cfg)

	// U+E0A0 in UTF-8 is: 0xEE 0x82 0xA0
	icon := "\uE0A0"
	em.Process([]byte("X" + icon + "Y"))

	snap := em.Snapshot()
	vp := snap.Viewport

	// Find "X" in the viewport bytes
	xIdx := -1
	for i, b := range vp {
		if b == 'X' {
			xIdx = i
			break
		}
	}
	require.NotEqual(t, -1, xIdx, "should find X in viewport")

	// After X, the next 3 bytes should be the UTF-8 encoding of U+E0A0
	require.Greater(t, len(vp), xIdx+4, "viewport should have enough bytes after X")

	t.Logf("Bytes after X: % 02X", vp[xIdx+1:xIdx+6])
	t.Logf("Expected UTF-8 for U+E0A0: EE 82 A0")

	assert.Equal(t, byte(0xEE), vp[xIdx+1], "first byte of U+E0A0 should be 0xEE")
	assert.Equal(t, byte(0x82), vp[xIdx+2], "second byte of U+E0A0 should be 0x82")
	assert.Equal(t, byte(0xA0), vp[xIdx+3], "third byte of U+E0A0 should be 0xA0")
	assert.Equal(t, byte('Y'), vp[xIdx+4], "Y should follow immediately after the icon")
}

func TestScrollback_NerdFont_ByteLevel(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000
	em := newEmulator("unicode-sb-bytes", 80, 3, cfg)

	icon := "\uE0A0"
	em.Process([]byte("X" + icon + "Y\r\n"))
	for i := range 10 {
		em.Process([]byte(fmt.Sprintf("filler %02d\r\n", i)))
	}

	snap := em.Snapshot()
	require.NotNil(t, snap.Scrollback)
	sb := snap.Scrollback

	// Find "X" in the scrollback bytes
	xIdx := -1
	for i, b := range sb {
		if b == 'X' {
			xIdx = i
			break
		}
	}
	require.NotEqual(t, -1, xIdx, "should find X in scrollback")

	require.Greater(t, len(sb), xIdx+4, "scrollback should have enough bytes after X")

	t.Logf("Bytes after X: % 02X", sb[xIdx+1:xIdx+6])
	t.Logf("Expected UTF-8 for U+E0A0: EE 82 A0")

	assert.Equal(t, byte(0xEE), sb[xIdx+1], "first byte of U+E0A0 should be 0xEE")
	assert.Equal(t, byte(0x82), sb[xIdx+2], "second byte of U+E0A0 should be 0x82")
	assert.Equal(t, byte(0xA0), sb[xIdx+3], "third byte of U+E0A0 should be 0xA0")
	assert.Equal(t, byte('Y'), sb[xIdx+4], "Y should follow immediately after the icon")
}

// --- Width handling evidence ---

func TestViewport_WideChar_NoExtraSpaces(t *testing.T) {
	cfg := defaultConfig()
	em := newEmulator("unicode-vp-width", 80, 24, cfg)

	// Wide chars occupy 2 columns. Verify no phantom spaces from placeholders.
	em.Process([]byte("A\uE0A0B"))

	snap := em.Snapshot()
	vp := string(snap.Viewport)

	// The viewport should contain A, the icon, and B without extra spaces between them.
	// If placeholder cells leak as spaces, we'd see "A B" instead of "A\uE0A0B".
	idx := strings.Index(vp, "A")
	require.NotEqual(t, -1, idx)

	// Extract the segment between A and B
	bIdx := strings.Index(vp[idx+1:], "B")
	require.NotEqual(t, -1, bIdx, "should find B after A")

	segment := vp[idx+1 : idx+1+bIdx]
	t.Logf("Segment between A and B: %q (bytes: % 02X)", segment, []byte(segment))

	// Should contain the icon, not spaces
	assert.Contains(t, segment, "\uE0A0",
		"segment between A and B should contain the icon, not spaces")
	assert.NotEqual(t, " ", segment,
		"segment should not be just a space (placeholder leak)")
}

func TestScrollback_WideChar_NoExtraSpaces(t *testing.T) {
	cfg := defaultConfig()
	cfg.scrollback = 1000
	em := newEmulator("unicode-sb-width", 80, 3, cfg)

	em.Process([]byte("A\uE0A0B\r\n"))
	for i := range 10 {
		em.Process([]byte(fmt.Sprintf("filler %02d\r\n", i)))
	}

	snap := em.Snapshot()
	require.NotNil(t, snap.Scrollback)
	sb := string(snap.Scrollback)

	idx := strings.Index(sb, "A")
	require.NotEqual(t, -1, idx)

	bIdx := strings.Index(sb[idx+1:], "B")
	require.NotEqual(t, -1, bIdx, "should find B after A in scrollback")

	segment := sb[idx+1 : idx+1+bIdx]
	t.Logf("Segment between A and B: %q (bytes: % 02X)", segment, []byte(segment))

	assert.Contains(t, segment, "\uE0A0",
		"scrollback segment between A and B should contain the icon")
	// Check no extra space from placeholder cell
	assert.NotContains(t, segment, " ",
		"scrollback should not have space from placeholder cell between A and icon")
}
