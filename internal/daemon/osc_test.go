package daemon

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseOSC7(t *testing.T) {
	data := []byte("\x1b]7;file://localhost/home/user/project\x1b\\")
	result := ParseOSC(data)
	assert.Len(t, result, 1)
	assert.Equal(t, OSCTypeCwd, result[0].Type)
	assert.Equal(t, "/home/user/project", result[0].Value)
}

func TestParseOSC9(t *testing.T) {
	data := []byte("\x1b]9;Build complete\x1b\\")
	result := ParseOSC(data)
	assert.Len(t, result, 1)
	assert.Equal(t, OSCTypeNotification, result[0].Type)
	assert.Equal(t, "Build complete", result[0].Value)
}

func TestParseOSC99(t *testing.T) {
	data := []byte("\x1b]99;d=0:p=body;Test done\x1b\\")
	result := ParseOSC(data)
	assert.Len(t, result, 1)
	assert.Equal(t, OSCTypeNotification, result[0].Type)
	assert.Equal(t, "Test done", result[0].Value)
}

func TestParseOSC777(t *testing.T) {
	data := []byte("\x1b]777;notify;Title;Body text\x1b\\")
	result := ParseOSC(data)
	assert.Len(t, result, 1)
	assert.Equal(t, OSCTypeNotification, result[0].Type)
	assert.Equal(t, "Body text", result[0].Value)
}

func TestParseOSC_NoOSC(t *testing.T) {
	data := []byte("just plain text with no escape sequences")
	result := ParseOSC(data)
	assert.Empty(t, result)
}

func TestParseOSC_Multiple(t *testing.T) {
	data := []byte("\x1b]7;file:///tmp\x1b\\\x1b]9;Done\x1b\\")
	result := ParseOSC(data)
	assert.Len(t, result, 2)
}

func TestParseOSC_BELTerminator(t *testing.T) {
	data := []byte("\x1b]9;Alert\x07")
	result := ParseOSC(data)
	assert.Len(t, result, 1)
	assert.Equal(t, "Alert", result[0].Value)
}
