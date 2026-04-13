import { describe, it } from "node:test";
import * as assert from "node:assert/strict";
import {
  decodeRequest,
  encodeResponse,
  encodeSnapshotPayload,
  readFrame,
  METHOD_CREATE,
  METHOD_SNAPSHOT,
  STATUS_OK,
} from "../src/protocol.js";

describe("protocol", () => {
  it("decodes a create request", () => {
    const payload = Buffer.from('{"cols":80,"rows":24}');
    const buf = Buffer.alloc(1 + 1 + 2 + 4 + payload.length);
    let offset = 0;
    buf[offset++] = METHOD_CREATE;
    buf[offset++] = 2;
    buf.write("s1", offset, "utf-8");
    offset += 2;
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

  it("encodes a response", () => {
    const resp = encodeResponse({
      method: METHOD_CREATE,
      sessionId: "s1",
      status: STATUS_OK,
      payload: Buffer.alloc(0),
    });
    assert.equal(resp[0], METHOD_CREATE);
    assert.equal(resp[1], 2);
    assert.equal(resp.subarray(2, 4).toString(), "s1");
    assert.equal(resp[4], STATUS_OK);
    assert.equal(resp.readUInt32BE(5), 0);
  });

  it("encodes snapshot payload roundtrip", () => {
    const sb = Buffer.from("scrollback");
    const vp = Buffer.from("viewport");
    const encoded = encodeSnapshotPayload(sb, vp);
    const sbLen = encoded.readUInt32BE(0);
    assert.equal(sbLen, sb.length);
    assert.deepEqual(encoded.subarray(4, 4 + sbLen), sb);
    assert.deepEqual(encoded.subarray(4 + sbLen), vp);
  });

  it("reads a length-prefixed frame", () => {
    const inner = Buffer.from("hello");
    const buf = Buffer.alloc(4 + inner.length);
    buf.writeUInt32BE(inner.length, 0);
    inner.copy(buf, 4);
    const result = readFrame(buf, 0);
    assert.ok(result);
    assert.deepEqual(result.frame, inner);
    assert.equal(result.newOffset, 4 + inner.length);
  });

  it("returns null when frame is incomplete", () => {
    const buf = Buffer.from([0, 0, 0, 10, 1, 2]);
    assert.equal(readFrame(buf, 0), null);
  });
});
