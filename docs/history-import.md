# Available history import

This is an explicit corpus update. It runs inside the existing authenticated
service session and never starts a second listener or login. Use the tools only
when they appear in the connected client's actual catalogue. Installing a new
binary does not prove that an external client refreshed discovery.

## Workflow

1. Resolve the exact typed conversation ID through the catalogue. Names are not
   identifiers; direct and group IDs are separate namespaces.
2. Choose an explicit RFC3339 `[since, until)` interval in the user's time zone.
   Clarify ambiguous calendar dates before converting to instants.
3. Call `zalo_import_conversation_history` with a stable UUID `request_id`.
   Example input using synthetic identifiers:

```json
{
  "conversation_type": "group",
  "conversation_id": "123456789",
  "request_id": "00000000-0000-4000-8000-000000000001",
  "since": "2026-09-01T00:00:00Z",
  "until": "2026-10-01T00:00:00Z",
  "page_size": 50,
  "max_pages": 20,
  "max_messages": 1000
}
```

4. Save the returned `operation_id`. Poll `zalo_get_history_import_status` with
   that identity. A lost start response is retried with the same UUID and inputs;
   changed inputs conflict. Queued/running/paused states are not success.
5. To cancel, call `zalo_cancel_history_import` with `operation_id`. Cancellation
   is terminal for that UUID; it prevents late source results from persisting.
6. Read imported messages through browse/search/context and the full-text URI.
   Import status contains counts and source evidence, not message bodies.

## Limits and interpretation

The default group_cloud source requests paginated group history; a direct peer
with that source returns unsupported. Explicit source=conversation_preload
selects one currently available direct/group snapshot. It always stops
partial/source_window_limited, never claims upstream exhaustion, and cannot
page deeper by increasing max_pages. Source absence returns unsupported.
Profile enrichment and new subscriptions do not recover old message text.
Missing source support never falls back to guessed upstream URLs.

The whole preload snapshot has its own fixed parsing budget (1,000 records
across categories); only the selected typed dialogue is passed to the import
journal, newest first and limited by page_size/max_messages. Operation counts
refer to those selected dialogue records. A new UUID reads a fresh snapshot;
retries retain the previous result. None of this establishes a deeper-history
source for direct/Strangers.

Defaults are 50 records per page, 20 pages and 1,000 source records. Maximums are
50, 100 and 5,000 respectively. The record budget includes duplicates and records
outside the interval. Active work has a durable 120-second budget; paused auth
waiting does not consume it. The account ledger retains at most 100 active and
100,000 total operations. Capacity errors preserve request identity evidence.

Pages commit with message identities and checkpoints atomically. Restart resumes
committed work; authentication loss pauses it until a restored session. Revoking
collection access prevents further persistence. Existing live messages, send
outcomes and subscription boundaries remain intact.

`completed` means the available source reported exhaustion, not complete Zalo
history. `partial`, `failed`, `cancelled` and `unsupported` require interpreting
`stop_reason`. Nullable filtering/join flags distinguish unknown from false.
Earliest/latest timestamps describe newly inserted records only; they do not
prove continuous coverage. `history_complete` stays false. Collector gaps and
retention still apply, including when the operation inserts no new records.

## Silent ingestion

`notification_policy` is always `none`; it cannot be overridden. Explicit history
creates no Events or delivery jobs. Its permanent typed identities suppress later
live/replay duplicate notifications even after text retention. Ordinary incoming
and reconnect replay preserve their existing subscription semantics.

## Deployment

SQLite migration 8 adds the operation journal and cursor ledger. Follow the
[backed-up update procedure](mac-deployment.md): stop the sole service, acquire
its account lock, verify a complete private snapshot, and then install/start the
new binary. Opening storage is not permission to recover another active worker.
Rollback restores the previous binary and its matching full snapshot, not an old
binary against an independently changed database. Do not publish backups.

[Executable operation contract](contracts/history-import-operation.md) ·
[Source page contract](contracts/group-history-page.md) ·
[Acceptance evidence](conversation-completion-acceptance.md)

Before each source request, the journal reserves up to 30 seconds of remaining
work. A normal result reconciles actual elapsed work. Crash recovery charges the
reservation conservatively; repeated crashes cannot reset the time bound.
Authentication waiting remains outside active work.

Group cloud continuation preserves the exact cursor together with the source's
recent/old phase. Missing continuation metadata stops with partial coverage;
never describe a successful recent page as proof that older pages were fetched.
Phase-aware traversal is implemented in the current source candidate but still
requires installation and live acceptance. Historical imports remain silent.
