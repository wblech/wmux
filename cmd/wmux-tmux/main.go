// Package main implements a tmux CLI shim that translates a subset of tmux
// commands into wmux SDK calls. It is NOT installed system-wide — integrators
// inject it into child process PATH via symlink or directory prepend.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/wblech/wmux/pkg/client"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		printUsage()
		os.Exit(1)
	}

	var exitCode int
	switch args[0] {
	case "new-session":
		exitCode = cmdNewSession(args[1:])
	case "send-keys":
		exitCode = cmdSendKeys(args[1:])
	case "capture-pane":
		exitCode = cmdCapturePane(args[1:])
	case "kill-session":
		exitCode = cmdKillSession(args[1:])
	case "list-sessions":
		exitCode = cmdListSessions(args[1:])
	case "has-session":
		exitCode = cmdHasSession(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "wmux-tmux: unsupported command: %s\n", args[0])
		printUsage()
		exitCode = 1
	}

	os.Exit(exitCode)
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: tmux <command> [flags]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Supported commands (wmux-tmux shim):")
	fmt.Fprintln(os.Stderr, "  new-session     -d -s NAME [-x COLS] [-y ROWS]")
	fmt.Fprintln(os.Stderr, "  send-keys       -t NAME KEYS...")
	fmt.Fprintln(os.Stderr, "  capture-pane    -t NAME -p")
	fmt.Fprintln(os.Stderr, "  kill-session    -t NAME")
	fmt.Fprintln(os.Stderr, "  list-sessions")
	fmt.Fprintln(os.Stderr, "  has-session     -t NAME")
}

func newClient() (*client.Client, error) {
	var opts []client.Option

	ns := os.Getenv("WMUX_NAMESPACE")
	if ns != "" {
		opts = append(opts, client.WithNamespace(ns))
	}

	return client.New(opts...)
}

// cmdNewSession: tmux new-session -d -s NAME [-x COLS] [-y ROWS]
func cmdNewSession(args []string) int {
	var name string
	cols := 80
	rows := 24

	i := 0
	for i < len(args) {
		switch args[i] {
		case "-d":
			// Detached mode — always true for wmux, skip.
			i++
		case "-s":
			if i+1 < len(args) {
				name = args[i+1]
				i += 2
			} else {
				fmt.Fprintln(os.Stderr, "wmux-tmux: -s requires a session name")
				return 1
			}
		case "-x":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &cols)
				i += 2
			} else {
				i++
			}
		case "-y":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &rows)
				i += 2
			} else {
				i++
			}
		default:
			i++
		}
	}

	if name == "" {
		fmt.Fprintln(os.Stderr, "wmux-tmux: new-session requires -s NAME")
		return 1
	}

	c, err := newClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "wmux-tmux: %v\n", err)
		return 1
	}
	defer func() { _ = c.Close() }()

	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}

	_, err = c.Create(name, client.CreateParams{
		Shell: shell,
		Args:  nil,
		Cols:  cols,
		Rows:  rows,
		Cwd:   "",
		Env:   nil,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "wmux-tmux: %v\n", err)
		return 1
	}

	return 0
}

// cmdSendKeys: tmux send-keys -t NAME KEYS...
func cmdSendKeys(args []string) int {
	var target string

	i := 0
	for i < len(args) {
		if args[i] == "-t" && i+1 < len(args) {
			target = args[i+1]
			i += 2
			continue
		}
		break
	}

	if target == "" {
		fmt.Fprintln(os.Stderr, "wmux-tmux: send-keys requires -t NAME")
		return 1
	}

	keys := strings.Join(args[i:], " ")
	if keys == "" {
		return 0
	}

	c, err := newClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "wmux-tmux: %v\n", err)
		return 1
	}
	defer func() { _ = c.Close() }()

	keys = translateKeys(keys)

	err = c.Exec(target, keys, client.WithNewline(false))
	if err != nil {
		fmt.Fprintf(os.Stderr, "wmux-tmux: %v\n", err)
		return 1
	}

	return 0
}

// translateKeys converts tmux key names to their terminal escape equivalents.
func translateKeys(keys string) string {
	replacer := strings.NewReplacer(
		"Enter", "\n",
		"C-c", "\x03",
		"C-d", "\x04",
		"C-z", "\x1a",
		"C-l", "\x0c",
		"Escape", "\x1b",
		"Tab", "\t",
		"Space", " ",
		"BSpace", "\x7f",
	)
	return replacer.Replace(keys)
}

// cmdCapturePane: tmux capture-pane -t NAME -p
func cmdCapturePane(args []string) int {
	var target string
	printMode := false

	i := 0
	for i < len(args) {
		switch args[i] {
		case "-t":
			if i+1 < len(args) {
				target = args[i+1]
				i += 2
				continue
			}
			i++
		case "-p":
			printMode = true
			i++
		default:
			i++
		}
	}

	if target == "" {
		fmt.Fprintln(os.Stderr, "wmux-tmux: capture-pane requires -t NAME")
		return 1
	}

	if !printMode {
		fmt.Fprintln(os.Stderr, "wmux-tmux: capture-pane requires -p flag")
		return 1
	}

	c, err := newClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "wmux-tmux: %v\n", err)
		return 1
	}
	defer func() { _ = c.Close() }()

	result, err := c.Attach(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wmux-tmux: %v\n", err)
		return 1
	}

	// Print viewport content and immediately detach.
	_, _ = os.Stdout.Write(result.Snapshot.Viewport)
	_ = c.Detach(target)

	return 0
}

// cmdKillSession: tmux kill-session -t NAME
func cmdKillSession(args []string) int {
	var target string

	i := 0
	for i < len(args) {
		if args[i] == "-t" && i+1 < len(args) {
			target = args[i+1]
			i += 2
			continue
		}
		i++
	}

	if target == "" {
		fmt.Fprintln(os.Stderr, "wmux-tmux: kill-session requires -t NAME")
		return 1
	}

	c, err := newClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "wmux-tmux: %v\n", err)
		return 1
	}
	defer func() { _ = c.Close() }()

	if err := c.Kill(target); err != nil {
		fmt.Fprintf(os.Stderr, "wmux-tmux: %v\n", err)
		return 1
	}

	return 0
}

// cmdListSessions: tmux list-sessions
func cmdListSessions(_ []string) int {
	c, err := newClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "wmux-tmux: %v\n", err)
		return 1
	}
	defer func() { _ = c.Close() }()

	sessions, err := c.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "wmux-tmux: %v\n", err)
		return 1
	}

	// Output in a tmux-compatible format: name: N windows (created ...)
	for _, s := range sessions {
		fmt.Printf("%s: 1 windows (%s) [%dx%d]\n", s.ID, s.State, s.Cols, s.Rows)
	}

	return 0
}

// cmdHasSession: tmux has-session -t NAME — exit 0 if exists, 1 otherwise.
func cmdHasSession(args []string) int {
	var target string

	i := 0
	for i < len(args) {
		if args[i] == "-t" && i+1 < len(args) {
			target = args[i+1]
			i += 2
			continue
		}
		i++
	}

	if target == "" {
		fmt.Fprintln(os.Stderr, "wmux-tmux: has-session requires -t NAME")
		return 1
	}

	c, err := newClient()
	if err != nil {
		// Cannot connect — daemon not running, session doesn't exist.
		return 1
	}
	defer func() { _ = c.Close() }()

	_, err = c.Info(target)
	if err != nil {
		return 1
	}

	return 0
}
