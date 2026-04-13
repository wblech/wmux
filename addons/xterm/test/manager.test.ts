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

  it("ignores duplicate create", () => {
    mgr.create("s1", 80, 24);
    mgr.create("s1", 120, 40);
    assert.equal(mgr.size(), 1);
  });

  it("processes data and returns snapshot", () => {
    mgr.create("s1", 80, 24);
    mgr.process("s1", Buffer.from("hello world\r\n"));
    const snap = mgr.snapshot("s1");
    assert.ok(snap.length > 0);
  });

  it("returns empty snapshot for unknown session", () => {
    const snap = mgr.snapshot("unknown");
    assert.equal(snap.readUInt32BE(0), 0);
  });

  it("resizes instance", () => {
    mgr.create("s1", 80, 24);
    mgr.resize("s1", 120, 40);
    // No assertion beyond no-crash
  });

  it("destroyAll cleans up all instances", () => {
    mgr.create("s1", 80, 24);
    mgr.create("s2", 80, 24);
    mgr.create("s3", 80, 24);
    mgr.destroyAll();
    assert.equal(mgr.size(), 0);
  });
});
