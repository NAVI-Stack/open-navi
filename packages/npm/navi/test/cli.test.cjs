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

