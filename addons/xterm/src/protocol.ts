/**
 * Binary protocol for communication between the wmux daemon and the xterm addon.
 *
 * Transport: length-prefixed frames over stdin/stdout pipes.
 * Each frame is preceded by a 4-byte big-endian uint32 containing the frame length.
 *
 * Request wire format:
 *   [method:1][session_id_len:1][session_id:N][payload_len:4][payload:M]
 *
 * Response wire format:
 *   [method:1][session_id_len:1][session_id:N][status:1][payload_len:4][payload:M]
 *
 * Snapshot payload format:
 *   [scrollback_len:4][scrollback:N][viewport:M]
 */

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

/** Size of the length prefix that wraps every frame on the wire. */
export const FRAME_LENGTH_PREFIX_SIZE = 4;

/** Size of a uint32 field (payload_len, scrollback_len). */
const UINT32_SIZE = 4;

/** Size of a single-byte field (method, session_id_len, status). */
const BYTE_SIZE = 1;

// -- Methods ----------------------------------------------------------------

/** Daemon asks addon to create a new terminal instance. */
export const METHOD_CREATE = 0x01;
/** Daemon sends raw PTY output for the addon to process. */
export const METHOD_PROCESS = 0x02;
/** Daemon requests a scrollback + viewport snapshot. */
export const METHOD_SNAPSHOT = 0x03;
/** Daemon tells the addon to resize a terminal instance. */
export const METHOD_RESIZE = 0x04;
/** Daemon tells the addon to destroy a terminal instance. */
export const METHOD_DESTROY = 0x05;
/** Daemon tells the addon to shut down all instances and exit. */
export const METHOD_SHUTDOWN = 0x06;

// -- Status -----------------------------------------------------------------

/** The request completed successfully. */
export const STATUS_OK = 0x00;
/** The request failed; payload contains an error message. */
export const STATUS_ERROR = 0x01;

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface AddonRequest {
  method: number;
  sessionId: string;
  payload: Buffer;
}

export interface AddonResponse {
  method: number;
  sessionId: string;
  status: number;
  payload: Buffer;
}

/** Reusable empty payload to avoid allocations. */
export const EMPTY_PAYLOAD = Buffer.alloc(0);

// ---------------------------------------------------------------------------
// Codec
// ---------------------------------------------------------------------------

/**
 * Decode a request frame (after the length prefix has been stripped).
 *
 * Layout: [method:1][id_len:1][id:N][payload_len:4][payload:M]
 */
export function decodeRequest(buf: Buffer): AddonRequest {
  let offset = 0;

  const method = buf[offset];
  offset += BYTE_SIZE;

  const idLen = buf[offset];
  offset += BYTE_SIZE;

  const sessionId = buf.subarray(offset, offset + idLen).toString("utf-8");
  offset += idLen;

  const payloadLen = buf.readUInt32BE(offset);
  offset += UINT32_SIZE;

  const payload = buf.subarray(offset, offset + payloadLen);

  return { method, sessionId, payload };
}

/**
 * Encode a response frame (without the length prefix).
 *
 * Layout: [method:1][id_len:1][id:N][status:1][payload_len:4][payload:M]
 */
export function encodeResponse(resp: AddonResponse): Buffer {
  const idBuf = Buffer.from(resp.sessionId, "utf-8");
  const headerSize =
    BYTE_SIZE + BYTE_SIZE + idBuf.length + BYTE_SIZE + UINT32_SIZE;
  const buf = Buffer.alloc(headerSize + resp.payload.length);

  let offset = 0;

  buf[offset] = resp.method;
  offset += BYTE_SIZE;

  buf[offset] = idBuf.length;
  offset += BYTE_SIZE;

  idBuf.copy(buf, offset);
  offset += idBuf.length;

  buf[offset] = resp.status;
  offset += BYTE_SIZE;

  buf.writeUInt32BE(resp.payload.length, offset);
  offset += UINT32_SIZE;

  resp.payload.copy(buf, offset);

  return buf;
}

/**
 * Encode a snapshot payload from scrollback and viewport buffers.
 *
 * Layout: [scrollback_len:4][scrollback:N][viewport:M]
 */
export function encodeSnapshotPayload(
  scrollback: Buffer,
  viewport: Buffer
): Buffer {
  const buf = Buffer.alloc(UINT32_SIZE + scrollback.length + viewport.length);
  buf.writeUInt32BE(scrollback.length, 0);
  scrollback.copy(buf, UINT32_SIZE);
  viewport.copy(buf, UINT32_SIZE + scrollback.length);
  return buf;
}

/**
 * Try to read one length-prefixed frame starting at `offset` in `buf`.
 *
 * Returns the frame and the new offset past it, or null if the buffer
 * does not yet contain a complete frame.
 */
export function readFrame(
  buf: Buffer,
  offset: number
): { frame: Buffer; newOffset: number } | null {
  const remaining = buf.length - offset;

  if (remaining < FRAME_LENGTH_PREFIX_SIZE) {
    return null;
  }

  const frameLen = buf.readUInt32BE(offset);

  if (remaining - FRAME_LENGTH_PREFIX_SIZE < frameLen) {
    return null;
  }

  const frameStart = offset + FRAME_LENGTH_PREFIX_SIZE;
  const frame = buf.subarray(frameStart, frameStart + frameLen);

  return { frame, newOffset: frameStart + frameLen };
}
