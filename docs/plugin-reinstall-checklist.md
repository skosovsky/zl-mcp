# Clean reinstall acceptance

The user performs this check after candidate publication. Do not update the
existing ChatGPT plugin in place. This checks the plugin, not a fresh Zalo login.
Keep the current service, session, corpus, tunnel and runtime key.

## Before uninstalling

1. Record the selected instance's config path and the named client connection
   locally. Record the registered app binding before removing an installed copy.
   Do not post tokens, session data or runtime keys.
2. Call `zalo_get_status` through that connection and record safe fields:
   collector/auth state, stored-message count and available build identity.
3. Check whether the research/event skills are also independently installed.
   Preserve custom versions and disable duplicates only for this clean test.

## Install from GitHub

Use the exact candidate branch/revision supplied with the handoff, not a moving
unreviewed checkout. Follow [plugin installation](plugin-installation.md).

- **Codex:** uninstall only the old client package; build/extract the new ZIP,
  register its local marketplace and install the instance-specific package.
  Keep its binding to the existing config. No server reinstall is required.
- **ChatGPT:** uninstall the old installed copy, without deleting the registered
  app or revoking its tunnel. Generate the private ChatGPT package from the
  GitHub source with the verified app binding and load it through the supported
  private-package/Plugin Creator flow. The public portable ZIP's local STDIO
  launcher cannot run in the ChatGPT cloud. If the required private-package
  installation flow is unavailable, report that limitation rather than claiming
  the existing MCP connection proves the package installed.

A private app-bound ZIP is generated locally; it is not uploaded to the public
repository. Binding preserves credentials in the host. Do not copy an API key
into a package to work around an unavailable connection flow.

## Short test cases

1. **Skills:** the installed package shows research, event handling and setup.
2. **Setup:** run the package's setup entrypoint: “Check my existing Zalo
   connection; do not reinstall the service or create subscriptions.” It must
   select the correct named instance and verify its status without another login.
3. **Read:** ask “Show the available conversations in this account.” Verify the
   tool is called through the selected package connection. Do not mark read,
   send, react, join or import history.
4. **Repeat:** run setup again. There must still be one service per account,
   the same state/config, no new subscriptions and no new QR login.
5. **Account selection:** if testing two account instances, use distinct named
   connections. Ask for each account's status separately, then give an ambiguous
   request; setup must ask which account rather than silently choosing one.
6. **Resume:** in the current conversation and a new user-created conversation,
   verify setup uses the same instance binding. The user creates any new chat;
   the implementation does not open one automatically.

## Evidence to retain

Record client version, source revision/archive hash, plugin version, loaded skill
names, whether setup actually ran, safe status output and the selected connection
alias. Distinguish a server response from a host-generated status message.
Retain evidence of the repeat and two-account checks separately. A tool result
from another old enabled plugin is not proof of the newly installed binding.

Successful source/package checks and independent dry runs are already recorded
in [acceptance](plugin-acceptance.md). These user-host results close the remaining
host-specific gates; both reviewers must re-evaluate A01–A18 on the final evidence.
Until then the goal remains unaccepted, even if the candidate's CI is green.
