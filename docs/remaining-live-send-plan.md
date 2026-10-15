# Approved local-absence direct-send acceptance

Status: execution and cleanup accepted on October 15, 2026; see
[the acceptance evidence](local-callback-acceptance.md#approved-local-absence-send-accepted).
The user explicitly authorized this concrete plan under
[the task's live-send requirement](task-conversation-completion.md#1-завершить-оставшиеся-проверки).
No new account is required. The fresh-incoming test has already completed.

## Scope and distinction

Use only the user's existing controlled second account. Its exact verified peer
ID is held privately from the accepted first/repeat event identities. A read-only
preflight found two corpus messages, two quote-metadata rows and no prior send
operation for this peer. Revalidate these counts and identities before execution;
do not broaden deletion if additional messages or changed metadata appear.

The test deliberately constructs local catalogue/corpus absence. It does not
prove first contact in Zalo, friendship state, or sending to an entirely new
upstream account. It therefore requires an explicit exception to the live plan's
default no-reset rule. Acceptance must retain this distinction; it cannot be
silently relabelled as genuine upstream first contact.

Approved exact text for one plain message, without quote:

> zl-mcp: проверка отправки без локальной истории. Отвечать не нужно.

A fresh stable request UUID and exact peer ID were saved in the private plan
before approval. User approval applies only to that recipient/text and
same-UUID retry, not a replacement request, quote or additional message.

## Approved execution

1. Stop the same sole Go service, checkpoint SQLite and take a hash-verified
   private state/configuration/binary/agent backup. Preserve the production
   subscription definition/boundary and client processing checkpoint.
2. Temporarily enable sending only to the exact controlled peer, then restart
   that same service and wait for actual MCP readiness. Do not create a second
   listener, log in again, request phone sync or import history.
3. Only after startup preload has finished, save a private row-level restoration
   plan. In one transaction remove only the two approved corpus rows, their two
   quote-metadata rows and that direct conversation's catalogue row. Do not remove
   permanent identities, first-incoming evidence, sends, Events, subscription
   rows or checkpoints. Abort if the agreed two-message shape has changed.
4. Confirm exact typed peer absence in corpus/catalogue and the real MCP
   catalogue before sending. Unexpected replay/preload rediscovery invalidates
   the attempt: stop and restore rather than manufacture a passing result.
   No friend/contact insertion is used to authorize sending.
5. Call the installed MCP `zalo_send_direct_message` with the approved exact
   peer/text/UUID and no quote. Record accepted upstream ID or exact safe upstream
   limitation. An `unknown` result permits only status lookup with the original
   UUID; never send a replacement. A successful repeat of the same UUID must
   return the same receipt with no second send. Verify the exact outgoing record
   and corresponding permanent identity without text-only reconciliation.
6. Restore the two original messages and quote metadata by exact identity and
   original sequence if still free. Restore original catalogue metadata only when
   absent; preserve newer live observations and the outgoing message. Conflicts
   require comparison, never overwrite or full-database rollback. Preserve both
   the accepted send receipt and original novelty fact.
7. Restore the original sending policy/configuration through the single service,
   then verify readiness, integrity, production subscription/checkpoint
   preservation and exact original/new message identities. Saved-result recovery
   across this restart must return the same request/message/recipient receipt.

No user-response deadline or expiring test window is used. Stage network/runtime
budgets still apply. A failed precondition is inconclusive, not permission to
repeat the reset or spend another UUID. Recovery/cleanup runs after success or
failure and preserves ambiguous upstream operations.

## Evidence boundary

Publish only anonymized counts, outcomes and source version. Private row copies,
IDs, UUID, configuration, payloads and backups remain outside Git. This plan does
not enable mobile import, waive the producer/WAL admission gate or reduce other
requirements. The accepted result covers the approved artificial local absence,
not genuine first upstream contact or an account never previously observed.
