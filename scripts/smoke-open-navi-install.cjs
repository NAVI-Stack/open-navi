#!/usr/bin/env node
"use strict";

const childProcess = require("child_process");
const fs = require("fs");
const os = require("os");
const path = require("path");

const repoRoot = path.resolve(__dirname, "..");
const currentPlatform = process.platform;
const currentArch = process.arch;

const platformTargets = {
  "darwin-arm64": {
    nativePackage: "open-navi-darwin-arm64",
    wheelPlatform: "macosx_11_0_arm64",
    nativeName: "navi",
  },
  "darwin-x64": {
    nativePackage: "open-navi-darwin-x64",
    wheelPlatform: "macosx_10_15_x86_64",
    nativeName: "navi",
  },
  "linux-arm64": {
    nativePackage: "open-navi-linux-arm64",
    wheelPlatform: "manylinux2014_aarch64",
    nativeName: "navi",
  },
  "linux-x64": {
    nativePackage: "open-navi-linux-x64",
    wheelPlatform: "manylinux2014_x86_64",
    nativeName: "navi",
  },
  "win32-x64": {
    nativePackage: "open-navi-win32-x64",
    wheelPlatform: "win_amd64",
    nativeName: "navi.exe",
  },
};

function parseArgs(argv) {
  const opts = {
    npmDist: path.join(repoRoot, "dist", "npm"),
    pythonDist: path.join(repoRoot, "python", "dist"),
    python: process.env.PYTHON || "python",
    workDir: os.tmpdir(),
    keep: false,
  };
  for (let i = 2; i < argv.length; i++) {
    const arg = argv[i];
    if (arg === "--npm-dist") {
      opts.npmDist = path.resolve(argv[++i]);
      continue;
    }
    if (arg === "--python-dist") {
      opts.pythonDist = path.resolve(argv[++i]);
      continue;
    }
    if (arg === "--python") {
      opts.python = argv[++i];
      continue;
    }
    if (arg === "--work-dir") {
      opts.workDir = path.resolve(argv[++i]);
      continue;
    }
    if (arg === "--keep") {
      opts.keep = true;
      continue;
    }
    throw new Error(`Unknown argument: ${arg}`);
  }
  return opts;
}

function readJson(relativePath) {
  return JSON.parse(fs.readFileSync(path.join(repoRoot, relativePath), "utf8"));
}

function command(name) {
  return process.platform === "win32" ? `${name}.cmd` : name;
}

function run(cmd, args, options = {}) {
  const result = childProcess.spawnSync(cmd, args, {
    encoding: "utf8",
    stdio: options.stdio || ["ignore", "pipe", "pipe"],
    cwd: options.cwd,
    env: options.env,
    shell: process.platform === "win32" && /\.(cmd|bat)$/i.test(cmd),
  });
  if (result.error) {
    throw new Error(`${cmd} ${args.join(" ")} failed to start: ${result.error.message}`);
  }
  if (result.status !== 0) {
    const output = [result.stdout, result.stderr].filter(Boolean).join("\n");
    throw new Error(`${cmd} ${args.join(" ")} failed with ${result.status}\n${output}`);
  }
  return result.stdout || "";
}

function requireFile(file, label = file) {
  if (!fs.existsSync(file)) {
    throw new Error(`Missing ${label}: ${file}`);
  }
}

function currentTarget() {
  const key = `${currentPlatform}-${currentArch}`;
  const target = platformTargets[key];
  if (!target) {
    throw new Error(`Unsupported smoke platform: ${key}`);
  }
  return { key, ...target };
}

function npmBinPath(installDir) {
  if (process.platform === "win32") {
    return path.join(installDir, "node_modules", ".bin", "navi.cmd");
  }
  return path.join(installDir, "node_modules", ".bin", "navi");
}

function pythonVenvPython(venvDir) {
  if (process.platform === "win32") {
    return path.join(venvDir, "Scripts", "python.exe");
  }
  return path.join(venvDir, "bin", "python");
}

function pythonConsoleScript(venvDir) {
  if (process.platform === "win32") {
    return path.join(venvDir, "Scripts", "navi.exe");
  }
  return path.join(venvDir, "bin", "navi");
}

function smokeNpm(opts, tempDir, version, target) {
  const npmDir = path.join(tempDir, "npm");
  fs.mkdirSync(npmDir, { recursive: true });
  const wrapperTarball = path.join(opts.npmDist, `open-navi-${version}.tgz`);
  const nativeTarball = path.join(opts.npmDist, `${target.nativePackage}-${version}.tgz`);
  requireFile(wrapperTarball, "open-navi npm tarball");
  requireFile(nativeTarball, `${target.nativePackage} npm tarball`);

  run(command("npm"), ["init", "-y"], { cwd: npmDir });
  run(
    command("npm"),
    [
      "install",
      "--ignore-scripts",
      "--no-audit",
      "--fund=false",
      "--cache",
      path.join(tempDir, "npm-cache"),
      nativeTarball,
      wrapperTarball,
    ],
    { cwd: npmDir }
  );

  requireFile(npmBinPath(npmDir), "installed npm navi command shim");

  const code = `
const navi = require("open-navi");
const resolved = navi.resolveNativeBinary({});
if (!resolved.includes(${JSON.stringify(target.nativePackage)})) {
  throw new Error("unexpected native package path: " + resolved);
}
if (!resolved.endsWith(${JSON.stringify(path.join("bin", target.nativeName))})) {
  throw new Error("unexpected native executable path: " + resolved);
}
const env = navi.buildNativeEnv({});
if (env.NAVI_DISTRIBUTION_CHANNEL !== "npm") {
  throw new Error("unexpected npm distribution channel: " + env.NAVI_DISTRIBUTION_CHANNEL);
}
console.log(resolved);
`;
  const resolved = run("node", ["-e", code], { cwd: npmDir }).trim();
  return { dir: npmDir, resolved };
}

function smokePython(opts, tempDir, version, target) {
  const venvDir = path.join(tempDir, "venv");
  const wheel = path.join(opts.pythonDist, `open_navi-${version}-py3-none-${target.wheelPlatform}.whl`);
  requireFile(wheel, `open-navi ${target.wheelPlatform} wheel`);

  run(opts.python, ["-m", "venv", venvDir]);
  const venvPython = pythonVenvPython(venvDir);
  requireFile(venvPython, "Python virtualenv interpreter");
  run(venvPython, [
    "-m",
    "pip",
    "install",
    "--no-index",
    "--no-deps",
    "--only-binary",
    ":all:",
    "--find-links",
    opts.pythonDist,
    `open-navi==${version}`,
  ]);

  requireFile(pythonConsoleScript(venvDir), "installed pip navi command");

  const code = `
from navi import cli
resolved = cli.resolve_native_binary({})
if not resolved.endswith(${JSON.stringify(path.join("bin", target.nativeName))}):
    raise SystemExit(f"unexpected native executable path: {resolved}")
env = cli.build_native_env({})
if env.get("NAVI_DISTRIBUTION_CHANNEL") != "pip":
    raise SystemExit(f"unexpected pip distribution channel: {env.get('NAVI_DISTRIBUTION_CHANNEL')}")
print(resolved)
`;
  const resolved = run(venvPython, ["-c", code]).trim();
  return { dir: venvDir, resolved };
}

function main() {
  const opts = parseArgs(process.argv);
  const version = readJson("packages/npm/navi/package.json").version;
  const target = currentTarget();
  const tempDir = fs.mkdtempSync(path.join(opts.workDir, "open-navi-install-smoke-"));
  try {
    const npmResult = smokeNpm(opts, tempDir, version, target);
    const pythonResult = smokePython(opts, tempDir, version, target);
    console.log(`open-navi install smoke passed for ${target.key}.`);
    console.log(`npm resolved: ${npmResult.resolved}`);
    console.log(`pip resolved: ${pythonResult.resolved}`);
    if (opts.keep) {
      console.log(`kept smoke directory: ${tempDir}`);
    }
  } finally {
    if (!opts.keep) {
      fs.rmSync(tempDir, { recursive: true, force: true });
    }
  }
}

try {
  main();
} catch (err) {
  console.error(err.message || String(err));
  process.exit(1);
}
