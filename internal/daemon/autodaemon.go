package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"time"
)

const (
	// socketPollInterval is how often WaitForSocket checks for the file.
	socketPollInterval = 50 * time.Millisecond
)

// WaitForSocket polls for a socket file at path to appear within timeout.
// Returns ErrDaemonNotRunning if the timeout expires.
func WaitForSocket(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return nil
		}

		time.Sleep(socketPollInterval)
	}

	return ErrDaemonNotRunning
}

// BuildDaemonArgs constructs the argument list for spawning the daemon
// as a subprocess.
func BuildDaemonArgs(socketPath, pidFilePath, logLevel string) []string {
	return []string{
		"daemon",
		"--socket", socketPath,
		"--pid-file", pidFilePath,
		"--log-level", logLevel,
	}
}

// executableFunc is the function used to find the current executable path.
// Overridable in tests.
var executableFunc = os.Executable

// Autodaemonize spawns the current binary as a background daemon process.
// It returns once the daemon is started and the socket appears, or an error
// if it fails to start within timeout.
func Autodaemonize(socketPath, pidFilePath, logLevel string, timeout time.Duration) error {
	executable, err := executableFunc()
	if err != nil {
		return fmt.Errorf("daemon: find executable: %w", err)
	}

	args := BuildDaemonArgs(socketPath, pidFilePath, logLevel)
	cmd := exec.Command(executable, args...) //nolint:gosec

	// Detach from terminal.
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("daemon: start background process: %w", err)
	}

	if err := cmd.Process.Release(); err != nil {
		return fmt.Errorf("daemon: release process: %w", err)
	}

	if err := WaitForSocket(socketPath, timeout); err != nil {
		return fmt.Errorf("daemon: socket not ready after %s: %w", timeout, err)
	}

	return nil
}
