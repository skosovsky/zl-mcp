# Resume and diagnose

- No tools loaded: use the host's discovery mechanism first. If unavailable,
  state that connectivity is unverified, not that installation is missing.
- Configured but unavailable: distinguish local service, STDIO bridge, tunnel,
  ChatGPT connection and catalogue discovery. Check each component separately.
- auth_required: retained corpus reads may still work. For new collection,
  stop only the selected service, run one local login, then start it again.
- reconnecting: inspect safe error categories; do not force a fresh login for
  an ordinary network retry. Never expose raw session/token or message logs.
- Port/lock conflict: identify the existing owner. Do not kill unrelated jobs,
  choose another account automatically, or create another state for the same
  account to bypass a lock. Resolve before starting.
- Interrupted setup: record instance/config, completed stage and next action.
  Recheck actual processes and files before resuming. A previous plan is not
  evidence a process is running. Do not duplicate login or tunnel requests.
- Shared binary update: stop all affected instances and preserve per-instance
  backup/rollback. Removing a plugin does not uninstall services, cancel
  subscriptions or revoke credentials. Removing one instance must preserve the
  shared binary and other agents; keep data by default.

A running LaunchAgent is not proof of collector readiness. RunAtLoad means
start after user login; KeepAlive with SuccessfulExit=false restarts failures,
not intentional successful shutdown. A sleep interval is not collection time.
Test crash recovery only for an authorized exact target and never infer reboot
or tunnel recovery from a service-only test.
