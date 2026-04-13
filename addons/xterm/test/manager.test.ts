import { describe, it, beforeEach } from "node:test";
import * as assert from "node:assert/strict";
import { InstanceManager } from "../src/manager.js";

describe("InstanceManager", () => {
  let mgr: InstanceManager;

  beforeEach(() => {
    mgr = new InstanceManager();
  });

  it("creates and destroys instances", () => {
    mgr.create("s1", 80, 24);
    assert.equal(mgr.size(), 1);

    mgr.destroy("s1");
    assert.equal(mgr.size(), 0);
  });

  it("ignores duplicate create for the same session ID", () => {
    mgr.create("s1", 80, 24);
    mgr.create("s1", 120, 40); // should be a no-op
    assert.equal(mgr.size(), 1);
  });

  it("processes data and produces a non-empty snapshot", () => {
    mgr.create("s1", 80, 24);
    mgr.process("s1", Buffer.from("hello world\r\n"));

    const snapshot = mgr.snapshot("s1");
    assert.ok(snapshot.length > 0, "snapshot should contain data");
  });

  it("returns an empty snapshot for an unknown session", () => {
    const snapshot = mgr.snapshot("nonexistent");
    // Empty snapshot: [scrollback_len:4(=0)] — at least 4 bytes with value 0.
    assert.equal(snapshot.readUInt32BE(0), 0);
  });

  it("resizes an instance without errors", () => {
    mgr.create("s1", 80, 24);
    mgr.resize("s1", 120, 40);
    // Resize is a side-effect operation; success = no throw.
  });

  it("silently ignores operations on unknown sessions", () => {
    // None of these should throw.
    mgr.process("unknown", Buffer.from("data"));
    mgr.resize("unknown", 80, 24);
    mgr.destroy("unknown");
  });

  it("destroyAll cleans up every instance", () => {
    mgr.create("s1", 80, 24);
    mgr.create("s2", 80, 24);
    mgr.create("s3", 80, 24);
    assert.equal(mgr.size(), 3);

    mgr.destroyAll();
    assert.equal(mgr.size(), 0);
  });
});
