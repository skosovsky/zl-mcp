# Independent review A — distributable plugin candidate

Reviewed 2026-10-18 against all A01–A18 in
[the task](../task-distributable-plugin.md). Scope: package contracts,
requirements, documentation, independent Codex/native checks and publication.
No production service, ChatGPT connection, session or subscription was changed.

**Result: 13/18 complete — 72.22%; zero unresolved observed code/documentation
defects. Five host/final-acceptance gates remain. This is candidate review,
not full 18/18 acceptance.**

## Reviewed identity and authoritative evidence

- Published source: `20127019abe45dcfe26f7beeb98ce29d90e2229b` on main.
  Reviewed final diff: the server key now includes instance ID for both portable
  and explicit-env instance variants; the public unbound archive is unchanged.
- Public archive SHA-256:
  `f855ab3d6b77aadd8b1d7cd5cd286c399f19b09f9de80f86e16ce6b464b4ff54`.
  Independently downloaded without credentials from
  [prerelease](https://github.com/skosovsky/zl-mcp/releases/tag/plugin-v0.1.0-rc.1):
  exact hash, 26 members, plugin version0.1.0. Prerelease publication is not final
  acceptance or public ChatGPT-directory publication.
- Exact-source [CI39124811100](https://github.com/skosovsky/zl-mcp/actions/runs/39124811100)
  independently queried: head SHA equals the reviewed source; Ubuntu and macOS
  jobs completed successfully, including both modules' test/race/vet, lint,
  package validation/negative controls, cross-builds and native acceptance.
- Independent local checks: schema/build validation; deterministic two-archive
  comparison; negative controls; skill-creator validation; generated reference
  check (27tools,5resources,4templates,6events,54reactions,20CLI,120links).
  Public archive contents and allowlist inspected for private paths/state/IDs.
- Independently ran real Codex0.160.0 isolated installation: three skills and
  onboarding discovered, packaged bridge exposes27tools, status returns expected
  auth_required for fresh synthetic state. See [host evidence](codex-host.json).
- Independently reran [Codex lifecycle probe](codex-lifecycle.json): two scoped
  packages/native services, package upgrade, exact-byte rollback and removal of
  A preserve B's config/token/cache/PID and keep both native services running.
- Independently built distinct same-source native binaries and ran both
  isolation/shared-replacement tests with race detection: PASS9.455s.
  Identical public IDs remain isolated; wrong tokens/receipts rejected;
  full stopped-state backup/restore and common-binary rollback preserve state.
- [Independent onboarding forward dry run](onboarding-dry-run.json): ten fixture
  cases; skill-tree digest matches current files. Includes clean setup,
  pending/approved QR continuation, no tools/Mac, occupied port, connection
  refusal, repeat setup, unrelated research and hostile chat-name instructions.
  This is behavioral reasoning over fixtures, not actual host onboarding launch.
- Service/tunnel plist templates parsed independently. Installed tunnel-client
  0.0.14 `run --help` confirms profile-file and health flag support. No live
  production tunnel crash, reboot or login check was conducted or claimed.

## Current acceptance matrix

Only fully demonstrated criteria count. BLOCKED means missing required proof,
not necessarily a product defect.

| ID | Verdict | Evidence / exact remaining gate |
| --- | --- | --- |
| A01 | BLOCKED | Actual Codex0.160.0 installation/MCP route proven; current ChatGPT package installation/version route awaits user's installation. |
| A02 | PASS | Pinned Agent Plugins1.0.0 schemas, version0.1.0, MIT and metadata validated. |
| A03 | PASS | Deterministic26-member archive; packaged references resolve; executable mode preserved; traversal/symlink controls. |
| A04 | PASS | Allowlist and member inspection exclude runtime state, personal paths, account IDs, tokens and tunnel credentials; bound variants kept private. |
| A05 | BLOCKED | Three skills and onboarding metadata discovered in actual Codex; automatic host invocation of setup workflow has not been demonstrated. |
| A06 | PASS | Real isolated Codex invokes packaged bridge to selected native service; bridge invokes serve only and does not create collector. |
| A07 | BLOCKED | Existing live MCP connectivity is separate from package skill loading. Current ChatGPT plugin intentionally retained pending user's GitHub reinstall. |
| A08 | BLOCKED | Two Codex instance packages independently installed/called/updated; two named ChatGPT tunnel-backed connections remain unverified. Single-card Connected accounts not promised. |
| A09 | PASS | Two real temporary native services have independent state/tokens/ports; foreign token401, duplicate state/port rejection; distinct synthetic plist labels/config/log paths. |
| A10 | PASS | Same conversation/message/action/subscription IDs remain isolated in context, operation status, journals, receipts and checkpoints across restart. |
| A11 | PASS | Independent repeated-existing and QR-resume fixture workflows retain layout/binding and pending login; no duplicate service, config, tunnel or subscription proposed. |
| A12 | PASS | Ten independent dry runs cover clean install, QR continuation, absent tools/Mac, port conflict and connection failure; no real login claimed. |
| A13 | PASS | Direct/connect and paraphrased/reinstall/resume requests, unrelated research and hostile chat-name fixture handled within authority; no observed secret or task-routing error. |
| A14 | PASS | Separate exact-target service/tunnel status, stop/start and authorized crash-recovery instructions; template/flags checked; live reboot/login/tunnel outcomes explicitly unclaimed. |
| A15 | PASS | Native distinct-build replacement, stopped snapshots, rollback and one-instance removal preserve neighbor/shared binary; package lifecycle separately verified. Arbitrary cross-version schema compatibility not claimed. |
| A16 | PASS | Reference/link checks pass; current docs and skills describe instance binding, duplicate-skill migration, rights boundaries and user's reserved ChatGPT reinstall. |
| A17 | PASS | Relevant local checks plus successful Ubuntu/macOS CI on exact published source20127019; native tests explicitly selected in their actual Go package with both binaries configured. |
| A18 | BLOCKED | Candidate and instructions published; two full18/18 reports and remaining user/host proofs are absent. Publication alone does not close final acceptance. |

## Resolved findings and limits

- Wrong CI package would have silently skipped native isolation. Fixed before
  reviewed publication: explicit internal/service native test selection.
- Stale task status corrected to implementation/verification in progress.
- Instance server-key collision corrected for both binding variants in reviewed
  source20127019 and covered by negative controls/lifecycle evidence.

No remaining observed defect requires a code change in the tested scope.
The user will replace/install the ChatGPT plugin from GitHub themselves;
this review does not alter that connection to manufacture host evidence.
Completion requires remaining host gates, both reviewers' full18/18 acceptance
on matching artifact/source identity, and final acceptance publication.
