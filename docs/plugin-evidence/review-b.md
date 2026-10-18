# Independent review B — distributable plugin

Date: 2026-10-18. Reviewer: independent subagent `plugin_review_b` (no implementation edits).
Review base: Git HEAD `a64c5a874a4677f5753f947a48209bc59b0a5a25` plus uncommitted plugin implementation. This is an **interim review**, not release acceptance.

Rebuilt public archive SHA-256: `f855ab3d6b77aadd8b1d7cd5cd286c399f19b09f9de80f86e16ce6b464b4ff54` (26 files). Two independent invocations in a fresh temporary directory produced identical bytes. Saved Codex evidence currently identifies a different archive (`345ac8c3ec0018004c7b4205d4bc6e89640b1aca7d9a7c2bc5a58c1c2c6b84a4`), so it is historical evidence only.

## Independently executed checks

- Package unittest suite: 5/5 PASS, including literal argument handling, extra connection-line rejection, invalid instance/path rejection, missing-reference rejection and private ChatGPT app variant isolation.
- `ZL_MCP_ACCEPTANCE_BINARY=/tmp/zl-plugin-isolation-native go test -race ./internal/service -run '^TestNativePluginInstanceIsolation$' -count=1 -v`: PASS (actual native processes and Go bridges, 4.945s package run).
- Reference generation check: PASS (27 tools, 5 resources, 4 templates, 6 events, 54 reactions, 20 CLI commands, 120 local links).
- Both schema hashes match pinned provenance; public artifact scan found no actual `/Users/...`, app ID, tunnel ID, Stripe-style webhook secret or API-key pattern. This is a finite pattern scan, not proof of arbitrary secret absence.
- Additional source-boundary negative control: copying the source into a temporary directory and replacing `skills/setting-up-zalo` itself with a symlink to a sibling external directory is **accepted** by `files(root=...)`. Existing tests only reject symlink descendants; they do not reject source roots or manifest/license/launcher symlinks.

## Requirement verdicts

Unproven criteria are not counted as PASS. `PARTIAL` below is FAIL for acceptance scoring.

| ID | Verdict | Evidence / remaining work |
| --- | --- | --- |
| A01 | PARTIAL | Codex CLI 0.160.0 historical install/discovery recorded; no actual ChatGPT package-install version/route evidence and Codex record hash differs from current artifact. |
| A02 | PASS | Both pinned schemas validate manifests; version 0.1.0, author, MIT and source metadata consistent. |
| A03 | PARTIAL | Current archive deterministic and packaged references resolve; source root-symlink negative control is accepted, contrary to documented rejection. |
| A04 | PASS | Actual public artifact is allowlisted, contains no runtime files or personal binding; finite private-pattern scan and manual package review clear. |
| A05 | PARTIAL | Three skills and onboarding path exist; saved host record explicitly says onboarding host turn NOT_RUN. Discovery does not prove workflow execution. |
| A06 | PARTIAL | Native bridge isolation passes and historical Codex MCP discovery/status passes; rerun Codex probe on final current artifact for exact evidence binding. |
| A07 | BLOCKED | Existing personal MCP connectivity is not ChatGPT loading packaged skills. No package-upload/run proof. |
| A08 | BLOCKED | Two native services verified; neither two-account Codex plugin install nor two independently named ChatGPT tunnel-backed connections fully demonstrated. |
| A09 | PASS | Native services use independent state/config/token/listener; foreign token returns 401. Instance plist uses parameterized label. No actual LaunchAgent deployment performed (intentionally). |
| A10 | PASS | Same conversation/message/action/subscription IDs yield isolated context/action result/journal; foreign receipt rejected and ack checkpoint remains per-instance. |
| A11 | PARTIAL | Instructions forbid duplication and preserve existing layout; repeated actual onboarding behavior not demonstrated. |
| A12 | PARTIAL | Native port/lock collisions tested; full setup/QR resume/no-Mac/no-tools/connect-error dry runs not evidenced. |
| A13 | PARTIAL | Trigger and distrust rules present; direct/paraphrased/unrelated request behavioral results not evidenced. |
| A14 | PARTIAL | Accurate separation of service/tunnel and reboot limitations documented; actionable separate tunnel start/stop/recovery verification still absent. |
| A15 | PARTIAL | Stop/restart neighbor isolation passes; actual shared-binary update, rollback, package update and one-instance removal not tested. |
| A16 | PASS | Documentation, state ownership matrix, duplicate-skill migration, host/server permission distinctions and packaged reference links reviewed. |
| A17 | PARTIAL | Relevant package/native/reference checks pass; final revision CI and complete required checks pending. |
| A18 | FAIL | No final artifact/revision with both 18/18 reviews and GitHub publication exists yet. |

**Fully confirmed: 5/18 = 27.78%. Known implementation defects: 1. Outstanding acceptance gates: 12 additional partially proven/blocked criteria. Overall: NOT ACCEPTED.**

## Required corrections

1. Reject symlinks for every package input, including skill root directories and directly read manifest/license/launcher files. Check source containment before reading, and add negative tests for root-directory and top-level-file symlinks. The generated ZIP currently contains flattened regular files, so the observed issue is unintended external source ingestion rather than an executable symlink in the ZIP.
2. Bind all host evidence to the final archive hash/revision; current saved Codex evidence does not describe this exact artifact.
3. Complete behavioral and lifecycle checks and actual ChatGPT gates; do not relabel them PASS based on schemas, Codex discovery, or an already working unrelated personal connection.

No production service, configuration, session, tunnel or subscription was changed by this review.

## Re-review B — corrected candidate, 11:35 UTC

Public archive remains `f855ab3d6b77aadd8b1d7cd5cd286c399f19b09f9de80f86e16ce6b464b4ff54`; builder SHA-256 is now `1a9c47eaecd16e3757ee34e60d813d433181ddc46b2f77819281b23bf921aa5f`. Package input-tree digest: `6b9a64cdcc0964a80bb8606434a4d079dc037198fda9c26798596b5790ed1e78`. The builder fix changes tooling rather than packaged runtime bytes.

The source-boundary defect is **resolved**: `source()` checks every ancestor before reading direct manifests, LICENSE, launcher and skill roots/descendants. Independently reran all **6 package tests: PASS**, including root/parent-symlink negatives. Saved Codex host proof now identifies this current archive. Reviewed seven independently executed onboarding dry-run cases; they are behavioral fixture evidence, explicitly not host onboarding execution.

Independently reran the real isolated Codex lifecycle probe at 11:35:22 UTC: two native services, two scoped plugins, 27 tools each, read-only status, upgrade A, rollback A, remove A while B remains callable with cache/config/token/PID unchanged: **PASS**. Probe SHA `cc800b7043fe1840827eee51289fdcb8c7c7239c8b8a891e0e607fddd3a61a8a`; native binary SHA `f583da494a6178ce104b8a37f101c54548a948adedbe0219dae5521225e5aef5`. Temporary private archive hashes naturally differ because each run uses fresh temporary config paths; public archive and input-tree digests match saved evidence.

| ID | Updated verdict | Remaining boundary / authoritative evidence |
| --- | --- | --- |
| A01 | PARTIAL | Codex 0.160.0 demonstrated; new ChatGPT package route/version not yet exercised. |
| A02 | PASS | Pinned manifests/schemas and metadata consistent. |
| A03 | PASS | Deterministic current archive, self-contained links, source-symlink negatives fixed and passing. |
| A04 | PASS | Public archive allowlist/private-data scan remains clear. |
| A05 | PARTIAL | Actual host onboarding invocation still NOT_RUN; discovery is proven. |
| A06 | PASS | Real Codex host calls packaged bridge for both explicitly bound native services; no extra collector. |
| A07 | PENDING EXTERNAL HOST | User will install from GitHub; existing ChatGPT MCP and Codex evidence do not prove new ChatGPT skills loaded. Do not modify their current plugin in place. |
| A08 | PARTIAL | Two scoped Codex account-instance packages proven; two ChatGPT named tunnel-backed connections remain unverified. |
| A09 | PASS | Native isolation plus independent per-instance config/token/port and parameterized labels. |
| A10 | PASS | Native duplicate IDs/foreign credentials/checkpoint controls pass; two scoped Codex server keys avoid a discovered host collision. |
| A11 | PARTIAL | Repeat-existing behavioral case returns correct safe workflow; no actual repeated setup execution proving absence of duplicate resources. |
| A12 | PARTIAL | Seven cases cover QR wait, missing tools/Mac and collision; clean-install and connection-error dry-run cases not yet represented. |
| A13 | PASS | Direct setup, resume/reinstall paraphrases, unrelated research and injected untrusted name checked independently; no errors observed. This does not imply a full model benchmark. |
| A14 | PARTIAL | Service/tunnel separation and honest reboot limitation documented; complete distinct tunnel lifecycle verification instructions still not demonstrated. |
| A15 | PARTIAL | Codex package update/rollback/removal proven; shared-Go-binary operational update/rollback and one-service-instance removal remain distinct unverified items. |
| A16 | PASS | Documentation/ref links, duplicates migration and host-versus-service permissions remain consistent. |
| A17 | PARTIAL | Relevant local checks pass; final revision CI pending. |
| A18 | PENDING | Candidate publication and both final 18/18 reviews cannot be claimed before external ChatGPT acceptance. |

**Updated fully confirmed completeness: 8/18 = 44.44%. Known unresolved implementation defects: 0. Acceptance remains incomplete; no claim of final 18/18.** Candidate publication may communicate these explicit pending gates, but must not label the original goal complete. Production state and the current ChatGPT plugin remain unchanged by this reviewer.

## Prepublication candidate review B — final local additions

Reviewed the expanded ten-case independent onboarding dry run, direct tunnel-agent template and separate lifecycle instructions, and the shared-binary replacement acceptance test. Public archive SHA remains `f855ab3d6b77aadd8b1d7cd5cd286c399f19b09f9de80f86e16ce6b464b4ff54`.

Independent rerun:

```text
ZL_MCP_ACCEPTANCE_BINARY=/tmp/zl-plugin-isolation-native \
ZL_MCP_UPGRADE_BINARY=/tmp/zl-plugin-upgrade-native \
go test -race ./internal/service \
  -run '^TestNativePlugin(InstanceIsolation|SharedBinaryReplacement)$' -count=1 -v
TestNativePluginInstanceIsolation PASS (3.46s)
TestNativePluginSharedBinaryReplacement PASS (3.98s)
package PASS (8.958s)
plutil -lint deploy/macos/com.skosovsky.zl-mcp.INSTANCE.tunnel.plist.example: OK
```

Evidence bindings:

- Baseline binary: `f583da494a6178ce104b8a37f101c54548a948adedbe0219dae5521225e5aef5`.
- Replacement binary: `d002cfff53a7efba428864d5f1155046b510499d8a79baa82ff43f635297b488`.
- Native test source: `fce42f002f1f7d9562751c26aba7d9705b300a38136dc5cb762112e1562a6c5c`.
- Ten-case dry-run evidence: `cca66e4bff7da258a70c6b2c872c7c371d8d8dfd5bf262b57512ac6bcccf3d36`.

Three additional criteria now pass literally within their defined scope:

- **A12 PASS:** ten forward dry runs now include clean install/policy gate, approved-QR continuation and connection refusal as well as missing tools/Mac and port collision. These are intentionally synthetic dry runs, not actual host installation proof.
- **A14 PASS:** instructions now provide distinct service/tunnel status checks, health/readiness, intentional stop/start and exact-target crash recovery; direct-Go/direct-tunnel templates validate. Reboot/login and production tunnel crash checks are explicitly not claimed. This criterion concerns correct verification instructions, not unperformed live outcomes.
- **A15 PASS:** actual native test stops both shared-binary users, snapshots each complete stopped instance, atomically installs a distinct executable, verifies both corpora/bridges, stops both again, restores baseline plus matching snapshots, verifies both, removes A and confirms B and the shared executable remain intact. This is a same-source compatible-binary replacement proof, not arbitrary schema downgrade compatibility. Codex package upgrade/rollback/removal was independently verified separately.

**Current complete criteria:** A02, A03, A04, A06, A09, A10, A12, A13, A14, A15, A16 — **11/18 = 61.11%**. **Known unresolved implementation defects: 0.** Remaining: A01/A05 real ChatGPT/onboarding host evidence, A07 external user installation, A08 ChatGPT two-connection route, A11 actual repeated onboarding resource outcome, A17 final revision CI, A18 final review/publication gate. Earlier score tables above are historical; this is the current candidate verdict.

**Candidate publication: acceptable with explicit pending acceptance status. Full objective: not yet accepted or complete.** Do not replace the user's current ChatGPT plugin in place; their planned GitHub reinstall remains external follow-up. No production data, configuration, running services, tunnels or subscriptions were changed by this review.

## Narrow re-review B — instance-qualified PLUGIN_DATA packages

Candidate base HEAD: `2ad55fd14158eb5ee8eb5da6c279751233d63b08`, plus the reviewed builder/test changes. Builder SHA-256: `806c2418f70faf8e9a40024b23cb179148b4bdbbd89782d198ed60e4c2cee8cc`; test source SHA-256: `a0cd2833b94b3f0bb3a560e755524fc2526bbf62046306947d64c61e32a77cc4`. Public archive remains `f855ab3d6b77aadd8b1d7cd5cd286c399f19b09f9de80f86e16ce6b464b4ff54`.

Reviewed the narrow fix: every explicit `--instance` qualifies the server key as `zalo-<instance>` before deciding between environment binding and host `PLUGIN_DATA` binding. Thus two portable instance variants cannot collide on the unqualified `zalo` key. The public unbound package retains its original key; private ChatGPT variants still intentionally remove STDIO servers.

Independently reran all **7 package tests: PASS** (0.273s), including `test_portable_data_bindings_have_disjoint_server_keys`. The negative control builds personal/work without environment bindings, asserts disjoint server keys and verifies that the PLUGIN_DATA path did not accidentally acquire an env binding.

No additional defects found in this diff. This update changes no host acceptance gates or prior score: **11/18 = 61.11%, 0 known unresolved implementation defects; candidate publication acceptable with pending external/host acceptance, full goal not complete.**
