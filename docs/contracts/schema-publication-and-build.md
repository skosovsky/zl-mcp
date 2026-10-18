# Published schema and build identity contracts

Implemented under R06/R07 of [MCP hardening](../task-mcp-skills-hardening.md).

## Schema publication

`contracts.Document` and `contracts.Compile` continue to use the full embedded
schema. `contracts.PublishedDocument` produces a caller-owned catalog projection
with only reachable root `$defs`. The server publishes this projection on both
HTTP and the STDIO bridge. This changes discovery size, not accepted inputs or
successful output DTOs.

Reachability follows transitive local `$ref` JSON Pointers, including composition,
cycles, pointers into arrays/subschemas, URI percent escaping and `~0`/`~1`.
Definitions are retained before following their dependencies, so cycles terminate.
Only root definitions are deleted. Unknown reference scopes (anchors, dynamic or
recursive references, external references, nested `$id`/`$defs`) retain the full
schema rather than risk changing validation. The current tool schemas use the
supported local root definition profile. Root `$defs` is omitted if empty.

Publication is deterministic and does not mutate embedded files. Existing
validators and validation boundaries remain in place. Manifest and discovery
baselines contain the published projection. Byte savings are measured in tests;
this does not claim equal token savings or improved model task success.

## Build identity

Every tool result carries an additive MCP metadata entry, independent of its
strict successful `outputSchema` or structured error body:

```json
{
  "_meta": {
    "zl-mcp/build": {
      "backend": {
        "version": "unknown",
        "revision": "0123456789abcdef0123456789abcdef01234567",
        "dirty": false
      }
    }
  }
}
```

The [executable schema](build_identity.output.json) validates the value of
`_meta["zl-mcp/build"]`. `backend` always describes the process serving the
request. STDIO forwarding adds `bridge` with the same identity fields and retains
the received backend identity. An older backend without this metadata becomes
an explicit unknown identity; the bridge never substitutes its own revision.

Fields are always present. `version` is the safe module release version or
`unknown` for absent/development metadata; `revision` is the full hexadecimal
VCS revision or `unknown`; `dirty` is boolean when Go embeds `vcs.modified`,
otherwise null. Unknown does not imply a clean build. Local paths, module replace
paths, environment and arbitrary build settings are never exposed.

`serverInfo.version` is a human-readable rendering
`<version>+revision.<revision>.<clean|dirty|unknown>` of the executable at that
transport boundary. Thus an HTTP endpoint identifies its backend, and an STDIO
endpoint identifies its bridge. Tool result metadata identifies both explicitly.
`protocolVersion` remains the independently negotiated MCP version.

No stored messages, subscriptions, operation outcomes or checkpoints are rewritten.
Clients that ignore additional `_meta` retain their former output behavior.
Compatibility snapshots normalize only volatile build metadata; runtime tests
must assert the actual unnormalized identities separately.
