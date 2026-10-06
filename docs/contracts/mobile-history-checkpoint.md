# Atomic selected-archive history checkpoint

Status: internal journal port; public start still accepts only existing sources.

A trusted mobile driver prepares a history operation with source `mobile_archive`.
It shares the existing request UUID/fingerprint, account, typed selection, limits,
cancellation, permanent identities and silent history policy. Public request
normalization continues to reject this source until driver/producer acceptance;
this internal entry point is not a phone request or another listener. Legacy
workers exclude mobile operations rather than accidentally call group history.

Persist one private source binding: request/snapshot UUID, image SHA-256, creation
and fixed 15-minute expiry, immutable source row/period/invalid-timestamp counts.
No path, transfer URL/key, account ID or body is part of a checkpoint. The driver
obtains this evidence from its authenticated selected snapshot. Known unverified
WAL/control images remain ineligible. A source cannot change image, expiry or
coverage after its first committed page. Missing/expired sources cannot be
replaced by another phone transfer under the same UUID.

Each page owns expiring normalized records and exact examined/rejected/expired/
unsupported-type/content/missing-metadata/invalid-metadata/deferred-control counts.
Examined rows equal retained records plus these mutually exclusive gaps. Unknown
metadata fields, expired quotes and unresolved quotes/mentions are orthogonal
counts and never consume additional source-row budget. Validate nonnegative
bounded counters, complete records and original expiry before writing anything.
Every submitted record must lie in the requested interval.

Use the remaining source-row budget and page size even when zero records remain
eligible. A nonterminal page needs positive examined rows and a strictly advancing
SQLite (timestamp milliseconds, signed rowid) keyset in the requested interval.
A page reaching the operation page/record limit may omit further continuation:
it stops terminal partial while preserving source_has_more=true. Source exhaustion
must agree with the frozen period-row count. Retain integer precision; neither
component passes through float64. The opaque
source checkpoint is private, not the generic cloud decimal cursor. A terminal
empty page is allowed; continuing empty source evidence is rejected.

The transaction validates operation account/revision/running state and policy,
then commits silent records, original TTL markers, immutable source binding,
raw-row counters and next keyset together. A failed checkpoint write rolls back
messages, expiry, permanent identities, novelty and all progress. A stale or
cancelled worker cannot write a page. Policy revocation cancels without records.
Restart retains the exact source/checkpoint and counts; it never dispatches a
phone request. Replaying a committed page with its old revision fails.

Status exposes `records_observed` as examined source rows and safe cumulative
`mobile_coverage` counts. Source UUID/digest/position/expiry remain private.
Available-source exhaustion stops completed only when no known source gaps exist;
otherwise partial/source_gaps. Every result retains history_complete=false.
Limits stop partial with existing page/message/time reasons. Exhaustion never
claims complete Zalo history. Historical writes produce no Events or deliveries,
do not change subscriptions/sends and preserve observed tombstones/novelty.

Mobile active work has a separate bounded 420-second allowance, including durable
in-flight reservations up to 180 seconds for phone acquisition. Legacy sources
retain their existing 120-second/30-second bounds. Authentication waiting does
not consume active work. Source loss is partial/source_unavailable, never implicit
redispatch. Snapshot storage, producer eligibility, page reader/converter and
single-session driver still require integration; storage tests do not prove that
integration or live import.
