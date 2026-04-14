package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseGlobalFlags_SubcommandOnly(t *testing.T) {
	// Reset globals.
	socketPath = "~/.wmux/daemon.sock"
	tokenPath = "~/.wmux/daemon.token"

	cmd, args := parseGlobalFlags([]string{"list"})
	assert.Equal(t, "list", cmd)
	assert.Empty(t, args)
}

func TestParseGlobalFlags_WithSocket(t *testing.T) {
	socketPath = "~/.wmux/daemon.sock"
	tokenPath = "~/.wmux/daemon.token"

	cmd, args := parseGlobalFlags([]string{"--socket", "/tmp/test.sock", "status"})
	assert.Equal(t, "status", cmd)
	assert.Empty(t, args)
	assert.Equal(t, "/tmp/test.sock", socketPath)
	// Token path derived from socket path.
	assert.Equal(t, "/tmp/test.token", tokenPath)
}

func TestParseGlobalFlags_WithSocketAndToken(t *testing.T) {
	socketPath = "~/.wmux/daemon.sock"
	tokenPath = "~/.wmux/daemon.token"

	cmd, args := parseGlobalFlags([]string{"--socket", "/tmp/s.sock", "--token", "/tmp/t.token", "info", "sess1"})
	assert.Equal(t, "info", cmd)
	assert.Equal(t, []string{"sess1"}, args)
	assert.Equal(t, "/tmp/s.sock", socketPath)
	assert.Equal(t, "/tmp/t.token", tokenPath)
}

func TestParseGlobalFlags_Empty(t *testing.T) {
	socketPath = "~/.wmux/daemon.sock"
	tokenPath = "~/.wmux/daemon.token"

	cmd, args := parseGlobalFlags([]string{})
	assert.Empty(t, cmd)
	assert.Nil(t, args)
}

func TestParseGlobalFlags_SubcommandWithArgs(t *testing.T) {
	socketPath = "~/.wmux/daemon.sock"
	tokenPath = "~/.wmux/daemon.token"

	cmd, args := parseGlobalFlags([]string{"create", "my-session", "--shell", "/bin/zsh"})
	assert.Equal(t, "create", cmd)
	assert.Equal(t, []string{"my-session", "--shell", "/bin/zsh"}, args)
}

func TestPrintUsage_DoesNotPanic(t *testing.T) {
	assert.NotPanics(t, func() {
		printUsage()
	})
}

func TestPrintExecUsage_DoesNotPanic(t *testing.T) {
	assert.NotPanics(t, func() {
		printExecUsage()
	})
}

func TestCmdCreate_NoArgs(t *testing.T) {
	code := cmdCreate([]string{})
	assert.Equal(t, 1, code)
}

func TestCmdAttach_NoArgs(t *testing.T) {
	code := cmdAttach([]string{})
	assert.Equal(t, 1, code)
}

func TestCmdDetach_NoArgs(t *testing.T) {
	code := cmdDetach([]string{})
	assert.Equal(t, 1, code)
}

func TestCmdKill_NoArgs(t *testing.T) {
	code := cmdKill([]string{})
	assert.Equal(t, 1, code)
}

func TestCmdInfo_NoArgs(t *testing.T) {
	code := cmdInfo([]string{})
	assert.Equal(t, 1, code)
}

func TestCmdWait_NoArgs(t *testing.T) {
	code := cmdWait([]string{})
	assert.Equal(t, 1, code)
}

func TestCmdWait_OnlySessionID(t *testing.T) {
	code := cmdWait([]string{"sess1"})
	assert.Equal(t, 1, code)
}

func TestCmdRecord_NoArgs(t *testing.T) {
	code := cmdRecord([]string{})
	assert.Equal(t, 1, code)
}

func TestCmdRecord_OnlyAction(t *testing.T) {
	code := cmdRecord([]string{"start"})
	assert.Equal(t, 1, code)
}

func TestCmdHistory_NoArgs(t *testing.T) {
	code := cmdHistory([]string{})
	assert.Equal(t, 1, code)
}

func TestCmdExec_NoArgs(t *testing.T) {
	code := cmdExec([]string{})
	assert.Equal(t, 1, code)
}
