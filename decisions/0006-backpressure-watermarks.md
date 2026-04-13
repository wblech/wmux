---
status: accepted
date: 2026-04-11
decision-makers: wblech
---

# High/Low Watermark Backpressure

## Context and Problem Statement

Programs that produce output faster than clients can consume (e.g., `cat` on a large file) will cause unbounded memory growth in the daemon's output buffer. The daemon needs flow control.

## Considered Options

* Fixed-size ring buffer (drop old data)
* Backpressure with high/low watermarks (pause/resume PTY reads)
* Rate limiting output per second

## Decision Outcome

Chosen option: "High/low watermark backpressure", because it preserves all data (no drops), automatically adapts to client speed, and uses the proven TCP-style flow control pattern. When the buffer reaches the high watermark (default 1MB), PTY reads are paused. When it drains to the low watermark (default 256KB), reads resume.

### Consequences

* Good, because zero data loss — all PTY output is eventually delivered
* Good, because bounded memory usage per session
* Bad, because pausing PTY reads can cause the program to block on write() — but this is the correct backpressure signal
