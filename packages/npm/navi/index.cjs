const { spawn } = require("node:child_process");
const { existsSync } = require("node:fs");
const path = require("node:path");

const CHANNEL = "npm";

function executableName(platform = process.platform) {
  return platform === "win32" ? "navi.exe" : "navi";
}

function nativePackageName(platform = process.platform, arch = process.arch) {
  if (platform === "darwin" && arch === "arm64") return "open-navi-darwin-arm64";
  if (platform === "darwin" && arch === "x64") return "open-navi-darwin-x64";
  if (platform === "linux" && arch === "arm64") return "open-navi-linux-arm64";
  if (platform === "linux" && arch === "x64") return "open-navi-linux-x64";
  if (platform === "win32" && arch === "x64") return "open-navi-win32-x64";

  throw new Error(`Unsupported platform for open-navi native binaries: ${platform}-${arch}`);
}

function resolveNativeBinary(env = process.env, platform = process.platform, arch = process.arch) {
  const override = (env.NAVI_NATIVE_BIN || "").trim();
  if (override) {
    return override;
  }

  const bundled = path.join(__dirname, "bin", executableName(platform));
  if (existsSync(bundled)) {
    return bundled;
  }

  try {
    const nativePkgJson = require.resolve(`${nativePackageName(platform, arch)}/package.json`);
    const nativeRoot = path.dirname(nativePkgJson);
    const nativeBin = path.join(nativeRoot, "bin", executableName(platform));
    if (existsSync(nativeBin)) {
      return nativeBin;
    }
  } catch (err) {
    if (err && err.code !== "MODULE_NOT_FOUND") {
      throw err;
    }
  }

  throw new Error(
    "NAVI native binary was not found. Reinstall the open-navi npm package " +
      "with optional dependencies enabled, or set NAVI_NATIVE_BIN to a built navi executable."
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
  nativePackageName,
  resolveNativeBinary,
  run,
};
