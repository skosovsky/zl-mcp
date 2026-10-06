# MCP Event payload absent from an automation run

Status: unresolved integration defect, investigated 2026-10-06 local time.

## Verified boundaries

- The active conversation v2 subscription covers direct chats, direction `all`,
  with `first_incoming_only=false`. The former conversation v1 subscription was
  cancelled; it has no pending/sending deliveries.
- The latest retained v2 delivery contains a complete event envelope and nonempty
  `data.text`, `message_id`, typed conversation identity, schema version 2,
  direction and novelty fields. The worker recorded one attempt accepted by the
  callback. Private payload, recipient identity and callback credentials are not
  included in this report.
- The actual client update selected `zalo.conversation.message.created.v2` with
  those exact filters and enabled its existing automation.
- A temporally adjacent automation turn contains the saved task and the sentence
  saying that event data is supplied in an `automations.mcp_event` tool result.
  The available thread reader exposes no such result. The assistant independently
  reported that its run context contained neither event data nor message ID and
  used a chronological conversation read instead.
- The thread reader also omits ordinary tool result bodies, so absence from its
  output alone does not prove absence from the underlying run. A full run export
  or receiver-side trace is required. Temporal adjacency is not an event-ID match.

## Contract comparison and offline verification

The current [OpenAI MCP Events guide](https://developers.openai.com/plugins/build/mcp-events)
requires `eventId`, matching event name, occurrence timestamp, `data` matching
`payloadSchema`, and cursor; it specifies Standard Webhooks signatures and
`X-MCP-Subscription-Id`. The implementation uses this shape and sends the
persisted bytes unchanged. HTTP 2xx acknowledges receipt, not asynchronous agent
processing. No incompatible shape has been established in this investigation.

The existing tests `TestFilteredProfileCanonicalIdentitySignedPayloadAndCancel`
and `TestConversationEventsThroughHTTPAndSignedTLSReceiver` passed again. They
validate application delivery, signatures and schemas against a synthetic
receiver; they do not validate ChatGPT runtime event injection.

## Remaining diagnostic step

Obtain the complete triggering run items from the owning client or OpenAI
receiver/runtime trace. Correlate callback event ID and subscription ID with the
run; inspect whether `automations.mcp_event` is injected and its result structure,
including batching. Distinguish event receipt/routing, run item construction and
model-visible input. Keep event contents private. Do not claim successful
end-to-end payload delivery based on callback acceptance or a message subsequently
found by browsing. Do not replay acknowledged events, mutate subscriptions or
add an undocumented payload wrapper merely to provoke another run.

The loss point and root cause remain unproven; this is not a completed fix.

## Instrumentation follow-up

Opt-in `logging.event_trace_file` now records the exact outgoing event JSON and
bounded callback response in a separate private rotating file, together with
body digest, queue/attempt/event/subscription IDs and receiver request IDs.
Normal service logs retain metadata only. This evidence can establish the exact
request/response boundary for future deliveries; it cannot inspect ChatGPT's
asynchronous run input. Do not equate successful trace tests with live model
payload acceptance. Disable the private trace after diagnosis.

## First full-trace observation, 2026-10-06

A natural outgoing direct message produced all three trace records. Its request
contained nonempty text, message ID, typed conversation ID and the selected v2
profile. The queue body digest matched the traced request digest. The callback
returned HTTP 200 with an empty response body, no read error and no retained
request IDs. The delivery receipt was persisted after one attempt.

Five seconds later, the client history contains an automation invocation with
the saved task and the `automations.mcp_event` result reference, but no exposed
result item. The assistant then read the conversation and reported a matching
message; this does not establish an event-ID match. The sender-side evidence
therefore rules out an empty local envelope for this message. The exact loss
point still requires the run's original input items or receiver-side tracing.
Do not resend this acknowledged event as a diagnostic shortcut. Exact private
event/subscription/run identifiers remain outside published documentation.

## Owning assistant confirmation

The user authorized a diagnostic request to the owning assistant. The assistant
reported that its visible model context for this run contained the task and the
reference to `automations.mcp_event`, but neither that result nor an event
attachment. Event fields were unavailable there; message contents appeared only
after a separate Zalo read. This independently distinguishes the reported model
context omission from the thread reader's known result omission.

The owning assistant has no read-only raw automation input, callback trace or
run export tool. Its history reader and automation configuration listing cannot
inspect this internal boundary. Therefore the available evidence localizes the
failure between webhook receipt and model-visible automation input; it does not
establish which receiver/runtime component drops the payload. A per-run event-ID
correlation remains unavailable on the receiver side.

### Escalation checklist

Expected: the triggered run includes an `automations.mcp_event` result containing
the accepted event envelope (or the documented event data representation).
Observed: the run includes only the instruction referencing that result; the
owning assistant confirms no event data in its visible context.

Provide privately to the platform operator: event ID, subscription ID, connector
and automation identifiers when available, triggering run/thread ID, request and
response timestamps with timezone, request body digest/byte length, and HTTP
receipt. Request receiver-to-run mapping and original model input items. Share
message contents only if required and explicitly authorized. The separate private
trace on the Mac contains the original request; it is not a public bug attachment.
Do not add undocumented envelope fields, replay an accepted event, or declare
the integration fixed based on a later conversation read.

## Reopened protocol and implementation audit

The request to localize this without a support escalation adds two checks:

- The actual retained failing event was validated against `payloadSchema` from
  the running service's authenticated `events/list`, as well as the v2 envelope
  schema. Event name, schema version, text and message-ID presence all passed.
  Private temporary validation inputs were removed.
- The existing real HTTP subscription plus separate TLS receiver test previously
  exercised conversation v1 only. It now runs both v1 and v2, verifying HMAC,
  content type, webhook/event identity, verification/subscription routing identity,
  typed content, truncation/resource retrieval and cancellation. Both passed
  with the race detector. Each run uses a separate temporary database and ports;
  no real Zalo connection or production subscription is involved.

The active callback host is the OpenAI connector receiver. Production callback
transport does not use environment proxies and rejects redirects. Application
fields are correctly nested in `data`, as required by both the OpenAI guide and
the linked MCP Events design sketch. There is no documented requirement to wrap
the webhook in a tool result: that transformation belongs to the receiving client.

The owning assistant actually called its cloud history reader. It exposes user
and assistant text items, but also omits ordinary tool results. This reader is
therefore inconclusive about injected event results. The assistant's direct
context observation remains separate evidence, not a raw model-input capture.

The sender audit has found no demonstrated envelope/signature/routing defect.
This does not establish end-to-end compatibility with the actual receiver. A
controlled pair of synthetic events to the existing callback can test whether
nullable fields or richer values affect the model-visible outcome, without any
Zalo sends. Such a live probe must be explicitly authorized and must not replay
a real acknowledged event or silently modify the production subscription.

## Authorized live synthetic comparison, 2026-10-06

The user explicitly approved exactly two synthetic v2 callbacks. Both used the
existing production callback sender and active subscription credentials in
memory, with schema-valid synthetic conversation/message IDs and independent
random text markers. No Zalo message, corpus record or subscription was created
or changed. Markers were not disclosed to the receiving assistant in advance.

| Variant | Callback receipt (Asia/Saigon) | Receiver observation |
| --- | --- | --- |
| Null conversation/sender names and null first_incoming | 10:22:06, HTTP 200, empty body | Assistant reported a run without payload and could not identify sender/content from the event |
| Populated synthetic names and first_incoming=false | 10:23:18, HTTP 200, empty body | Assistant reported a later run at 10:24:18, again without eventId/data/text or marker |

Both variants had direction=incoming, untruncated text, null text_resource_uri and
null cursor. Only name/novelty nullability was varied; this does not rule out every
schema incompatibility. Callback acceptance is proven; run correspondence remains
temporal because receiver-side event IDs are unavailable. The first diagnostic
run's fallback Zalo reads found no matching real messages, as expected.

Read-only automations.list confirmed the automation remained enabled. It exposes
no batching/debounce/cooldown configuration, pending run queue or error details;
its last_run_time preceded the observed runs and cannot serve as a full run log.

Conclusion: the visible failure reproduces independently of collector ingestion,
Zalo storage and real message content. Populating the tested nullable fields does
not resolve it. No sender contract defect was demonstrated. The receiver/model
input boundary remains suspect, but the exact internal cause is not established.
The one-off sender test file was removed; private synthetic request/response
evidence remains outside the repository. No extra callbacks were sent.

## Recovery implementation

The production API now adds owned subscription discovery, exact retained callback reads, and signed generation-scoped acknowledgements. See [event recovery](contracts/event-recovery.md). Conversation v1 is removed; tests now exercise v2 and the legacy group profile. Earlier sections describe diagnostic observations before this change. This workaround does not establish or repair a receiver-side root cause.
