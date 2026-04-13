import {
  decodeRequest,
  encodeResponse,
  readFrame,
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

const manager = new InstanceManager();

// Chunked input buffer: stores incoming pieces without copying on every chunk.
const chunks: Buffer[] = [];
let totalLen = 0;

function flushChunks(): Buffer {
  if (chunks.length === 1) {
    const buf = chunks[0];
    chunks.length = 0;
    totalLen = 0;
    return buf;
  }
  const buf = Buffer.concat(chunks, totalLen);
  chunks.length = 0;
  totalLen = 0;
  return buf;
}

function parseJSON(buf: Buffer): Record<string, unknown> | null {
  try {
    return JSON.parse(buf.toString()) as Record<string, unknown>;
  } catch {
    return null;
  }
}

function handleRequest(frame: Buffer): Buffer | null {
  const req = decodeRequest(frame);

  switch (req.method) {
    case METHOD_CREATE: {
      const params = parseJSON(req.payload);
      if (!params) {
        return encodeResponse({
          method: req.method,
          sessionId: req.sessionId,
          status: STATUS_ERROR,
          payload: Buffer.from("invalid JSON payload"),
        });
      }
      manager.create(
        req.sessionId,
        params.cols as number,
        params.rows as number
      );
      return encodeResponse({
        method: req.method,
        sessionId: req.sessionId,
        status: STATUS_OK,
        payload: Buffer.alloc(0),
      });
    }

    case METHOD_PROCESS: {
      manager.process(req.sessionId, req.payload);
      return null; // fire-and-forget, no response
    }

    case METHOD_SNAPSHOT: {
      const snap = manager.snapshot(req.sessionId);
      return encodeResponse({
        method: req.method,
        sessionId: req.sessionId,
        status: STATUS_OK,
        payload: snap,
      });
    }

    case METHOD_RESIZE: {
      const params = parseJSON(req.payload);
      if (!params) {
        return encodeResponse({
          method: req.method,
          sessionId: req.sessionId,
          status: STATUS_ERROR,
          payload: Buffer.from("invalid JSON payload"),
        });
      }
      manager.resize(
        req.sessionId,
        params.cols as number,
        params.rows as number
      );
      return encodeResponse({
        method: req.method,
        sessionId: req.sessionId,
        status: STATUS_OK,
        payload: Buffer.alloc(0),
      });
    }

    case METHOD_DESTROY: {
      manager.destroy(req.sessionId);
      return encodeResponse({
        method: req.method,
        sessionId: req.sessionId,
        status: STATUS_OK,
        payload: Buffer.alloc(0),
      });
    }

    case METHOD_SHUTDOWN: {
      manager.destroyAll();
      const resp = encodeResponse({
        method: req.method,
        sessionId: req.sessionId,
        status: STATUS_OK,
        payload: Buffer.alloc(0),
      });
      writeResponse(resp);
      process.exit(0);
    }

    default: {
      return encodeResponse({
        method: req.method,
        sessionId: req.sessionId,
        status: STATUS_ERROR,
        payload: Buffer.from("unknown method"),
      });
    }
  }
}

function writeResponse(resp: Buffer): void {
  const out = Buffer.alloc(4 + resp.length);
  out.writeUInt32BE(resp.length, 0);
  resp.copy(out, 4);
  process.stdout.write(out);
}

process.stdin.on("data", (chunk: Buffer) => {
  chunks.push(chunk);
  totalLen += chunk.length;

  let inputBuffer = flushChunks();
  let result: ReturnType<typeof readFrame>;

  while ((result = readFrame(inputBuffer, 0)) !== null) {
    const resp = handleRequest(result.frame);
    if (resp !== null) {
      writeResponse(resp);
    }
    inputBuffer = inputBuffer.subarray(result.newOffset);
  }

  // Put remainder back if there's leftover data
  if (inputBuffer.length > 0) {
    chunks.push(inputBuffer);
    totalLen = inputBuffer.length;
  }
});

process.stdin.on("end", () => {
  manager.destroyAll();
  process.exit(0);
});

process.stderr.write("wmux-emulator-xterm: started\n");
