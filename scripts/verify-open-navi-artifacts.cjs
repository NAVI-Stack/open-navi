#!/usr/bin/env node
"use strict";

const childProcess = require("child_process");
const fs = require("fs");
const path = require("path");
const zlib = require("zlib");

const repoRoot = path.resolve(__dirname, "..");
const nativePackages = [
  ["open-navi-darwin-arm64", "darwin-arm64", "navi", "navid", "macosx_11_0_arm64"],
  ["open-navi-darwin-x64", "darwin-x64", "navi", "navid", "macosx_10_15_x86_64"],
  ["open-navi-linux-arm64", "linux-arm64", "navi", "navid", "manylinux2014_aarch64"],
  ["open-navi-linux-x64", "linux-x64", "navi", "navid", "manylinux2014_x86_64"],
  ["open-navi-win32-x64", "win32-x64", "navi.exe", "navid.exe", "win_amd64"],
];

function parseArgs(argv) {
  const opts = {
    npmDist: path.join(repoRoot, "dist", "npm"),
    pythonDist: path.join(repoRoot, "python", "dist"),
    python: process.env.PYTHON || "python",
    allowExtra: false,
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
    if (arg === "--allow-extra") {
      opts.allowExtra = true;
      continue;
    }
    throw new Error(`Unknown argument: ${arg}`);
  }
  return opts;
}

function readJson(relativePath) {
  return JSON.parse(fs.readFileSync(path.join(repoRoot, relativePath), "utf8"));
}

function fail(failures, message) {
  failures.push(message);
}

function requireEntry(failures, entries, file, entry) {
  if (!entries.has(entry)) {
    fail(failures, `${file} is missing ${entry}`);
  }
}

function listTarGz(file) {
  const buffer = zlib.gunzipSync(fs.readFileSync(file));
  const entries = [];
  let offset = 0;
  while (offset + 512 <= buffer.length) {
    const header = buffer.subarray(offset, offset + 512);
    if (header.every((byte) => byte === 0)) {
      break;
    }
    const name = header.subarray(0, 100).toString("utf8").replace(/\0.*$/, "");
    const prefix = header.subarray(345, 500).toString("utf8").replace(/\0.*$/, "");
    const fullName = prefix ? `${prefix}/${name}` : name;
    const sizeText = header.subarray(124, 136).toString("utf8").replace(/\0.*$/, "").trim();
    const size = sizeText ? Number.parseInt(sizeText, 8) : 0;
    if (!Number.isFinite(size)) {
      throw new Error(`Invalid tar entry size in ${file}: ${sizeText}`);
    }
    entries.push(fullName);
    offset += 512 + Math.ceil(size / 512) * 512;
  }
  return entries;
}

function inspectWheel(python, file) {
  const code = String.raw`
import json
import sys
import zipfile

wheel = sys.argv[1]
with zipfile.ZipFile(wheel) as z:
    names = z.namelist()
    metadata = ""
    wheel_metadata = ""
    for name in names:
        if name.endswith(".dist-info/METADATA"):
            metadata = z.read(name).decode("utf-8", errors="replace")
        if name.endswith(".dist-info/WHEEL"):
            wheel_metadata = z.read(name).decode("utf-8", errors="replace")
print(json.dumps({"names": names, "metadata": metadata, "wheel": wheel_metadata}))
`;
  const result = childProcess.spawnSync(python, ["-c", code, file], {
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
  });
  if (result.status !== 0) {
    throw new Error(`Failed to inspect wheel ${file}: ${result.stderr || result.stdout}`);
  }
  return JSON.parse(result.stdout);
}

function verifyNpmArtifacts(opts, version, failures) {
  const expected = new Set([`open-navi-${version}.tgz`]);
  for (const [packageName] of nativePackages) {
    expected.add(`${packageName}-${version}.tgz`);
  }

  if (!fs.existsSync(opts.npmDist)) {
    fail(failures, `Missing npm artifact directory: ${opts.npmDist}`);
    return;
  }

  const actual = fs.readdirSync(opts.npmDist).filter((name) => /^open-navi.*\.tgz$/.test(name));
  for (const name of expected) {
    if (!actual.includes(name)) {
      fail(failures, `Missing npm artifact: ${name}`);
    }
  }
  if (!opts.allowExtra) {
    for (const name of actual) {
      if (!expected.has(name)) {
        fail(failures, `Unexpected npm artifact: ${name}`);
      }
    }
  }

  const wrapperFile = path.join(opts.npmDist, `open-navi-${version}.tgz`);
  if (fs.existsSync(wrapperFile)) {
    const entries = new Set(listTarGz(wrapperFile));
    for (const entry of [
      "package/package.json",
      "package/README.md",
      "package/LICENSE",
      "package/NOTICE",
      "package/cli.cjs",
      "package/index.cjs",
    ]) {
      requireEntry(failures, entries, wrapperFile, entry);
    }
  }

  for (const [packageName, _pair, naviName, navidName] of nativePackages) {
    const file = path.join(opts.npmDist, `${packageName}-${version}.tgz`);
    if (!fs.existsSync(file)) {
      continue;
    }
    const entries = new Set(listTarGz(file));
    requireEntry(failures, entries, file, "package/package.json");
    requireEntry(failures, entries, file, "package/LICENSE");
    requireEntry(failures, entries, file, "package/NOTICE");
    requireEntry(failures, entries, file, `package/bin/${naviName}`);
    requireEntry(failures, entries, file, `package/bin/${navidName}`);
  }
}

function verifyPythonArtifacts(opts, version, failures) {
  const expected = new Set();
  for (const [_packageName, _pair, _naviName, _navidName, platform] of nativePackages) {
    expected.add(`open_navi-${version}-py3-none-${platform}.whl`);
  }

  if (!fs.existsSync(opts.pythonDist)) {
    fail(failures, `Missing Python artifact directory: ${opts.pythonDist}`);
    return;
  }

  const actual = fs.readdirSync(opts.pythonDist).filter((name) => /^open_navi-.*\.whl$/.test(name));
  for (const name of expected) {
    if (!actual.includes(name)) {
      fail(failures, `Missing PyPI wheel: ${name}`);
    }
  }
  if (!opts.allowExtra) {
    for (const name of actual) {
      if (!expected.has(name)) {
        fail(failures, `Unexpected PyPI wheel: ${name}`);
      }
    }
  }

  for (const [_packageName, pair, naviName, navidName, platform] of nativePackages) {
    const wheelName = `open_navi-${version}-py3-none-${platform}.whl`;
    const file = path.join(opts.pythonDist, wheelName);
    if (!fs.existsSync(file)) {
      continue;
    }
    const inspected = inspectWheel(opts.python, file);
    const entries = new Set(inspected.names);
    for (const entry of [
      "navi/__init__.py",
      "navi/cli.py",
      "navi/_generated/__init__.py",
      "navi/_generated/governed.py",
      `navi/bin/${naviName}`,
      `navi/bin/${navidName}`,
    ]) {
      requireEntry(failures, entries, file, entry);
    }
    if (!inspected.metadata.includes("Name: open-navi")) {
      fail(failures, `${wheelName} metadata is missing Name: open-navi`);
    }
    if (!inspected.metadata.includes(`Version: ${version}`)) {
      fail(failures, `${wheelName} metadata is missing Version: ${version}`);
    }
    if (
      !inspected.metadata.includes("License-Expression: Apache-2.0") &&
      !inspected.metadata.includes("License: Apache-2.0")
    ) {
      fail(failures, `${wheelName} metadata is missing Apache-2.0 license metadata`);
    }
    if (!inspected.names.some((name) => name.endsWith(".dist-info/licenses/LICENSE"))) {
      fail(failures, `${wheelName} is missing packaged LICENSE`);
    }
    if (!inspected.names.some((name) => name.endsWith(".dist-info/licenses/NOTICE"))) {
      fail(failures, `${wheelName} is missing packaged NOTICE`);
    }
    if (!inspected.wheel.includes("Root-Is-Purelib: false")) {
      fail(failures, `${wheelName} must be marked Root-Is-Purelib: false`);
    }
    if (!inspected.wheel.includes(`Tag: py3-none-${platform}`)) {
      fail(failures, `${wheelName} is missing wheel tag py3-none-${platform}`);
    }
    if (pair !== "win32-x64" && (naviName.endsWith(".exe") || navidName.endsWith(".exe"))) {
      fail(failures, `${wheelName} has Windows executable names for non-Windows target`);
    }
  }
}

try {
  const opts = parseArgs(process.argv);
  const version = readJson("packages/npm/navi/package.json").version;
  const failures = [];
  verifyNpmArtifacts(opts, version, failures);
  verifyPythonArtifacts(opts, version, failures);

  if (failures.length > 0) {
    for (const message of failures) {
      console.error(`ERROR: ${message}`);
    }
    process.exit(1);
  }
  console.log(`open-navi release artifacts verified for ${version}.`);
} catch (err) {
  console.error(err.message || String(err));
  process.exit(1);
}
