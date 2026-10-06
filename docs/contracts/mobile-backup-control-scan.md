# Whole-source own-direct recall scan

Status: internal whole-source scan and journal prelude candidate. Public mobile
import remains disabled; deployment and real phone source acceptance are separate.

A selected source is bound to the exact normalized request/fingerprint, typed
direct conversation and current session account. Scan type-33/36 rows across its
entire immutable image, independently of the requested message interval and message
page position. Reuse the existing header, integrity, schema, scalar, cursor and
SQLite execution limits. Project only sender/global/client IDs, timestamp, TTL,
type and status; do not load MsgContent or BinNet for control classification.

Read at most 50 rows per page, at most 5,000 controls overall, and at most 30 seconds
for the complete scan/mapping. The caller chooses a control ceiling within this
fixed bound. Detect an oversized whole-file count before sender mapping. Maintain
strict forward progress and bind cursors to image digest/name/full source time
range. All examined controls must match the frozen whole-file count. Invalid-time,
rejected, duplicate-target or unsupported rows fail without a returned prefix.
An empty control set is explicit proof only for that exact image.

Only the verified own-direct type-36/status-3 case is supported. Every retained
sender must map one-to-one to the current account through the existing scoped
mapper; partial, duplicate, foreign, group or alias mappings fail. Type 33,
received-message, group and other-status controls remain unsupported. WAL images
remain unsupported even if all visible controls are classified.

The private result owns typed tombstone targets and binds request, fingerprint,
account, image digest and filename. It exposes no bodies or author names and has
redacted formatting/explicit clearing. Source timestamps are original row times,
not recall times. Scanning performs no corpus writes, live recall, phone request,
new authentication or Events.

## Required journal integration

Before ordinary-message paging, a distinct durable control prelude must apply
all verified tombstones and publish complete immutable control evidence in one
transaction. Control work/targets are counted separately from message-period
examined rows; outside-period controls must not manufacture ordinary coverage.
Restart resumes ordinary paging from the committed prelude without another phone
transfer or an expiry extension. Cancellation/revocation/stale revisions and
rollback must apply to this phase. No ordinary page may precede complete control
proof. The reader primitive alone is not that transaction or public acceptance.

## Prelude representation and compatibility

A private `MobileHistoryPage` with `ControlPrelude=true` carries the complete
verified target set, no ordinary records/counts/cursor, and `HasMore=true` to
continue ordinary paging. Its snapshot marks `ControlPreludeComplete`. The
transaction freezes source binding and records `control_prelude_complete=true`
and `source_recalls` in public mobile coverage. Both fields are optional in the
output schema for legacy checkpoints; missing means false/zero.

`source_recalls` counts whole-source tombstones applied once. `own_recalls` counts
classified control rows examined in the requested message period. Prelude work
changes neither ordinary `records_observed` nor `pages_observed`, and does not
consume their limits. Its elapsed work consumes the existing durable work budget.
The source identity/digest and fixed expiry cannot change between phases.

A committed prelude with zero ordinary progress resumes the first ordinary page
without a message cursor. The worker loads a source checkpoint when mobile
coverage exists, even before the first ordinary page. Subsequent ordinary pages
carry no new tombstone targets: period-local recalls only affect row coverage.
Legacy accepted first-page recall checkpoints retain their original behavior;
they are not upgraded by guessing or rescanning a different snapshot.
