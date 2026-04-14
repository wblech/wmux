package ansi

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToHTML_PlainText(t *testing.T) {
	result := ToHTML([]byte("hello world"))
	assert.Contains(t, result, "hello world")
	assert.Contains(t, result, "<pre")
}

func TestToHTML_Bold(t *testing.T) {
	input := []byte("\x1b[1mhello\x1b[0m")
	result := ToHTML(input)
	assert.Contains(t, result, "font-weight:bold")
	assert.Contains(t, result, "hello")
}

func TestToHTML_FGColor_Basic(t *testing.T) {
	input := []byte("\x1b[31mred text\x1b[0m")
	result := ToHTML(input)
	assert.Contains(t, result, "color:")
	assert.Contains(t, result, "red text")
}

func TestToHTML_256Color(t *testing.T) {
	input := []byte("\x1b[38;5;196mred\x1b[0m")
	result := ToHTML(input)
	assert.Contains(t, result, "color:")
	assert.Contains(t, result, "red")
}

func TestToHTML_TrueColor(t *testing.T) {
	input := []byte("\x1b[38;2;255;128;0morange\x1b[0m")
	result := ToHTML(input)
	assert.Contains(t, result, "rgb(255,128,0)")
	assert.Contains(t, result, "orange")
}

func TestToHTML_BGColor(t *testing.T) {
	input := []byte("\x1b[41mhighlighted\x1b[0m")
	result := ToHTML(input)
	assert.Contains(t, result, "background-color:")
	assert.Contains(t, result, "highlighted")
}

func TestToHTML_Italic(t *testing.T) {
	input := []byte("\x1b[3mitalic\x1b[0m")
	result := ToHTML(input)
	assert.Contains(t, result, "font-style:italic")
	assert.Contains(t, result, "italic")
}

func TestToHTML_Underline(t *testing.T) {
	input := []byte("\x1b[4munderlined\x1b[0m")
	result := ToHTML(input)
	assert.Contains(t, result, "text-decoration:underline")
	assert.Contains(t, result, "underlined")
}

func TestToHTML_Combined(t *testing.T) {
	input := []byte("\x1b[1;31mbold red\x1b[0m")
	result := ToHTML(input)
	assert.Contains(t, result, "font-weight:bold")
	assert.Contains(t, result, "color:")
	assert.Contains(t, result, "bold red")
}

func TestToHTML_Empty(t *testing.T) {
	result := ToHTML(nil)
	assert.Contains(t, result, "<pre")
	assert.Contains(t, result, "</pre>")
}

func TestToHTML_HTMLEscaping(t *testing.T) {
	input := []byte("<script>alert('xss')</script>")
	result := ToHTML(input)
	assert.NotContains(t, result, "<script>")
	assert.Contains(t, result, "&lt;script&gt;")
}

func TestToHTML_ResetMidStream(t *testing.T) {
	input := []byte("\x1b[31mred\x1b[0m plain \x1b[32mgreen\x1b[0m")
	result := ToHTML(input)
	require.True(t, strings.Contains(result, "red"))
	require.True(t, strings.Contains(result, "plain"))
	require.True(t, strings.Contains(result, "green"))
}

func TestToHTML_BG256Color(t *testing.T) {
	input := []byte("\x1b[48;5;21mblue bg\x1b[0m")
	result := ToHTML(input)
	assert.Contains(t, result, "background-color:")
	assert.Contains(t, result, "blue bg")
}

func TestToHTML_TrueColorBG(t *testing.T) {
	input := []byte("\x1b[48;2;0;255;0mgreen bg\x1b[0m")
	result := ToHTML(input)
	assert.Contains(t, result, "rgb(0,255,0)")
	assert.Contains(t, result, "green bg")
}
