#!/usr/bin/env node
"use strict";

const childProcess = require("child_process");
const fs = require("fs");
const https = require("https");
const path = require("path");

const repoRoot = path.resolve(__dirname, "..");
const npmPackages = [
  "open-navi",
  "open-navi-darwin-arm64",
  "open-navi-darwin-x64",
  "open-navi-linux-arm64",
  "open-navi-linux-x64",
  "open-navi-win32-x64",
];

function parseArgs(argv) {
  const opts = {
    expect: "available",
    version: "",
    retries: 0,
    retryDelayMs: 10000,
    checkNpmAuth: false,
  };
  for (let i = 2; i < argv.length; i++) {
    const arg = argv[i];
    if (arg === "--expect") {
      opts.expect = argv[++i];
      continue;
    }
    if (arg === "--version") {
      opts.version = argv[++i];
      continue;
    }
    if (arg === "--retries") {
      opts.retries = Number.parseInt(argv[++i], 10);
      continue;
    }
    if (arg === "--retry-delay-ms") {
      opts.retryDelayMs = Number.parseInt(argv[++i], 10);
      continue;
    }
    if (arg === "--check-npm-auth") {
      opts.checkNpmAuth = true;
      continue;
    }
    throw new Error(`Unknown argument: ${arg}`);
  }
  if (!["available", "unpublished-version", "published"].includes(opts.expect)) {
    throw new Error(`Unsupported --expect value: ${opts.expect}`);
  }
  if (!Number.isFinite(opts.retries) || opts.retries < 0) {
    throw new Error("--retries must be a non-negative integer");
  }
  if (!Number.isFinite(opts.retryDelayMs) || opts.retryDelayMs < 0) {
    throw new Error("--retry-delay-ms must be a non-negative integer");
  }
  return opts;
}

function packageVersion() {
  const pkg = JSON.parse(fs.readFileSync(path.join(repoRoot, "packages", "npm", "navi", "package.json"), "utf8"));
  return pkg.version;
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function requestJson(url) {
  return new Promise((resolve, reject) => {
    const req = https.get(
      url,
      {
        headers: {
          "Accept": "application/json",
          "User-Agent": "open-navi-release-preflight",
        },
      },
      (res) => {
        let body = "";
        res.setEncoding("utf8");
        res.on("data", (chunk) => {
          body += chunk;
        });
        res.on("end", () => {
          if (res.statusCode === 404) {
            resolve({ status: 404, data: null });
            return;
          }
          if (!res.statusCode || res.statusCode < 200 || res.statusCode >= 300) {
            reject(new Error(`${url} returned HTTP ${res.statusCode}: ${body.slice(0, 200)}`));
            return;
          }
          try {
            resolve({ status: res.statusCode, data: JSON.parse(body) });
          } catch (err) {
            reject(new Error(`${url} returned invalid JSON: ${err.message}`));
          }
        });
      }
    );
    req.setTimeout(15000, () => {
      req.destroy(new Error(`Timed out fetching ${url}`));
    });
    req.on("error", reject);
  });
}

async function npmInfo(name) {
  return requestJson(`https://registry.npmjs.org/${encodeURIComponent(name)}`);
}

async function pypiInfo(name) {
  return requestJson(`https://pypi.org/pypi/${encodeURIComponent(name)}/json`);
}

function npmVersionPublished(data, version) {
  return Boolean(data?.versions && data.versions[version]);
}

function pypiVersionPublished(data, version) {
  return Boolean(data?.releases && data.releases[version]);
}

function checkNpmAuth(failures) {
  const npmCmd = process.platform === "win32" ? "npm.cmd" : "npm";
  const result = childProcess.spawnSync(npmCmd, ["whoami", "--cache", path.join(repoRoot, ".npm-cache")], {
    cwd: repoRoot,
    encoding: "utf8",
    shell: process.platform === "win32",
  });
  if (result.error) {
    failures.push(`npm whoami could not start: ${result.error.message}`);
    return;
  }
  if (result.status !== 0) {
    failures.push(`npm whoami failed: ${(result.stderr || result.stdout || "").trim()}`);
    return;
  }
  console.log(`npm authenticated as ${(result.stdout || "").trim()}`);
}

async function runOnce(opts, version) {
  const failures = [];
  for (const name of npmPackages) {
    const { status, data } = await npmInfo(name);
    if (opts.expect === "available") {
      if (status !== 404) {
        failures.push(`npm package ${name} already exists`);
      } else {
        console.log(`npm package ${name}: available`);
      }
      continue;
    }
    if (opts.expect === "unpublished-version") {
      if (status === 404) {
        console.log(`npm package ${name}: available; version ${version} is unpublished`);
      } else if (npmVersionPublished(data, version)) {
        failures.push(`npm package ${name} already has version ${version}`);
      } else {
        console.log(`npm package ${name}: version ${version} is unpublished`);
      }
      continue;
    }
    if (status === 404) {
      failures.push(`npm package ${name} is not published`);
    } else if (!npmVersionPublished(data, version)) {
      failures.push(`npm package ${name} does not have version ${version}`);
    } else {
      console.log(`npm package ${name}: version ${version} is published`);
    }
  }

  const pypi = await pypiInfo("open-navi");
  if (opts.expect === "available") {
    if (pypi.status !== 404) {
      failures.push("PyPI project open-navi already exists");
    } else {
      console.log("PyPI project open-navi: available");
    }
  } else if (opts.expect === "unpublished-version") {
    if (pypi.status === 404) {
      console.log(`PyPI project open-navi: available; version ${version} is unpublished`);
    } else if (pypiVersionPublished(pypi.data, version)) {
      failures.push(`PyPI project open-navi already has version ${version}`);
    } else {
      console.log(`PyPI project open-navi: version ${version} is unpublished`);
    }
  } else if (pypi.status === 404) {
    failures.push("PyPI project open-navi is not published");
  } else if (!pypiVersionPublished(pypi.data, version)) {
    failures.push(`PyPI project open-navi does not have version ${version}`);
  } else {
    console.log(`PyPI project open-navi: version ${version} is published`);
  }

  if (opts.checkNpmAuth) {
    checkNpmAuth(failures);
  }
  return failures;
}

async function main() {
  const opts = parseArgs(process.argv);
  const version = opts.version || packageVersion();
  let failures = [];
  for (let attempt = 0; attempt <= opts.retries; attempt++) {
    failures = await runOnce(opts, version);
    if (failures.length === 0) {
      console.log(`open-navi registry check passed: ${opts.expect} ${version}.`);
      return;
    }
    if (attempt < opts.retries) {
      console.error(`Registry check attempt ${attempt + 1} failed; retrying in ${opts.retryDelayMs}ms.`);
      await sleep(opts.retryDelayMs);
    }
  }
  for (const failure of failures) {
    console.error(`ERROR: ${failure}`);
  }
  process.exit(1);
}

main().catch((err) => {
  console.error(err.message || String(err));
  process.exit(1);
});
