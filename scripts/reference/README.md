# System reference workflow

Run from any working directory, using Python 3.10+ and its standard library:

```sh
python3 scripts/reference/reference.py --write
python3 scripts/reference/reference.py --check
python3 -m unittest discover -s scripts/reference -p 'test_*.py'
```

The workflow updates only marked catalog sections in
`docs/system-and-tools-reference.md`, counters and navigation. It never starts
zl-mcp, reads a token, opens an account database, sends messages or requests
upstream access. No `/tmp` helpers, network or Python packages are required.

Authority is layered:

- Actual MCP SDK integration tests compare tools/list to manifest and discovery
  snapshots. Root contracts are executable embedded JSON Schema.
- This workflow checks manifest tool sets, parameter/result properties and strict
  keys against source schema files, resources/templates against SDK snapshots,
  Events/CLI against source registries, and renders exact descriptions and hints.
- Reaction code/name/display is read from the finite production source table.
- `editorial.json` contains reviewed explanatory prose, parameter fallback notes
  and operator command descriptions. It contains no credentials or account data.

Update the SDK manifest/snapshots through the existing explicit freeze workflow
before writing the reference. Source registration shape changes fail closed when
unsupported by the parser; update the parser and its negative controls together.
`--check` fails for drift. CI runs it on both operating systems.

Free-form architectural prose and editorial explanations require human/agent
review; matching generated sections does not prove their semantics or agent
behavior. Historical acceptance reports retain their original evidence boundary.
