import copy
import json
import hashlib
import tempfile
import subprocess
import sys
from pathlib import Path
import unittest

from compare_performance import compare, P95_LIMITS, ACCEPTANCE_REVISION


class PerformanceGateTests(unittest.TestCase):
    def setUp(self):
        self.baseline = json.loads((Path(__file__).resolve().parents[2] / 'docs/mcp-migration/evidence/T03/performance-baseline/manifest.json').read_text())

        self.baseline['build_manifest'] = str(Path(__file__).resolve().parents[2] / 'docs/mcp-migration/evidence/T03/baseline-build-manifest-v2.json')
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)

    def candidate(self):
        candidate = copy.deepcopy(self.baseline)
        candidate['allocations_ratio_threshold'] = 2.0
        candidate['alloc_bytes_ratio_threshold'] = 1.20
        candidate['performance_gate_revision'] = '2026-10-12-objects-and-bytes-v2'
        candidate['binary_sha256'] = 'a' * 64
        binary = '/synthetic/candidate'
        for row in candidate['records']:
            row['command'][0] = binary
        build = {'status': 'PASS', 'toolchain': 'go version go1.27.1 darwin/arm64', 'candidate_sources': {'source_digest_sha256': 'b' * 64},
                 'builds': [{'kind': 'candidate', 'target': './cmd/zl-mcp-eval',
                             'exit_status': 0, 'binary': binary, 'build_info': 'go1.27.1',
                             'binary_sha256': candidate['binary_sha256']}]}
        raw = json.dumps(build).encode()
        file = Path(self.temp.name) / 'build.json'
        file.write_bytes(raw)
        candidate['build_manifest'] = str(file)
        candidate['build_manifest_sha256'] = hashlib.sha256(raw).hexdigest()
        return candidate

    def test_reported_summary_cannot_hide_latency_regression(self):
        # Arrange: summary claims success while every raw read latency regresses.
        candidate = self.candidate()
        for row in candidate['records']:
            if row['kind'] == 'read':
                row['measurement']['p95_seconds'] = P95_LIMITS['read'] * 1.01
        # Act.
        result = compare(self.baseline, candidate)
        # Assert.
        self.assertEqual(result['status'], 'FAIL')
        self.assertFalse(result['gates']['read']['p95_pass'])

    def test_zero_baseline_and_different_machine_cannot_pass(self):
        # Arrange.
        candidate = self.candidate()
        candidate['cpu_model'] = 'different machine'
        for row in self.baseline['records']:
            row['measurement']['allocations_per_op'] = 0
        # Act.
        result = compare(self.baseline, candidate)
        # Assert.
        self.assertEqual(result['status'], 'FAIL')
        self.assertIn('incompatible cpu_model', result['errors'])
        self.assertFalse(result['gates']['read']['allocations_pass'])

    def test_duplicate_run_and_changed_threshold_cannot_pass(self):
        # Arrange.
        candidate = self.candidate()
        candidate['records'][3] = copy.deepcopy(candidate['records'][0])
        candidate['p95_ratio_threshold'] = 9
        # Act.
        result = compare(self.baseline, candidate)
        # Assert.
        self.assertEqual(result['status'], 'FAIL')
        self.assertIn('candidate changed thresholds', result['errors'])
        self.assertIn('candidate missing/duplicate run read', result['errors'])

    def test_same_complete_raw_runs_pass(self):
        # Arrange.
        candidate = self.candidate()
        # Act.
        result = compare(self.baseline, candidate)
        # Assert.
        self.assertEqual(result['status'], 'PASS')

    def test_invalid_measurements_fail_closed(self):
        for value in (-1, float('nan'), float('inf'), True, '0', None):
            for field in ('p95_seconds', 'allocations_per_op', 'alloc_bytes_per_op', 'elapsed_seconds'):
                with self.subTest(value=value, field=field):
                    # Arrange.
                    candidate = self.candidate()
                    candidate['records'][0]['measurement'][field] = value
                    # Act.
                    result = compare(self.baseline, candidate)
                    # Assert.
                    self.assertEqual(result['status'], 'FAIL')

    def test_matching_missing_provenance_cannot_pass(self):
        for field in ('cpu_model', 'cpu_count', 'fixture_sha256'):
            with self.subTest(field=field):
                # Arrange.
                baseline = copy.deepcopy(self.baseline)
                candidate = self.candidate()
                baseline.pop(field)
                candidate.pop(field)
                # Act.
                result = compare(baseline, candidate)
                # Assert.
                self.assertEqual(result['status'], 'FAIL')
        for field in ('go', 'os', 'arch'):
            # Arrange.
            baseline = copy.deepcopy(self.baseline)
            candidate = self.candidate()
            for row in baseline['records'] + candidate['records']:
                row['measurement'].pop(field)
            # Act.
            result = compare(baseline, candidate)
            # Assert.
            self.assertEqual(result['status'], 'FAIL')

    def test_baseline_binary_cannot_masquerade_as_candidate(self):
        # Arrange.
        candidate = copy.deepcopy(self.baseline)
        # Act.
        result = compare(self.baseline, candidate)
        # Assert.
        self.assertEqual(result['status'], 'FAIL')
        self.assertTrue(any('role/digest mismatch' in e for e in result['errors']))

    def test_invalid_counts_and_matching_changed_protocol_cannot_pass(self):
        for value in (True, -1, '1000', None):
            # Arrange.
            candidate = self.candidate()
            candidate['records'][0]['measurement']['calls'] = value
            # Act.
            result = compare(self.baseline, candidate)
            # Assert.
            self.assertEqual(result['status'], 'FAIL')
        for field, value in [('pacing_millis', 0), ('admission_enabled', False), ('warmup_calls', 0)]:
            # Arrange.
            baseline = copy.deepcopy(self.baseline)
            candidate = self.candidate()
            baseline[field] = candidate[field] = value
            # Act.
            result = compare(baseline, candidate)
            # Assert.
            self.assertEqual(result['status'], 'FAIL')

    def test_malformed_json_cli_preserves_fail_report(self):
        # Arrange.
        bad = Path(self.temp.name) / 'bad.json'
        report = Path(self.temp.name) / 'report.json'
        bad.write_text('{incomplete')
        script = Path(__file__).with_name('compare_performance.py')
        # Act.
        result = subprocess.run([sys.executable, str(script), '--baseline', str(bad),
                                 '--candidate', str(bad), '--output', str(report)],
                                capture_output=True, text=True, timeout=10)
        # Assert.
        self.assertEqual(result.returncode, 1)
        self.assertEqual(json.loads(report.read_text())['status'], 'FAIL')
        self.assertNotIn('Traceback', result.stderr)

    def test_enormous_json_integer_cli_preserves_fail_report(self):
        for field in ('p95_seconds', 'allocations_per_op', 'alloc_bytes_per_op', 'elapsed_seconds'):
            with self.subTest(field=field):
                # Arrange: valid JSON with an integer outside the float domain.
                candidate = self.candidate()
                candidate['records'][0]['measurement'][field] = 10 ** 1000
                baseline_path = Path(self.temp.name) / 'baseline.json'
                candidate_path = Path(self.temp.name) / 'candidate.json'
                report = Path(self.temp.name) / 'huge-report.json'
                baseline_path.write_text(json.dumps(self.baseline))
                candidate_path.write_text(json.dumps(candidate))
                script = Path(__file__).with_name('compare_performance.py')
                # Act.
                result = subprocess.run([sys.executable, str(script), '--baseline', str(baseline_path),
                                         '--candidate', str(candidate_path), '--output', str(report)],
                                        capture_output=True, text=True, timeout=10)
                # Assert.
                self.assertEqual(result.returncode, 1)
                self.assertEqual(json.loads(report.read_text())['status'], 'FAIL')
                self.assertNotIn('Traceback', result.stderr)

    def test_more_objects_cannot_hide_allocated_byte_regression(self):
        # Arrange: objects meet the revised budget; bytes exceed their own gate.
        candidate = self.candidate()
        for row in candidate['records']:
            row['measurement']['allocations_per_op'] *= 1.9
            row['measurement']['alloc_bytes_per_op'] *= 1.21
        # Act.
        result = compare(self.baseline, candidate)
        # Assert.
        self.assertEqual(result['status'], 'FAIL')
        for gate in result['gates'].values():
            self.assertTrue(gate['allocations_pass'])
            self.assertFalse(gate['alloc_bytes_pass'])

    def test_small_objects_cannot_hide_object_count_regression(self):
        # Arrange.
        candidate = self.candidate()
        for row in candidate['records']:
            row['measurement']['allocations_per_op'] *= 2.01
        # Act.
        result = compare(self.baseline, candidate)
        # Assert.
        self.assertEqual(result['status'], 'FAIL')
        for gate in result['gates'].values():
            self.assertFalse(gate['allocations_pass'])
            self.assertTrue(gate['alloc_bytes_pass'])

    def test_zero_allocated_byte_baseline_requires_zero(self):
        # Arrange.
        candidate = self.candidate()
        for row in self.baseline['records']:
            row['measurement']['alloc_bytes_per_op'] = 0
        # Act.
        result = compare(self.baseline, candidate)
        # Assert.
        self.assertEqual(result['status'], 'FAIL')
        self.assertTrue(all(not gate['alloc_bytes_pass'] for gate in result['gates'].values()))

    def test_only_authorized_gate_revision_and_budgets_can_pass(self):
        for field, value in [('performance_gate_revision', 'other'),
                             ('allocations_ratio_threshold', 3.0),
                             ('alloc_bytes_ratio_threshold', 2.0),
                             ('zero_baseline_allocation_threshold', False)]:
            # Arrange.
            candidate = self.candidate()
            candidate[field] = value
            # Act.
            result = compare(self.baseline, candidate)
            # Assert.
            self.assertEqual(result['status'], 'FAIL', field)

    def test_fixed_budgets_pass_at_boundary_and_fail_above(self):
        # Arrange: do not derive the budget from the input under test.
        candidate = self.candidate()
        for row in candidate['records']:
            row['measurement']['p95_seconds'] = P95_LIMITS[row['kind']]
        # Act.
        result = compare(self.baseline, candidate)
        # Assert.
        self.assertEqual(result['status'], 'PASS')
        self.assertEqual(result['gate_contract_revision'], ACCEPTANCE_REVISION)
        for row in candidate['records']:
            row['measurement']['p95_seconds'] *= 1.000001
        self.assertEqual(compare(self.baseline, candidate)['status'], 'FAIL')

    def test_input_cannot_recalibrate_budget(self):
        # Arrange: both raw sides have extreme latency; a candidate-derived
        # floating budget or a ratio against a changed baseline would pass.
        candidate = self.candidate()
        for row in self.baseline['records'] + candidate['records']:
            row['measurement']['p95_seconds'] = 10
        candidate['summary'] = {'read': {'median_of_run_p95_seconds': 0}}
        candidate['p95_limits_seconds'] = {kind: 100 for kind in P95_LIMITS}
        # Act.
        result = compare(self.baseline, candidate)
        # Assert.
        self.assertEqual(result['status'], 'FAIL')
        self.assertTrue(all(not g['p95_pass'] for g in result['gates'].values()))


if __name__ == '__main__':
    unittest.main()
