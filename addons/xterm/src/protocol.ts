export const METHOD_CREATE = 0x01;
export const METHOD_PROCESS = 0x02;
export const METHOD_SNAPSHOT = 0x03;
export const METHOD_RESIZE = 0x04;
export const METHOD_DESTROY = 0x05;
export const METHOD_SHUTDOWN = 0x06;

export const STATUS_OK = 0x00;
export const STATUS_ERROR = 0x01;

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

/**
 * Decode a request frame (after length prefix has been stripped):
 * [method:1][session_id_len:1][session_id:N][payload_len:4][payload:N]
 */
export function decodeRequest(buf: Buffer): AddonRequest {
  let offset = 0;
  const method = buf[offset++];
  const idLen = buf[offset++];
  const sessionId = buf.subarray(offset, offset + idLen).toString("utf-8");
  offset += idLen;
  const payloadLen = buf.readUInt32BE(offset);
  offset += 4;
  const payload = buf.subarray(offset, offset + payloadLen);
  return { method, sessionId, payload };
}

/**
 * Encode a response frame:
 * [method:1][session_id_len:1][session_id:N][status:1][payload_len:4][payload:N]
 */
export function encodeResponse(resp: AddonResponse): Buffer {
  const idBuf = Buffer.from(resp.sessionId, "utf-8");
  const totalLen = 1 + 1 + idBuf.length + 1 + 4 + resp.payload.length;
  const buf = Buffer.alloc(totalLen);
  let offset = 0;
  buf[offset++] = resp.method;
  buf[offset++] = idBuf.length;
  idBuf.copy(buf, offset);
  offset += idBuf.length;
  buf[offset++] = resp.status;
  buf.writeUInt32BE(resp.payload.length, offset);
  offset += 4;
  resp.payload.copy(buf, offset);
  return buf;
}

/**
 * Encode a snapshot payload: [scrollback_len:4][scrollback:N][viewport:N]
 */
export function encodeSnapshotPayload(
  scrollback: Buffer,
  viewport: Buffer
): Buffer {
  const buf = Buffer.alloc(4 + scrollback.length + viewport.length);
  buf.writeUInt32BE(scrollback.length, 0);
  scrollback.copy(buf, 4);
  viewport.copy(buf, 4 + scrollback.length);
  return buf;
}

/**
 * Read one length-prefixed frame from a buffer starting at offset.
 * Returns the frame and new offset, or null if not enough data.
 */
export function readFrame(
  buf: Buffer,
  offset: number
): { frame: Buffer; newOffset: number } | null {
  if (buf.length - offset < 4) return null;
  const frameLen = buf.readUInt32BE(offset);
  if (buf.length - offset - 4 < frameLen) return null;
  const frame = buf.subarray(offset + 4, offset + 4 + frameLen);
  return { frame, newOffset: offset + 4 + frameLen };
}
