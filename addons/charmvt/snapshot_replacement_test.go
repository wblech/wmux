package charmvt

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSnapshot_FullReplacement(t *testing.T) {
	cfg := defaultConfig()
	src := newEmulator("src", 80, 24, cfg)
	src.Process([]byte("$ cd /project\r\n"))
	src.Process([]byte("$ claude\r\n"))
	src.Process([]byte("\x1b[2J\x1b[H"))
	src.Process([]byte("\x1b[2;12HBANNER: Claude Code v2.1.112\r\n"))

	snap := src.Snapshot()
	require.NotEmpty(t, snap.Replay)

	dst := newEmulator("dst", 80, 24, cfg)
	dst.Process([]byte("OLD CONTENT FROM PREVIOUS TAB\r\n"))
	dst.Process([]byte("\x1b[10;1HSTALE ARTIFACT\r\n"))

	dst.Process(snap.Replay)

	replayed := string(dst.Snapshot().Replay)
	assert.NotContains(t, replayed, "OLD CONTENT FROM PREVIOUS TAB",
		"replay must erase prior destination state")
	assert.NotContains(t, replayed, "STALE ARTIFACT",
		"replay must clear prior destination cells")
	assert.Contains(t, replayed, "BANNER: Claude Code")
}

func TestSnapshot_Idempotent(t *testing.T) {
	cfg := defaultConfig()
	src := newEmulator("src", 80, 24, cfg)
	for i := 0; i < 30; i++ {
		src.Process([]byte("line\r\n"))
	}
	src.Process([]byte("\x1b[2J\x1b[H"))
	src.Process([]byte("FINAL STATE"))

	snap1 := src.Snapshot()

	dst := newEmulator("dst", 80, 24, cfg)
	dst.Process(snap1.Replay)
	snap2 := dst.Snapshot()

	assert.Equal(t, string(snap1.Replay), string(snap2.Replay),
		"snapshot must be idempotent under replay")
	assert.Contains(t, string(snap2.Replay), "FINAL STATE")
}
