#!/usr/bin/env node
"use strict";

const fs = require("fs");
const path = require("path");

const repoRoot = path.resolve(__dirname, "..");
const forPublish = process.argv.includes("--for-publish");
const failures = [];
const warnings = [];

const nativePackages = [
  ["open-navi-darwin-arm64", "darwin", "arm64"],
  ["open-navi-darwin-x64", "darwin", "x64"],
  ["open-navi-linux-arm64", "linux", "arm64"],
  ["open-navi-linux-x64", "linux", "x64"],
  ["open-navi-win32-x64", "win32", "x64"],
];
const homepage = "https://github.com/NAVI-Stack/open-navi#readme";
const repositoryUrl = "git+https://github.com/NAVI-Stack/open-navi.git";
const issuesUrl = "https://github.com/NAVI-Stack/open-navi/issues";
const pypiRepositoryUrl = "https://github.com/NAVI-Stack/open-navi";

function rel(...parts) {
  return path.join(repoRoot, ...parts);
}

function fail(message) {
  failures.push(message);
}

function warn(message) {
  warnings.push(message);
}

function readJson(relativePath) {
  return JSON.parse(fs.readFileSync(rel(relativePath), "utf8"));
}

function readText(relativePath) {
  return fs.readFileSync(rel(relativePath), "utf8");
}

function requireFile(relativePath) {
  if (!fs.existsSync(rel(relativePath))) {
    fail(`Missing required release file: ${relativePath}`);
  }
}

function projectScalar(pyproject, key) {
  const project = /^\[project\]$(?<body>[\s\S]*?)(?:^\[|\z)/m.exec(pyproject);
  if (!project) {
    fail("python/pyproject.toml is missing [project]");
    return "";
  }
  const match = new RegExp(`^${key}\\s*=\\s*"([^"]+)"`, "m").exec(project.groups.body);
  return match ? match[1] : "";
}

function hasProjectScript(pyproject) {
  const scripts = /^\[project\.scripts\]$(?<body>[\s\S]*?)(?:^\[|\z)/m.exec(pyproject);
  return Boolean(scripts && /^navi\s*=\s*"navi\.cli:main"/m.test(scripts.groups.body));
}

function checkPackageLicense(label, license) {
  if (!license) {
    fail(`${label} is missing license metadata`);
    return;
  }
  if (forPublish && license === "UNLICENSED") {
    fail(`${label} still uses UNLICENSED; choose the public package license before publishing`);
  }
}

function checkNpmLinks(label, pkg) {
  if (pkg.homepage !== homepage) {
    fail(`${label} homepage should be ${homepage}`);
  }
  if (pkg.repository?.type !== "git" || pkg.repository?.url !== repositoryUrl) {
    fail(`${label} repository should point to ${repositoryUrl}`);
  }
  if (pkg.bugs?.url !== issuesUrl) {
    fail(`${label} bugs URL should be ${issuesUrl}`);
  }
}

function checkNpmPublishConfig(label, pkg) {
  if (pkg.publishConfig?.access !== "public") {
    fail(`${label} publishConfig.access should be public`);
  }
  if (pkg.publishConfig?.registry !== "https://registry.npmjs.org") {
    fail(`${label} publishConfig.registry should be https://registry.npmjs.org`);
  }
  if (pkg.publishConfig?.provenance !== true) {
    fail(`${label} publishConfig.provenance should be true`);
  }
}

const mainPackage = readJson("packages/npm/navi/package.json");
const pyproject = readText("python/pyproject.toml");
const pythonName = projectScalar(pyproject, "name");
const pythonVersion = projectScalar(pyproject, "version");
const pythonDescription = projectScalar(pyproject, "description");
const pythonAuthor = /authors\s*=\s*\[[\s\S]*?\{\s*name\s*=\s*"([^"]+)"/m.exec(pyproject)?.[1] || "";
const pythonLicense = /license\s*=\s*"([^"]+)"/m.exec(pyproject)?.[1] || "";
const pythonUrls = /^\[project\.urls\]$(?<body>[\s\S]*?)(?:^\[|\z)/m.exec(pyproject)?.groups.body || "";

if (mainPackage.name !== "open-navi") {
  fail(`npm package name must be open-navi, found ${mainPackage.name}`);
}
if (pythonName !== "open-navi") {
  fail(`PyPI project name must be open-navi, found ${pythonName || "<missing>"}`);
}
if (!mainPackage.version) {
  fail("npm package version is missing");
}
if (!pythonVersion) {
  fail("PyPI project version is missing");
}
if (mainPackage.version && pythonVersion && mainPackage.version !== pythonVersion) {
  fail(`Version mismatch: npm ${mainPackage.version} != PyPI ${pythonVersion}`);
}
if (mainPackage.description !== "NAVI local daemon CLI wrapper for Node.js installs.") {
  fail("npm package description does not match the approved public wrapper description");
}
if (pythonDescription !== "NAVI Python SDK and local daemon CLI wrapper.") {
  fail("PyPI project description does not match the approved public wrapper description");
}
if (mainPackage.author !== "NAVI Stack") {
  fail(`npm package author should be NAVI Stack, found ${mainPackage.author || "<missing>"}`);
}
if (pythonAuthor !== "NAVI Stack") {
  fail(`PyPI author should be NAVI Stack, found ${pythonAuthor || "<missing>"}`);
}
if (mainPackage.bin?.navi !== "./cli.cjs") {
  fail('npm package must install the "navi" command from ./cli.cjs');
}
if (!hasProjectScript(pyproject)) {
  fail('PyPI package must install the "navi" command via navi.cli:main');
}
checkNpmLinks("npm package open-navi", mainPackage);
checkNpmPublishConfig("npm package open-navi", mainPackage);
if (!pythonUrls.includes(`Homepage = "${homepage}"`)) {
  fail(`PyPI Homepage URL should be ${homepage}`);
}
if (!pythonUrls.includes(`Repository = "${pypiRepositoryUrl}"`)) {
  fail(`PyPI Repository URL should be ${pypiRepositoryUrl}`);
}
if (!pythonUrls.includes(`Issues = "${issuesUrl}"`)) {
  fail(`PyPI Issues URL should be ${issuesUrl}`);
}
checkPackageLicense("npm package open-navi", mainPackage.license);
if (forPublish && !pythonLicense) {
  fail("PyPI project is missing license metadata; choose the public package license before publishing");
} else if (!pythonLicense) {
  warn("PyPI project license metadata is pending the public license decision");
}

for (const [packageName, osName, cpuName] of nativePackages) {
  const pkg = readJson(`packages/npm/${packageName}/package.json`);
  if (pkg.name !== packageName) {
    fail(`Native package name mismatch in ${packageName}: ${pkg.name}`);
  }
  if (pkg.version !== mainPackage.version) {
    fail(`Native package ${packageName} version ${pkg.version} does not match ${mainPackage.version}`);
  }
  if (mainPackage.optionalDependencies?.[packageName] !== mainPackage.version) {
    fail(`open-navi optionalDependency ${packageName} must be pinned to ${mainPackage.version}`);
  }
  if (pkg.os?.[0] !== osName || pkg.cpu?.[0] !== cpuName) {
    fail(`Native package ${packageName} platform metadata should be ${osName}/${cpuName}`);
  }
  checkNpmLinks(`native npm package ${packageName}`, pkg);
  checkNpmPublishConfig(`native npm package ${packageName}`, pkg);
  checkPackageLicense(`native npm package ${packageName}`, pkg.license);
}

for (const [name] of nativePackages) {
  if (!mainPackage.optionalDependencies || !(name in mainPackage.optionalDependencies)) {
    fail(`open-navi is missing optionalDependency ${name}`);
  }
}

for (const required of [
  "README.md",
  "CHANGELOG.md",
  "docs/runbooks/publish-open-navi.md",
  "packages/npm/navi/README.md",
  "python/README.md",
  "python/setup.py",
  "python/MANIFEST.in",
  "python/navi/bin/__init__.py",
  "python/navi/_generated/__init__.py",
]) {
  requireFile(required);
}

const repoReadme = readText("README.md");
const npmReadme = readText("packages/npm/navi/README.md");
const pythonReadme = readText("python/README.md");
const publishRunbook = readText("docs/runbooks/publish-open-navi.md");
for (const [label, text] of [
  ["README.md", repoReadme],
  ["packages/npm/navi/README.md", npmReadme],
  ["python/README.md", pythonReadme],
  ["docs/runbooks/publish-open-navi.md", publishRunbook],
]) {
  if (!text.includes("open-navi")) {
    fail(`${label} does not document the open-navi package name`);
  }
  if (!text.includes("navi")) {
    fail(`${label} does not document the installed navi command`);
  }
}
if (!repoReadme.includes("NAVI_NATIVE_BIN") || !publishRunbook.includes("NAVI_NATIVE_BIN")) {
  fail("README and publish runbook must document NAVI_NATIVE_BIN");
}
if (!readText("CHANGELOG.md").includes(`## ${mainPackage.version} -`)) {
  fail(`CHANGELOG.md is missing a ${mainPackage.version} release entry`);
}

for (const [label, text] of [
  ["README.md", repoReadme],
  ["packages/npm/navi/README.md", npmReadme],
  ["python/README.md", pythonReadme],
  ["docs/runbooks/publish-open-navi.md", publishRunbook],
]) {
  if (/npm install navi|pip install navi/.test(text)) {
    fail(`${label} contains stale install copy for the old navi package name`);
  }
}

for (const message of warnings) {
  console.warn(`WARN: ${message}`);
}

if (failures.length > 0) {
  for (const message of failures) {
    console.error(`ERROR: ${message}`);
  }
  process.exit(1);
}

console.log(`open-navi release metadata check passed${forPublish ? " for publish" : ""}.`);
