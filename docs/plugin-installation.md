# Install the Zalo MCP plugin

This package bundles three skills and a client-side STDIO launcher for an already
running Go service. Each Zalo account owns a separate service instance. Source
installation on macOS is supported; a plugin install does not install the server,
log in to Zalo, create subscriptions or keep a sleeping Mac awake.

The package is distributed through GitHub, not the public ChatGPT directory.
See [acceptance evidence](plugin-acceptance.md) for exactly what has been tested.
Do not interpret a documented configuration as a verified host capability.

## Build the public package

From a complete clone with the Go version in `go.mod`:

```sh
git clone https://github.com/skosovsky/zl-mcp.git
cd zl-mcp
go build -trimpath -o bin/zl-mcp ./cmd/zl-mcp
python3 -m venv /tmp/zl-plugin-tools
/tmp/zl-plugin-tools/bin/pip install -r scripts/plugin/requirements.txt
/tmp/zl-plugin-tools/bin/python scripts/plugin/package.py --check
/tmp/zl-plugin-tools/bin/python scripts/plugin/package.py --output /tmp/zl-mcp-plugin.zip
```

Use a new output filename if it exists. The builder refuses overwrites, copies
only the manifest, launcher, LICENSE and three skills, rejects broken packaged
references and symlinks, and produces deterministic ZIP bytes. It never reads
real configuration, state, sessions, tokens or logs. Python is build/validation
tooling only; neither LaunchAgent nor the plugin runs a Python service wrapper.
The included schema files pin Agent Plugins 1.0.0; [schema provenance](../scripts/plugin/schemas/README.md).

Extract the ZIP into a new directory. Preserve executable permission on
`scripts/bridge.sh`. Do not install the entire checkout as a plugin: the package
allowlist excludes private local files and unrelated development material.

## Install or select the account's service

Existing installation: keep its actual config/state/label. Do not move it into
the new instance layout merely to install a plugin. Check existing LaunchAgents,
including the historical `me.skosovsky.zl-mcp.collector`, before starting anything.

New instance: choose a non-personal identifier, e.g. `personal` or `work`, and a
free loopback port. These are examples, not assigned ports. Use independent paths:

| Item | Instance `personal` |
| --- | --- |
| Shared executable | `~/.local/bin/zl-mcp` |
| Config | `~/.config/zl-mcp/instances/personal/config.toml` |
| Session, database, archives, token | `~/.local/share/zl-mcp/instances/personal` |
| Logs | `~/Library/Logs/zl-mcp/personal` |
| Agent label | `com.skosovsky.zl-mcp.personal.service` |
| Endpoint | A free `127.0.0.1:PORT`, distinct from every other instance |

Create private directories with mode 0700 and config mode 0600. Start from
`config.example.toml`, then set **absolute** paths for `state_dir`,
`logging.file`, `logging.event_trace_file` and `mcp.token_file`, and the chosen
`mcp.listen`. Set collection policy before startup. TOML does not expand `$HOME`
or `~`. The token is generated privately on service startup, not by the plugin.

Follow [macOS operation](mac-deployment.md) for source installation, local QR
login while the service is stopped, foreground readiness, and direct-Go
LaunchAgent setup. For a new instance, adapt
[the instance template](../deploy/macos/com.skosovsky.zl-mcp.INSTANCE.service.plist.example),
replacing `INSTANCE`, every `/Users/YOUR_USER`, and any chosen paths. Escape XML
characters. Run `plutil -lint` on the resulting file, stop the foreground process,
then bootstrap only that instance's plist. Never run both old and new agents
for the same account. Each account needs its own local login/session.

## Codex: manual local marketplace

The portable `mcp.json` launches `./scripts/bridge.sh`. The script reads two
literal absolute paths (binary and config) from `PLUGIN_DATA/connection`, or from
an explicitly personalized package. It never evaluates shell data, logs in, or
starts `service`. It fails closed when unconfigured; the setup skill stays useful.

For a new install, creating a private instance-specific package avoids guessing
the host's `PLUGIN_DATA` location. Replace the example username and paths:

```sh
/tmp/zl-plugin-tools/bin/python scripts/plugin/package.py \
  --instance personal \
  --binary /Users/YOUR_USER/.local/bin/zl-mcp \
  --config /Users/YOUR_USER/.config/zl-mcp/instances/personal/config.toml \
  --output /tmp/zl-mcp-personal.zip
```

This variant contains local paths, not tokens; keep it private. Extract it into
`/tmp/zl-marketplace/plugins/zl-mcp-personal` (or a persistent local directory),
then create `/tmp/zl-marketplace/.agents/plugins/marketplace.json`:

```json
{
  "name": "zl-local",
  "interface": {"displayName": "Local Zalo"},
  "plugins": [
    {"name": "zl-mcp-personal", "source": {"source": "local", "path": "./plugins/zl-mcp-personal"}, "policy": {"installation": "AVAILABLE", "authentication": "ON_USE"}}
  ]
}
```

The marketplace source layout and commands must match your client version:

```sh
codex --version
codex plugin marketplace add /tmp/zl-marketplace --json
codex plugin add zl-mcp-personal@zl-local --json
```

A second account uses a second private package name, config and instance-qualified
MCP server key (`zalo-personal`, `zalo-work`), generated by the builder. Codex
0.160.0 does not expose both plugins reliably when their server keys collide. Do not append
another collector to the first package or change its account implicitly.
Open/reload the plugin in Codex, check all three skills and run setup. Verify a
harmless `zalo_get_status` through the intended connection. Use the actual host
catalogue; an identical build version does not identify the account. Resources
and tool reads work through STDIO; webhook subscription methods and automatic
wakeups require HTTP plus a compatible receiver/host.

For a portable unpersonalized install, find the actual `PLUGIN_DATA` directory
from the client's runtime metadata and place the two paths in a `connection`
file there, each followed by a newline. Do not guess the cache path or write
credentials into it. Cache can be replaced on upgrade; PLUGIN_DATA persists.
The private package's explicit env binding takes precedence over that file.

## ChatGPT: skills plus personal tunnel connection

ChatGPT cloud cannot execute the package's local STDIO process. Install the
skills as a private package and separately connect a named personal MCP app to
the account's own Secure MCP Tunnel. The local service remains loopback-only.
The public archive has no account IDs or registered app mapping.

Create the tunnel in OpenAI Platform following
[Secure MCP Tunnel](https://developers.openai.com/api/docs/guides/secure-mcp-tunnels).
Run tunnel-client separately with its own local credentials and LaunchAgent.
Choose a separate profile, tunnel, agent label and health port for each instance
unless a tested routing configuration proves otherwise. Do not share the
runtime API key or put it into the plugin. Check tunnel health/readiness and
then an actual MCP call from ChatGPT; either check alone is insufficient.

In ChatGPT Plugins create a personal custom MCP connection, choose Tunnel,
and select that instance's tunnel. Name connections distinctly. Preserve an
existing working connection rather than recreating it. Multi-account credentials
for one registered endpoint do not automatically route to several tunnel IDs.
The two-connections route and its current acceptance limits are recorded in
[host evidence](plugin-acceptance.md); it is not a promise of Google Drive's
Connected accounts interface.

To bind skills to an **already verified** personal app, create a private variant:

```sh
/tmp/zl-plugin-tools/bin/python scripts/plugin/package.py \
  --instance personal --app zalo=VERIFIED_APP_ID \
  --output /tmp/zl-mcp-personal-chatgpt.zip
```

Replace VERIFIED_APP_ID with the exact registered app ID from trusted discovery.
The builder validates its format but cannot establish ownership or connectivity.
It removes STDIO and writes `.app.json`. Never publish this personalized variant
or substitute somebody else's app ID. Use the client's supported private-package
upload flow; if no such surface/tool is available, stop at the prepared ZIP and
record host installation as pending. Do not claim packaged skills loaded merely
because the existing MCP tool works. Installation or replacement of the current
personal plugin is a separate action, not implicit in source packaging.
For the current project acceptance the user will uninstall the old installed
copy and install from GitHub personally after the candidate is ready. Do not
update the existing plugin in place. Uninstalling a copy is different from
deleting its registered app or revoking the tunnel; retain the latter unless
explicitly requested. Save the current connection identity locally first.

After loading, check three skills, run setup in the actual host, and verify a
status call through the intended named connection. Account selection must be
explicit when ambiguous. Confirm subscriptions separately; package install and
skill toggles do not register, cancel or authorize notifications.

## Verify the service and tunnel separately

For a new instance, the [tunnel agent template](../deploy/macos/com.skosovsky.zl-mcp.INSTANCE.tunnel.plist.example)
runs tunnel-client directly. Replace its binary path with the installed location
(`command -v tunnel-client`), INSTANCE and the absolute profile/log paths. Create
private profile/key directories first; the profile references a private runtime
key file, never a literal key in the plist. Choose a distinct loopback
`--health.listen-addr` in the profile using your installed version's supported
configuration fields. Confirm flags with `tunnel-client run --help` before use.
Check foreground readiness before registering the agent. The template does not
create a Platform tunnel/profile or migrate an existing supervisor.

Use the **actual** labels of an existing installation rather than installing new
agents alongside them. For a new `personal` instance, the following checks are
separate. Replace the example health port with the port actually configured:

```sh
launchctl print "gui/$(id -u)/com.skosovsky.zl-mcp.personal.service" \
  | rg '^\s*(state|pid|runs|last exit code|last terminating signal) ='
launchctl print "gui/$(id -u)/com.skosovsky.zl-mcp.personal.tunnel" \
  | rg '^\s*(state|pid|runs|last exit code|last terminating signal) ='
curl --fail --silent --show-error http://127.0.0.1:19876/healthz
curl --fail --silent --show-error http://127.0.0.1:19876/readyz
```

Also call `zalo_get_status` through the intended client. Health endpoints above
belong to tunnel-client; zl-mcp does not expose them. A tunnel can be healthy while
the collector requires login; collector readiness and MCP transport are different.
Do not paste raw profile/launchctl output containing credentials into an issue.

To stop/start an agent intentionally, use `launchctl bootout gui/UID PLIST` and
`launchctl bootstrap gui/UID PLIST` for that exact plist (replace UID and PLIST).
For an explicitly authorized crash-recovery test, record the exact target PID,
then `launchctl kill SIGKILL gui/UID/LABEL`. Wait for a **different** PID without
kickstart/bootstrap, and verify readiness again. Test one component at a time:
service recovery plus local MCP, then tunnel recovery plus health/readiness and
an actual ChatGPT MCP call. Preserve private backups before service crash tests.
Never kill an unverified process or use a machine-wide kill command.

`RunAtLoad=true` starts at user login. `KeepAlive.SuccessfulExit=false` restarts
failed exits with a 30-second throttle, not intentional successful shutdowns.
This installation does not collect before login or while the Mac is asleep.
No reboot/login or current production tunnel crash test was performed for this
package acceptance; configuration validation is not proof of those live outcomes.

## Updates, rollback and uninstall

Record the package version, source revision and archive hash. Build and validate
before replacing an installed package; regenerate private variants with the same
instance binding. Refresh/reinstall with the client's supported flow. Preserve
PLUGIN_DATA. Check the package's skills and MCP independently after the update.

Before replacing a shared Go binary, inventory and stop **all** using instances,
verify their exits, and back up each full state/config/plist with private
permissions outside Git/iCloud. Preserve the previous binary. Replace atomically,
restart each instance, verify each connection and coverage. For rollback use a
schema-compatible binary or restore the stopped instance's complete pre-update
snapshot; data after that snapshot requires separate reconciliation.

Uninstalling the plugin removes client instructions/binding only. It does not
stop collectors/tunnels, revoke tokens, cancel subscriptions or remove Zalo data.
For one instance, boot out only its exact service and tunnel agents, verify exits,
then remove its plists/config if requested. Keep state/backups by default. Preserve
the shared binary and other instances. Use the ordinary
[macOS removal instructions](mac-deployment.md) only with the correct instance paths.

If the two skills were previously installed independently, compare origin and
content before disabling the duplicates in favor of the package. Do not delete
user-customized skills automatically. A disabled skill is not revoked MCP access.

| Component | State owner | Update/removal responsibility |
| --- | --- | --- |
| Package and skills | Client package manager | Install/update package; preserve user variants |
| Go service | Per-instance LaunchAgent | Local operator; preserve account-bound data |
| Tunnel | Separate tunnel-client agent | Local operator and Platform; independent readiness |
| Connection | Codex/ChatGPT | Select exact instance; refresh discovery |
| Zalo account/session | Instance's private state | Local QR login; no session sharing |
| Subscription/checkpoint | Instance and receiving host | Explicit subscription/automation management |
