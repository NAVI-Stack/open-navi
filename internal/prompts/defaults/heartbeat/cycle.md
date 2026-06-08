## Heartbeat Cycle — {{.Timestamp}}{{if .IsLocal}} (Local: {{.TZLabel}}){{end}}

Please execute the following scheduled tasks:

{{range .Tasks}}{{.N}}. {{.Description}}
{{end}}
If there is nothing useful to surface to the owner, reply with exactly `HEARTBEAT_OK`.
If there is something worth surfacing proactively, reply with:
HEARTBEAT_NOTIFY:
<concise owner-facing update>
