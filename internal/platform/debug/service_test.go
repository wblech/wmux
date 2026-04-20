package debug

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTracer_NilIsDisabled(t *testing.T) {
	var tr *Tracer
	assert.False(t, tr.Enabled())
	assert.Equal(t, LevelOff, tr.Level())
	tr.Emit(Event{Stage: StagePtyRead}) //nolint:exhaustruct
}

func TestTracer_LevelOffIsDisabled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")
	tr, err := NewTracer(path, LevelOff)
	require.NoError(t, err)
	defer tr.Close() //nolint:errcheck
	assert.False(t, tr.Enabled())
}

func TestTracer_EmitWritesJSONLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")
	tr, err := NewTracer(path, LevelLifecycle)
	require.NoError(t, err)

	tr.Emit(Event{ //nolint:exhaustruct
		SessionID: "sess-1",
		Stage:     StageSessionCreate,
		Seq:       -1,
		Cols:      80,
		Rows:      24,
	})
	require.NoError(t, tr.Close())

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var record map[string]any
	require.NoError(t, json.Unmarshal(data, &record))

	assert.Equal(t, "session.create", record["msg"])
	assert.Equal(t, "sess-1", record["session_id"])
	assert.EqualValues(t, -1, record["seq"])
	assert.EqualValues(t, 80, record["cols"])
	assert.EqualValues(t, 24, record["rows"])
	assert.Contains(t, record, "time")
}

func TestTracer_LevelFiltersChunkFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")
	tr, err := NewTracer(path, LevelLifecycle)
	require.NoError(t, err)

	tr.Emit(Event{ //nolint:exhaustruct
		SessionID: "sess-1",
		Stage:     StagePtyRead,
		Seq:       1,
		ByteLen:   100,
		HeadHex:   "aabbccdd",
		Sha1:      "12345678",
	})
	require.NoError(t, tr.Close())

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var record map[string]any
	require.NoError(t, json.Unmarshal(data, &record))

	assert.NotContains(t, record, "byte_len")
	assert.NotContains(t, record, "sha1")
	assert.NotContains(t, record, "head_hex")
}

func TestTracer_LevelChunkIncludesChunkFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")
	tr, err := NewTracer(path, LevelChunk)
	require.NoError(t, err)

	tr.Emit(Event{ //nolint:exhaustruct
		SessionID: "sess-1",
		Stage:     StagePtyRead,
		Seq:       1,
		ByteLen:   100,
		HeadHex:   "aabbccdd",
		TailHex:   "eeff0011",
		Sha1:      "12345678",
		FullHex:   "should-not-appear",
	})
	require.NoError(t, tr.Close())

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var record map[string]any
	require.NoError(t, json.Unmarshal(data, &record))

	assert.EqualValues(t, 100, record["byte_len"])
	assert.Equal(t, "aabbccdd", record["head_hex"])
	assert.Equal(t, "eeff0011", record["tail_hex"])
	assert.Equal(t, "12345678", record["sha1"])
	assert.NotContains(t, record, "full_hex")
}

func TestTracer_LevelFullIncludesFullHex(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")
	tr, err := NewTracer(path, LevelFull)
	require.NoError(t, err)

	tr.Emit(Event{ //nolint:exhaustruct
		SessionID: "sess-1",
		Stage:     StagePtyRead,
		Seq:       1,
		ByteLen:   10,
		HeadHex:   "aa",
		TailHex:   "bb",
		Sha1:      "12345678",
		FullHex:   "aabbccddeeff",
	})
	require.NoError(t, tr.Close())

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var record map[string]any
	require.NoError(t, json.Unmarshal(data, &record))
	assert.Equal(t, "aabbccddeeff", record["full_hex"])
}

func TestTracer_OmitsZeroValueExtras(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")
	tr, err := NewTracer(path, LevelLifecycle)
	require.NoError(t, err)

	tr.Emit(Event{ //nolint:exhaustruct
		SessionID: "sess-1",
		Stage:     StageAttach,
		Seq:       -1,
	})
	require.NoError(t, tr.Close())

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var record map[string]any
	require.NoError(t, json.Unmarshal(data, &record))

	assert.NotContains(t, record, "buffer_size")
	assert.NotContains(t, record, "cols")
	assert.NotContains(t, record, "rows")
	assert.NotContains(t, record, "exit_code")
	assert.NotContains(t, record, "error")
	assert.NotContains(t, record, "paused")
}

func TestTracer_NextSeq_PerSession(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")
	tr, err := NewTracer(path, LevelChunk)
	require.NoError(t, err)
	defer tr.Close() //nolint:errcheck

	assert.Equal(t, int64(1), tr.NextSeq("a"))
	assert.Equal(t, int64(2), tr.NextSeq("a"))
	assert.Equal(t, int64(1), tr.NextSeq("b"))
	assert.Equal(t, int64(3), tr.NextSeq("a"))
}

func TestTracer_ResetSeq(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")
	tr, err := NewTracer(path, LevelChunk)
	require.NoError(t, err)
	defer tr.Close() //nolint:errcheck

	tr.NextSeq("a")
	tr.NextSeq("a")
	tr.ResetSeq("a")
	assert.Equal(t, int64(1), tr.NextSeq("a"))
}

func TestTracer_MultipleEvents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")
	tr, err := NewTracer(path, LevelLifecycle)
	require.NoError(t, err)

	tr.Emit(Event{SessionID: "s1", Stage: StageSessionCreate, Seq: -1}) //nolint:exhaustruct
	tr.Emit(Event{SessionID: "s1", Stage: StageAttach, Seq: -1})        //nolint:exhaustruct
	tr.Emit(Event{SessionID: "s1", Stage: StageDetach, Seq: -1})        //nolint:exhaustruct
	require.NoError(t, tr.Close())

	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close() //nolint:errcheck

	scanner := bufio.NewScanner(f)
	var lines int
	for scanner.Scan() {
		lines++
	}
	assert.Equal(t, 3, lines)
}

func TestTracer_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b", "c", "test.log")
	tr, err := NewTracer(nested, LevelLifecycle)
	require.NoError(t, err)

	tr.Emit(Event{SessionID: "s1", Stage: StageSessionCreate, Seq: -1}) //nolint:exhaustruct
	require.NoError(t, tr.Close())

	_, err = os.Stat(nested)
	assert.NoError(t, err)
}
