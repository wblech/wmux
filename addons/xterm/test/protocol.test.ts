import { describe, it } from "node:test";
import * as assert from "node:assert/strict";
import {
  decodeRequest,
  encodeResponse,
  encodeSnapshotPayload,
  readFrame,
  FRAME_LENGTH_PREFIX_SIZE,
  EMPTY_PAYLOAD,
  METHOD_CREATE,
  METHOD_SNAPSHOT,
  STATUS_OK,
} from "../src/protocol.js";

describe("protocol", () => {
  describe("decodeRequest", () => {
    it("decodes a CREATE request with JSON payload", () => {
      const payload = Buffer.from('{"cols":80,"rows":24}');
      const sessionId = "s1";
      const idBuf = Buffer.from(sessionId);

      // Build: [method:1][id_len:1][id:N][payload_len:4][payload:M]
      const buf = Buffer.alloc(1 + 1 + idBuf.length + 4 + payload.length);
      let offset = 0;
      buf[offset++] = METHOD_CREATE;
      buf[offset++] = idBuf.length;
      idBuf.copy(buf, offset);
      offset += idBuf.length;
      buf.writeUInt32BE(payload.length, offset);
      offset += 4;
      payload.copy(buf, offset);

      const req = decodeRequest(buf);
      assert.equal(req.method, METHOD_CREATE);
      assert.equal(req.sessionId, "s1");
      assert.deepEqual(JSON.parse(req.payload.toString()), {
        cols: 80,
        rows: 24,
      });
    });
  });

  describe("encodeResponse", () => {
    it("encodes a response with correct wire layout", () => {
      const resp = encodeResponse({
        method: METHOD_CREATE,
        sessionId: "s1",
        status: STATUS_OK,
        payload: EMPTY_PAYLOAD,
      });

      // [method:1][id_len:1][id:2][status:1][payload_len:4]
      assert.equal(resp[0], METHOD_CREATE);
      assert.equal(resp[1], 2); // "s1".length
      assert.equal(resp.subarray(2, 4).toString(), "s1");
      assert.equal(resp[4], STATUS_OK);
      assert.equal(resp.readUInt32BE(5), 0); // empty payload
    });
  });

  describe("encodeSnapshotPayload", () => {
    it("roundtrips scrollback and viewport", () => {
      const scrollback = Buffer.from("scrollback-content");
      const viewport = Buffer.from("viewport-content");
      const encoded = encodeSnapshotPayload(scrollback, viewport);

      const scrollbackLen = encoded.readUInt32BE(0);
      assert.equal(scrollbackLen, scrollback.length);

      const decodedScrollback = encoded.subarray(4, 4 + scrollbackLen);
      assert.deepEqual(decodedScrollback, scrollback);

      const decodedViewport = encoded.subarray(4 + scrollbackLen);
      assert.deepEqual(decodedViewport, viewport);
    });
  });

  describe("readFrame", () => {
    it("reads a complete length-prefixed frame", () => {
      const body = Buffer.from("hello");
      const buf = Buffer.alloc(FRAME_LENGTH_PREFIX_SIZE + body.length);
      buf.writeUInt32BE(body.length, 0);
      body.copy(buf, FRAME_LENGTH_PREFIX_SIZE);

      const result = readFrame(buf, 0);
      assert.ok(result);
      assert.deepEqual(result.frame, body);
      assert.equal(result.newOffset, FRAME_LENGTH_PREFIX_SIZE + body.length);
    });

    it("returns null when the buffer is too short for the length prefix", () => {
      const buf = Buffer.from([0, 0]);
      assert.equal(readFrame(buf, 0), null);
    });

    it("returns null when the frame body is incomplete", () => {
      // Length says 10 bytes, but only 2 bytes of body present.
      const buf = Buffer.from([0, 0, 0, 10, 1, 2]);
      assert.equal(readFrame(buf, 0), null);
    });
  });
});
