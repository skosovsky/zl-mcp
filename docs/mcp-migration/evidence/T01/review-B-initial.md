# T01 initial independent correctness review
Reviewer: /root/t01_correctness. Parent: 8d913c48b797f54f277ffc1106e6bf28be9e697d.
Source digest: 0dc5471d1b8a4638f96564703b11c229b363774ac4c185c9d7ae7bc66ff85bb9.
Verdict: FAIL; two unresolved P2 findings before fixes.
1. T01-05, baseline.py buffered readline after select can hang on child partial line.
Repro: write '{', flush, keep stdout open; finally never executes. Require nonblocking
bounded reads/watchdog and negative partial-response test.
2. T01-07, Unicode snapshot overwrites complete text from structured payload; missing
coverage in original text can be hidden. Require independent parity check before specific
timestamp normalization and a negative control of another field.
Independent MCP/contracts tests PASS; source/log hashes and manifest checked;
frozen binary rebuild smoke PASS in /tmp/zl-t01-review-b-20261011.
No production/persistence/third_party changes. Model/live NOT_RUN.
