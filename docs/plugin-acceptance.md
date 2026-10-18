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
| A11 | Ten independent behavioral dry runs recorded; actual repeated host setup remains pending (reviewers distinguish dry-run evidence from real host outcomes) |
| A12–A16 | Local dry runs, package and native lifecycle checks and separate service/tunnel verification instructions are recorded; no production tunnel crash/reboot outcomes are claimed |
| A18 | Final two-review 18/18 acceptance remains pending the user-run host checks; candidate publication and CI are complete |

No current review score is a final 18/18 acceptance. Tests and host probes must
identify the reviewed revision/archive hash; do not turn untested host behavior
into PASS by changing the wording of a requirement.

## Candidate publication

Implementation began at `e22dfbc53a6fe4823970b9be1ea4b56ea984ba92`;
current published candidate: `20127019abe45dcfe26f7beeb98ce29d90e2229b`.
Public archive SHA-256: `f855ab3d6b77aadd8b1d7cd5cd286c399f19b09f9de80f86e16ce6b464b4ff54`.
The signed commit was exported without local runtime files and rebuilt; schema,
package negative controls and the archive hash matched. Gitleaks scanned both a
clean indexed tree (136 MB) and the published commit diff: no leaks found.

The [clean reinstall checklist](plugin-reinstall-checklist.md) is the next
user-run acceptance stage. The candidate CI is green on both platforms. Existing ChatGPT
files were inspected read-only: no account plugin update was applied.

[Publication proof](plugin-evidence/publication.json) pins the public release,
unauthenticated asset download hash and the successful Linux/macOS CI jobs and
steps to the exact candidate commit. [CI run](https://github.com/skosovsky/zl-mcp/actions/runs/39124811100)
includes the seven package controls, full root and nested-module test/race/vet,
architecture/lint checks, offline synthetic checks, three platform builds and
both native acceptance tests. A17 is confirmed for this implementation revision;
this does not close the separate user-host criteria. Documentation-only follow-up
records these already completed checks without changing the plugin artifact.
