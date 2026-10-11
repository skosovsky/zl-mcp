# T04 application read boundary

Status: ACCEPTED offline; completeness40/40=100%, correctnessPASS with zero findings on the same candidate. All mandatory checks PASS.
Parent: da3d7b6f0369f85e58d17b546c933b362c342224.
Source digest: 9101f22bd6effc1458f0ea7b0cfb549f8565cb13a7fbaac33c574639a6ba33e0.
Source-only tree recorded in candidate-source-tree.json; 40 equal leaf requirements fixed before implementation.

Typed app/search and app/conversations own search/coverage/empty reasons and context/full-message requests. Borrowed adapters/store retains native storage page and cursor continuation. Service/fixture composition injects handlers into internal MCP constructors; known internal callers updated atomically. No external protocol, schema, persisted data, archive/source_id paths, SDK, dependency pins, sibling library or third_party changed.

MCP owns original schema/admission/result projection and wire limits. Search performs one FTS query; app continuation shortens native pages without requery. Native context clipping and full resource policy remain Store-owned. The new real SDK boundary test proves one FTS page/one trim, opaque big IDs and Unicode, snapshot exclusion of late backdated insert, continuation completeness, full resource and repeat access revocation. Fake-port tests reject group/direct identity collision before dispatch.

Frozen evaluation overlay now reads accepted T03 sources from its commit, rather than current candidate composition. Frozen business SHA remains original T01 baseline. Build manifests disclose each overlay SHA and binary hash; final candidate build exactly matches source digest. Earlier build/check attempts are retained; a self-review restored exact existing permission text before final build and revalidation. v2 baseline is identical frozen T03 overlay; the sole candidate delta v2->v3 is that exact permission wording.

Root tests, vet, relevant contracts/storage/MCP HTTP/STDIO/resources, application boundary, race checks and architecture100/100 including isolated/all-build negative controls PASS. Final candidate offline 50/50 cases,480/480 assertions; frozen baseline25/25,240/240. Independent fixture copies, raw traces and durable effects retained. No performance comparison claimed in T04; accepted T03 baseline unchanged.

Model/live gates NOT_RUN. SDK numeric JSON-RPC ID precision gate DEFERRED_TO_SDK-01; ID01-ID07 not claimed PASS. Offline acceptance only. No deployment, LaunchAgent, live account, paid eval or publication.

Sole commit after both reviewers accept:
`refactor(search): extract application handlers and read ports`.
