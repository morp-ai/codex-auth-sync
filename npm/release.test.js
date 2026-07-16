import assert from "node:assert/strict";
import test from "node:test";

import { expectedChecksum, releaseAsset } from "./release.js";

test("releaseAsset maps every supported platform and architecture", () => {
  assert.equal(releaseAsset("linux", "x64"), "codex-auth-sync-linux-amd64");
  assert.equal(releaseAsset("linux", "arm64"), "codex-auth-sync-linux-arm64");
  assert.equal(releaseAsset("darwin", "x64"), "codex-auth-sync-darwin-amd64");
  assert.equal(releaseAsset("darwin", "arm64"), "codex-auth-sync-darwin-arm64");
  assert.equal(releaseAsset("win32", "x64"), "codex-auth-sync-windows-amd64.exe");
  assert.equal(releaseAsset("win32", "arm64"), "codex-auth-sync-windows-arm64.exe");
});

test("releaseAsset rejects unsupported targets", () => {
  assert.throws(() => releaseAsset("freebsd", "x64"), /Unsupported operating system/u);
  assert.throws(() => releaseAsset("linux", "ia32"), /Unsupported CPU architecture/u);
});

test("expectedChecksum selects an exact release asset", () => {
  const hashA = "a".repeat(64);
  const hashB = "b".repeat(64);
  const contents = `${hashA}  first\n${hashB} *second.exe\n`;
  assert.equal(expectedChecksum(contents, "first"), hashA);
  assert.throws(() => expectedChecksum(contents, "second.exe"), /does not contain/u);
});
