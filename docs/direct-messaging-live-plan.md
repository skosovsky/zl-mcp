# Direct messaging live acceptance plan

Status: prepared; not executed or authorized for a particular recipient.
This plan uses placeholders. Account IDs, private message contents and credentials
must remain outside the repository.

## User agreement

Agree an exact direct recipient and two texts before sending. Suggested synthetic
texts are `zl-mcp: проверка личной отправки. Отвечать не нужно.` and
`zl-mcp: проверка ответа с цитатой. Отвечать не нужно.`. The quote must reference a
retained message in that recipient's dialogue; the service must have supported
upstream metadata. If unavailable, report the limitation without silently sending
an unquoted replacement. The account's own outgoing events are not an incoming
subscription test.

The test requires a short service maintenance window: stop the existing service,
verify a private state/config/binary backup and run the candidate foreground.
Only one collector may use the account. Use a private copy of state for the trial
and restrict temporary send permission to the agreed recipient. Keep the original
state intact while the old process is stopped. Do not retain test credentials or
message contents in this repository. On failure, stop the candidate before restoring
the original service. Preserve candidate evidence privately so an ambiguous send
cannot later be retried with a new identity.

## Sending scenarios

1. Persist a fresh UUID and exact agreed arguments privately before the first call.
   Send the unquoted text through MCP and read its status. `sent` requires an actual
   Zalo message ID. Repeat exactly once with the original UUID and arguments;
   confirm the same message ID and no second upstream send.
2. Resolve a supported retained quote from the same direct conversation and send
   the agreed quoted text with its own UUID. Verify the returned status and, where
   available, collector observation of the outgoing message and quoted relationship.
   Upstream acceptance, listener observation and recipient read status are different
   facts. `failed`/`unknown` must not trigger an automatic replacement send.
3. Restart the candidate, read both operation statuses and repeat the same UUIDs;
   no additional network send may occur. Verify the session still authenticates
   and the corpus/subscriptions remain intact.

## Events and client scenarios

Compare `server/discover` and `events/list` at the candidate endpoint and through
the actual client connection. The local profile catalogue alone does not prove
ChatGPT discovery. Do not diagnose client caching without observing both sides.

Subscription changes require an explicit scope and trusted client's callback
settings. Verify incoming direction and, with a genuinely new locally observed
peer if available, the first-incoming filter. Existing messages can verify read
coverage and quote metadata; they cannot prove a new event after activation.
An account with incomplete history must not be described as newly contacting it.
Callback acceptance and agent processing must be recorded separately. Do not
create a broad live subscription merely because collection mode is all.

## Deployment gate

After the agreed live scenarios pass, promote the verified candidate state so the
send ledger and newly collected records are preserved. Install the reviewed Go
binary outside iCloud, use the existing single LaunchAgent, verify permissions,
service readiness, discovery and restart. Retain the pre-migration backup for a
compatible rollback. Restore the agreed final sending policy; temporary trial
permission does not authorize arbitrary future replies or recipient expansion.

Commit/push source, contracts and public documentation only. Record anonymized
live outcomes and limitations in the acceptance report, never runtime-state,
recipient IDs, private quoted text, session data or signing secrets.
