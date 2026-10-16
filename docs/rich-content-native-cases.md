# Rich-content native case plan

This is the original case plan. The user completed the controlled direct media,
group multi-photo/reaction and fresh two-call actions on 2026-10-16. Individual
results and their limits are recorded in the current
[acceptance matrix](rich-content-acceptance.md). Use the existing test account;
no new account, phone synchronization, automatic media download or model eval is
needed. User actions have no expiry window.

## Preparation gate

Before requesting actions, install the verified bounded media diagnostics, check
actual MCP readiness, and verify the existing exact-peer message/reaction
subscriptions and independent signed receiver. Preserve Ann's production
subscription. Save private sequence and connection-generation baselines. An
empty journal or connected status alone does not prove source receipt.

Resolve the time/account of the two already reported calls before requesting any
additional call. Keep new call observations unacknowledged for the planned
restart check, then compare exact callback bytes and journal receipts before
ordered acknowledgement and its idempotent repeat.

## Minimal media batch

| Action | Purpose and acceptance |
| --- | --- |
| Direct photo with the exact synthetic caption `rich-caption-A` | Establish the incoming caption field and distinguish author text from labels. A matching title alone does not authorize a mapping for other attachment types |
| React to that photo | Resolve a media target to that exact visible message; no extra message or novelty fact |
| Send two images together in an existing approved test group | Establish whether the source uses independent records or one multi-element record. Preserve every available record/element; do not invent an album by merging unrelated messages |
| React to one group image | Verify exact typed group/media target resolution and isolation from the direct namespace |
| Short synthetic voice message | Verify the native audio shape, available metadata and honest lack of media bytes/transcription |
| Small synthetic text file, e.g. `rich-file-A.txt` | Verify file kind and only metadata whose incoming provenance is established |
| Public ordinary URL with a preview | Distinguish author text, link preview title and unsupported recommendation shapes; never expose signed media URLs |
| Location of a public landmark | Verify the native location projection without using a private home address |

The group steps require an explicitly approved existing group and a participant
who can send there. If unavailable, first inspect retained permitted examples;
leave the native group gate open rather than silently creating or joining a
group. The direct steps can use the already prepared test-peer subscriptions.

## Evidence and follow-up

For each action correlate native bounded shape with the stored identity, actual
MCP context/resource, journal and independently verified signed callback. Retain
private evidence outside Git; publish only aggregates and synthetic fixtures.
Compare body bytes before acknowledging; HTTP 2xx does not prove model handling.

The first caption/album observation may reveal a missing normalization rule.
Update the contract and tests before implementation, and reuse retained evidence
instead of asking the user to repeat already established actions. Unverified
fields remain unavailable; unperformed examples remain pending, not declared
upstream limitations. Do not send Zalo messages or initiate calls from the
service as part of this batch.
