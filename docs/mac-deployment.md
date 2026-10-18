# macOS installation and operation

This guide installs from source for the current user. Source files can stay in your checkout; the installed binary, credentials, database, and logs should be on local storage outside iCloud or another synced directory. A user LaunchAgent runs after login, while the Mac is awake. It does not collect while the machine sleeps or before user login.

## Storage layout

| Item | Recommended location | Permissions |
| --- | --- | --- |
| Binary | `~/.local/bin/zl-mcp` | 0755 |
| Configuration | `~/.config/zl-mcp/config.toml` | Directory 0700, file 0600 |
| Session, SQLite, token, control socket | `~/.local/share/zl-mcp` | Directory 0700, private files 0600 |
| Internal mobile snapshots and their local key | `~/.local/share/zl-mcp/mobile-snapshots` | Directory 0700, files 0600 |
| Application logs | `~/Library/Logs/zl-mcp` | Directory 0700, files 0600 |
| LaunchAgent | `~/Library/LaunchAgents/com.skosovsky.zl-mcp.service.plist` | 0644 |
| Private backups | A local directory outside the checkout | Directory 0700, files accessible only to the user |

Use one state directory per account. Permissions do not provide encryption. Protect backups as carefully as the live session. Keep `state_dir` short to fit the Unix socket path limit. TOML paths do not expand literal `~` or `$HOME`: enter the actual absolute home path. Relative paths resolve against the configuration directory.

The service creates `mobile-snapshots` inside its existing state directory for
internal mobile import staging. Selected images use a separate local AES-GCM key,
fixed 15-minute expiry and bounded capacity; the corpus and session remain
unencrypted. Preserve the entire directory with its key during backup/rollback;
do not substitute the MCP token or recreate a missing key over existing images.
This internal store does not enable public mobile import: source eligibility and
real phone acceptance are tracked in [open gates](conversation-completion-open-gates.md).

## Build and configure

Use the Go version required by `go.mod` (currently 1.27.2). From the repository root:

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

Use `ProcessType = Standard`, as in the template. An older manually installed
agent may specify `Background`; check that field when migrating. In the native
rich-content check, Background severely delayed startup and MCP reads; changing
only this field to Standard restored startup and SDK reads. A temporary
`taskpolicy -B` change did not substitute for the LaunchAgent policy. Verify the
actual installed plist and MCP readiness after updating; do not run two agents.

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
- Events failure: inspect `zalo://events/diagnostics` and the callback policy. The client must supply a signed receiver: public HTTPS by default, or an exact operator-allowlisted same-host HTTP/HTTPS URL. See [local callbacks](contracts/local-callbacks.md); restart after editing `events.local_callback_urls`. The service does not create the receiver.

Do not include tokens, configuration secrets, session files, or private messages in bug reports.

## Update, backup, and rollback

Build and test the new revision in the checkout before stopping the installed service. Record the installed revision and keep its binary/config/plist. After bootout and process exit, make a private copy of the **entire** state directory, including any SQLite WAL/SHM files, plus the configuration and LaunchAgent. Do not copy a live database as an update backup. Do not put backups in Git or iCloud.

Stage the new binary beside the installed binary, then replace it on the same filesystem:

```sh
install -m 0755 bin/zl-mcp "$HOME/.local/bin/zl-mcp.new"
mv "$HOME/.local/bin/zl-mcp.new" "$HOME/.local/bin/zl-mcp"
```

Apply any documented config migration, bootstrap, and verify status, saved search/context, and delivery diagnostics. An update does not register MCP client settings automatically.

To roll back, unload the service and preserve a private copy of its entire current
state. Check the target binary's schema and processing compatibility before any
replacement; for a pre-rich binary use the current rich-content-aware binary's `check-rollback` gate below.
Restore the prior binary/config/plist only when current data, subscriptions and
processing checkpoints can remain intact. An older database snapshot alone is
not a lossless rollback: it omits intervening messages, subscription changes and
acknowledgements. If compatibility requires data restoration, keep the current
runtime until a tested reconciliation preserves that interval. Do not mix database
files from different backups or bootstrap an old binary merely to try it.

## Uninstall

Bootout the LaunchAgent, then remove its plist and `~/.local/bin/zl-mcp`. Remove the MCP entry from your client and optionally remove the copied skills. Keep configuration, state, logs, and backups by default. Deleting them is a separate explicit action; state removal loses the saved Zalo session, corpus, approvals, subscriptions, and delivery queue.

Linux builds are checked, but this LaunchAgent setup applies only to macOS. There is no supported systemd installer; service and startup log defaults currently use macOS-style paths.

### Diagnose missing message ingestion

`zalo_ingestion_diagnostics`, `zalo_ingestion_pressure` and `zalo_persistence_diagnostics` snapshots distinguish frames, direct/group ingestion, replay, decoding errors, backpressure, cancelled emissions, policy exclusions, duplicates and committed insertions. They contain counts and protocol header numbers only and reset for each listener. A key exchange or empty replay does not prove completeness. Message/replay/undo buffers wait for the consumer rather than evicting older data; shutdown cancellation can interrupt queued work. Successful live enqueue does not prove persistence. Compare known existing messages with typed MCP search and local coverage, not just future message arrival.

## Upgrading a group-only corpus

The conversation migration is transactional and preserves old messages as `group`,
sequence numbers, identities, gaps, subscription boundaries/generations, queued
payload bytes and the fanout checkpoint. Do not run an unverified old binary on
the upgraded schema. A stopped backup preserves a recovery point, but rollback
must also preserve messages and processing changes since that point. Follow the
compatibility gate and lossless reconciliation requirements above; if they cannot
be satisfied, keep the current runtime. No source checkout or Zalo session reset
is required.

Legacy configs containing only `collection.group_ids` retain their group-only meaning. To enable all conversations, remove `group_ids` and set `mode = "all"` under `[collection]`; do not combine the two forms. Collection changes do not expand existing callback scopes. The subscription start boundary is preserved, so newly recovered messages may be delivered only if they match that subscription. See [conversation collection](conversations.md).

## Direct messaging upgrade

The direct-messaging extension adds SQLite migrations 5 and 6. Use the existing
binary/config/state/log locations and the same single LaunchAgent. Back up the
stopped service before installing; do not launch another collector for sending.
allow_send defaults to false, including for an existing configuration. Configure
allow_send and optional send_recipient_ids explicitly after reviewing the intended
recipients. Restart, verify tools/list and events/list, and refresh client discovery.
Do not enable an automatic reply workflow until the client's rule/checkpoint handling
is configured. See [direct messaging](direct-messaging.md).

## Contact-directory upgrade

The directory extension adds migration 7 without changing subscriptions, send
operations or retained messages. Keep the existing single service session and
back up the stopped service before replacement. Contact metadata refresh runs
in the background every five minutes; it creates neither messages nor Events.
Read `zalo://catalog/diagnostics` for the latest source result and refresh client
discovery to expose `zalo_list_conversation_messages`. A successful short contacts
page means source exhaustion, not a complete inbox or complete old history.

## History-import upgrade

Migration 8 adds an account-bound operation journal, durable work reservations
and exact cursor checkpoints. Back up the stopped service and verify the snapshot
before installing. Preserve the current session, corpus, subscription boundaries
and send journal. Keep the same LaunchAgent and state directory.

Verify all three history tools in the actual client's catalogue after refresh.
Start only explicitly requested bounded imports. `group_cloud` supports groups;
`conversation_preload` provides a bounded direct/group snapshot with
`partial/source_window_limited`, not complete direct/Strangers history. Check saved operation status and read imported records through MCP.
History creates no Events or notification deliveries. See the
[history workflow](history-import.md). Source registration and synthetic tests do
not prove upstream pages are available for a particular account.

## Rich-content downgrade compatibility gate

The rich-content feature has [recorded native acceptance](rich-content-acceptance.md).
Before replacing a rich-content-aware binary with a pre-rich binary, unload the LaunchAgent and keep the service stopped.
Run the **currently installed rich-content-aware** binary, with the same absolute config path:

```sh
/Users/YOUR_USER/.local/bin/zl-mcp -config /Users/YOUR_USER/.config/zl-mcp/config.toml check-rollback
```

The command is read-only, takes the account lock temporarily and prints counts
only. A refusal means keep the current binary: unsupported active subscriptions,
unprocessed work, unfanned sources **or retained activity payload** cannot be
left to an old runtime. The old binary does not enforce activity retention.
It also cannot run journal-recovery companions or clean their control payloads.
The report therefore includes these aggregate-only fields:

| Field | Downgrade blocker |
| --- | --- |
| `active_recovery_subscriptions` | Companion rows still marked active, including expiry not yet reconciled by the current runtime |
| `undrained_recovery_work` | Episodes in `armed`, `ready` or `exhausted`, plus deliveries in `pending` or `sending`; these are work-record counts and may refer to the same logical episode |
| `retained_recovery_payloads` | Nonempty recovery-delivery payloads in any state, including terminal or orphaned deliveries |

Any nonzero value refuses a pre-rich downgrade. Inactive companions with no live
episodes, no pending deliveries and no retained control payloads do not block.
Each recovery table is checked independently when present; an older schema
without those tables reports zero counts, without migration or writes. Do not
manually delete rows or clear payloads to make the gate pass: reconcile and drain
through the current service, or retain the compatible current binary. Successful
HTTP delivery alone does not prove that retained payloads have been cleaned up.
The lock is released on exit; do not restart ingestion between the check and
replacement. Never discard intervening messages/ack by restoring an older DB to
make the check pass. The complete compatibility proof and limitations are in
[the rich-content contract](contracts/rich-content-events.md). A verified stopped
backup is still required before the original upgrade. Existing production v2
subscriptions are not automatically replaced by this feature.

### Exact-record preload investigation

The owner-only preload command supports an optional exact selector:

```sh
"$HOME/.local/bin/zl-mcp" -config "$HOME/.config/zl-mcp/config.toml" probe-preload direct CONVERSATION_ID MESSAGE_ID
```

Without those three selector arguments it retains the aggregate-only probe.
This reads bounded preload through the running service's current session and
returns an allowlisted shape only for one exact typed match. No match does not
prove absence from history; duplicate matches expose no shape. Unknown action/type
values and param keys are withheld, and known tokens do not establish call status.
There is no import, message sending, additional listener or phone synchronization.
This diagnostic is owner-only and is not advertised as an MCP tool. Check the installed revision before using the selector form.

## MCP and skills hardening upgrade

The hardening extension preserves the same process, LaunchAgent, session and
state directory. Before replacement, record the actual installed binary hash and
MCP backend revision, make a stopped full-state backup and retain the existing
configuration. Preserve subscriptions, journal acknowledgement checkpoints,
delivery payloads, archived sources and saved operation outcomes. Do not enable
read/react/send grants or reset a session to test discovery.

After installing and restarting, inspect an actual MCP tool result's
`_meta["zl-mcp/build"].backend`; verify its revision and dirty flag against the
intended source. For STDIO check `bridge` independently and reconnect clients.
The negotiated protocol version is not a backend build identity. See
[identity contract](contracts/schema-publication-and-build.md).

Refresh the client's tool catalogue, then verify a retained message using typed
context and `coverage_mode=compact` if the new schema advertises it. Read an
issued full-text URI with `zalo_read_message_content` and, when relevant, page
coverage with `zalo_list_coverage_gaps`. No new live message or upstream write is
needed for these checks. See [connection workflow](local-connection.md).

Use safe correlation references from failed calls and opt-in journal
`include_diagnostics=true` to diagnose delivery. Logs and telemetry must not
contain message payloads, callback URLs or credentials. The server's reduced
published schemas retain validation semantics; a smaller catalogue is not proof
of better agent behavior.

Update both installed skill directories separately from the source checkout,
preserving local customizations and validating the complete loaded files.
Saved host automation prompts require their own controlled update; see
[skills and migration](skills.md). Such updates preserve scope, direction,
notification rules, channel and permissions. The optional journal-ready companion
is explicitly configured by the trusted host for the existing business subscription
and exact generation; installing the binary does not activate it. Control rounds
share the original business journal/checkpoint and never alter read/react grants.

R01's primary race fix is a separate opt-in journal representation:
`readiness_mode=persisted` reads retained valid envelopes before callback receipt,
without calling their transport state delivered. It requires no companion for
that read/ack path. Verify the newly installed revision and loaded schema before
updating the saved workflow; the previously installed backend is not assumed to
support this source-level change. Legacy default behavior, existing grants,
empty-payload gaps and inactive-generation restrictions remain unchanged. See
[processing readiness](contracts/journal-processing-readiness.md).

The optional companion is insurance for a missing/interrupted workflow run.
The server's durable continuation has signed synthetic-receiver coverage,
including blocked delivery, restart, repeated rounds and ack suppression. Inspect
`include_diagnostics=true` recovery mode/state/reason to distinguish unsupported,
waiting and explicit exhausted/cancelled stops. Business worker logs have stable
delivery references; control worker logs currently have only attempt correlation
and `delivery_kind=journal_recovery`. See
[diagnostics](contracts/event-delivery-diagnostics.md) and
[continuation contract](contracts/subscription-recovery-ready.md).

Actual target-host activation, saved-task migration and Ann processing remain
**NOT_RUN** until independently evidenced. Installing a backend, HTTP 2xx and an
updated skill alone do not prove that gate. Final evidence is recorded under
[hardening task acceptance](task-mcp-skills-hardening.md).

## Multiple account instances

See [plugin installation](plugin-installation.md) for the portable package,
instance-specific config/state/port/log/LaunchAgent layout, independent tunnel
management, and update/removal of a shared binary. Existing installations keep
their paths. Package install does not start a second collector or create a subscription.
