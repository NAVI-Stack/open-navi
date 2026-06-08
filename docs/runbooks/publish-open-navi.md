# Publish open-navi

**Status:** Active
**Last Updated:** 2026-06-08

This runbook publishes the public Node and Python install surface:

```bash
npm install open-navi
navi daemon start
```

or:

```bash
pip install open-navi
navi daemon start
```

The public package/project name is `open-navi`. The installed command remains `navi`, and the Python import package remains `navi`.

## Package Shape

### npm

The user-facing package is `open-navi` under `packages/npm/navi/`. It is a thin CommonJS wrapper that installs the `navi` command and resolves a platform-native binary package through optional dependencies.

Native payload packages:

| Package | Platform | Contents |
|---------|----------|----------|
| `open-navi-darwin-arm64` | macOS arm64 | `bin/navi`, `bin/navid` |
| `open-navi-darwin-x64` | macOS x64 | `bin/navi`, `bin/navid` |
| `open-navi-linux-arm64` | Linux arm64 | `bin/navi`, `bin/navid` |
| `open-navi-linux-x64` | Linux x64 | `bin/navi`, `bin/navid` |
| `open-navi-win32-x64` | Windows x64 | `bin/navi.exe`, `bin/navid.exe` |

Publish the native packages first, then publish `open-navi`, so the wrapper's optional dependencies can resolve.

### PyPI

The PyPI project is `open-navi` under `python/`. It builds platform-specific wheels containing the Python wrapper package plus `navi/bin/` native payload files. The console script remains:

```toml
[project.scripts]
navi = "navi.cli:main"
```

Do not upload a universal `py3-none-any` wheel with native binaries. Build and upload platform-tagged wheels.

## Prepare Native Payloads

Build the native Go binaries for the release platform, then copy them into the npm native package and Python package payload:

```bash
node scripts/prepare-open-navi-release.cjs --platform win32 --arch x64
```

Supported pairs:

- `darwin-arm64`
- `darwin-x64`
- `linux-arm64`
- `linux-x64`
- `win32-x64`

The script expects built binaries in `bin/` using the runtime names for that platform:

| Platform | Required source files |
|----------|-----------------------|
| Windows | `bin/navi.exe`, `bin/navid.exe` |
| macOS/Linux | `bin/navi`, `bin/navid` |

`NAVI_NATIVE_BIN` remains the developer escape hatch for wrapper tests and local wrapper development. Set it to the absolute path of a locally built native `navi` executable when you want the npm or pip wrapper to use that binary instead of the packaged payload.

## Verify Before Upload

Run the normal wrapper and command checks:

```bash
go test ./cmd/navi -count=1
go test ./cmd/... -count=1
node --test packages/npm/navi/test/cli.test.cjs
cd python
python -m unittest navi.tests.test_cli navi.tests.test_query_context
cd ..
git diff --check
```

Check npm package payloads:

```bash
rm -rf dist/npm
mkdir -p dist/npm
cd packages/npm/open-navi-win32-x64
npm pack --pack-destination ../../../dist/npm
cd ../navi
npm pack --pack-destination ../../../dist/npm
```

Check Python distributions:

```bash
cd python
rm -rf dist build *.egg-info
python setup.py bdist_wheel --plat-name win_amd64 --python-tag py3
python -m twine check dist/*
```

The wheel filename must include a platform tag, for example `py3-none-win_amd64`, not `py3-none-any`. The GitHub workflow builds all supported platform wheels.

When verifying a complete release artifact set, run:

```bash
node scripts/verify-open-navi-artifacts.cjs --npm-dist dist/npm --python-dist python/dist
node scripts/smoke-open-navi-install.cjs --npm-dist dist/npm --python-dist python/dist --work-dir .release-check
```

The verifier fails on stale extra `open-navi` tarballs or `open_navi` wheels unless `--allow-extra` is passed for exploratory local checks.
The smoke script installs the current platform's npm tarballs and PyPI wheel into clean temporary environments, then verifies the installed `navi` command shim, wrapper import, native binary resolution, and distribution-channel stamping without starting a daemon.

## GitHub Release Workflow

The manual workflow `.github/workflows/publish-open-navi.yml` builds all npm tarballs and PyPI platform wheels.

Run it with `publish=false` to build and inspect artifacts without uploading.

Run it with `publish=true` only after release approval and registry access are configured:

- GitHub environments: `npm` and `pypi`, preferably with required reviewer approval.
- First npm publish: `NPM_TOKEN`, owned by the release maintainer or release organization, unless npm trusted publishing is already configured for every package.
- PyPI pending trusted publisher for project `open-navi`, repository `NAVI-Stack/open-navi`, workflow filename `publish-open-navi.yml`, and environment `pypi`.
- Final package license metadata in npm and PyPI. The workflow blocks publish mode while the license is missing or still marked `UNLICENSED`.

Before the first public upload, confirm the registry names are still available:

```bash
node scripts/check-open-navi-registries.cjs --expect available --check-npm-auth
```

For every release, confirm the target version has not already been published:

```bash
node scripts/check-open-navi-registries.cjs --expect unpublished-version --version 0.1.0
```

The workflow publishes npm native packages first, then the `open-navi` wrapper package. It uploads all PyPI wheels for the same version.

After the first npm publish creates the packages, configure npm trusted publishing for each npm package with repository `NAVI-Stack/open-navi`, workflow filename `publish-open-navi.yml`, and environment `npm`. The workflow supports this future mode: if `NPM_TOKEN` is absent, npm publish will rely on the GitHub Actions OIDC identity. npm's current trusted publishing flow removes the need for long-lived publish tokens after the package settings are configured.

The workflow runs:

```bash
node scripts/check-open-navi-release.cjs
node scripts/check-open-navi-release.cjs --for-publish
node scripts/check-open-navi-registries.cjs --expect unpublished-version --version <version>
node scripts/verify-open-navi-artifacts.cjs --npm-dist dist/npm --python-dist python/dist
node scripts/smoke-open-navi-install.cjs --npm-dist dist/npm --python-dist python/dist --python python
node scripts/check-open-navi-registries.cjs --expect published --version <version>
```

The first check validates package names, aligned versions, native optional dependencies, docs, release notes, and wrapper command metadata. The publish check adds the final license gate. The registry check verifies either first-release name availability or post-publish package visibility, depending on `--expect`.

## Ownership And Access

Before the first release:

- Decide the legal release license and add it to npm package metadata, PyPI metadata, and the root license file.
- Confirm the public owner identity for npm/PyPI. Current metadata uses `NAVI Stack` and repository `NAVI-Stack/open-navi`.
- Configure GitHub environments `npm` and `pypi` with release maintainers as required reviewers.
- Configure a PyPI pending trusted publisher for `open-navi`; pending publishers can create the project on first successful publish.
- Ensure the release maintainer's npm account or organization owns the initial npm publish token.

After the first release:

- Add the intended maintainers/organization as owners for `open-navi` and all five native npm packages.
- Configure npm trusted publishing on each npm package for `.github/workflows/publish-open-navi.yml` and environment `npm`.
- Remove or rotate the first-publish `NPM_TOKEN` once trusted publishing is verified.
- Add the intended maintainers/organization as PyPI project owners or maintainers.
- Keep future releases on the same aligned version across npm and PyPI unless a package-specific hotfix forces a documented exception.

## Publish Manually

Publishing requires authenticated registry access. Do not put tokens in the repo.

npm:

```bash
cd packages/npm/open-navi-win32-x64
npm publish --access public
cd ../navi
npm publish --access public
```

Repeat native package publishing for every supported platform package before publishing `open-navi`.

PyPI:

```bash
cd python
python -m twine upload dist/*
```

## Post-Publish Smoke

After the registries show the uploaded versions:

```bash
node scripts/check-open-navi-registries.cjs --expect published
```

Then verify fresh installs in a clean environment:

```bash
npm install -g open-navi
navi daemon status
```

and:

```bash
pip install open-navi
navi daemon status
```

[runbooks INDEX](INDEX.md) | [docs INDEX](../INDEX.md)
