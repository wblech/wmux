// Package main implements the wmux CLI binary.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wblech/wmux/internal/platform/auth"
	"github.com/wblech/wmux/internal/platform/protocol"
	"github.com/wblech/wmux/internal/transport"
)

const connectTimeout = 5 * time.Second

// Global paths set by parseGlobalFlags before subcommand dispatch.
var (
	socketPath = "~/.wmux/daemon.sock"
	tokenPath  = "~/.wmux/daemon.token"
)

func main() {
	cmd, args := parseGlobalFlags(os.Args[1:])

	if cmd == "" {
		printUsage()
		os.Exit(1)
	}

	var exitCode int
	switch cmd {
	case "daemon":
		exitCode = cmdDaemon(args)
	case "create":
		exitCode = cmdCreate(args)
	case "attach":
		exitCode = cmdAttach(args)
	case "detach":
		exitCode = cmdDetach(args)
	case "kill":
		exitCode = cmdKill(args)
	case "list":
		exitCode = cmdList(args)
	case "info":
		exitCode = cmdInfo(args)
	case "status":
		exitCode = cmdStatus(args)
	case "events":
		exitCode = cmdEvents(args)
	case "exec":
		exitCode = cmdExec(args)
	case "wait":
		exitCode = cmdWait(args)
	case "record":
		exitCode = cmdRecord(args)
	case "history":
		exitCode = cmdHistory(args)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		printUsage()
		exitCode = 1
	}
	os.Exit(exitCode)
}

// parseGlobalFlags extracts --socket and --token from args before the subcommand.
// Returns the subcommand name and remaining args.
func parseGlobalFlags(args []string) (string, []string) {
	customSocket := false

	i := 0
	for i < len(args) {
		switch args[i] {
		case "--socket":
			if i+1 < len(args) {
				socketPath = args[i+1]
				customSocket = true
				i += 2
				continue
			}
		case "--token":
			if i+1 < len(args) {
				tokenPath = args[i+1]
				i += 2
				continue
			}
		}
		break
	}

	// Derive token path from socket path if only --socket was given.
	if customSocket && tokenPath == "~/.wmux/daemon.token" {
		socketBase := strings.TrimSuffix(socketPath, ".sock")
		tokenPath = socketBase + ".token"
	}

	if i >= len(args) {
		return "", nil
	}

	return args[i], args[i+1:]
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "Usage: wmux <command> [args]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Commands:")
	fmt.Fprintln(os.Stderr, "  daemon    Start the daemon in foreground")
	fmt.Fprintln(os.Stderr, "  create    Create a new session")
	fmt.Fprintln(os.Stderr, "  attach    Attach to an existing session")
	fmt.Fprintln(os.Stderr, "  detach    Detach from a session")
	fmt.Fprintln(os.Stderr, "  kill      Kill a session")
	fmt.Fprintln(os.Stderr, "  list      List all sessions")
	fmt.Fprintln(os.Stderr, "  info      Show session information")
	fmt.Fprintln(os.Stderr, "  status    Show daemon status")
	fmt.Fprintln(os.Stderr, "  exec      Send input to a session without attaching")
	fmt.Fprintln(os.Stderr, "  wait      Wait for a session condition (exit, idle, match)")
	fmt.Fprintln(os.Stderr, "  record    Start or stop session recording")
	fmt.Fprintln(os.Stderr, "  history   Export session scrollback history")
	fmt.Fprintln(os.Stderr, "  events    Stream session events")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Global flags:")
	fmt.Fprintln(os.Stderr, "  --socket <path>  Daemon socket path (default: ~/.wmux/daemon.sock)")
	fmt.Fprintln(os.Stderr, "  --token <path>   Auth token path (default: ~/.wmux/daemon.token)")
}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[1:])
}

// dialDaemon connects to the daemon and performs the control channel auth handshake.
func dialDaemon(socketPath, tokenPath string) (*protocol.Conn, string, error) {
	token, err := auth.LoadFromFile(expandHome(tokenPath))
	if err != nil {
		return nil, "", fmt.Errorf("read token: %w", err)
	}

	raw, err := net.DialTimeout("unix", expandHome(socketPath), connectTimeout)
	if err != nil {
		return nil, "", fmt.Errorf("connect to daemon: %w", err)
	}

	conn := protocol.NewConn(raw)

	payload := make([]byte, 0, 1+auth.TokenSize)
	payload = append(payload, byte(transport.ChannelControl))
	payload = append(payload, token...)

	if err := conn.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgAuth,
		Payload: payload,
	}); err != nil {
		_ = conn.Close()
		return nil, "", fmt.Errorf("auth handshake: %w", err)
	}

	resp, err := conn.ReadFrame()
	if err != nil {
		_ = conn.Close()
		return nil, "", fmt.Errorf("auth response: %w", err)
	}

	if resp.Type != protocol.MsgOK {
		_ = conn.Close()
		return nil, "", fmt.Errorf("auth failed: %s", string(resp.Payload))
	}

	return conn, string(resp.Payload), nil
}

// dialStream opens a stream channel for the given client ID.
func dialStream(socketPath, tokenPath, clientID string) (*protocol.Conn, error) {
	token, err := auth.LoadFromFile(expandHome(tokenPath))
	if err != nil {
		return nil, fmt.Errorf("read token: %w", err)
	}

	raw, err := net.DialTimeout("unix", expandHome(socketPath), connectTimeout)
	if err != nil {
		return nil, fmt.Errorf("connect stream: %w", err)
	}

	conn := protocol.NewConn(raw)

	payload := make([]byte, 0, 1+auth.TokenSize+1+len(clientID))
	payload = append(payload, byte(transport.ChannelStream))
	payload = append(payload, token...)
	payload = append(payload, byte(len(clientID)))
	payload = append(payload, []byte(clientID)...)

	if err := conn.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    protocol.MsgAuth,
		Payload: payload,
	}); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("stream auth: %w", err)
	}

	resp, err := conn.ReadFrame()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("stream response: %w", err)
	}

	if resp.Type != protocol.MsgOK {
		_ = conn.Close()
		return nil, fmt.Errorf("stream auth failed: %s", string(resp.Payload))
	}

	return conn, nil
}

// sendRequest sends a control frame and reads the response.
func sendRequest(conn *protocol.Conn, msgType protocol.MessageType, payload any) (protocol.Frame, error) {
	var data []byte
	if payload != nil {
		var err error
		data, err = json.Marshal(payload)
		if err != nil {
			return protocol.Frame{}, fmt.Errorf("marshal request: %w", err)
		}
	}

	if err := conn.WriteFrame(protocol.Frame{
		Version: protocol.ProtocolVersion,
		Type:    msgType,
		Payload: data,
	}); err != nil {
		return protocol.Frame{}, fmt.Errorf("send request: %w", err)
	}

	resp, err := conn.ReadFrame()
	if err != nil {
		return protocol.Frame{}, fmt.Errorf("read response: %w", err)
	}

	return resp, nil
}

// checkError prints any error from a response frame and returns true if error.
func checkError(resp protocol.Frame) bool {
	if resp.Type == protocol.MsgError {
		var errResp struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(resp.Payload, &errResp); err == nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", errResp.Error)
		} else {
			fmt.Fprintf(os.Stderr, "error: %s\n", string(resp.Payload))
		}
		return true
	}
	return false
}

// --- subcommands ---

func cmdCreate(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: wmux create <session-id> [--shell /bin/zsh] [--cwd /path]")
		return 1
	}

	sessionID := args[0]
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	cwd, _ := os.Getwd()

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--shell":
			if i+1 < len(args) {
				shell = args[i+1]
				i++
			}
		case "--cwd":
			if i+1 < len(args) {
				cwd = args[i+1]
				i++
			}
		}
	}

	conn, _, err := dialDaemon(socketPath, tokenPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer func() { _ = conn.Close() }()

	type createReq struct {
		ID    string   `json:"id"`
		Shell string   `json:"shell"`
		Args  []string `json:"args,omitempty"`
		Cols  int      `json:"cols"`
		Rows  int      `json:"rows"`
		Cwd   string   `json:"cwd,omitempty"`
		Env   []string `json:"env,omitempty"`
	}

	resp, err := sendRequest(conn, protocol.MsgCreate, createReq{
		ID:    sessionID,
		Shell: shell,
		Args:  nil,
		Cols:  80,
		Rows:  24,
		Cwd:   cwd,
		Env:   nil,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	if checkError(resp) {
		return 1
	}

	var sr struct {
		ID    string `json:"id"`
		State string `json:"state"`
		Pid   int    `json:"pid"`
	}
	if err := json.Unmarshal(resp.Payload, &sr); err == nil {
		fmt.Printf("Created session %s (pid %d)\n", sr.ID, sr.Pid)
	}

	return 0
}

func cmdAttach(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: wmux attach <session-id>")
		return 1
	}

	sessionID := args[0]

	ctrl, clientID, err := dialDaemon(socketPath, tokenPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer func() { _ = ctrl.Close() }()

	stream, err := dialStream(socketPath, tokenPath, clientID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer func() { _ = stream.Close() }()

	type sessIDReq struct {
		SessionID string `json:"session_id"`
	}

	resp, err := sendRequest(ctrl, protocol.MsgAttach, sessIDReq{SessionID: sessionID})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if checkError(resp) {
		return 1
	}

	// Read stream output in background and copy to stdout.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			frame, err := stream.ReadFrame()
			if err != nil {
				return
			}
			if frame.Type == protocol.MsgData && len(frame.Payload) > 1 {
				idLen := int(frame.Payload[0])
				if 1+idLen <= len(frame.Payload) {
					_, _ = os.Stdout.Write(frame.Payload[1+idLen:])
				}
			}
		}
	}()

	// Read stdin and forward as MsgInput.
	go func() {
		buf := make([]byte, 4096)
		for {
			n, readErr := os.Stdin.Read(buf)
			if n > 0 {
				idBytes := []byte(sessionID)
				payload := make([]byte, 0, 1+len(idBytes)+n)
				payload = append(payload, byte(len(idBytes)))
				payload = append(payload, idBytes...)
				payload = append(payload, buf[:n]...)

				_ = ctrl.WriteFrame(protocol.Frame{
					Version: protocol.ProtocolVersion,
					Type:    protocol.MsgInput,
					Payload: payload,
				})

				// Read the ack.
				_, _ = ctrl.ReadFrame()
			}
			if readErr != nil {
				return
			}
		}
	}()

	<-done
	return 0
}

func cmdDetach(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: wmux detach <session-id>")
		return 1
	}

	conn, _, err := dialDaemon(socketPath, tokenPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer func() { _ = conn.Close() }()

	type sessIDReq struct {
		SessionID string `json:"session_id"`
	}

	resp, err := sendRequest(conn, protocol.MsgDetach, sessIDReq{SessionID: args[0]})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	if checkError(resp) {
		return 1
	}

	fmt.Printf("Detached from session %s\n", args[0])
	return 0
}

func cmdKill(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: wmux kill <session-id>")
		return 1
	}

	conn, _, err := dialDaemon(socketPath, tokenPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer func() { _ = conn.Close() }()

	type sessIDReq struct {
		SessionID string `json:"session_id"`
	}

	resp, err := sendRequest(conn, protocol.MsgKill, sessIDReq{SessionID: args[0]})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	if checkError(resp) {
		return 1
	}

	fmt.Printf("Killed session %s\n", args[0])
	return 0
}

func cmdList(_ []string) int {
	conn, _, err := dialDaemon(socketPath, tokenPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer func() { _ = conn.Close() }()

	resp, err := sendRequest(conn, protocol.MsgList, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	if checkError(resp) {
		return 1
	}

	var sessions []struct {
		ID    string `json:"id"`
		State string `json:"state"`
		Pid   int    `json:"pid"`
		Cols  int    `json:"cols"`
		Rows  int    `json:"rows"`
		Shell string `json:"shell"`
	}

	if err := json.Unmarshal(resp.Payload, &sessions); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions")
		return 0
	}

	fmt.Printf("%-20s %-10s %-8s %-6s %-6s %s\n", "ID", "STATE", "PID", "COLS", "ROWS", "SHELL")
	for _, s := range sessions {
		fmt.Printf("%-20s %-10s %-8d %-6d %-6d %s\n", s.ID, s.State, s.Pid, s.Cols, s.Rows, s.Shell)
	}

	return 0
}

func cmdInfo(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: wmux info <session-id>")
		return 1
	}

	conn, _, err := dialDaemon(socketPath, tokenPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer func() { _ = conn.Close() }()

	type sessIDReq struct {
		SessionID string `json:"session_id"`
	}

	resp, err := sendRequest(conn, protocol.MsgInfo, sessIDReq{SessionID: args[0]})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	if checkError(resp) {
		return 1
	}

	var s struct {
		ID    string `json:"id"`
		State string `json:"state"`
		Pid   int    `json:"pid"`
		Cols  int    `json:"cols"`
		Rows  int    `json:"rows"`
		Shell string `json:"shell"`
	}
	if err := json.Unmarshal(resp.Payload, &s); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	fmt.Printf("ID:    %s\n", s.ID)
	fmt.Printf("State: %s\n", s.State)
	fmt.Printf("PID:   %d\n", s.Pid)
	fmt.Printf("Size:  %dx%d\n", s.Cols, s.Rows)
	fmt.Printf("Shell: %s\n", s.Shell)

	return 0
}

func cmdStatus(_ []string) int {
	conn, _, err := dialDaemon(socketPath, tokenPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer func() { _ = conn.Close() }()

	resp, err := sendRequest(conn, protocol.MsgStatus, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	if checkError(resp) {
		return 1
	}

	var sr struct {
		Version      string `json:"version"`
		Uptime       string `json:"uptime"`
		SessionCount int    `json:"session_count"`
		ClientCount  int    `json:"client_count"`
	}
	if err := json.Unmarshal(resp.Payload, &sr); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	fmt.Printf("Version:  %s\n", sr.Version)
	fmt.Printf("Uptime:   %s\n", sr.Uptime)
	fmt.Printf("Sessions: %d\n", sr.SessionCount)
	fmt.Printf("Clients:  %d\n", sr.ClientCount)

	return 0
}

func cmdEvents(args []string) int {
	sessionID := ""
	for _, arg := range args {
		if arg != "--all" && !strings.HasPrefix(arg, "--") {
			sessionID = arg
		}
	}

	conn, _, err := dialDaemon(socketPath, tokenPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer func() { _ = conn.Close() }()

	type eventReq struct {
		SessionID string `json:"session_id,omitempty"`
	}

	resp, err := sendRequest(conn, protocol.MsgEvent, eventReq{SessionID: sessionID})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	if checkError(resp) {
		return 1
	}

	// Stream events as NDJSON.
	enc := json.NewEncoder(os.Stdout)
	for {
		frame, readErr := conn.ReadFrame()
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				fmt.Fprintf(os.Stderr, "error: %v\n", readErr)
			}
			return 0
		}

		if frame.Type == protocol.MsgEvent {
			var evt map[string]any
			if json.Unmarshal(frame.Payload, &evt) == nil {
				_ = enc.Encode(evt)
			}
		}
	}
}

func cmdWait(args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: wmux wait <session-id> exit [--timeout <ms>]")
		fmt.Fprintln(os.Stderr, "       wmux wait <session-id> idle <idle-ms> [--timeout <ms>]")
		fmt.Fprintln(os.Stderr, "       wmux wait <session-id> match <pattern> [--timeout <ms>]")
		return 1
	}

	sessionID := args[0]
	mode := args[1]

	var (
		timeout int64
		idleFor int64
		pattern string
	)

	remaining := args[2:]

	switch mode {
	case "exit":
		// No extra positional args.
	case "idle":
		if len(remaining) < 1 {
			fmt.Fprintln(os.Stderr, "error: idle mode requires <idle-ms> argument")
			return 1
		}
		val, err := strconv.ParseInt(remaining[0], 10, 64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: invalid idle-ms: %s\n", remaining[0])
			return 1
		}
		idleFor = val
		remaining = remaining[1:]
	case "match":
		if len(remaining) < 1 {
			fmt.Fprintln(os.Stderr, "error: match mode requires <pattern> argument")
			return 1
		}
		pattern = remaining[0]
		remaining = remaining[1:]
	default:
		fmt.Fprintf(os.Stderr, "error: unknown wait mode: %s\n", mode)
		return 1
	}

	// Parse optional --timeout flag.
	for i := 0; i < len(remaining); i++ {
		if remaining[i] == "--timeout" && i+1 < len(remaining) {
			val, err := strconv.ParseInt(remaining[i+1], 10, 64)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: invalid timeout: %s\n", remaining[i+1])
				return 1
			}
			timeout = val
			i++
		}
	}

	conn, _, err := dialDaemon(socketPath, tokenPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer func() { _ = conn.Close() }()

	type waitReq struct {
		SessionID string `json:"session_id"`
		Mode      string `json:"mode"`
		Timeout   int64  `json:"timeout"`
		IdleFor   int64  `json:"idle_for,omitempty"`
		Pattern   string `json:"pattern,omitempty"`
	}

	resp, err := sendRequest(conn, protocol.MsgWait, waitReq{
		SessionID: sessionID,
		Mode:      mode,
		Timeout:   timeout,
		IdleFor:   idleFor,
		Pattern:   pattern,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	if checkError(resp) {
		return 1
	}

	type waitResp struct {
		SessionID string `json:"session_id"`
		Mode      string `json:"mode"`
		ExitCode  *int   `json:"exit_code"`
		Matched   bool   `json:"matched"`
		TimedOut  bool   `json:"timed_out"`
	}

	var result waitResp
	if err := json.Unmarshal(resp.Payload, &result); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	if result.TimedOut {
		fmt.Fprintf(os.Stderr, "wait: timed out (%s mode)\n", result.Mode)
		return 2
	}

	switch result.Mode {
	case "exit":
		if result.ExitCode != nil {
			fmt.Printf("session %s exited with code %d\n", result.SessionID, *result.ExitCode)
		}
	case "idle":
		fmt.Printf("session %s idle\n", result.SessionID)
	case "match":
		if result.Matched {
			fmt.Printf("session %s matched\n", result.SessionID)
		} else if result.ExitCode != nil {
			fmt.Printf("session %s exited (code %d) before match\n", result.SessionID, *result.ExitCode)
			return 1
		}
	}

	return 0
}

func cmdExec(args []string) int {
	if len(args) < 1 {
		printExecUsage()
		return 1
	}

	var (
		syncMode  bool
		prefix    string
		noNewline bool
	)

	i := 0
	for i < len(args) {
		switch args[i] {
		case "--sync":
			syncMode = true
			i++
			continue
		case "--prefix":
			if i+1 < len(args) {
				prefix = args[i+1]
				i += 2
				continue
			}
			fmt.Fprintln(os.Stderr, "error: --prefix requires a value")
			return 1
		case "--no-newline":
			noNewline = true
			i++
			continue
		}
		break
	}

	if prefix != "" {
		return execPrefix(args[i:], prefix, !noNewline)
	}
	if syncMode {
		return execSync(args[i:], !noNewline)
	}
	return execSingle(args[i:], !noNewline)
}

func printExecUsage() {
	fmt.Fprintln(os.Stderr, "Usage: wmux exec <session-id> <input>")
	fmt.Fprintln(os.Stderr, "       wmux exec --sync <id1> <id2> ... -- <input>")
	fmt.Fprintln(os.Stderr, "       wmux exec --prefix <prefix> -- <input>")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Flags:")
	fmt.Fprintln(os.Stderr, "  --no-newline  Do not append newline to input")
	fmt.Fprintln(os.Stderr, "  --sync        Send to multiple sessions by ID")
	fmt.Fprintln(os.Stderr, "  --prefix      Send to all sessions matching prefix")
}

func execSingle(args []string, newline bool) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: wmux exec <session-id> <input>")
		return 1
	}

	sessionID := args[0]
	input := strings.Join(args[1:], " ")

	conn, _, err := dialDaemon(socketPath, tokenPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer func() { _ = conn.Close() }()

	type execReq struct {
		SessionID string `json:"session_id"`
		Input     string `json:"input"`
		Newline   bool   `json:"newline"`
	}

	resp, err := sendRequest(conn, protocol.MsgExec, execReq{
		SessionID: sessionID,
		Input:     input,
		Newline:   newline,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	if checkError(resp) {
		return 1
	}

	return 0
}

func execSync(args []string, newline bool) int {
	sepIdx := -1
	for i, arg := range args {
		if arg == "--" {
			sepIdx = i
			break
		}
	}

	if sepIdx < 1 || sepIdx >= len(args)-1 {
		fmt.Fprintln(os.Stderr, "Usage: wmux exec --sync <id1> <id2> ... -- <input>")
		return 1
	}

	sessionIDs := args[:sepIdx]
	input := strings.Join(args[sepIdx+1:], " ")

	conn, _, err := dialDaemon(socketPath, tokenPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer func() { _ = conn.Close() }()

	type execSyncReq struct {
		SessionIDs []string `json:"session_ids"`
		Input      string   `json:"input"`
		Newline    bool     `json:"newline"`
	}

	resp, err := sendRequest(conn, protocol.MsgExecSync, execSyncReq{
		SessionIDs: sessionIDs,
		Input:      input,
		Newline:    newline,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	if checkError(resp) {
		return 1
	}

	return printExecSyncResults(resp)
}

func execPrefix(args []string, prefix string, newline bool) int {
	sepIdx := -1
	for i, arg := range args {
		if arg == "--" {
			sepIdx = i
			break
		}
	}

	var input string
	switch {
	case sepIdx >= 0 && sepIdx < len(args)-1:
		input = strings.Join(args[sepIdx+1:], " ")
	case len(args) > 0 && args[0] != "--":
		input = strings.Join(args, " ")
	default:
		fmt.Fprintln(os.Stderr, "Usage: wmux exec --prefix <prefix> -- <input>")
		return 1
	}

	conn, _, err := dialDaemon(socketPath, tokenPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer func() { _ = conn.Close() }()

	type execSyncReq struct {
		Prefix  string `json:"prefix"`
		Input   string `json:"input"`
		Newline bool   `json:"newline"`
	}

	resp, err := sendRequest(conn, protocol.MsgExecSync, execSyncReq{
		Prefix:  prefix,
		Input:   input,
		Newline: newline,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	if checkError(resp) {
		return 1
	}

	return printExecSyncResults(resp)
}

func cmdRecord(args []string) int {
	if len(args) < 2 {
		fmt.Fprintln(os.Stderr, "Usage: wmux record <start|stop> <session-id>")
		return 1
	}

	action := args[0]
	sessionID := args[1]

	if action != "start" && action != "stop" {
		fmt.Fprintln(os.Stderr, "error: action must be 'start' or 'stop'")
		return 1
	}

	conn, _, err := dialDaemon(socketPath, tokenPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer func() { _ = conn.Close() }()

	type recordReq struct {
		SessionID string `json:"session_id"`
		Action    string `json:"action"`
	}

	resp, err := sendRequest(conn, protocol.MsgRecord, recordReq{
		SessionID: sessionID,
		Action:    action,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	if checkError(resp) {
		return 1
	}

	var rr struct {
		SessionID string `json:"session_id"`
		Recording bool   `json:"recording"`
		Path      string `json:"path,omitempty"`
	}
	if err := json.Unmarshal(resp.Payload, &rr); err == nil {
		if rr.Recording {
			fmt.Printf("Recording started: %s -> %s\n", rr.SessionID, rr.Path)
		} else {
			fmt.Printf("Recording stopped: %s\n", rr.SessionID)
		}
	}

	return 0
}

func cmdHistory(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "Usage: wmux history <session-id> [--format ansi|text|html] [--lines N]")
		return 1
	}

	sessionID := args[0]
	format := "ansi"
	lines := 0

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--format":
			if i+1 < len(args) {
				format = args[i+1]
				i++
			}
		case "--lines":
			if i+1 < len(args) {
				n, err := strconv.Atoi(args[i+1])
				if err != nil {
					fmt.Fprintf(os.Stderr, "error: invalid --lines value: %s\n", args[i+1])
					return 1
				}
				lines = n
				i++
			}
		}
	}

	conn, _, err := dialDaemon(socketPath, tokenPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	defer func() { _ = conn.Close() }()

	type histReq struct {
		SessionID string `json:"session_id"`
		Format    string `json:"format"`
		Lines     int    `json:"lines,omitempty"`
	}

	resp, err := sendRequest(conn, protocol.MsgHistory, histReq{
		SessionID: sessionID,
		Format:    format,
		Lines:     lines,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	if checkError(resp) {
		return 1
	}

	// Read streamed history frames until MsgHistoryEnd.
	if resp.Type == protocol.MsgHistory {
		_, _ = os.Stdout.Write(resp.Payload)
	}

	for {
		frame, readErr := conn.ReadFrame()
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				fmt.Fprintf(os.Stderr, "error: %v\n", readErr)
			}
			break
		}
		if frame.Type == protocol.MsgHistoryEnd {
			break
		}
		if frame.Type == protocol.MsgHistory {
			_, _ = os.Stdout.Write(frame.Payload)
		}
	}

	return 0
}

func printExecSyncResults(resp protocol.Frame) int {
	type execResult struct {
		SessionID string `json:"session_id"`
		OK        bool   `json:"ok"`
		Error     string `json:"error,omitempty"`
	}
	type execSyncResp struct {
		Results []execResult `json:"results"`
	}

	var syncResp execSyncResp
	if err := json.Unmarshal(resp.Payload, &syncResp); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	hasError := false
	for _, r := range syncResp.Results {
		if r.OK {
			fmt.Printf("%s: ok\n", r.SessionID)
		} else {
			fmt.Printf("%s: error: %s\n", r.SessionID, r.Error)
			hasError = true
		}
	}

	if hasError {
		return 1
	}
	return 0
}
