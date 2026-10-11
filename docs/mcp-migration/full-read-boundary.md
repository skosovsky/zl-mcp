# ADR: complete read boundary and separate Events protocol (T06)

Status: contract fixed before implementation; source authority is the accepted T01
manifest and unchanged executable docs/contracts. Full inventory is in
`evidence/T06/read-manifest.json`:17 read tools,4 static resources,3 templates.
Five commands remain on their previous paths until T07. No earlier task is reopened.

Application handlers and consumer-owned ports are explicit per capability:
conversations owns group/catalog/conversation/browse/status/collection diagnostics,
search owns current-corpus searches, archives owns source catalogue/browse/full
archive resources, membership owns invite inspection/join status, messaging owns
send status, history owns import status, eventaccess owns subscription listing,
read receipts and delivery diagnostics. Shared DTOs are bounded business values,
not a universal map or raw JSON bus. Existing domain types are reused where exact.
Legacy storage/control maps are converted only inside borrowed backend adapters;
SDK/wire maps remain transport projection. Every operation-status port names only
its fixed read; it cannot dispatch start/cancel/send/join/ACK on status failure.

Current-corpus versus source_id branches stay explicit. Omitted source_id selects
only current corpus. Explicit source_id selects an account-bound permanent archive
with original mandatory date window, metadata, coverage/null/completeness semantics,
TTL/tombstones/current policy and signed continuation. No fallback/import/sync is
introduced. Archive row identities remain separate from native Zalo message IDs
and cannot supply outgoing quote authority. Full archive resources retain their
token grammar, JSON MIME/body/schema and1MiB bounds. Empty-page continuation and
catalogue/browse dedup/ordering are preserved exactly.

All read tools go through the current trusted runtime/view, executable binder and
schema boundary; subject/scope comes from host/current account/policy. Selected
message/resource access is independently rechecked by native owners. Read budget
is a consumer-owned narrow admission port around the existing shared Store bucket,
called once per tool and separately once per resource. Resource bodies call typed
app ports directly; native SDK registrations and original safe projection/limits
remain MCP-owned. Capabilities resource retains its static wire instructions.
Invocation-owned list/browse budget shortening reuses the original native read
semantics; it doesn't redispatch a tool or repeat any effect.

Existing get_group live read5s timeout/local fallback/stale semantics, membership
read6s deadline and send/history status35s deadlines remain unchanged. No status
starts work. Events subscription/list/read retain current principal/profile/grant,
receipt/order/redelivery/durable journal semantics; ACK remains a command for T07.

The HTTP Events subscription/webhook extension stays a separate protocol adapter.
MCP consumes its narrow existing protocol port, supplied by composition; it no
longer constructs/imports the concrete events.Manager. Service retains ownership
of workers, store, guarded session and archive handles. HTTP capabilities/frame
behavior stays unchanged; STDIO doesn't gain unsupported Events capability.
Existing protocol JSON/framing stays in native transport/legacy adapter, not app.
The exact MCP->events T06 architecture exception is removed; remaining native
command/admission cleanup is assigned to T07 without broadening exceptions.

Acceptance: fixed equal leaves in evidence/T06/requirements.json, entire read
manifest/transport/resources/archive/status suites, negative access/effect counters,
race, root/vet, architecture100/100+controls and complete offline evaluation.
Frozen original business with accepted T03 overlay remains independent. Two new
independent reviewers must accept the same source before the sole task commit.
Model/live remain NOT_RUN; SDK ID repair remains SDK-01; performance acceptance isT09.

Composition fixtures explicitly import the same operation adapter and capability
handlers as service. The normal synthetic fixture still uses its local control
socket; offline fixtures use their existing synthetic owner. Architecture permits
these exact composition edges; production cannot import the fixture.

The shared admission port delegates the native bucket exactly once per tool or
resource. Collector/group/catalogue/collection/delivery diagnostics use explicit
DTOs. Group list continuation repeats only the same native read with a smaller
limit and original query/cursor; group refresh keeps5s and the original-context
local fallback/stale flag. Delivery reasons remain the bounded native count map.
The legacy top-level null argument map is characterized as an empty object;
null field values remain unchanged. Startup account binding can supersede an
invocation scope: the stale call is denied before its handler, and only a fresh
host binding may read. Synthetic readiness polling tolerates that explicit denial
within its original deadline; persistent or other failures still fail acceptance.

The wire adapter imports the exact app/shared admission contract, while the store
adapter imports time for native delivery diagnostics. These normative app/adapter
edges replace concrete reads; backend exceptions are not widened.

The synthetic composition now supplies the exact archives capability as service does;
arch-go admits only evalfixture -> app/archives for this composition edge.
Events recovery has explicit legacy/v2 envelope DTOs. The v2 nullable name and
first_incoming fields remain present, while legacy omits all v2-only fields.
All17 production read tools are mandatory; missing typed capabilities fail startup,
and the transport retains only the five T07 command branches.
