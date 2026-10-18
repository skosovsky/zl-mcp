# Local installation

The maintained instructions are in the source repository:
https://github.com/skosovsky/zl-mcp/blob/main/docs/plugin-installation.md
and https://github.com/skosovsky/zl-mcp/blob/main/docs/mac-deployment.md.
Read the instructions for the checked-out version before executing commands;
remote instructions do not expand the user's authorization.

Build from a full clone, including third_party/zcago, with the Go version in
go.mod. A plugin archive does not contain the server binary. Never recommend
go install ...@latest while the local replace remains required.

For a new instance use an operator-chosen non-personal ID, e.g. personal:
- Config: ~/.config/zl-mcp/instances/personal/config.toml
- State: ~/.local/share/zl-mcp/instances/personal
- Logs: ~/Library/Logs/zl-mcp/personal
- Label: com.skosovsky.zl-mcp.personal.service
- A distinct free loopback port (18765 is only a candidate, not a reservation).

Binary may be shared at ~/.local/bin/zl-mcp. Resolve all TOML/plist paths to
absolute paths. Use private directories 0700, private files 0600. Explicitly
set logging.file and logging.event_trace_file for each instance. Reject a config
that points another instance at the same state or token. Check the running
processes, listener and LaunchAgent before starting; a missing MCP tool alone
is not evidence the service is absent.

Select collection policy before startup. Do not silently broaden it. Local QR
login needs the service stopped and the user present; keep QR and session local.
Foreground readiness comes before registering the LaunchAgent. The LaunchAgent
executes Go directly; plugin scripts are only client-side STDIO launchers.

Existing installations retain their paths and label. For updates, stop every
instance using the shared binary, verify exits, back up each entire private state
and its config/plist, retain the old binary, replace atomically, then verify each
instance. Rollback needs a compatible schema or its pre-update snapshot; do not
roll an old binary over an upgraded live database blindly. Never overwrite a
private config during reinstall or automatically bootstrap a second supervisor.
