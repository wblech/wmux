package ansi

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStrip_PlainText(t *testing.T) {
	assert.Equal(t, "hello world", Strip([]byte("hello world")))
}

func TestStrip_SGR(t *testing.T) {
	input := []byte("\x1b[1;31mhello\x1b[0m")
	assert.Equal(t, "hello", Strip(input))
}

func TestStrip_CSI_CursorMovement(t *testing.T) {
	input := []byte("before\x1b[3Aafter")
	assert.Equal(t, "beforeafter", Strip(input))
}

func TestStrip_OSC(t *testing.T) {
	input := []byte("text\x1b]0;my-title\x07more")
	assert.Equal(t, "textmore", Strip(input))
}

func TestStrip_OSC_ST(t *testing.T) {
	input := []byte("text\x1b]0;title\x1b\\more")
	assert.Equal(t, "textmore", Strip(input))
}

func TestStrip_MultipleSGR(t *testing.T) {
	input := []byte("\x1b[32mgreen\x1b[0m \x1b[1;4munderline\x1b[0m")
	assert.Equal(t, "green underline", Strip(input))
}

func TestStrip_Empty(t *testing.T) {
	assert.Empty(t, Strip(nil))
	assert.Empty(t, Strip([]byte{}))
}

func TestStrip_OnlyEscapes(t *testing.T) {
	input := []byte("\x1b[31m\x1b[0m")
	assert.Empty(t, Strip(input))
}

func TestStrip_256Color(t *testing.T) {
	input := []byte("\x1b[38;5;196mred\x1b[0m")
	assert.Equal(t, "red", Strip(input))
}

func TestStrip_TrueColor(t *testing.T) {
	input := []byte("\x1b[38;2;255;0;0mred\x1b[0m")
	assert.Equal(t, "red", Strip(input))
}
