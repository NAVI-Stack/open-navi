#!/usr/bin/env node

const fs = require("node:fs");
const path = require("node:path");
const process = require("node:process");

const supported = new Set([
  "darwin-arm64",
  "darwin-x64",
  "linux-arm64",
  "linux-x64",
  "win32-x64",
]);

function usage() {
  console.error(
    [
      "Usage: node scripts/prepare-open-navi-release.cjs [--platform <platform>] [--arch <arch>]",
      "",
      "Copies built native navi/navid binaries into:",
      "  packages/npm/open-navi-<platform>-<arch>/bin/",
      "  python/navi/bin/",
      "",
      "Defaults to the current Node platform/arch. Supported pairs:",
      "  darwin-arm64, darwin-x64, linux-arm64, linux-x64, win32-x64",
    ].join("\n")
  );
}

function parseArgs(argv) {
  const out = {
    platform: process.platform,
    arch: process.arch,
    repoRoot: path.resolve(__dirname, ".."),
  };
  for (let i = 2; i < argv.length; i++) {
    const arg = argv[i];
    if (arg === "--help" || arg === "-h") {
      usage();
      process.exit(0);
    }
    if (arg === "--platform") {
      out.platform = argv[++i];
      continue;
    }
    if (arg === "--arch") {
      out.arch = argv[++i];
      continue;
    }
    if (arg === "--repo-root") {
      out.repoRoot = path.resolve(argv[++i]);
      continue;
    }
    throw new Error(`Unknown argument: ${arg}`);
  }
  return out;
}

function executableNames(platform) {
  if (platform === "win32") {
    return { navi: "navi.exe", navid: "navid.exe" };
  }
  return { navi: "navi", navid: "navid" };
}

function assertInsideRepo(repoRoot, candidate) {
  const relative = path.relative(repoRoot, candidate);
  if (relative.startsWith("..") || path.isAbsolute(relative)) {
    throw new Error(`Refusing to write outside repo: ${candidate}`);
  }
}

function copyPayload(repoRoot, platform, arch) {
  const pair = `${platform}-${arch}`;
  if (!supported.has(pair)) {
    throw new Error(`Unsupported release platform: ${pair}`);
  }

  const names = executableNames(platform);
  const sourceDir = path.join(repoRoot, "bin");
  const sourceNavi = path.join(sourceDir, names.navi);
  const sourceNavid = path.join(sourceDir, names.navid);
  for (const source of [sourceNavi, sourceNavid]) {
    if (!fs.existsSync(source)) {
      throw new Error(`Missing built binary: ${source}`);
    }
  }

  const npmBinDir = path.join(repoRoot, "packages", "npm", `open-navi-${pair}`, "bin");
  const pythonBinDir = path.join(repoRoot, "python", "navi", "bin");
  const pythonGeneratedDir = path.join(repoRoot, "python", "navi", "_generated");
  const governedSource = path.join(repoRoot, "schema", "python", "navi_schema", "governed.py");
  if (!fs.existsSync(governedSource)) {
    throw new Error(`Missing generated governed contract: ${governedSource}`);
  }
  for (const target of [npmBinDir, pythonBinDir]) {
    assertInsideRepo(repoRoot, target);
    fs.rmSync(target, { recursive: true, force: true });
    fs.mkdirSync(target, { recursive: true });
    fs.copyFileSync(sourceNavi, path.join(target, names.navi));
    fs.copyFileSync(sourceNavid, path.join(target, names.navid));
    if (platform !== "win32") {
      fs.chmodSync(path.join(target, names.navi), 0o755);
      fs.chmodSync(path.join(target, names.navid), 0o755);
    }
  }
  fs.writeFileSync(
    path.join(pythonBinDir, "__init__.py"),
    "# Native binary payload package for open-navi wheels.\n"
  );
  assertInsideRepo(repoRoot, pythonGeneratedDir);
  fs.rmSync(pythonGeneratedDir, { recursive: true, force: true });
  fs.mkdirSync(pythonGeneratedDir, { recursive: true });
  fs.writeFileSync(
    path.join(pythonGeneratedDir, "__init__.py"),
    "# Generated schema payload package for open-navi wheels.\n"
  );
  fs.copyFileSync(governedSource, path.join(pythonGeneratedDir, "governed.py"));

  return { pair, npmBinDir, pythonBinDir, files: [names.navi, names.navid] };
}

try {
  const opts = parseArgs(process.argv);
  const result = copyPayload(opts.repoRoot, opts.platform, opts.arch);
  console.log(`Prepared open-navi native payload for ${result.pair}`);
  console.log(`npm:    ${result.npmBinDir}`);
  console.log(`python: ${result.pythonBinDir}`);
  console.log(`files:  ${result.files.join(", ")}`);
} catch (err) {
  console.error(err.message || String(err));
  process.exit(1);
}
