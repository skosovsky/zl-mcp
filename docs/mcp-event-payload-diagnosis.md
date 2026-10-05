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
