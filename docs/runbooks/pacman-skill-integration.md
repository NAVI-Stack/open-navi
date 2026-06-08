# PACMAN skill integration runbook

How to run the PACMAN (contact manager) example app and the NAVI pacman skill.

---

## Running the original PACMAN app

1. **From the example-code directory:**

   ```bash
   cd example-code/pacman-master
   go run main.go
   ```

   Or run the prebuilt binary (adjust for your OS):

   ```bash
   ./dist/pacman_windows_amd64   # Windows
   ./dist/pacman_linux_amd64     # Linux
   ./dist/pacman_darwin_amd64    # macOS
   ```

2. **Default:** Server listens on `http://localhost:8080`.  
   **Default Basic Auth:** `username` / `password` (configurable in `main.go`).

3. **Manual test:**

   - Open `http://localhost:8080` in a browser and sign in.
   - Add/edit/delete contacts and use Export to download VCF.

---

## Running the PACMAN skill

NAVI executes `subprocess_python` skills, so to run PACMAN:

1. Ensure the PACMAN server is running (see above).

2. Ensure the skill is loaded from the workspace tier:
   - Skill path: `workspace/skills/pacman/` (or `skills/pacman/` if using builtin tier).
   - Contains `SKILL.yaml` and `run_skill.py`.

3. Start NaviD with Docker Compose (`docker compose -f compose.yml up -d --build`). The skill will appear in the agent context as **pacman** with interfaces:
   - `pacman_health_check`
   - `pacman_add_contact`
   - `pacman_export_contacts`

4. In a session, ask NAVI to e.g. "check if my contact manager is up" or "add a contact named Alice with phone +1234567890". The agent will call the skill; the runner will invoke `run_skill.py` with the tool arguments and return the SkillResult.

**Note:** Skills whose transport has no executor return an honest "registered but not yet executable" message rather than faking success. PACMAN uses `subprocess_python`, which is executable.

---

## Testing the skill bridge manually

Without the full NAVI executor, you can drive the Python bridge directly:

```bash
cd workspace/skills/pacman
```

**Health check (PACMAN server must be running):**

```bash
echo '{"interface":"health_check","username":"username","password":"password"}' | python run_skill.py
```

**Add contact:**

```bash
echo '{"interface":"add_contact","username":"username","password":"password","name":"Alice","phone":"+15551234567","email":"alice@example.com"}' | python run_skill.py
```

**Export contacts:**

```bash
echo '{"interface":"export_contacts","username":"username","password":"password"}' | python run_skill.py
```

Expected: one JSON line to stdout with `status` (`success` or `error`), `output`, and `duration_ms`.

---

## References

- Example code: `example-code/pacman-master/`
- Review memo: `docs-local/pacman-example-code-review.md`
- Skill concepts: `docs/concepts/skills.md`
- OSS-27 spec: `internal/navi/skill/spec.go`
