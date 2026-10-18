# Select and connect the client

The package has three skills. Each operation must select one named MCP
connection/instance. Matching display names, message IDs or build revisions do
not establish account identity. Bind using trusted client configuration and the
operator's account selection, and retain that binding throughout the workflow.

## Codex

Install from the generated local marketplace using the version's supported CLI.
The package's ./scripts/bridge.sh only execs the installed Go binary with
-config CONFIG serve. It neither logs in nor starts a collector. The public
package can read two absolute paths (binary then config) from
${PLUGIN_DATA}/connection. That is private host data, not a secret-bearing config.
Find PLUGIN_DATA using actual host metadata; do not invent its location.

For hosts without an observable PLUGIN_DATA, the package builder can create a
private instance-specific variant with ZL_MCP_BINARY and ZL_MCP_CONFIG in the
server's env. These are literal absolute paths, not secret values or portable
placeholder expansions. Do not distribute that personalized artifact. See the
installation guide's commands. An environment variable in the parent shell
is not proof it reaches a GUI client.

After installation verify skills discovery and a harmless status call in the
actual client. If setup needs a new conversation, explain it; do not silently
create chats. STDIO exposes tools/resources, including journal reads, but does
not supply webhook subscriptions or automatic event wakeups.

## ChatGPT

ChatGPT cannot run the local STDIO command in the cloud. Create/select a named
personal MCP connection to that instance through its own Secure MCP Tunnel.
The tunnel runtime key belongs to tunnel-client, not the plugin or Zalo config.
Do not request that the user paste it into chat.

A skills upload and an existing registered MCP connection are separate facts.
Use only an actual verified app ID if creating a private package variant with
.app.json. Never publish that binding in the public package. Preserve the
current plugin/connection; do not recreate it to refresh discovery.

Do not promise that multiple tunnel-backed instances appear as Connected
accounts in one card. Prefer explicitly named personal connections if that is
the supported route. Verify two accounts separately; never substitute the
working first connection when the second fails. Tunnel setup, receivers,
subscriptions and notification rules remain separate user decisions.
