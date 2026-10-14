# Publication copies of historical migration evidence

Historical evidence under `docs/mcp-migration/evidence` and `docs/lint-evidence`
contains captured results, not a fresh validation of the latest installation.
Public copies were sanitized on October 15, 2026. Original Git objects and an
exact original/published artifact checksum inventory are retained in a verified
private archive outside the checkout and cloud-synced storage.

The transformation:

- Redacts synthetic `plan_token` values in offline join traces, including JSON
  embedded in trace strings. These tokens belong to isolated fixture state;
  public traces are evidence, not reusable authorization or executable requests.
- Replaces the original checkout/home/temp paths and hostnames with generic
  locations such as `/workspace/zl-mcp`, `/home/LOCAL_USER` and `/tmp/local-temp`.
  Historical command vectors show roles and arguments; their local paths are
  not installation instructions or promises that old temporary binaries exist.
- Recalculates SHA-256 references transitively when they refer to a transformed
  documentation artifact. Recorded executable/fixture/source digests and measured
  counts, timings and allocations are preserved unless the referenced artifact
  itself was transformed. A changed artifact digest is not a new measurement.
- Removes trailing whitespace from path-transformed logs. Source code,
  executable contracts, fixture inputs, real account state and installed binaries
  are not modified by this documentation transformation.

The current-tree transformation covers 571 files and 217 synthetic approval-token
occurrences. The private original/published inventory verifies every transformed
file. The transformed docs passed gitleaks 8.30.1 with no findings; performance
harness tests passed against their updated build-manifest/checksum references.
Those focused checks do not replace full CI or real Zalo acceptance.

Removing these values from the final tree alone does not remove them from Git
history. The unpublished development commits after `8d913c4` have received the
same transformation and been signed again, preserving order, authorship and
messages. Final review also corrected paths used as overlay JSON keys and
hostnames embedded in uname strings; the transformation covers keys and values.
The [history map](publication-history-map.json) records original/sanitized IDs;
original IDs are retained privately and may not resolve on GitHub. The reviewed
new history passes gitleaks with no findings, and each original/sanitized pair
differs only in documentation. The sanitized head `f93869a` matches the
independently checked current-tree publication copy, before adding the history
map and this completion note.

The reviewed chain contains 28 signed commits. Secret scanning found no matches;
manual scanning of 10,385 unique historical text blobs found no remaining private
home paths, machine names or known personal-chat markers. Performance manifest
tests also passed after the final metadata corrections.

Full `make check` passed locally: pinned lint, root/harness/zcago test/race/vet,
architecture, platform builds and offline contracts. No model evals or live Zalo
effects were performed by those checks. GitHub CI still must pass on the published
head. Do not suppress broad secret-scanner rules to publish raw fixture
capabilities, or relabel sanitized historical output as a new test run.
