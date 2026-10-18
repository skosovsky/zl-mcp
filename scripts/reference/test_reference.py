"""Contract-focused negative controls for the offline reference workflow."""
import copy
import importlib.util
import json
from pathlib import Path
import unittest

SPEC = importlib.util.spec_from_file_location("reference", Path(__file__).with_name("reference.py"))
REFERENCE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(REFERENCE)


class ReferenceTests(unittest.TestCase):
    def setUp(self):
        self.document = REFERENCE.DOC.read_text()
        self.manifest = json.loads((REFERENCE.ROOT / "docs/mcp-migration/manifest.json").read_text())
        self.editorial = json.loads(REFERENCE.EDITORIAL.read_text())

    def test_repeated_generation_is_stable_and_links_resolve(self):
        # Arrange / Act
        rendered = REFERENCE.render(self.document, self.manifest, self.editorial)
        repeated = REFERENCE.render(rendered, self.manifest, self.editorial)
        # Assert
        self.assertEqual(rendered, repeated)
        self.assertGreater(REFERENCE.validate_links(rendered), 0)

    def test_description_input_output_and_hints_drift_changes_generated_document(self):
        # Arrange
        mutations = [lambda t: t.update(description="changed exact description"),
                     lambda t: t["inputSchema"].setdefault("properties", {}).update(new_parameter={"type": "string"}),
                     lambda t: t["outputSchema"].setdefault("properties", {}).update(new_output={"type": "boolean"}),
                     lambda t: t["annotations"].update(readOnlyHint=not t["annotations"]["readOnlyHint"])]
        original = REFERENCE.render(self.document, self.manifest, self.editorial)
        for mutate in mutations:
            manifest = copy.deepcopy(self.manifest)
            mutate(manifest["tools"][0])
            # Act / Assert
            self.assertNotEqual(original, REFERENCE.render(self.document, manifest, self.editorial))

    def test_executable_manifest_guards_reject_missing_tools_and_stale_shapes(self):
        # Arrange / Act / Assert
        manifest = copy.deepcopy(self.manifest)
        manifest["tools"].pop()
        with self.assertRaisesRegex(ValueError, "tool set differs"):
            REFERENCE.validate_manifest(manifest)
        manifest = copy.deepcopy(self.manifest)
        manifest["tools"][0]["inputSchema"]["properties"]["invented"] = {"type": "string"}
        with self.assertRaisesRegex(ValueError, "schema drift"):
            REFERENCE.validate_manifest(manifest)
        manifest = copy.deepcopy(self.manifest)
        manifest["resources"].pop()
        with self.assertRaisesRegex(ValueError, "resources set differs"):
            REFERENCE.validate_manifest(manifest)

    def test_broken_local_link_is_rejected(self):
        # Arrange / Act / Assert
        with self.assertRaisesRegex(ValueError, "broken local reference"):
            REFERENCE.validate_links(self.document + "\n[broken](definitely-absent.md)\n")

    def test_source_registries_have_expected_independent_entries(self):
        # Arrange / Act
        events = dict(REFERENCE.event_catalog())
        reactions = dict((code, (name, display)) for code, name, display in REFERENCE.reaction_choices())
        cli = REFERENCE.cli_commands()
        # Assert: names come from production source, not private live state.
        self.assertIn("zalo.conversation.call.observed.v1", events)
        self.assertEqual(reactions["heart"], ("Heart", "❤️"))
        self.assertIn("probe-message-action", cli)
        self.assertIn("check-rollback", cli)


if __name__ == "__main__":
    unittest.main()
