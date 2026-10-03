# macOS installation and operation

This guide installs from source for the current user. Source files can stay in your checkout; the installed binary, credentials, database, and logs should be on local storage outside iCloud or another synced directory. A user LaunchAgent runs after login, while the Mac is awake. It does not collect while the machine sleeps or before user login.

## Storage layout

| Item | Recommended location | Permissions |
| --- | --- | --- |
| Binary | `~/.local/bin/zl-mcp` | 0755 |
| Configuration | `~/.config/zl-mcp/config.toml` | Directory 0700, file 0600 |
| Session, SQLite, token, control socket | `~/.local/share/zl-mcp` | Directory 0700, private files 0600 |
| Application logs | `~/Library/Logs/zl-mcp` | Directory 0700, files 0600 |
| LaunchAgent | `~/Library/LaunchAgents/com.skosovsky.zl-mcp.service.plist` | 0644 |
| Private backups | A local directory outside the checkout | Directory 0700, files accessible only to the user |

Use one state directory per account. Permissions do not provide encryption. Protect backups as carefully as the live session. Keep `state_dir` short to fit the Unix socket path limit. TOML paths do not expand literal `~` or `$HOME`: enter the actual absolute home path. Relative paths resolve against the configuration directory.

## Build and configure

Install Go 1.26.5 or a compatible newer version. From the repository root:

```sh
go build -o bin/zl-mcp ./cmd/zl-mcp
mkdir -p "$HOME/.local/bin"
install -m 0755 bin/zl-mcp "$HOME/.local/bin/zl-mcp"
mkdir -p "$HOME/.config/zl-mcp" "$HOME/.local/share/zl-mcp" "$HOME/Library/Logs/zl-mcp"
chmod 0700 "$HOME/.config/zl-mcp" "$HOME/.local/share/zl-mcp" "$HOME/Library/Logs/zl-mcp"
```

For a **new installation**, copy the example without replacing an existing private configuration:

```sh
cp -n config.example.toml "$HOME/.config/zl-mcp/config.toml"
chmod 0600 "$HOME/.config/zl-mcp/config.toml"
```

Edit the installed file. Replace `state_dir` with `/Users/YOUR_USER/.local/share/zl-mcp` using your actual home path. The example uses `collection.mode = "all"`. Choose `selected` with typed entries, or an empty list, if you want to restrict collection before first startup. Keep `allow_join = false` unless you need joining, and the loopback MCP listen address. Logs default to the layout above. The service generates its private `mcp-token` file on first startup.

If an installation already exists, follow the update section instead. Before starting, check for another LaunchAgent managing this account, including the historical label `me.skosovsky.zl-mcp.collector`. Unload the old agent and ensure its process has stopped; do not install a second supervisor for the same service. A duplicate state directory is rejected by the account lock; a duplicate default endpoint also conflicts on its port.

## Sign in and check the service

With the service stopped:

```sh
"$HOME/.local/bin/zl-mcp" -config "$HOME/.config/zl-mcp/config.toml" login
"$HOME/.local/bin/zl-mcp" -config "$HOME/.config/zl-mcp/config.toml" service
```

Open the QR file reported by `login` locally and scan it with Zalo on your phone. It is removed after login; credentials stay in the state directory. Do not upload the QR image or session files. The service remains in the foreground for this check; connect an MCP client using [these examples](local-connection.md), then call `zalo_get_status` and `zalo_list_groups`.

Stop the foreground service with Ctrl-C. Choose `all` or typed selected conversations before installing launchd, following [conversation collection](conversations.md). Empty selected policy means no collection. In selected mode, joining a group does not enable collection automatically; all mode admits accessible newly discovered groups.

## Install the LaunchAgent

From the repository root:

```sh
mkdir -p "$HOME/Library/LaunchAgents"
cp -n deploy/macos/com.skosovsky.zl-mcp.service.plist.example "$HOME/Library/LaunchAgents/com.skosovsky.zl-mcp.service.plist"
```

Edit the copied plist, replacing **every** `/Users/YOUR_USER` with your actual absolute home path. Launchd does not expand shell variables or `~`. Preserve the separate `ProgramArguments` elements. If your path contains XML special characters, escape them correctly. The log directory must already exist.

```sh
chmod 0644 "$HOME/Library/LaunchAgents/com.skosovsky.zl-mcp.service.plist"
plutil -lint "$HOME/Library/LaunchAgents/com.skosovsky.zl-mcp.service.plist"
launchctl bootstrap "gui/$(id -u)" "$HOME/Library/LaunchAgents/com.skosovsky.zl-mcp.service.plist"
launchctl print "gui/$(id -u)/com.skosovsky.zl-mcp.service"
```

The template launches the Go binary directly, starts at login, restarts after failure, and throttles failed restarts. It needs no Python wrapper. Stop the foreground service before bootstrap. Verify `zalo_get_status` through MCP after startup; a running launchd job alone does not prove Zalo connectivity.

## Stop, start, and reauthenticate

Stop before changing the binary, doing a consistent backup, or QR login:

```sh
launchctl bootout "gui/$(id -u)" "$HOME/Library/LaunchAgents/com.skosovsky.zl-mcp.service.plist"
```

After editing the collection policy or completing login, start again:

```sh
launchctl bootstrap "gui/$(id -u)" "$HOME/Library/LaunchAgents/com.skosovsky.zl-mcp.service.plist"
```

Reconnect STDIO clients after a restart. `auth_required` keeps MCP and corpus reads available while stopping upstream operations. Stop the service, run the `login` command above, then bootstrap. Network failures retry with backoff and do not require QR login. Avoid starting login while service owns the account lock.

## Logs and troubleshooting

Application JSON logs rotate in Go: default maximum 5 MiB and three backups. Main logs are in `service.log`; safe early-startup diagnostics are in `startup.log`. For the full startup error, stop the agent and run the service interactively. Launchd stderr points to `startup.log`; OS loader errors or runtime crash traces can bypass application rotation and require separate inspection.

- Missing service/token: start the main service before a STDIO bridge.
- Lock or port error: check for an existing foreground service or LaunchAgent before retrying.
- Empty search results: check the collection mode and typed scope, first persistence time, and corpus coverage.
- Storage permission error: verify ownership and private permissions; do not run the service with sudo.
- Events failure: inspect `zalo://events/diagnostics` and the callback policy. The client must supply a supported public HTTPS receiver.

Do not include tokens, configuration secrets, session files, or private messages in bug reports.

## Update, backup, and rollback

Build and test the new revision in the checkout before stopping the installed service. Record the installed revision and keep its binary/config/plist. After bootout and process exit, make a private copy of the **entire** state directory, including any SQLite WAL/SHM files, plus the configuration and LaunchAgent. Do not copy a live database as an update backup. Do not put backups in Git or iCloud.

Stage the new binary beside the installed binary, then replace it on the same filesystem:

```sh
install -m 0755 bin/zl-mcp "$HOME/.local/bin/zl-mcp.new"
mv "$HOME/.local/bin/zl-mcp.new" "$HOME/.local/bin/zl-mcp"
```

Apply any documented config migration, bootstrap, and verify status, saved search/context, and delivery diagnostics. An update does not register MCP client settings automatically.

To roll back, stop the new service and preserve a private copy of its current state. Restore the previous binary/config/plist. Restore the matching pre-update state only if required for database compatibility; this discards messages and subscription changes collected since that backup, so retain the newer copy for recovery. Bootstrap and verify again. Do not mix database files from different backups.

## Uninstall

Bootout the LaunchAgent, then remove its plist and `~/.local/bin/zl-mcp`. Remove the MCP entry from your client and optionally remove the copied skills. Keep configuration, state, logs, and backups by default. Deleting them is a separate explicit action; state removal loses the saved Zalo session, corpus, approvals, subscriptions, and delivery queue.

Linux builds are checked, but this LaunchAgent setup applies only to macOS. There is no supported systemd installer; service and startup log defaults currently use macOS-style paths.

### Diagnose missing message ingestion

`zalo_ingestion_diagnostics`, `zalo_ingestion_pressure` and `zalo_persistence_diagnostics` snapshots distinguish frames, direct/group ingestion, replay, decoding errors, backpressure, cancelled emissions, policy exclusions, duplicates and committed insertions. They contain counts and protocol header numbers only and reset for each listener. A key exchange or empty replay does not prove completeness. Message/replay/undo buffers wait for the consumer rather than evicting older data; shutdown cancellation can interrupt queued work. Successful live enqueue does not prove persistence. Compare known existing messages with typed MCP search and local coverage, not just future message arrival.

## Upgrading a group-only corpus

The conversation migration is transactional and preserves old messages as `group`, sequence numbers, identities, gaps, subscription boundaries/generations, queued payload bytes and the fanout checkpoint. Do not restore an old binary onto the upgraded schema. A compatible rollback restores the matching stopped-service backup of binary/config and the entire database state, retaining the newer state privately for later recovery. No source checkout or Zalo session reset is required.

Legacy configs containing only `collection.group_ids` retain their group-only meaning. To enable all conversations, remove `group_ids` and set `mode = "all"` under `[collection]`; do not combine the two forms. Collection changes do not expand existing callback scopes. The subscription start boundary is preserved, so newly recovered messages may be delivered only if they match that subscription. See [conversation collection](conversations.md).
