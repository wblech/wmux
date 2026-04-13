/**
 * Manages headless xterm.js terminal instances, one per wmux session.
 *
 * Each instance processes raw PTY output and can produce a two-phase
 * snapshot (scrollback history + visible viewport) on demand.
 */

// CJS interop: @xterm/headless ships a CJS default export.
// With ESM + esModuleInterop we get the namespace object, so we
// destructure Terminal from it.
import xterm from "@xterm/headless";
const { Terminal } = xterm;
type TerminalInstance = InstanceType<typeof Terminal>;

import { encodeSnapshotPayload, EMPTY_PAYLOAD } from "./protocol.js";

/** Maximum number of scrollback lines retained per terminal. */
const MAX_SCROLLBACK_LINES = 10_000;

/**
 * Pre-built empty snapshot returned when a session doesn't exist
 * or the terminal has no buffer. Allocated once to avoid repeated work.
 */
const EMPTY_SNAPSHOT = encodeSnapshotPayload(EMPTY_PAYLOAD, EMPTY_PAYLOAD);

export class InstanceManager {
  private readonly instances = new Map<string, TerminalInstance>();

  /** Create a new headless terminal for the given session. Duplicate IDs are ignored. */
  create(sessionId: string, cols: number, rows: number): void {
    if (this.instances.has(sessionId)) {
      return;
    }

    const term = new Terminal({
      cols,
      rows,
      scrollback: MAX_SCROLLBACK_LINES,
      // Required for buffer.active access in xterm.js >=5.
      allowProposedApi: true,
    });

    this.instances.set(sessionId, term);
  }

  /** Feed raw PTY output into the terminal's state machine. */
  process(sessionId: string, data: Buffer): void {
    const term = this.instances.get(sessionId);
    if (!term) return;
    term.write(data);
  }

  /**
   * Capture the terminal's current state as a two-phase snapshot.
   *
   * - **Scrollback**: lines [0, baseY) — content that has scrolled off screen.
   * - **Viewport**: lines [baseY, baseY + rows) — the visible screen content.
   *
   * Returns the encoded snapshot payload ready to be sent as a response.
   */
  snapshot(sessionId: string): Buffer {
    const term = this.instances.get(sessionId);
    if (!term) return EMPTY_SNAPSHOT;

    const buffer = term.buffer.active;
    if (!buffer) return EMPTY_SNAPSHOT;

    const scrollbackLines = extractLines(buffer, 0, buffer.baseY);
    const scrollback = Buffer.from(scrollbackLines.join("\n"), "utf-8");

    const viewportStart = buffer.baseY;
    const viewportEnd = buffer.baseY + term.rows;
    const viewportLines = extractLines(buffer, viewportStart, viewportEnd);
    const viewport = Buffer.from(viewportLines.join("\n"), "utf-8");

    return encodeSnapshotPayload(scrollback, viewport);
  }

  /** Resize the terminal to new dimensions. */
  resize(sessionId: string, cols: number, rows: number): void {
    const term = this.instances.get(sessionId);
    if (!term) return;
    term.resize(cols, rows);
  }

  /** Dispose and remove a terminal instance. */
  destroy(sessionId: string): void {
    const term = this.instances.get(sessionId);
    if (!term) return;
    term.dispose();
    this.instances.delete(sessionId);
  }

  /** Dispose and remove all terminal instances. */
  destroyAll(): void {
    const ids = [...this.instances.keys()];
    for (const id of ids) {
      this.destroy(id);
    }
  }

  /** Return the number of active terminal instances. */
  size(): number {
    return this.instances.size;
  }
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/**
 * Extract lines from an xterm buffer between [startRow, endRow).
 * Each line is trimmed of trailing whitespace via translateToString(true).
 */
function extractLines(
  buffer: TerminalInstance["buffer"]["active"],
  startRow: number,
  endRow: number
): string[] {
  const lines: string[] = [];
  for (let i = startRow; i < endRow; i++) {
    const line = buffer.getLine(i);
    if (line) {
      lines.push(line.translateToString(true));
    }
  }
  return lines;
}
