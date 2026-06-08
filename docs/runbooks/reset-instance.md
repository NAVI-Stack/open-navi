# Reset a NAVI Instance

You can return a NAVI instance to an unclaimed, fresh state by using the in-product API or by stopping the runtime and wiping its state files.

Use the reset path that matches how NaviD is running:

- **Docker mode:** stop Compose and wipe the Docker data volume or bind-mounted data directory.
- **Local daemon mode:** stop the host daemon and wipe the local SQLite / JetStream files.

---

## Preferred: In-Product API

Use this when NaviD is running and you have the owner secret.

1. Request
   - Method: `POST`
   - URL: `http://<gateway>/api/instance/reset` (for example, `http://localhost:6284/api/instance/reset`)
   - Header: `X-Owner-Secret: <your-owner-secret>`
   - Body: none or empty JSON

2. Behavior
   - SQLite state is wiped in a transaction.
   - JetStream streams are purged.
   - NaviD keeps running.

3. After reset
   - PET or NaviExe can run the claim flow again.
   - Connectors are cleared; reconfigure after claiming.

Errors:

- `401`: missing or invalid `X-Owner-Secret`
- `500`: database or stream purge failure; check daemon logs

PET exposes this as an owner-only **Hard reset NAVI instance** action when the stored connection includes the owner secret.

---

## Manual Reset: Docker Mode

From the repository root containing `compose.yml`:

1. Stop the stack.

   ```bash
   docker compose -f compose.yml down
   ```

   If you use strict mode:

   ```bash
   docker compose -f compose.strict.yml down
   ```

2. Wipe the data volume. The default strict-mode volume name is usually `navi_navid-data`.

   ```bash
   docker run --rm -v navi_navid-data:/data alpine sh -c "rm -f /data/navi.db /data/navi.db-wal /data/navi.db-shm; rm -rf /data/jetstream; ls -la /data"
   ```

   If you use the permissive bind-mounted stack, wipe the configured `${NAVI_DATA_DIR:-${NAVI_DATA_DRIVE:-./.navi}/data}` directory instead.

3. Start NaviD again.

   ```bash
   docker compose -f compose.yml up -d --build
   ```

Shortcut for the default Docker volume path:

```bash
make force-reset-docker
```

---

## Manual Reset: Local Daemon Mode

From the repository root used to start `navi daemon start`:

1. Stop the managed local daemon.

   ```bash
   ./bin/navi.exe daemon stop
   ```

2. Wipe local runtime state.

   ```bash
   rm -f navi.db navi.db-wal navi.db-shm
   rm -rf jetstream
   ```

   If you set `NAVI_SQLITE_PATH` or `NAVI_WORKSPACE_DIR`, wipe those configured paths instead of the repository-root defaults.

3. Start the local daemon again.

   ```bash
   ./bin/navi.exe daemon start
   ```

Shortcut for repository-root default state:

```bash
make force-reset
make daemon-start
```

---

## After Reset

- `GET /api/onboarding/status` and claim flows show an unclaimed instance.
- PET or `./bin/navi.exe init` can establish a new owner and API key.

## Security Note

Anyone with filesystem access to the database or Docker volume can reset the instance. Restrict access to runtime data, backups, and `~/.navi/daemon/` metadata on shared machines.

[runbooks INDEX](INDEX.md) | [docs INDEX](../INDEX.md)
