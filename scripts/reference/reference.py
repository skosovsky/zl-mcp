#!/usr/bin/env python3
"""Render/check executable reference sections offline; standard library only.

Sources: checked migration manifest (independently compared to actual SDK in Go
integration tests), embedded JSON schemas, explicit Go registries for reactions,
Events and CLI. No live account, local service or temporary helper is required.
"""
import argparse
import json
from pathlib import Path
import re
import sys
from urllib.parse import unquote

ROOT = Path(__file__).resolve().parents[2]
DOC = ROOT / "docs/system-and-tools-reference.md"
EDITORIAL = ROOT / "scripts/reference/editorial.json"
ALIASES = {
    "zalo_import_conversation_history": ("history_import_request", "history_import_operation"),
    "zalo_get_history_import_status": ("history_import_operation", "history_import_operation"),
    "zalo_cancel_history_import": ("history_import_operation", "history_import_operation"),
}


def cell(value):
    return str(value).replace("|", "\\|").replace("\n", " ")


def schema_type(schema):
    if "type" in schema:
        value = schema["type"]
        return " / ".join(value) if isinstance(value, list) else value
    if "anyOf" in schema:
        return " / ".join(dict.fromkeys(schema_type(item) for item in schema["anyOf"]))
    return "oneOf" if "oneOf" in schema else "$ref" if "$ref" in schema else "по schema"


def bounds(schema):
    items = []
    if "enum" in schema:
        items.append("enum: " + ", ".join(f"`{item}`" for item in schema["enum"]))
    for key, label in [("minLength", "длина ≥"), ("maxLength", "длина ≤"),
                       ("minimum", "≥"), ("maximum", "≤"), ("minItems", "элементов ≥"),
                       ("maxItems", "элементов ≤")]:
        if key in schema:
            items.append(f"{label} {schema[key]}")
    if schema.get("uniqueItems"):
        items.append("без дубликатов")
    for key in ["format", "pattern", "default"]:
        if key in schema:
            items.append(f"{key}: `{json.dumps(schema[key], ensure_ascii=False)}`")
    return "; ".join(items) or "—"


def marker(name, text):
    return f"<!-- reference:{name}:start -->\n{text.strip()}\n<!-- reference:{name}:end -->"


def put_section(document, name, generated, start=None, end=None):
    pattern = rf"<!-- reference:{name}:start -->.*?<!-- reference:{name}:end -->"
    if re.search(pattern, document, re.S):
        return re.sub(pattern, lambda _: marker(name, generated), document, flags=re.S)
    if start is None:
        raise ValueError(f"missing generated section {name}")
    a = document.index(start)
    b = document.index(end, a) if end else len(document)
    return document[:a] + marker(name, generated) + "\n\n" + document[b:]


def tool_section(tools, editorial):
    lines = [f"## 4. Полный каталог {len(tools)} инструментов", "",
             "Ниже все tools/list в алфавитном порядке. Параметры, описания и поля ответа",
             "проверяются по manifest и исполняемым контрактам; пояснения проверяются ревью.", "",
             "| № | Tool | Read-only hint | Open-world hint | Destructive hint |",
             "| --- | --- | --- | --- | --- |"]
    for index, tool in enumerate(tools, 1):
        annotations = tool["annotations"]
        name = tool["name"]
        flags = " | ".join(str(annotations[key]).lower() for key in
                           ["readOnlyHint", "openWorldHint", "destructiveHint"])
        lines.append(f"| {index} | [`{name}`](#4{index}-{name}) | {flags} |")
    for index, tool in enumerate(tools, 1):
        name = tool["name"]
        note = editorial["tools"][name]
        lines += ["", f"### 4.{index}. `{name}`", "", note["purpose"], "",
                  "**Точное описание из discovery:**", "", "> " + tool["description"], "",
                  "**Эффект и разрешения.** " + note["effect"], "",
                  "**Как устроено.** " + note["implementation"], "", "**Параметры:**", ""]
        schema = tool["inputSchema"]
        props = schema.get("properties", {})
        if props:
            lines += ["| Поле | Обязательно | Тип | Ограничения | Назначение |",
                      "| --- | --- | --- | --- | --- |"]
            for key, value in props.items():
                required = "да" if key in schema.get("required", []) else "нет"
                if name == "zalo_set_message_reaction" and key == "reaction":
                    required = "только set; запрещено для remove"
                if name == "zalo_search_conversation_messages" and key == "conversation_type":
                    required = "при conversation_id"
                if name == "zalo_list_conversation_messages" and key in ["since", "until"]:
                    required = "при source_id"
                description = value.get("description") or editorial["parameters"].get(key, "См. исполняемый контракт.")
                lines.append(f"| `{key}` | {required} | {cell(schema_type(value))} | {cell(bounds(value))} | {cell(description)} |")
        else:
            lines += ["Нет прикладных параметров: `{}`."]
        result_fields = ", ".join(f"`{key}`" for key in tool["outputSchema"].get("properties", {}))
        inp, out = ALIASES.get(name, (name, name))
        for filename in [inp + ".input.json", out + ".output.json"]:
            if not (ROOT / "docs/contracts" / filename).exists():
                raise ValueError(f"missing executable contract {filename}")
        lines += ["", "**Результат.** " + note["result"], "",
                  "Все верхнеуровневые поля успешного результата: " + result_fields + ".", "",
                  f"Исполняемые контракты: [input](contracts/{inp}.input.json), [output](contracts/{out}.output.json)."]
    return "\n".join(lines)


def cli_commands():
    vocabulary = (ROOT / "internal/service/vocabulary.go").read_text()
    constants = dict(re.findall(r'(command\w+)\s*=\s*"([^"]+)"', vocabulary))
    source = (ROOT / "internal/service/cli.go").read_text()
    body = source.split("func runCLICommand(", 1)[1].split("\nfunc ", 1)[0]
    names = []
    for group in re.findall(r"case\s+(.+?):", body, re.S):
        for token in group.split(","):
            token = token.strip()
            names.append(json.loads(token) if token.startswith('"') else constants[token])
    if not names or len(names) != len(set(names)):
        raise ValueError("CLI parsing failed or duplicate commands")
    return names


def reaction_choices():
    source = (ROOT / "internal/domain/reaction_catalog.go").read_text()
    choices = re.findall(r'\{Code: ("(?:[^"\\]|\\.)*"), Name: ("(?:[^"\\]|\\.)*"), Display: ("(?:[^"\\]|\\.)*")\}', source)
    if not choices or len(choices) != len(re.findall(r"\bCode:", source)):
        raise ValueError("reaction catalogue source form changed; update parser")
    return [tuple(json.loads(part) for part in choice) for choice in choices]


def event_catalog():
    constants = {}
    for package in ["domain", "events"]:
        for path in (ROOT / "internal" / package).glob("*.go"):
            if not path.name.endswith("_test.go"):
                constants.update(re.findall(r'(\w+)\s+(?:string\s+)?=\s*"(zalo\.[^"]+)"', path.read_text()))
    source = (ROOT / "internal/events/subscriptions.go").read_text()
    body = source.split("func listEventContracts()", 1)[1].split("func (", 1)[0]
    pairs = re.findall(r'fieldName:\s*(?:domain\.)?(\w+),\s*"description":\s*("(?:[^"\\]|\\.)*")', body)
    if not pairs or len(pairs) != len(re.findall(r"fieldName:", body)):
        raise ValueError("Events registry source form changed; update parser")
    constants["MessageCreated"] = constants["LegacyMessageCreated"]
    return [(constants[name], json.loads(description)) for name, description in pairs]


def validate_manifest(manifest):
    """Fail if handwritten manifest silently omits executable registrations."""
    contracts = ROOT / "docs/contracts"
    expected = {path.name.removesuffix(".input.json") for path in contracts.glob("zalo_*.input.json")}
    expected.update(ALIASES)
    actual = {tool["name"] for tool in manifest["tools"]}
    if expected != actual:
        raise ValueError(f"manifest tool set differs from executable schemas: {expected ^ actual}")
    for tool in manifest["tools"]:
        inp, out = ALIASES.get(tool["name"], (tool["name"], tool["name"]))
        for kind, filename in [("input", inp), ("output", out)]:
            schema = json.loads((contracts / f"{filename}.{kind}.json").read_text())
            published = tool[kind + "Schema"]
            for key in ["properties", "required", "additionalProperties"]:
                if published.get(key) != schema.get(key):
                    raise ValueError(f"manifest schema drift: {tool['name']}.{kind}.{key}")
    for key, snapshot, field, identity in [
        ("resources", "resources", "resources", "uri"),
        ("resource_templates", "resource-templates", "resourceTemplates", "uriTemplate")
    ]:
        descriptors = json.loads((ROOT / f"internal/mcpserver/testdata/migration/{snapshot}.json").read_text())[field]
        expected = {item[identity]: item for item in descriptors}
        actual = {item[identity]: item for item in manifest[key]}
        if expected.keys() != actual.keys():
            raise ValueError(f"manifest {key} set differs from SDK snapshots")
        for name, descriptor in expected.items():
            if any(actual[name].get(k) != v for k, v in descriptor.items()):
                raise ValueError(f"manifest {key} descriptor drift: {name}")
    if set(manifest["cli"]["commands"]) != set(cli_commands()):
        raise ValueError("manifest CLI differs from dispatch source")
    if set(manifest["events"]["names"]) != {name for name, _ in event_catalog()}:
        raise ValueError("manifest Events differs from source registry")


def render(document, manifest, editorial):
    tools = manifest["tools"]
    if [tool["name"] for tool in tools] != sorted(tool["name"] for tool in tools):
        raise ValueError("manifest tools must be sorted")
    document = put_section(document, "tools", tool_section(tools, editorial), "## 4. Полный каталог", "## 5. Resources")
    resources = ["| URI | MIME | Содержание |", "| --- | --- | --- |"]
    for resource in manifest["resources"]:
        resources.append(f"| `{resource['uri']}` | {resource['mimeType']} | {cell(resource['name'])} |")
    document = put_section(document, "resources", "\n".join(resources), "| URI | MIME | Содержание |", "### 5.2.")
    templates = ["| URI template | MIME | Назначение |", "| --- | --- | --- |"]
    for resource in manifest["resource_templates"]:
        templates.append(f"| `{resource['uriTemplate']}` | {resource['mimeType']} | {cell(resource['name'])} |")
    document = put_section(document, "templates", "\n".join(templates), "| URI template | MIME | Назначение |", "## 6.")
    events = event_catalog()
    lines = ["| Event | Точное описание из events/list |", "| --- | --- |"]
    lines += [f"| `{name}` | {cell(description)} |" for name, description in events]
    document = put_section(document, "events", "\n".join(lines), "| Event | Что наблюдается | Ограничения |", "`zalo.conversation.message.created` без")
    reactions = reaction_choices()
    lines = ["| Code | Name | Display |", "| --- | --- | --- |"]
    lines += [f"| `{code}` | {cell(name)} | {cell(display)} |" for code, name, display in reactions]
    document = put_section(document, "reactions", "\n".join(lines), "| Code | Name | Display |", "## 8.")
    lines = ["| Команда | Назначение |", "| --- | --- |"]
    for name in cli_commands():
        lines.append(f"| `{name}` | {editorial['cli'][name]} |")
    document = put_section(document, "cli", "\n".join(lines), "| Команда | Назначение |", "У этих workflows")
    document = re.sub(r"Полный каталог \d+ инструментов", f"Полный каталог {len(tools)} инструментов", document)
    document = re.sub(r"#4-полный-каталог-\d+-инструментов", f"#4-полный-каталог-{len(tools)}-инструментов", document)
    document = re.sub(r"Каталог \d+ реакций", f"Каталог {len(reactions)} реакций", document)
    document = re.sub(r"#7-каталог-\d+-реакций", f"#7-каталог-{len(reactions)}-реакций", document)
    document = re.sub(r"Все \d+ входных контрактов", f"Все {len(tools)} входных контрактов", document)
    document = re.sub(r"у всех \d+ tools", f"у всех {len(tools)} tools", document)
    document = re.sub(r"не входят в \d+ tools", f"не входят в {len(tools)} tools", document)
    summary = (f"Каталог этой версии: **{len(tools)} инструментов, {len(manifest['resources'])} статических\n"
               f"resources, {len(manifest['resource_templates'])} шаблона resources и {len(events)} определений Events**.")
    document = re.sub(r"Проверено 18 октября 2026 года.*?определений Events\*\*\.", summary, document, flags=re.S)
    return document


def validate_links(document):
    headings = re.findall(r"^#{1,6} (.+)$", document, re.M)
    anchors = {re.sub(r"\s", "-", re.sub(r"[^\w\s-]", "", heading.lower())) for heading in headings}
    checked = 0
    for url in re.findall(r"\[[^\]]+\]\(([^)]+)\)", document):
        if "://" in url:
            continue
        target, _, fragment = url.partition("#")
        if target and not (DOC.parent / unquote(target)).resolve().exists():
            raise ValueError(f"broken local reference: {target}")
        if not target and fragment and fragment not in anchors:
            raise ValueError(f"broken local anchor: {fragment}")
        checked += 1
    return checked


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--check", action="store_true")
    mode.add_argument("--write", action="store_true")
    args = parser.parse_args()
    manifest = json.loads((ROOT / "docs/mcp-migration/manifest.json").read_text())
    editorial = json.loads(EDITORIAL.read_text())
    existing = DOC.read_text()
    validate_manifest(manifest)
    rendered = render(existing, manifest, editorial)
    links = validate_links(rendered)
    if args.check and existing != rendered:
        print("system-and-tools-reference.md is stale; run python3 scripts/reference/reference.py --write", file=sys.stderr)
        return 1
    if args.write:
        DOC.write_text(rendered)
    print(json.dumps({"tools": len(manifest["tools"]), "resources": len(manifest["resources"]),
                      "templates": len(manifest["resource_templates"]), "events": len(event_catalog()),
                      "reactions": len(reaction_choices()), "cli": len(cli_commands()), "local_links": links,
                      "result": "PASS"}))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
