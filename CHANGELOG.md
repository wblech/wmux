# Changelog

All notable changes to wmux are documented in this file.

## [addons/charmvt/v0.1.0-beta.3](https://github.com/wblech/wmux/releases/tag/addons/charmvt/v0.1.0-beta.3) - 2026-04-16

### Bug Fixes

- Use operation-level mutex to prevent Snapshot starvation *(charmvt)*
- Add RPC timeout to prevent client-wide freeze *(client)*
- Resolve lint issues from attach-blocking fix

### Testing

- Convert contention evidence tests to regression tests *(charmvt)*
- Convert rpcMu evidence tests to regression tests *(client)*

## [0.1.0-beta.8](https://github.com/wblech/wmux/releases/tag/v0.1.0-beta.8) - 2026-04-16

### Bug Fixes

- Resolve deadlock between Bus.Close and Subscription.Unsubscribe *(event)*

### Documentation

- Document Snapshot \r\n line ending contract *(client)*

### Features

- Add trimTrailingEmptyRows and toTerminalLineEndings helpers *(charmvt)*
- Make viewport terminal-ready (strip trailing rows, use \r\n) *(charmvt)*
- Use \r\n line endings in scrollback *(charmvt)*

### Testing

- Add Unicode/Nerd Font preservation regression tests *(charmvt)*

## [0.1.0-beta.6](https://github.com/wblech/wmux/releases/tag/v0.1.0-beta.6) - 2026-04-16

### Documentation

- Update persistent-daemon guide for ServeDaemon options

### Features

- Accept integrator options in ServeDaemon *(client)*

## [addons/charmvt/v0.1.0-beta.1](https://github.com/wblech/wmux/releases/tag/addons/charmvt/v0.1.0-beta.1) - 2026-04-16

### Bug Fixes

- Lint fixups for EmulatorFactory (exhaustruct, gofmt)
- Lint fixups for UpdateEmulatorScrollback (gofmt, exhaustruct, testifylint)

### Documentation

- Add ADR 0022 — emulator addons as optional Go modules
- Update embedded-daemon guide for charmvt addon
- Remove stale WithEmulatorBackend references from integration docs
- Add Addons section with install and usage guide
- Add ADR 0023 and document UpdateEmulatorScrollback

### Features

- Add EmulatorFactory interface to session.Service
- Add EmulatorFactory interface, remove WithEmulatorBackend/WithXtermBinPath
- Add charmvt addon module with Backend(), emulator, and scrollback rendering
- Add MsgUpdateEmulatorScrollback RPC and daemon handler
- Add Client.UpdateEmulatorScrollback method
- Implement ScrollbackConfigurable *(charmvt)*
- Add ScrollbackConfigurable interface and UpdateEmulatorScrollback (fix unstaged)

### Refactoring

- Move xterm addon wiring to CLI, use EmulatorFactory (fix unstaged)

### Testing

- Add emulator behavior unit tests *(charmvt)*
- Add scrollback rendering unit tests *(charmvt)*
- Add E2E tests for charmvt backend
- Add E2E tests for UpdateEmulatorScrollback

## [0.1.0-beta.4](https://github.com/wblech/wmux/releases/tag/v0.1.0-beta.4) - 2026-04-16

### Bug Fixes

- Wire AddonManager in daemon when emulatorBackend is xterm
- Resolve data race in mockAddonProcess stdin access

### Documentation

- Add WithXtermBinPath to embedded daemon integration guide

### Features

- Add CommandProcessStarter for addon process spawning

### Refactoring

- Export NewAddonManager for use by daemon wiring

### Testing

- Replace bug-documentation tests with regression guards
- Add E2E tests for Attach snapshot with xterm addon

## [0.1.0-beta.3](https://github.com/wblech/wmux/releases/tag/v0.1.0-beta.3) - 2026-04-16

### Bug Fixes

- Auto-subscribe to daemon events on connect
- Move checkAutomationMode after ReadFrame to prevent broken pipe on rejection

### Documentation

- Amend 0003 — events flow on control channel, not stream *(adr)*
- Add 0020 client auto-subscribe, 0021 E2E in-process daemon *(adr)*

### Testing

- Add default MsgEvent handler to mock server for subscribe support
- Add E2E test harness with in-process daemon
- Add E2E tests for event delivery scenarios
- Add unit tests for event.Type JSON marshal/unmarshal

## [0.1.0-beta.2](https://github.com/wblech/wmux/releases/tag/v0.1.0-beta.2) - 2026-04-15

### Bug Fixes

- HistoryReader reads from demuxed channel instead of raw conn
- Address race condition and lint issues from stream channel implementation

### Documentation

- Update IPC description to reflect dual-channel client

### Features

- Add decodeDataPayload to pkg/client
- Implement client stream channel and control demux

### Refactoring

- Update Client struct for dual-channel architecture
- Update mock server for dual-channel client tests

### Testing

- Verify stream data delivery to OnData handler
- Verify event delivery and RPC/event interleaving

## [0.1.0-beta.1](https://github.com/wblech/wmux/releases/tag/v0.1.0-beta.1) - 2026-04-15

### Bug Fixes

- Rename EventType to Type to avoid stutter, fix unused params *(event)*
- Lint fixes and coverage improvements (92%) *(client)*
- Harden JSON parsing, buffer handling, and shutdown response *(addon-xterm)*
- Replace wrong error sentinel, make Resize read response, single-write frames *(session)*
- Lint fixes for addon protocol, emulator, and manager *(session)*
- Lint and coverage fixes for cold restore
- Handle non-IsNotExist stat errors in CleanSessionHistory *(client)*
- Remove double sessions segment in handleEnvForward path *(daemon)*
- Log env forwarding errors instead of silently discarding *(daemon)*
- Lint issues in new test files (exhaustruct, testifylint)
- ServeDaemon returns error, parseDaemonArgs handles all options, validate backend *(client)*
- Address lint issues and add RecordingLimitReached event type
- Address exhaustruct and errcheck lint issues in session prefix implementation
- Address exhaustruct, gofmt, revive, and goframe lint issues in DA and shell-ready implementation
- Address errcheck and wrapcheck lint issues in tmux shim
- Add package comment and exported var docs to buildinfo
- Show build version in status, harden release workflow
- Correct goframe install path to wblech/goframe *(ci)*
- Migrate golangci-lint config to v2 format
- Remove goframe check from CI (local-only tool) *(ci)*
- Replace polling with channel subscribe in cold restore exit test
- Increase CI timeout for cold restore exit test
- Send SIGHUP on session kill for cross-platform PTY cleanup
- Return Session by value to eliminate data race in List/Get
- Use git-cliff action directly instead of CLI binary *(ci)*
- Inline remote URL in cliff.toml to fix template macro error *(ci)*

### Documentation

- Mark Phase 1 complete in ROADMAP, add event subscription tests
- Add Phase 2 design spec and MADR ADR templates
- Add Phase 2 implementation plans (5 sub-plans + overview)
- 0000 use MADR 4.0.0 for architectural decisions *(adr)*
- 0001-0004 Phase 1 foundation decisions *(adr)*
- 0005-0009 Phase 1 remaining decisions *(adr)*
- 0010-0013 Phase 2 core decisions *(adr)*
- 0014-0016 Phase 2 remaining decisions *(adr)*
- Phase 2 post-review fixes spec
- Phase 2 fixes implementation plan
- Client SDK redesign spec (functional options + embedded daemon)
- Client SDK redesign implementation plan
- 0017 client SDK with functional options and embedded daemon *(adr)*
- 0018 asciinema v2 format for session recording *(adr)*
- Add MkDocs Material documentation site with llms.txt
- Add README with install, quickstart, and SDK usage

### Features

- Add config loading with TOML parsing and defaults
- Add config fx module for dependency injection
- Add protocol frame types and constants
- Add protocol codec for binary frame encoding/decoding
- Add protocol fx module
- Add structured JSON logger with session context
- Add logger ParseLevel tests and fx module
- Add PTY spawner with process lifecycle management
- Add PTY fx module
- Add session service with PTY lifecycle management
- Add session options and fx module
- Add transport fx module
- Add OnClient callback to transport server
- Add daemon entity types and binary data codec
- Add PID file management for daemon lifecycle
- Add daemon service with control routing and output broadcast
- Add graceful shutdown with signal handling
- Add autodaemonize and socket wait helpers
- Add daemon fx module and improve autodaemon coverage
- Add history package with ParseSize and Metadata types
- Add ScrollbackWriter with size cap
- Add metadata persistence (meta.json read/write/update)
- Add history dir operations with LRU eviction
- Add attach/detach, activity tracking, history writer, and OnExit callback to session service
- Add spawn semaphore, idle reaper, and process watchdog *(session)*
- Extend SessionManager with attach/detach/lastActivity/onExit methods
- Add event bus entity types and errors *(event)*
- Add in-process event bus with fan-out and type filtering *(event)*
- Emit lifecycle events and add status command *(daemon)*
- Add orphaned session reconciliation on startup *(daemon)*
- Add wmux CLI binary with core commands *(cli)*
- Wire orphan reconciliation into Start, fix CLI lint *(daemon)*
- Add daemon subcommand and global --socket/--token flags *(cli)*
- Add addon binary protocol encoder/decoder *(session)*
- Add AddonEmulator proxying to external process *(session)*
- Add emulator.xterm.bin setting *(config)*
- Add Node project with binary protocol encoder/decoder *(addon-xterm)*
- Add xterm instance manager and main loop *(addon-xterm)*
- Add public types for wmux client library *(client)*
- Add Connect/Close with auth handshake *(client)*
- Add session operations (create, attach, detach, kill, write, resize, list, info) *(client)*
- Add OnData and OnEvent callbacks *(client)*
- Return snapshot in attach response for warm restore *(daemon)*
- Add AddonManager and wire into session creation *(session)*
- Add history.cold_restore setting (default false) *(config)*
- Add cold restore — persist metadata and scrollback when enabled *(daemon)*
- Add SessionHistory type for cold restore *(client)*
- Add LoadSessionHistory and CleanSessionHistory for cold restore *(client)*
- Add ForwardEnv method for environment variable forwarding *(client)*
- Load config.toml and wire WithColdRestore in daemon startup *(cmd)*
- Add WithMaxScrollbackSize option and wire from config *(daemon)*
- Add functional options with Option type and With* constructors *(client)*
- Add namespace-based path resolution *(client)*
- Add NewDaemon and ServeDaemon for embedded daemon support *(client)*
- Implement auto-start daemon in New() when no daemon running *(client)*
- Add token generation, verification, and file persistence *(auth)*
- Add Unix socket listener and peer credential verification *(ipc)*
- Add connection wrapper and new message types *(protocol)*
- Add new event types for Phase 2 *(event)*
- Add entity types, heartbeat manager, options, and ppid lookup *(transport)*
- Add metadata support and Phase 2 session changes *(session)*
- Add OSC parser, environment forwarding, and entity types *(daemon)*
- Add goframe config updates and client metadata module
- Add exec and exec-sync commands for programmatic session input
- Add MsgWait protocol message type
- Add WaitRequest and WaitResponse entity types to daemon
- Add waiter registry and handleWait with exit, idle, and match modes
- Add wmux wait CLI command with exit, idle, and match modes
- Add UntilExit, UntilIdle, UntilMatch to client SDK
- Add MsgRecord, MsgHistory, MsgHistoryEnd protocol message types
- Add recording and history entity types to daemon
- Add recording and history dump config fields
- Add internal/platform/ansi package with strip and HTML conversion
- Add internal/platform/recording package with asciinema v2 writer
- Add wmux record and history CLI commands
- Add RecordStart, RecordStop, History to client SDK
- Add Prefix field to session entity with validation
- Add prefix filter to handleList and MsgKillPrefix protocol type
- Implement handleKillPrefix daemon handler
- Add --prefix and --quiet flags to CLI list and kill commands
- Add List with WithListPrefix option and KillPrefix to client SDK
- Add OSCTypeShellReady to OSC parser for shell-ready detection
- Add ShellReady event type
- Add DA1/DA2 parser with request detection and response generation
- Add shell wrapper scripts for shell-ready OSC marker
- Hook DA1/DA2 auto-response into flushOutput for detached sessions
- Emit ShellReady event on OSC 777;wmux;shell-ready detection
- Add bash, zsh, and fish completion scripts for wmux
- Implement tmux shim binary with core command translations
- Add buildinfo package for version injection
- Add version subcommand with buildinfo
- Add build target with version ldflags

### Refactoring

- Move protocol package to internal/platform/protocol
- Move ADR templates from docs/decisions/ to decisions/
- Eliminate magic numbers, improve docs and readability *(addon-xterm)*
- Replace Connect(Options) with New(opts ...Option) *(client)*
- Migrate metadata_test.go to New(opts ...Option) *(client)*
- Dogfood client.NewDaemon in wmux CLI *(cmd)*
- Privatize internal sub-component constructors to fix goframe warnings

### Testing

- Increase coverage to 90% for config, pty, and protocol
- Increase session coverage to 90%
- Add error path tests for history package (93.1% coverage)
- Clean mock auth, add adapter integration tests for coverage *(client)*
- Add tmux shim unit tests for key translation and arg parsing


