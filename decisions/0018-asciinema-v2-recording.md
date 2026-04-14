---
status: accepted
date: 2026-04-14
decision-makers: wblech
---

# Asciinema v2 Format for Session Recording

## Context and Problem Statement

wmux needs a recording format for terminal sessions that captures PTY output with timing information. Recordings are used for replay, debugging, and audit. The format must support streaming writes (append-only during recording) and be usable outside of wmux.

## Decision Drivers

* Ecosystem compatibility — recordings should be playable without wmux-specific tooling
* Streaming writes — the daemon appends events as they happen, no post-processing
* Simplicity — easy to parse and generate
* Size limits — must support configurable max file size with graceful cutoff

## Considered Options

* Custom binary format (matches wmux wire protocol style)
* Asciinema v1 (JSON object with stdout array)
* Asciinema v2 (NDJSON: header line + `[timestamp, "o", data]` events)
* script(1) typescript format

## Decision Outcome

Chosen option: "Asciinema v2", because it is the de facto standard for terminal recordings, with broad tooling support (asciinema player, asciinema.org, svg-term, third-party players). The NDJSON format is naturally append-only — each output event is one JSON line — which maps directly to the daemon's streaming output broadcast. No buffering or post-processing is needed.

### Consequences

* Good, because recordings are immediately playable with `asciinema play`, web player, and dozens of third-party tools
* Good, because NDJSON is append-only — recording is a simple `fmt.Fprintf` per output event with no buffering
* Good, because max file size enforcement is trivial — check size before each append, stop when exceeded
* Good, because the format is human-readable (JSON lines) for debugging
* Bad, because JSON encoding of binary terminal data inflates file size (~33% for base64, though asciinema v2 uses escaped strings which is less)
* Bad, because the format only captures output timing, not input — input recording would need a separate mechanism if ever needed

### Confirmation

Implemented in `internal/platform/recording/` with streaming writer. Recordings validated with `asciinema play` and the asciinema web player.
