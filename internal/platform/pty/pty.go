// Package pty provides a PTY spawner for creating pseudo-terminal processes.
package pty

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	creackpty "github.com/creack/pty"
)

// SpawnOptions holds the configuration for spawning a new PTY process.
type SpawnOptions struct {
	Command string
	Args    []string
	Cols    int
	Rows    int
	Cwd     string
	Env     []string
}

// Spawner is the interface for creating PTY-backed processes.
type Spawner interface {
	Spawn(opts SpawnOptions) (*Process, error)
}

// Process represents a running PTY-backed process.
type Process struct {
	ptmx *os.File
	cmd  *exec.Cmd
}

// Read reads output from the PTY. Implements io.Reader.
func (p *Process) Read(buf []byte) (int, error) {
	n, err := p.ptmx.Read(buf)
	if err != nil {
		return n, fmt.Errorf("pty read: %w", err)
	}

	return n, nil
}

// Write sends input to the PTY. Implements io.Writer.
func (p *Process) Write(data []byte) (int, error) {
	n, err := p.ptmx.Write(data)
	if err != nil {
		return n, fmt.Errorf("pty write: %w", err)
	}

	return n, nil
}

// Fd returns the file descriptor of the PTY master for use with poll/epoll.
func (p *Process) Fd() uintptr {
	return p.ptmx.Fd()
}

// Resize changes the terminal dimensions of the PTY.
func (p *Process) Resize(cols, rows int) error {
	ws := &creackpty.Winsize{ //nolint:exhaustruct
		Cols: uint16(cols), //nolint:gosec
		Rows: uint16(rows), //nolint:gosec
	}

	if err := creackpty.Setsize(p.ptmx, ws); err != nil {
		return fmt.Errorf("pty resize: %w", err)
	}

	return nil
}

// Kill sends SIGHUP to the process group. If the process does not exit within
// 500ms, it sends SIGKILL as a fallback.
func (p *Process) Kill() error {
	pgid, err := syscall.Getpgid(p.cmd.Process.Pid)
	if err != nil {
		return fmt.Errorf("getpgid: %w", err)
	}

	// Send SIGHUP to the entire process group.
	if sigErr := syscall.Kill(-pgid, syscall.SIGHUP); sigErr != nil && !errors.Is(sigErr, syscall.ESRCH) {
		return fmt.Errorf("kill SIGHUP: %w", sigErr)
	}

	done := make(chan struct{})
	go func() {
		p.cmd.Wait() //nolint:errcheck
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(500 * time.Millisecond):
		// Fallback: SIGKILL the process group.
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		return nil
	}
}

// Wait blocks until the process exits and returns its exit code.
func (p *Process) Wait() (int, error) {
	err := p.cmd.Wait()
	if err == nil {
		return 0, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}

	return -1, fmt.Errorf("process wait: %w", err)
}

// Pid returns the process ID of the spawned process.
func (p *Process) Pid() int {
	return p.cmd.Process.Pid
}

// Close closes the PTY master file descriptor.
func (p *Process) Close() error {
	if err := p.ptmx.Close(); err != nil {
		return fmt.Errorf("pty close: %w", err)
	}

	return nil
}

// UnixSpawner is the Unix implementation of the Spawner interface.
type UnixSpawner struct{}

// Spawn creates a new PTY-backed process using the given options.
// It sets Setsid to true for process group isolation.
func (s *UnixSpawner) Spawn(opts SpawnOptions) (*Process, error) {
	cmd := exec.Command(opts.Command, opts.Args...) //nolint:gosec

	if opts.Cwd != "" {
		cmd.Dir = opts.Cwd
	}

	if len(opts.Env) > 0 {
		cmd.Env = opts.Env
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{ //nolint:exhaustruct
		Setsid: true,
	}

	cols := opts.Cols
	if cols <= 0 {
		cols = 80
	}

	rows := opts.Rows
	if rows <= 0 {
		rows = 24
	}

	ws := &creackpty.Winsize{ //nolint:exhaustruct
		Cols: uint16(cols), //nolint:gosec
		Rows: uint16(rows), //nolint:gosec
	}

	ptmx, err := creackpty.StartWithSize(cmd, ws)
	if err != nil {
		return nil, fmt.Errorf("pty start: %w", err)
	}

	return &Process{
		ptmx: ptmx,
		cmd:  cmd,
	}, nil
}
