import xterm from "@xterm/headless";
const { Terminal } = xterm;
type TerminalInstance = InstanceType<typeof Terminal>;
import { encodeSnapshotPayload } from "./protocol.js";

export class InstanceManager {
  private instances: Map<string, TerminalInstance> = new Map();

  create(sessionId: string, cols: number, rows: number): void {
    if (this.instances.has(sessionId)) {
      return;
    }
    const term = new Terminal({ cols, rows, scrollback: 10000, allowProposedApi: true });
    this.instances.set(sessionId, term);
  }

  process(sessionId: string, data: Buffer): void {
    const term = this.instances.get(sessionId);
    if (!term) return;
    term.write(data);
  }

  snapshot(sessionId: string): Buffer {
    const term = this.instances.get(sessionId);
    if (!term) return encodeSnapshotPayload(Buffer.alloc(0), Buffer.alloc(0));

    const buffer = term.buffer.active;
    const lines: string[] = [];

    // Scrollback: lines from 0 to baseY
    for (let i = 0; i < buffer.baseY; i++) {
      const line = buffer.getLine(i);
      if (line) lines.push(line.translateToString(true));
    }
    const scrollback = Buffer.from(lines.join("\n"), "utf-8");

    // Viewport: lines from baseY to baseY + rows
    const vpLines: string[] = [];
    for (let i = buffer.baseY; i < buffer.baseY + term.rows; i++) {
      const line = buffer.getLine(i);
      if (line) vpLines.push(line.translateToString(true));
    }
    const viewport = Buffer.from(vpLines.join("\n"), "utf-8");

    return encodeSnapshotPayload(scrollback, viewport);
  }

  resize(sessionId: string, cols: number, rows: number): void {
    const term = this.instances.get(sessionId);
    if (!term) return;
    term.resize(cols, rows);
  }

  destroy(sessionId: string): void {
    const term = this.instances.get(sessionId);
    if (!term) return;
    term.dispose();
    this.instances.delete(sessionId);
  }

  destroyAll(): void {
    for (const [id] of this.instances) {
      this.destroy(id);
    }
  }

  size(): number {
    return this.instances.size;
  }
}
