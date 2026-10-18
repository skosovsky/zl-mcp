# Distributable plugin acceptance

Status: **in progress**, 2026-10-18. The full goal is not accepted. In particular,
existing live MCP connectivity does not prove that ChatGPT loaded these packaged
skills or supports two independently tunnel-backed account connections.

Requirements: [task](task-distributable-plugin.md). Installation:
[manual guide](plugin-installation.md). Production service/config/state and its
personal ChatGPT connection have not been changed by packaging. The user will
uninstall the old installed plugin and install from GitHub personally once the
candidate is ready. In-place ChatGPT updates are not authorized for this task.

## Current evidence

- Agent Plugins 1.0.0 schemas pinned locally; package builder validates manifests,
  skill metadata, included references and allowlisted archive content.
- Builder negative controls cover missing references, invalid binding paths,
  literal launcher arguments, extra binding lines and private app variants.
- Two identical builds yield identical ZIP SHA-256; launcher mode is 0755.
- New setup skill initialized and validated with skill-creator; two existing
  workflow skills retain their original responsibilities and add account binding.
- `TestNativePluginInstanceIsolation` uses two actual production service processes
  and actual Go STDIO bridges with synthetic account-bound SQLite state.
  Same IDs return different per-instance content, operations and journals;
  foreign tokens receive 401, foreign receipts are rejected, ack checkpoints
  remain independent. Duplicate state/port fail without stopping neighbors;
  stopping/restarting one instance leaves the other readable.
- The native test intentionally has no Zalo sessions and reports auth_required.
  It is not a live Zalo login/collection test.
- `TestNativePluginSharedBinaryReplacement` exercises two temporary instances
  using a shared binary: stop both, snapshot both complete private states,
  atomically replace with a distinct same-source stripped build, restart/read,
  stop, restore baseline binary and stopped snapshots, restart/read, then remove
  A while B and the shared binary remain. This proves operational replacement
  and rollback, not database compatibility between different product versions.

Reproduce package checks with the pinned build-time virtual environment from
[installation](plugin-installation.md). Reproduce native isolation with:

```sh
CGO_ENABLED=0 go build -trimpath -o /tmp/zl-plugin-native ./cmd/zl-mcp
CGO_ENABLED=0 go build -trimpath -ldflags=-w -o /tmp/zl-plugin-upgrade ./cmd/zl-mcp
ZL_MCP_ACCEPTANCE_BINARY=/tmp/zl-plugin-native \
ZL_MCP_UPGRADE_BINARY=/tmp/zl-plugin-upgrade \
  go test -race ./internal/service -run '^TestNativePlugin(InstanceIsolation|SharedBinaryReplacement)$' -count=1
```

## Host and behavioral evidence

- [Codex host proof](plugin-evidence/codex-host.json): actual portable archive
  installed in isolated Codex 0.160.0, three skills loaded, onboarding discovered,
  27 tools and read-only status through the actual packaged launcher.
- [Codex lifecycle proof](plugin-evidence/codex-lifecycle.json): two real native
  services and instance-specific packages simultaneously expose 27 tools each;
  upgrading, rolling back and removing A preserve B's cache/config/token/PID.
  Package removal does not stop either service. The discovered server-key
  collision is fixed by instance-qualified `zalo-<instance>` keys.
- [Independent setup dry run](plugin-evidence/onboarding-dry-run.json): ten
  realistic requests cover no Mac, pending QR, occupied port, lazy tools,
  existing layout/reinstall, an unrelated research request, injected source
  instructions, first installation, approved-QR continuation and connection refusal. No behavioral errors observed. This is a subagent dry run,
  not a host onboarding invocation or live QR test.
- [Review A](plugin-evidence/review-a.md) and [review B](plugin-evidence/review-b.md)
  are explicitly interim reports. Their hashes and scores refer to the state
  each reviewer inspected; refresh them for final acceptance.

## Open acceptance gates

| Criterion | Current boundary |
| --- | --- |
| A01/A05 | Codex install/discovery/bridge proven; actual onboarding host turn and ChatGPT installation remain pending |
| A07 | Real loading of the newly packaged skills and setup in ChatGPT is not yet demonstrated; existing tools are separate evidence |
| A08 | Two native service instances are isolated; two independently named ChatGPT tunnel connections still require host verification |
| A11–A13 | Ten independent behavioral dry runs recorded; actual host setup remains distinct and pending |
| A14/A15 | Real Codex package upgrade/rollback/removal, service restart isolation and shared-binary operational replacement/rollback proven; separate tunnel lifecycle remains to be assessed |
| A17/A18 | Final complete checks, two independent reviews and GitHub publication remain pending |

No current review score is a final 18/18 acceptance. Tests and host probes must
identify the reviewed revision/archive hash; do not turn untested host behavior
into PASS by changing the wording of a requirement.
