const { spawn } = require("node:child_process");
const { existsSync } = require("node:fs");
const path = require("node:path");

const CHANNEL = "npm";

function executableName(platform = process.platform) {
  return platform === "win32" ? "navi.exe" : "navi";
}

function resolveNativeBinary(env = process.env) {
  const override = (env.NAVI_NATIVE_BIN || "").trim();
  if (override) {
    return override;
  }

  const bundled = path.join(__dirname, "bin", executableName());
  if (existsSync(bundled)) {
    return bundled;
  }

  throw new Error(
    "NAVI native binary was not found. Reinstall the navi npm package, " +
      "or set NAVI_NATIVE_BIN to a built navi executable."
  );
}

function buildNativeEnv(base = process.env) {
  return {
    ...base,
    NAVI_DISTRIBUTION_CHANNEL: CHANNEL,
  };
}

function run(args = process.argv.slice(2), options = {}) {
  const env = buildNativeEnv(options.env || process.env);
  const bin = resolveNativeBinary(env);
  return spawn(bin, args, {
    stdio: options.stdio || "inherit",
    env,
  });
}

module.exports = {
  buildNativeEnv,
  executableName,
  resolveNativeBinary,
  run,
};
