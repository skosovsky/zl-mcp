# Independent review B — distributable plugin candidate

Date: 2026-10-18. Reviewer: independent subagent `plugin_review_b`, not an implementation author. Source candidate: **`20127019abe45dcfe26f7beeb98ce29d90e2229b`**, published on `main`. This report consolidates the current verdict; earlier interim scores are superseded.

**Complete: 12/18 — 66.67%. Known unresolved implementation defects: 0. Full goal: not accepted.** The published candidate is suitable for the user's planned manual installation, but ChatGPT installation and actual host onboarding remain unproven. Do not update their existing ChatGPT plugin in place.

## Authoritative evidence

- Public archive: **`f855ab3d6b77aadd8b1d7cd5cd286c399f19b09f9de80f86e16ce6b464b4ff54`**, 53,889 bytes, 26 allowlisted files.
- [GitHub prerelease](https://github.com/skosovsky/zl-mcp/releases/tag/plugin-v0.1.0-rc.1): `plugin-v0.1.0-rc.1`, target commit exactly `20127019abe45dcfe26f7beeb98ce29d90e2229b`. Independently read GitHub release metadata and downloaded the [ZIP](https://github.com/skosovsky/zl-mcp/releases/download/plugin-v0.1.0-rc.1/zl-mcp-plugin-0.1.0.zip) without authentication: HTTP 200, size and SHA-256 match. This proves distribution, not client installation.
- [CI run 39124811100](https://github.com/skosovsky/zl-mcp/actions/runs/39124811100): independently queried GitHub; completed/success, head SHA exactly `20127019abe45dcfe26f7beeb98ce29d90e2229b`. Both `ubuntu-latest` and `macos-latest` jobs passed. Step results include package/schema/negative controls, reference checks, lint, architecture, existing offline checks, test/race/vet for root and nested module, three CGO-disabled target builds, native startup cleanup, instance isolation and shared-binary replacement. No failing or skipped mandatory step observed.
- [Codex host proof](codex-host.json): isolated Codex CLI 0.160.0 installs this exact public archive, discovers all three skills and onboarding metadata, then calls 27 tools/status through the actual packaged launcher. Explicitly no model/API onboarding turn.
- [Codex lifecycle proof](codex-lifecycle.json): two real sessionless native services plus two scoped packages, upgrade/rollback/removal of A with B's cache/config/token/PID unchanged. Independently reran the probe; public archive and input-tree hashes matched saved evidence. Removing the package did not stop either native service.
- [Onboarding dry runs](onboarding-dry-run.json): independent subagent executed ten fixture-based forward cases; no behavioral errors observed. Covers clean-install policy gate, QR wait/approved continuation, missing tools/Mac, port collision, connection refusal, repeat existing layout, unrelated research and injected untrusted name. Explicitly not actual host setup execution.
- Independently ran seven package unit tests: PASS, including missing references, root/parent symlinks, invalid bindings, literal shell arguments, extra connection lines, reproducibility and instance-qualified PLUGIN_DATA server keys.
- Independently ran both native service tests with race instrumentation: `TestNativePluginInstanceIsolation` PASS (3.46s), `TestNativePluginSharedBinaryReplacement` PASS (3.98s), package PASS (8.958s).
- Reference check passed with 27 tools, 5 resources, 4 templates, 6 events, 54 reactions, 20 CLI commands and 120 local links. Both schemas match pinned provenance; finite private-pattern scan and manual artifact review found no real personal paths, credentials or runtime state. Tunnel plist passes `plutil -lint`.

Additional evidence hashes:

| Source | SHA-256 |
| --- | --- |
| Builder | `806c2418f70faf8e9a40024b23cb179148b4bdbbd89782d198ed60e4c2cee8cc` |
| Package negative-control tests | `a0cd2833b94b3f0bb3a560e755524fc2526bbf62046306947d64c61e32a77cc4` |
| Public package input tree | `6b9a64cdcc0964a80bb8606434a4d079dc037198fda9c26798596b5790ed1e78` |
| Native baseline binary | `f583da494a6178ce104b8a37f101c54548a948adedbe0219dae5521225e5aef5` |
| Native replacement binary | `d002cfff53a7efba428864d5f1155046b510499d8a79baa82ff43f635297b488` |
| Native test source | `fce42f002f1f7d9562751c26aba7d9705b300a38136dc5cb762112e1562a6c5c` |
| Ten-case dry-run evidence | `cca66e4bff7da258a70c6b2c872c7c371d8d8dfd5bf262b57512ac6bcccf3d36` |

## Current A01–A18 matrix

PARTIAL/PENDING do not count toward completeness. Successful candidate publication cannot substitute for host acceptance.

| ID | Verdict | Evidence / remaining boundary |
| --- | --- | --- |
| A01 | PARTIAL | Actual Codex 0.160.0 package installation and MCP route proven. Actual new-package ChatGPT installation route/version not exercised. |
| A02 | PASS | Pinned schemas validate manifests; version, author, MIT and repository metadata agree. |
| A03 | PASS | Reproducible archive from canonical sources, internal references resolve, source root/parent symlinks rejected; published ZIP matches reviewed bytes. |
| A04 | PASS | Allowlisted public tree/artifact contain no runtime state, real account bindings, personal paths, credentials or tunnel keys in the checked scope. |
| A05 | PARTIAL | Three skills and onboarding path discovered in real Codex. An actual host onboarding invocation remains NOT_RUN. |
| A06 | PASS | Real Codex calls the packaged STDIO bridge for explicitly bound services; launcher only starts `serve`, not another collector. |
| A07 | PENDING EXTERNAL HOST | Existing ChatGPT MCP connectivity is separate from loading packaged skills. User's planned GitHub reinstall/report is not yet available. |
| A08 | PARTIAL | Two scoped Codex plugins/native instances work independently. Two named ChatGPT tunnel-backed account connections remain unverified; no claim of a shared Connected accounts card. |
| A09 | PASS | Independent native config/state/token/ports; foreign token rejected with 401. Instance templates parameterize distinct labels. No production LaunchAgent deployment implied. |
| A10 | PASS | Matching conversation/message/action/subscription IDs stay isolated; foreign receipts rejected; ack checkpoints independent. Scoped server keys avoid host catalogue collisions. |
| A11 | PARTIAL | Repeat-existing dry run selects safe reuse and old layout; actual repeated host setup has not demonstrated absence of duplicate service/config/tunnel/subscription resources. |
| A12 | PASS | Ten independent fixture dry runs cover all named clean/resume/no-tools/no-Mac/collision/connect-error scenarios. This is the requested dry-run scope, not real QR installation proof. |
| A13 | PASS | Direct setup, resume/reinstall paraphrases, unrelated research and adversarial untrusted-name instructions checked independently, no behavioral errors observed. |
| A14 | PASS | Separate service/tunnel status/start/stop/crash-recovery instructions and direct launch templates reviewed. Reboot/login and production tunnel crash outcomes explicitly not claimed. |
| A15 | PASS | Native test stops all users of shared binary, snapshots full stopped state, atomically replaces a distinct binary, verifies both services/bridges, restores baseline plus matching snapshots, removes A and preserves B/shared binary. Codex package lifecycle independently passes. Arbitrary cross-schema downgrade safety is not claimed. |
| A16 | PASS | Documentation/skills, packaged references, state ownership, duplicate-skill migration and host/server permission boundaries consistent. |
| A17 | PASS | Exact-candidate GitHub CI passes all required Linux/macOS checks; relevant positive/negative package and native tests also independently pass. |
| A18 | PARTIAL / NOT ACCEPTED | Accepted candidate is publicly distributed with instructions, but both final 18/18 reviews cannot be asserted before remaining host/onboarding requirements are proven. |

## Resolved findings

1. **External source symlink ingestion:** initial builder accepted an external symlink as a skill root. Fixed by checking every source ancestor before reads, including manifest/license/launcher and skill roots/descendants. Root/parent symlink negative tests independently pass.
2. **Host server-key collision:** instance packages originally shared `zalo`. Fixed to qualify keys for every explicit instance, including variants bound through PLUGIN_DATA without env. The seventh unit test independently proves disjoint keys; real two-package Codex lifecycle passes.
3. **Stale evidence binding:** initial host record described an older archive. Current host/lifecycle evidence and unauthenticated release download identify the reviewed `f855...` archive.

## Remaining acceptance work

Actual ChatGPT package installation and onboarding, the two-connection ChatGPT route, and repeated real host setup still need authoritative evidence (A01/A05/A07/A08/A11). Then both reviewers must reassess the final source/artifact before A18 and the full goal can be marked complete. The current 12/18 score is not rounded up or recast as 100% by excluding those requirements.

This reviewer changed only this review document and temporary test state. No production service, configuration, session, tunnel, subscription or current ChatGPT plugin was changed.
