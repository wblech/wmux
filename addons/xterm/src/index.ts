/**
 * wmux xterm addon — headless terminal emulator process.
 *
 * Runs as a child process of the wmux daemon, communicating over stdin/stdout
 * using a binary protocol with length-prefixed frames. Manages N xterm.js
 * headless instances (one per session) in a single Node.js process.
 *
 * Lifecycle: the daemon spawns this process once and reuses it for all sessions.
 * When the daemon shuts down (or stdin closes), all instances are destroyed.
 */

import {
  type AddonRequest,
  decodeRequest,
  encodeResponse,
  readFrame,
  FRAME_LENGTH_PREFIX_SIZE,
  EMPTY_PAYLOAD,
  METHOD_CREATE,
  METHOD_PROCESS,
  METHOD_SNAPSHOT,
  METHOD_RESIZE,
  METHOD_DESTROY,
  METHOD_SHUTDOWN,
  STATUS_OK,
  STATUS_ERROR,
} from "./protocol.js";
import { InstanceManager } from "./manager.js";

// ---------------------------------------------------------------------------
// Input buffering
// ---------------------------------------------------------------------------

/**
 * Incoming data arrives in arbitrarily-sized chunks. We accumulate chunks
 * without copying until we need to parse, then concatenate once per batch.
 */
const inputChunks: Buffer[] = [];
let inputLength = 0;

function flushInputChunks(): Buffer {
  if (inputChunks.length === 1) {
    const buf = inputChunks[0];
    inputChunks.length = 0;
    inputLength = 0;
    return buf;
  }
  const buf = Buffer.concat(inputChunks, inputLength);
  inputChunks.length = 0;
  inputLength = 0;
  return buf;
}

// ---------------------------------------------------------------------------
// Request handling
// ---------------------------------------------------------------------------

const manager = new InstanceManager();

interface TerminalParams {
  cols: number;
  rows: number;
}

function parseTerminalParams(payload: Buffer): TerminalParams | null {
  try {
    const parsed = JSON.parse(payload.toString()) as TerminalParams;
    if (typeof parsed.cols !== "number" || typeof parsed.rows !== "number") {
      return null;
    }
    return parsed;
  } catch {
    return null;
  }
}

function okResponse(req: AddonRequest, payload: Buffer = EMPTY_PAYLOAD): Buffer {
  return encodeResponse({
    method: req.method,
    sessionId: req.sessionId,
    status: STATUS_OK,
    payload,
  });
}

function errorResponse(req: AddonRequest, message: string): Buffer {
  return encodeResponse({
    method: req.method,
    sessionId: req.sessionId,
    status: STATUS_ERROR,
    payload: Buffer.from(message),
  });
}

/**
 * Process a single decoded request frame and return the response to send,
 * or null for fire-and-forget methods (PROCESS).
 */
function handleRequest(frame: Buffer): Buffer | null {
  const req = decodeRequest(frame);

  switch (req.method) {
    case METHOD_CREATE: {
      const params = parseTerminalParams(req.payload);
      if (!params) {
        return errorResponse(req, "invalid JSON payload");
      }
      manager.create(req.sessionId, params.cols, params.rows);
      return okResponse(req);
    }

    case METHOD_PROCESS: {
      manager.process(req.sessionId, req.payload);
      return null; // fire-and-forget — no response expected
    }

    case METHOD_SNAPSHOT: {
      const snapshot = manager.snapshot(req.sessionId);
      return okResponse(req, snapshot);
    }

    case METHOD_RESIZE: {
      const params = parseTerminalParams(req.payload);
      if (!params) {
        return errorResponse(req, "invalid JSON payload");
      }
      manager.resize(req.sessionId, params.cols, params.rows);
      return okResponse(req);
    }

    case METHOD_DESTROY: {
      manager.destroy(req.sessionId);
      return okResponse(req);
    }

    case METHOD_SHUTDOWN: {
      manager.destroyAll();
      writeFrame(okResponse(req));
      process.exit(0);
    }

    default: {
      return errorResponse(req, "unknown method");
    }
  }
}

// ---------------------------------------------------------------------------
// Wire I/O
// ---------------------------------------------------------------------------

/** Write a length-prefixed response frame to stdout. */
function writeFrame(responseBody: Buffer): void {
  const frame = Buffer.alloc(FRAME_LENGTH_PREFIX_SIZE + responseBody.length);
  frame.writeUInt32BE(responseBody.length, 0);
  responseBody.copy(frame, FRAME_LENGTH_PREFIX_SIZE);
  process.stdout.write(frame);
}

// ---------------------------------------------------------------------------
// Main loop
// ---------------------------------------------------------------------------

process.stdin.on("data", (chunk: Buffer) => {
  inputChunks.push(chunk);
  inputLength += chunk.length;

  let pending = flushInputChunks();
  let parsed: ReturnType<typeof readFrame>;

  while ((parsed = readFrame(pending, 0)) !== null) {
    const response = handleRequest(parsed.frame);
    if (response !== null) {
      writeFrame(response);
    }
    pending = pending.subarray(parsed.newOffset);
  }

  // Stash any incomplete frame data for the next chunk.
  if (pending.length > 0) {
    inputChunks.push(pending);
    inputLength = pending.length;
  }
});

process.stdin.on("end", () => {
  manager.destroyAll();
  process.exit(0);
});

process.stderr.write("wmux-emulator-xterm: started\n");
