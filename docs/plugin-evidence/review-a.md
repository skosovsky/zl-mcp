# Independent review A — distributable plugin

Reviewed 2026-10-18 against all A01–A18 in `task-distributable-plugin.md`.
Reviewer role: requirements, package contracts and documentation; no production
installation, session, subscription or tunnel was modified.

Base commit: `a64c5a874a4677f5753f947a48209bc59b0a5a25`; packaging changes are
currently uncommitted. Reviewed public archive SHA-256:
`f855ab3d6b77aadd8b1d7cd5cd286c399f19b09f9de80f86e16ce6b464b4ff54`.
Package-content digest (sorted name NUL bytes NUL, SHA-256):
`c91c4f777f41577d248ce5da7b6a6d5f1be0ea5f55628825d9c68de80769ff43`.
These hashes identify the package, not later edits to review reports or tooling.

## Independent checks

- Package schemas and builder validation PASS; six negative/reproducibility
  tests PASS, including newly added ancestor-symlink rejection.
- Two independent archive builds have identical bytes; 26 members; executable
  launcher mode 0755. Allowlisted archive read manually and checked for personal
  absolute paths, app/tunnel IDs and long numeric account IDs: no matches.
- `skill-creator/scripts/quick_validate.py skills/setting-up-zalo` PASS.
- Generated system reference check PASS: 27 tools, five resources, four templates,
  six events, 54 reactions, 20 CLI commands, 120 local links.
- Built a new native executable from the working source with CGO disabled.
  `ZL_MCP_ACCEPTANCE_BINARY=/tmp/zl-review-a-native go test -race ./internal/service
  -run '^TestNativePluginInstanceIsolation$' -count=1` PASS (5.588 seconds).
- Independently ran `scripts/plugin/codex_probe.py` on the reviewed archive in an
  isolated temporary Codex home with a fresh synthetic service. Codex CLI
  0.160.0 installed the package, loaded three skills, discovered onboarding,
  launched the STDIO bridge and exposed 27 tools; status returned expected
  auth_required. No model/API turn was run. Safe result is independently
  reproducible; the maintained `codex-host.json` now identifies the same archive.
- Substituted personal/work instance plist templates and parsed both with
  plistlib: labels, configs and log paths are distinct; no launchd job installed.
- Independently reran `codex_lifecycle_probe.py`: two scoped packages and native
  services expose 27 tools each; package A upgrade0.1.0→0.1.1, exact-byte
  rollback and removal preserve B cache/config/token/PID and leave both native
  services running. Public hash remains the reviewed `f855ab3d…b4ff54`.
  Script and builder SHA-256 match `codex-lifecycle.json`. This checks package
  lifecycle, not replacement of the shared service binary.
- Reviewed independent forward dry-run record `onboarding-dry-run.json`; its
  skill-tree digest `b4eb0db8f80ba995902f1002b49f95849878cdfec8217b2d32abdf06e96cd2d7`
  matches current skill files. Ten fixture-based behavioral cases exercised
  no-Mac, pending QR, port collision, lazy tools, repeat/existing setup, unrelated
  research, untrusted chat-name instructions, clean installation with unspecified
  collection policy, successful QR continuation and connection refusal. This is model reasoning over
  fixtures, not automatic invocation by the actual Codex/ChatGPT onboarding host.
- Independently built a distinct stripped candidate binary and ran both native
  isolation and shared replacement tests with race detection: PASS9.455s.
  Stopped backups include all state; rollback restores the matching snapshots
  before restarting either account. Removal retains the common binary and B.
- Parsed the new tunnel LaunchAgent template and checked installed
  tunnel-client0.0.14 `run --help`; profile-file/health flags are supported.
  Operator instructions now distinguish each component’s exact start, stop,
  status and authorized crash test; no live tunnel/reboot test was performed.
- The CI originally targeted the new native service test in the wrong Go
  package. This was reported and fixed during review; the final workflow now
  has a dedicated `./internal/service` step with its exact test name.

## Requirement verdicts

Only complete criteria count. Missing evidence is BLOCKED, not PASS.

| ID | Verdict | Evidence or remaining gate |
| --- | --- | --- |
| A01 | BLOCKED | Actual Codex 0.160.0 package/bridge route verified. Current ChatGPT version and package-install route remain unverified. |
| A02 | PASS | Pinned Agent Plugins 1.0.0 schemas, MIT, version0.1.0 and metadata validated. |
| A03 | PASS | Deterministic 26-file public archive, internal Markdown references resolve, launcher mode preserved; ancestor symlinks rejected. |
| A04 | PASS | Reviewed allowlist and actual member contents contain no runtime state, personal paths, account IDs, tokens or tunnel credentials. Personalized variants are separate/private. |
| A05 | BLOCKED | All three skills and onboarding metadata discovered in real Codex. No actual host onboarding workflow turn demonstrated. |
| A06 | PASS | Independent actual Codex probe calls status through packaged launcher to selected synthetic service. Native bridge and account isolation tests verify retained data routing; launcher only invokes serve. |
| A07 | BLOCKED | ChatGPT package skill loading has no evidence; existing live MCP tools do not satisfy this requirement. |
| A08 | BLOCKED | Two Codex packages now independently installed, called, upgraded, rolled back and removed; ChatGPT two named tunnel-backed connections remain user/host verification pending. |
| A09 | PASS | Real independent temporary service processes, separate state/tokens/listeners, 401 on foreign token, duplicate port/state rejection; independent plist labels/config/log paths verified synthetically. |
| A10 | PASS | Native race test uses identical conversation/message/action/subscription IDs; context, stored operation status, journal content, receipt rejection and checkpoints remain per-instance across restart. |
| A11 | PASS | Independent repeat-existing and pending-QR forward dry runs preserve existing layout, service, binding and pending login. Instructions prohibit duplicate tunnels/subscriptions; actual host invocation remains separately gated by A05. |
| A12 | PASS | Ten independent fixture-based forward dry runs cover clean installation, pending/approved QR continuation, absent tools/Mac access, occupied port and connection refusal. They do not claim real login or actual-host automatic invocation. |
| A13 | PASS | Independent forward dry run covers connect/setup wording, repeat/reinstall/resume wording, unrelated research, and malicious chat-name instructions. No observed authorization or secret-handling error; this does not prove automatic host selection. |
| A14 | PASS | Separate exact-target service/tunnel status, bootout/bootstrap and authorized crash-recovery instructions now specify independent readiness checks. Tunnel plist parses; installed tunnel-client0.0.14 run help confirms --profile-file and health flag support. No production tunnel/reboot/login success claimed. |
| A15 | PASS | Independently reran both native race tests: distinct shared-binary replacement while both stopped, full stopped-state backups/restore, rollback and removal of one synthetic instance preserve the neighbor and shared executable. Codex package upgrade/rollback/removal separately passes. This proves same-source binary lifecycle; arbitrary schema-version rollback remains unsupported without compatible snapshot. |
| A16 | PASS | Links/reference generation pass; duplicate-skill migration and host-vs-server rights documented. Stale task status was corrected during review and independently re-read. |
| A17 | BLOCKED | Relevant local package/native checks pass; corrected GitHub CI has not run for this uncommitted source revision. |
| A18 | BLOCKED | Full two-review 18/18 acceptance and publication of accepted package are not achieved. This review must not be presented as final acceptance. |

**Complete: 12/18 = 66.67%. Zero unresolved observed code/documentation defects;
six criteria have evidence/host/publication gates.** No production behavior defect
was found in the limited tested scope.

## Resolved finding

- P3 — `docs/task-distributable-plugin.md:3`: status says “реализация не начата”
  although manifests, builder, onboarding skill, native isolation test and host
  probe have been implemented. Change status to implementation/acceptance in
  progress. Fixed during review; final status was re-read as implementation and
  validation in progress with host-specific gates open.

## Acceptance boundary

This report does not certify ChatGPT package loading, automatic onboarding
execution, multi-account UI, real Zalo login, tunnel restart, reboot/login,
arbitrary cross-version schema migrations or GitHub publication.
Codex package upgrade/rollback/removal and same-source native shared-binary
replacement/rollback/service-instance removal are independently verified. The user
explicitly reserved current ChatGPT plugin replacement for their own later
GitHub installation; no current ChatGPT connection was changed for this review. Closing those gates requires the
specific evidence above, not removal or weakening of the original criteria.
