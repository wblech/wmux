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
let inputBuffer = Buffer.alloc(0);

function handleRequest(frame: Buffer): Buffer | null {
  const req = decodeRequest(frame);

  switch (req.method) {
    case METHOD_CREATE: {
      const params = JSON.parse(req.payload.toString());
      manager.create(req.sessionId, params.cols, params.rows);
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
      const params = JSON.parse(req.payload.toString());
      manager.resize(req.sessionId, params.cols, params.rows);
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
  const lenBuf = Buffer.alloc(4);
  lenBuf.writeUInt32BE(resp.length, 0);
  process.stdout.write(lenBuf);
  process.stdout.write(resp);
}

process.stdin.on("data", (chunk: Buffer) => {
  inputBuffer = Buffer.concat([inputBuffer, chunk]);

  let result: ReturnType<typeof readFrame>;
  while ((result = readFrame(inputBuffer, 0)) !== null) {
    const resp = handleRequest(result.frame);
    if (resp !== null) {
      writeResponse(resp);
    }
    inputBuffer = inputBuffer.subarray(result.newOffset);
  }
});

process.stdin.on("end", () => {
  manager.destroyAll();
  process.exit(0);
});

process.stderr.write("wmux-emulator-xterm: started\n");
