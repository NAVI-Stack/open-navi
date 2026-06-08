const assert = require("node:assert/strict");
const path = require("node:path");
const test = require("node:test");

const cli = require("../index.cjs");

test("buildNativeEnv marks daemon control as npm-distributed", () => {
  const env = cli.buildNativeEnv({ PATH: "x", NAVI_DISTRIBUTION_CHANNEL: "dev" });

  assert.equal(env.PATH, "x");
  assert.equal(env.NAVI_DISTRIBUTION_CHANNEL, "npm");
});

test("resolveNativeBinary honors NAVI_NATIVE_BIN override", () => {
  const bin = path.join("C:", "navi", "bin", "navi.exe");

  assert.equal(cli.resolveNativeBinary({ NAVI_NATIVE_BIN: bin }), bin);
});

test("nativePackageName maps supported platforms", () => {
  assert.equal(cli.nativePackageName("darwin", "arm64"), "open-navi-darwin-arm64");
  assert.equal(cli.nativePackageName("darwin", "x64"), "open-navi-darwin-x64");
  assert.equal(cli.nativePackageName("linux", "arm64"), "open-navi-linux-arm64");
  assert.equal(cli.nativePackageName("linux", "x64"), "open-navi-linux-x64");
  assert.equal(cli.nativePackageName("win32", "x64"), "open-navi-win32-x64");
});

test("nativePackageName rejects unsupported platforms", () => {
  assert.throws(
    () => cli.nativePackageName("freebsd", "x64"),
    /Unsupported platform/
  );
});

test("resolveNativeBinary points users to open-navi and NAVI_NATIVE_BIN when missing", () => {
  assert.throws(
    () => cli.resolveNativeBinary({}),
    /Reinstall the open-navi npm package.*optional dependencies.*NAVI_NATIVE_BIN/s
  );
});
