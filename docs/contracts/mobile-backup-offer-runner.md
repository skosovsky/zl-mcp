# Prepared mobile-offer execution candidate

Status: internal orchestration; no CLI/MCP route and no automatic startup dispatch.

RunPreparedOffer accepts a trusted operation ID already prepared in the durable
account/policy-bound journal, an existing session-guarded MobileBackupSource and
an operation context. It never creates another attempt or retries a request.
Attach the journal observer before invoking the source; only its durable
BeforeDispatch transition permits the source to send the phone request. Sources
must implement the correlated request/offer contract and use the existing
collector listener. Successful source return is accepted only when the persisted
state is offer_ready. Failure or premature success yields no offer or source error
text. Terminal/non-prepared attempts cannot execute, including retries of a
previous offer_ready attempt (private offer credentials are not persisted).

Failure to attach an observer leaves the operation unchanged, including an already
cancelled context before attachment. A retry cannot finalize another execution: only
the invocation that successfully committed BeforeDispatch owns a dispatched state.

After unsuccessful execution that owns a committed dispatch, use a separate bounded
three-second persistence context to close the active attempt. Before dispatch, leave
the journal unchanged: another observer may already be preparing the same operation.
A dispatched attempt becomes interrupted on cancellation,
unknown result or authentication loss, and failed on an explicit other failure.
Never replace terminal state or assume a dispatch failure means no request was
sent. Finalization reads the current durable revision and uses CAS; its failure
returns a fixed error and leaves evidence available for recovery. No automatic
network cancel is sent and no secret/URL/message content is logged or persisted.

A successful result remains a private offer, not a downloaded archive, selected
history import or coverage claim. The downloader enforces the attempt byte budget
and verified host policy in a separate stage. Recovery/service lifecycle wiring,
actual correlated offer acceptance and trusted local entry remain open gates.
