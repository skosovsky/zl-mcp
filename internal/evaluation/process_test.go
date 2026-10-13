package evaluation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProcessHelper(t *testing.T) {
	// Arrange: modes run only in the isolated test subprocess.
	mode := ""
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "helper=") {
			mode = strings.TrimPrefix(arg, "helper=")
		}
	}
	if mode == "" {
		return
	}
	// Act: simulate a child that violates its wall/output budget.
	switch mode {
	case "hang":
		fmt.Print(`{"partial":`)
		time.Sleep(time.Hour)
	case "flood":
		for {
			fmt.Fprint(os.Stderr, strings.Repeat("x", 1024))
		}
	case "normal":
		fmt.Print(`{"ok":true}`)
	}
	// Assert: parent tests verify termination; exit before test runner prints PASS.
	os.Exit(0)
}
func TestExternalWatchdogAndOutputBounds(t *testing.T) {
	for _, mode := range []string{"hang", "flood", "normal"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange
			binary, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			timeout := 300 * time.Millisecond
			if mode == "normal" || mode == "flood" {
				// Allow race-instrumented helper startup; flood still terminates
				// through its output bound, independently of the watchdog.
				timeout = 2 * time.Second
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			started := time.Now()
			// Act
			stdout, stderr, err := Process(ctx, binary, []string{"-test.run=^TestProcessHelper$", "--", "helper=" + mode}, 4096)
			// Assert
			if time.Since(started) > 3*time.Second {
				t.Fatal("watchdog did not terminate and Wait")
			}
			switch mode {
			case "hang":
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatal(err)
				}
			case "flood":
				if !errors.Is(err, ErrOutputBudget) || len(stderr) > 4096 {
					t.Fatal(err, len(stderr))
				}
			case "normal":
				if err != nil || string(stdout) != `{"ok":true}` {
					t.Fatal(err, string(stdout))
				}
			}
		})
	}
}
func TestFailedEvidenceCannotPassDeterministicGrader(t *testing.T) {
	// Arrange
	cs := Case{ID: "case", ExpectedEffects: map[string]int{}, Checks: []Check{{Name: "required", Step: 0, Op: "success"}}}
	out := Output{CaseID: "case", Complete: false, Cleanup: false, Effects: map[string]int{"send": 1}}
	// Act
	assertions := Grade(cs, out)
	passed := 0
	for _, a := range assertions {
		if a.Pass {
			passed++
		}
	}
	// Assert
	if passed == len(assertions) || assertions[0].Pass || assertions[1].Pass || assertions[5].Pass {
		t.Fatal("missing evidence or unauthorized effects passed")
	}
}

func TestDatasetRejectsTrailingAndUnknownChecks(t *testing.T) {
	// Arrange: the executable contract is the source of accepted check operators.
	valid, err := os.ReadFile("../../docs/evals/migration-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{string(valid) + ` {}`, strings.Replace(string(valid), `"op": "equal"`, `"op": "unrecognized"`, 1)} {
		file := filepath.Join(t.TempDir(), "dataset.json")
		if err := os.WriteFile(file, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		// Act
		_, err := loadDataset(file)
		// Assert
		if err == nil {
			t.Fatal("invalid dataset accepted")
		}
	}
}
func TestEffectJournalBoundAndMalformedRecord(t *testing.T) {
	for _, data := range []string{strings.Repeat("x", (64<<10)+1), `{"kind":"send"`} {
		// Arrange
		file := filepath.Join(t.TempDir(), "effects.jsonl")
		if err := os.WriteFile(file, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		// Act
		_, err := readEffects(file)
		// Assert
		if err == nil {
			t.Fatal("invalid journal accepted")
		}
	}
}
