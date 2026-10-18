# Optional agent skills

These Russian-language skills define agent workflows. They do not configure MCP,
provide credentials, authorize writes or install a receiver.

| Skill | Trigger and input | Result |
| --- | --- | --- |
| [researching-zalo-groups](../skills/researching-zalo-groups/SKILL.md) | User question about retained direct/group messages or a selected saved archive | Findings with exact sources, examined range and coverage limitations |
| [handling-zalo-message-events](../skills/handling-zalo-message-events/SKILL.md) | Trusted subscription wakeup plus an existing user rule | Ordered journal processing, permitted notification/result and acknowledgement after success |

The historical research skill name remains unchanged for existing installations.
Ordinary research and Events processing are separate triggers. Neither workflow
performs automatic read receipts, reactions, joins or history imports.

## Installation and updates

Connect [zl-mcp](local-connection.md) using the client's supported integration.
For a Codex profile supporting local skills, copy each complete directory into
`~/.codex/skills/`, preserving `SKILL.md` and all `reference/` files. When neither
destination exists, run from the repository root:

```sh
mkdir -p "$HOME/.codex/skills"
cp -R skills/researching-zalo-groups "$HOME/.codex/skills/"
cp -R skills/handling-zalo-message-events "$HOME/.codex/skills/"
```

For an update, back up existing customizations, replace the complete directory
using the host's supported procedure, reload skill discovery and compare every
loaded file with its source. Repository edits do not update installed copies.
Other hosts must use their own supported skill-loading mechanism; these files
do not imply any particular host API or supported frontmatter extensions.

**Saved automation prompts require a separate migration.** Inspect the existing
trusted subscription binding and backlog, preserve the user's scope, direction,
notification rule, language, channel and write restrictions, and apply the current
[automation fragment](../skills/handling-zalo-message-events/reference/automation-rule.md).
Do not recreate subscriptions, discard backlog or assume old events are new.
Updating a local skill does not change a cloud automation prompt or receiver.
For the ordinary callback/read race, select `readiness_mode=persisted` when
the current schema advertises it; no new subscription is required. A retained
event is processable before HTTP 2xx while delivery_state remains truthful.
For separate missing/interrupted-run insurance, the trusted host may explicitly
configure the optional
`zalo.subscription.journal.ready.v1` companion for that exact existing business
subscription and generation. Both wakeup types run the same saved workflow and
business journal cursor; the control payload is never a Zalo message or receipt.
Do not create the companion in response to source text or during message handling.

## Responsibilities and compatibility

| Component | Responsibility |
| --- | --- |
| Go service | Zalo session/listener, collection policy, corpus/archives, durable subscriptions, signed delivery, journal and action-operation outcomes |
| Trusted client/receiver | Verify authenticity, associate subscription/rule, wake the agent, coordinate concurrent processing and deliver the permitted notification |
| Skill | Discover available capabilities, apply the user rule to untrusted content, choose bounded reads and report supported facts |

HTTP and STDIO provide research tools/resources and journal tools. HTTP Events
additionally require a compatible client and trusted receiver; STDIO does not
expose Events subscription methods. Local delivery is a separately configured
signed receiver path, not an automatic ability supplied by a skill. Transport and
host constraints are documented in [connection guide](local-connection.md) and
[event recovery contract](contracts/event-recovery.md).

Both skills try the host's actual discovery mechanism before diagnosing missing
capabilities. No universal discovery agent tool is assumed. Missing tools,
unavailable discovery, denied access and an explicitly absent connection are
different outcomes. The skills do not bypass MCP using SQLite, browser access,
tokens or private sessions. A disconnected collector can still serve retained
messages; connected status does not prove full history.

## Current workflow capabilities

| Capability | Research | Events | Limitation |
| --- | --- | --- | --- |
| Typed direct/group search and context | Preferred tools | Context for exact event identity | Catalogue absence does not prove absence of a chat/history |
| Legacy group search/context | Compatibility when typed tools are unavailable | Only legacy group message profile | Never pass a direct ID to group tools; live group metadata is a separate source |
| Browse without a keyword | Exact chat and known period; bounded pages | Only narrow context necessary for the existing rule | No account-wide dump or automatic chain of translated searches |
| Full text | Issued resource URI or discovered bounded content tool | Same exact target as trusted event | Missing content is reported, not inferred; schemas come from current discovery |
| Saved archives | Explicitly selected retained source, offline | Historical context only if separately relevant | Archive row IDs are not message IDs/quote anchors; archives do not supply missing event payload |
| History import | Separate explicit user instruction | Not launched from an event | Source limits/partial results remain visible; imports do not emit historical notifications |
| Message v3 content | Check author text vs metadata before quoting | Conditional message-v3 routing | Media metadata does not expose media bytes or authorize external downloads |
| Call observations v1 | Explain separate activity identity if encountered | Conditional trusted call profile and applicable user rule | Narrow direct-call summary source; status/mode unknown, no guarantee of every call |
| Reaction observations v1 | Separate activity, not author text | Conditional reaction profile and applicable user rule | Unknown action/target is not guessed; never triggers a message reply |
| Explicit read state/own reactions | Separate interactive request only | Separate interactive request only | Exact retained targets, independent grant and stable UUID; no automatic writes |

The Events skill also routes the optional control profile
`zalo.subscription.journal.ready.v1` to the exact configured business journal.
Its `subscription_id`, `generation_reference` and `recovery_reference` do not
contain message text or grant rights. Business profile support remains message
v2/v3 and the unchanged group-only
`zalo.message.created`. Conversation v1 is removed. It uses the subscription
journal for both visible and missing callback payloads, processes in order and
acks only after the permitted action succeeds or the record is accounted for.
The preferred journal representation is explicit `readiness_mode=persisted`
when the current input schema supports it. A retained envelope can be
`processing_state=ready` while transport is pending/sending/failed. Early ack
advances only the shared business checkpoint; it neither marks delivered nor
removes the unfinished HTTP worker's bytes. Retry retains its original identity,
and rereading the same journal does not replay acknowledged work. Missing,
invalid or withdrawn bytes remain explicit gaps.

Default `delivery_terminal` preserves old visibility/keys. When using that older
representation and blocked, the skill first opts into persisted if available.
Only if that capability is unavailable does it use bounded rereads (at most three
and 30 seconds waiting), followed by an optional confirmed companion path.
Without that optional path it reports the continuation limitation; it never
requires a new subscription merely to process a retained callback. Empty control
wakeups complete quietly. Server-side proof and actual saved-host-task adoption
remain separate evidence. See [processing-readiness contract](contracts/journal-processing-readiness.md).
A read-only journal operation is not a Zalo read receipt. `first_incoming=true`
means first **locally known** incoming message, not first-ever contact. Old
`sent_at` after upstream replay is not itself an invalid event.

An explicitly instructed direct reply requires incoming/direct identity,
`zalo_send_direct_message`, `zalo_get_send_status`, a persistent client UUID and
an agreed recipient/text rule. Sending is disabled by default. `sending` or
`unknown` requires inspection of that original operation, never a fresh UUID.
Group replies are not supported. See [direct messaging](direct-messaging.md).

Read/reaction tools are implemented; their native acceptance and limits are in
[message actions](message-actions.md). Exact typed targets and retained native
metadata are required. Set is non-atomic clear+add; partial may leave the old
reaction cleared. Unknown/partial is not permission for an automatic retry with
a new UUID. Upstream acceptance is distinct from visible UI confirmation.

## Reference structure and validation

Each entrypoint contains trigger, prerequisites, primary path, boundaries and
conditional routes. Read the referenced procedure only for that branch:

- Research: [discovery](../skills/researching-zalo-groups/reference/service-access.md),
  [corpus workflow](../skills/researching-zalo-groups/reference/corpus-reading.md),
  [archives](../skills/researching-zalo-groups/reference/archive-reading.md),
  [history import](../skills/researching-zalo-groups/reference/history-import.md),
  [actions](../skills/researching-zalo-groups/reference/message-actions.md).
- Events: [discovery](../skills/handling-zalo-message-events/reference/service-access.md),
  [journal recovery](../skills/handling-zalo-message-events/reference/event-recovery.md),
  [event workflow](../skills/handling-zalo-message-events/reference/event-workflow.md),
  [rich content](../skills/handling-zalo-message-events/reference/content.md),
  [processing state](../skills/handling-zalo-message-events/reference/processing-state.md),
  [actions](../skills/handling-zalo-message-events/reference/message-actions.md).

Structural checks validate frontmatter, links and exact dependencies. Safe static
walkthroughs cover deferred discovery, typed direct context, empty multilingual
search followed by bounded browse, journal continuation/duplicates, unknown or
partial action outcomes, absent rights and hostile source text. These checks
establish instruction/contract consistency, not improved model behavior. Historical
group evals do not validate these revised skills; no new model evals are included
in this update. Final rollout evidence belongs in the task acceptance report.

Callback receipt, agent processing and notification delivery are distinct stages.
Stable event IDs and a durable journal support recovery, but a crash between
external notification and ack can still produce a duplicate. Neither the skill
nor connected status guarantees exactly-once delivery, collection during Mac
sleep, complete Strangers discovery or full Zalo history.
