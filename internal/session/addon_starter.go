package session

import (
	"fmt"
	"io"
	"os"
	"os/exec"
)

// CommandProcessStarter spawns an addon child process using exec.Command.
type CommandProcessStarter struct {
	binPath string
	args    []string
}

// NewCommandProcessStarter creates a ProcessStarter for the given binary and args.
// Example: NewCommandProcessStarter("node", "addons/xterm/dist/index.js")
func NewCommandProcessStarter(binPath string, args ...string) *CommandProcessStarter {
	return &CommandProcessStarter{binPath: binPath, args: args}
}

// Start launches the binary and returns an AddonProcess backed by os/exec pipes.
func (s *CommandProcessStarter) Start() (AddonProcess, error) {
	cmd := exec.Command(s.binPath, s.args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("addon starter: stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("addon starter: stdout pipe: %w", err)
	}

	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("addon starter: start: %w", err)
	}

	return &execAddonProcess{cmd: cmd, stdin: stdin, stdout: stdout}, nil
}

// execAddonProcess wraps *exec.Cmd to satisfy the AddonProcess interface.
type execAddonProcess struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

func (e *execAddonProcess) Stdin() io.Writer  { return e.stdin }
func (e *execAddonProcess) Stdout() io.Reader { return e.stdout }
func (e *execAddonProcess) Wait() error       { return e.cmd.Wait() } //nolint:wrapcheck
func (e *execAddonProcess) Kill() error {
	_ = e.stdin.Close()
	if e.cmd.Process != nil {
		_ = e.cmd.Process.Kill()
	}
	_ = e.cmd.Wait() //nolint:errcheck
	return nil
}
