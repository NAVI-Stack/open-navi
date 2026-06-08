# open-navi

Public Python wrapper for the local-first NAVI runtime.

```bash
pip install open-navi
navi daemon start
```

The installed command is `navi`, and the import package remains `navi`.

The wheel carries the native `navi` and `navid` binaries for its platform, stamps `NAVI_DISTRIBUTION_CHANNEL=pip`, and delegates to the native `navi` executable.

For wrapper development, set `NAVI_NATIVE_BIN` to a locally built native `navi` executable.

Licensed under Apache-2.0. See `LICENSE` and `NOTICE`.
