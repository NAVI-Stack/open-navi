# open-navi

Public Node wrapper for the local-first NAVI runtime.

```bash
npm install open-navi
navi daemon start
```

The installed command is `navi`. The package resolves a platform-native NAVI binary package through npm optional dependencies, stamps `NAVI_DISTRIBUTION_CHANNEL=npm`, and delegates to the native `navi` executable.

For wrapper development, set `NAVI_NATIVE_BIN` to a locally built native `navi` executable.

See the main repository README and `docs/runbooks/publish-open-navi.md` for release details.
