package evaluation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func parityOutput(t *testing.T, at string) []byte {
	t.Helper()
	payload := map[string]any{"snapshot_at": at, "messages": []any{map[string]any{"message_id": "9007199254740993", "excerpt": "Cà phê 🍀", "sent_at": "2026-10-01T10:00:00Z"}}, "next_cursor": nil}
	return marshaled(Output{CaseID: "normal-search", Complete: true, Cleanup: true, Facts: map[string]any{}, Effects: map[string]int{}, Trace: []Trace{{Kind: "tool", Name: "zalo_search_messages", Input: marshaled(map[string]any{"query": "Cà phê"}), Result: marshaled(map[string]any{"structuredContent": payload, "content": []any{map[string]any{"type": "text", "text": string(marshaled(payload))}}})}}})
}
func mutateParity(t *testing.T, raw []byte, mutate func(map[string]any)) []byte {
	t.Helper()
	value, err := exactJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	out := value.(map[string]any)
	mutate(out)
	return marshaled(out)
}
func TestFullCaptureParityRejectsUnassertedContentAndEffects(t *testing.T) {
	// Arrange.
	baseline := parityOutput(t, "2026-10-12T00:00:00Z")
	for _, name := range []string{"both-projections", "text-only-time", "effect", "missing-null", "fixed-time", "native-id"} {
		t.Run(name, func(t *testing.T) {
			candidate := mutateParity(t, parityOutput(t, "2026-10-12T00:00:01Z"), func(out map[string]any) {
				result := out["trace"].([]any)[0].(map[string]any)["result"].(map[string]any)
				payload := result["structuredContent"].(map[string]any)
				block := result["content"].([]any)[0].(map[string]any)
				switch name {
				case "both-projections":
					payload["messages"].([]any)[0].(map[string]any)["excerpt"] = "changed unasserted text"
					block["text"] = string(marshaled(payload))
				case "text-only-time":
					p, _ := exactJSON([]byte(block["text"].(string)))
					p.(map[string]any)["snapshot_at"] = "2026-10-12T00:00:02Z"
					block["text"] = string(marshaled(p))
				case "effect":
					out["effects"] = map[string]any{"send": json.Number("1")}
				case "missing-null":
					delete(payload, "next_cursor")
					block["text"] = string(marshaled(payload))
				case "fixed-time":
					payload["messages"].([]any)[0].(map[string]any)["sent_at"] = "2026-10-01T10:00:01Z"
					block["text"] = string(marshaled(payload))
				case "native-id":
					payload["messages"].([]any)[0].(map[string]any)["message_id"] = "9007199254740992"
					block["text"] = string(marshaled(payload))
				}
			})
			// Act.
			record := compareCaptures([]parityCapture{{"normal-search", baseline}}, []parityCapture{{"normal-search", candidate}})
			// Assert.
			if record.Status != "FAIL" {
				t.Fatalf("mutated %s accepted: %+v", name, record)
			}
		})
	}
}
func TestFullCaptureParityAcceptsOnlyConsistentDynamicTime(t *testing.T) {
	// Arrange.
	left, right := parityOutput(t, "2026-10-12T00:00:00Z"), parityOutput(t, "2026-10-12T00:00:01Z")
	// Act.
	record := compareCaptures([]parityCapture{{"normal-search", left}}, []parityCapture{{"normal-search", right}})
	// Assert.
	if record.Status != "PASS" || record.Matched != 1 {
		t.Fatalf("valid pair: %+v", record)
	}
}
func TestFullCaptureParityPreservesExactNumbersAndResourceText(t *testing.T) {
	// Arrange.
	left := []byte(`{"case_id":"test","trace":[{"kind":"resource","name":"uri","result":{"text":"2026-10-12T00:00:00Z"}}],"facts":{"id":9007199254740993}}`)
	variants := [][]byte{[]byte(`{"case_id":"test","trace":[{"kind":"resource","name":"uri","result":{"text":"2026-10-12T00:00:00Z"}}],"facts":{"id":9007199254740992}}`), []byte(`{"case_id":"test","trace":[{"kind":"resource","name":"uri","result":{"text":"2026-10-12T00:00:01Z"}}],"facts":{"id":9007199254740993}}`)}
	for _, right := range variants {
		// Act.
		result := compareCaptures([]parityCapture{{"test", left}}, []parityCapture{{"test", right}})
		// Assert.
		if result.Status != "FAIL" {
			t.Fatal("precision/resource regression accepted")
		}
	}
}

// Diagnostic replay of retained real captures; no acceptance is inferred from
// this optional replay. The CLI must pass fresh complete paired executions.
func TestRetainedPairedCaptureParity(t *testing.T) {
	root := filepath.Join("..", "..", "docs", "mcp-migration", "evidence", "T09", "paired-v3")
	if _, err := os.Stat(root); err != nil {
		t.Skip("retained diagnostic evidence unavailable")
	}
	load := func(side string) []parityCapture {
		files, err := filepath.Glob(filepath.Join(root, side, "*", "output.json"))
		if err != nil {
			t.Fatal(err)
		}
		var captures []parityCapture
		for _, file := range files {
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var out Output
			if err = json.Unmarshal(raw, &out); err != nil {
				t.Fatal(err)
			}
			captures = append(captures, parityCapture{out.CaseID, raw})
		}
		return captures
	}
	// Arrange/Act.
	result := compareCaptures(load("baseline"), load("candidate"))
	// Assert.
	if result.Status != "PASS" {
		t.Fatalf("retained captures differ: %+v", result)
	}
}

func TestSDKBaselineCorrectionIsOneFieldAndCandidateMustBeExact(t *testing.T) {
	// Arrange: frozen legacy SDK and corrected candidate, including independent native/string IDs.
	baseline := []byte(`{"case_id":"numeric-precision","facts":{"sdk-envelope-9007199254740993":"9007199254740992","string-envelope":"\"9007199254740993\"","native_message_id":"9007199254740995"},"trace":[],"effects":{"sends":0}}`)
	candidate := mutateParity(t, baseline, func(out map[string]any) {
		out["facts"].(map[string]any)["sdk-envelope-9007199254740993"] = "9007199254740993"
	})
	compare := func(raw []byte) ParityRecord {
		return compareSDKCaptures([]parityCapture{{"numeric-precision", baseline}}, []parityCapture{{"numeric-precision", raw}})
	}
	// Act.
	record := compare(candidate)
	// Assert: the explicit exception is recorded, never silent masking.
	if record.Status != "PASS" || len(record.Pairs[0].BaselineCorrections) != 1 {
		t.Fatal(record)
	}
	for _, kind := range []string{"still-rounded", "native-id", "string-id", "extra-fact", "effect"} {
		altered := mutateParity(t, candidate, func(out map[string]any) {
			facts := out["facts"].(map[string]any)
			switch kind {
			case "still-rounded":
				facts["sdk-envelope-9007199254740993"] = "9007199254740992"
			case "native-id":
				facts["native_message_id"] = "9007199254740994"
			case "string-id":
				facts["string-envelope"] = "different"
			case "extra-fact":
				facts["unasserted"] = true
			case "effect":
				out["effects"].(map[string]any)["sends"] = 1
			}
		})
		if compare(altered).Status != "FAIL" {
			t.Fatalf("%s mutation hidden", kind)
		}
	}
}
