package e2e

import (
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wblech/wmux/pkg/client"
)

// TestE2E_OSC133_BashRealCommand spawns bash with PS1/PROMPT_COMMAND/trap
// that emit OSC 133 markers, runs `true`, and verifies the daemon publishes
// command.started and command.finished events.
//
// The script includes a short sleep before exit so the batcher has time to
// flush the OSC output before the session exits and the batcher is stopped.
//
// Skipped when bash is not available.
func TestE2E_OSC133_BashRealCommand(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	env := startTestDaemon(t)
	c := connectTestClient(t, env)

	started := make(chan client.Event, 4)
	finished := make(chan client.Event, 4)
	c.OnEvent(func(evt client.Event) {
		switch evt.Type {
		case "command.started":
			select {
			case started <- evt:
			default:
			}
		case "command.finished":
			select {
			case finished <- evt:
			default:
			}
		}
	})

	// The script configures bash shell integration markers, runs `true`, then
	// sleeps briefly so the PTY output batcher flushes before the process exits.
	// PROMPT_COMMAND emits OSC 133;D (command finish) after each command.
	// trap on DEBUG emits OSC 133;C (command start) before each command runs.
	// PS1 emits OSC 133;A (prompt start) and OSC 133;B (user input start).
	bashScript := `PROMPT_COMMAND='__last=$?; printf "\x1b]133;D;%s\x07" "$__last"'
PS1='\[\x1b]133;A\x07\]$ \[\x1b]133;B\x07\]'
trap 'printf "\x1b]133;C\x07"' DEBUG
true
sleep 0.1
exit 0
`

	_, err := c.Create("osc133-sess", client.CreateParams{
		Shell: "/bin/bash",
		Args:  []string{"--norc", "--noprofile", "-i", "-c", bashScript},
		Cols:  80,
		Rows:  24,
	})
	require.NoError(t, err)

	// Attach so the daemon's broadcast loop processes PTY output (and thus
	// runs scanOSC on the session's output stream).
	_, err = c.Attach("osc133-sess")
	require.NoError(t, err)

	// Wait for at least one command.started and one command.finished.
	deadline := time.After(5 * time.Second)
	gotStart, gotFinish := false, false
	for !gotStart || !gotFinish {
		select {
		case ev := <-started:
			assert.Equal(t, "osc133-sess", ev.SessionID)
			gotStart = true
		case ev := <-finished:
			assert.Equal(t, "osc133-sess", ev.SessionID)
			gotFinish = true
		case <-deadline:
			t.Fatalf("timed out: gotStart=%v gotFinish=%v", gotStart, gotFinish)
		}
	}
}

// TestE2E_OSC133_DirectInjection feeds OSC 133 escape sequences directly
// via Write to a long-running /bin/cat process. cat echoes input back on
// its stdout, so the daemon's PTY-output scan path sees the OSC 133
// sequences. Faster and more deterministic than the bash-driven test.
//
// The PTY line discipline operates in canonical (cooked) mode when no shell
// has configured raw mode. In canonical mode, input is buffered until a
// newline, so the injection includes a trailing newline to flush the
// canonical buffer and cause cat to re-output the raw escape sequences.
func TestE2E_OSC133_DirectInjection(t *testing.T) {
	env := startTestDaemon(t)
	c := connectTestClient(t, env)

	finished := make(chan client.Event, 4)
	c.OnEvent(func(evt client.Event) {
		if evt.Type == "command.finished" {
			select {
			case finished <- evt:
			default:
			}
		}
	})

	// Use /bin/cat as a long-running process we can feed data to via Write.
	// cat echoes input back to the PTY output, so the daemon's broadcast scan
	// path will see the OSC 133 sequences in the output stream.
	_, err := c.Create("osc133-direct", client.CreateParams{
		Shell: "/bin/cat",
		Cols:  80,
		Rows:  24,
	})
	require.NoError(t, err)

	// Attach so the daemon's broadcast loop processes PTY output (and thus
	// runs scanOSC on the session's output stream).
	_, err = c.Attach("osc133-direct")
	require.NoError(t, err)

	// Small delay to let the session start before injecting data.
	time.Sleep(50 * time.Millisecond)

	// Inject the A→C→D sequence followed by a newline to flush the PTY's
	// canonical line buffer. cat will echo the raw bytes (including ESC
	// sequences) back to stdout; the daemon's scanOSC picks them up from
	// that stdout stream. Exit code 7 is encoded in the D sequence.
	err = c.Write("osc133-direct", []byte("\x1b]133;A\x07\x1b]133;C\x07\x1b]133;D;7\x07\n"))
	require.NoError(t, err)

	select {
	case ev := <-finished:
		assert.Equal(t, "osc133-direct", ev.SessionID)
		exit := payloadInt(ev.Data, "exit_code")
		assert.Equal(t, int64(7), exit, "exit_code should be 7")
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for command.finished")
	}
}

// payloadInt extracts an integer from an event Data map, handling both
// JSON-decoded float64 and direct int/int64 values.
func payloadInt(p map[string]any, key string) int64 {
	v, ok := p[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	default:
		return 0
	}
}
