package evaluation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestProcessHelper(t *testing.T) {
	// Arrange: modes run only in the isolated test subprocess.
	mode := ""
	for _, arg := range os.Args {
		if after, ok := strings.CutPrefix(arg, "helper="); ok {
			mode = after
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
	case "descendant":
		for {
			time.Sleep(time.Hour)
		}
	case "descendant-normal", "descendant-timeout":
		binary, err := os.Executable()
		if err != nil {
			os.Exit(1)
		}
		child := exec.Command(binary, "-test.run=^TestProcessHelper$", "--", "helper=descendant")
		// Inherit the watchdog-owned group, without holding its output pipes.
		if err := child.Start(); err != nil {
			os.Exit(1)
		}
		fmt.Print(child.Process.Pid)
		if mode == "descendant-timeout" {
			time.Sleep(time.Hour)
		}
	}
	// Assert: parent tests verify termination; exit before test runner prints PASS.
	os.Exit(0)
}

//nolint:gocognit // This AAA test keeps arrangement, operation and boundary assertions visible as one ordered scenario.
func TestWatchdogTerminatesDescendantsAfterNormalExitAndTimeout(t *testing.T) {
	for _, mode := range []string{"descendant-normal", "descendant-timeout"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange: the helper spawns a descendant in its owned process group.
			binary, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			started := time.Now()
			// Act.
			stdout, _, processErr := Process(
				ctx,
				binary,
				[]string{"-test.run=^TestProcessHelper$", "--", "helper=" + mode},
				4096,
			)
			// Assert: Wait is bounded and both exit paths terminate the owned group.
			pid, err := strconv.Atoi(string(stdout))
			if err != nil || pid <= 0 || time.Since(started) > 3*time.Second {
				t.Fatalf("helper did not spawn/finish: %q; %v; %v", stdout, err, processErr)
			}
			defer syscall.Kill(pid, syscall.SIGKILL)
			if mode == "descendant-normal" && processErr != nil {
				t.Fatal(processErr)
			}
			if mode == "descendant-timeout" && !errors.Is(processErr, context.DeadlineExceeded) {
				t.Fatal(processErr)
			}
			deadline := time.Now().Add(2 * time.Second)
			for !errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
				// A reparented zombie has terminated; reaping belongs to the host.
				state, _ := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
				if strings.HasPrefix(strings.TrimSpace(string(state)), "Z") {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("owned descendant %d still running", pid)
				}
				time.Sleep(10 * time.Millisecond)
			}
		})
	}
}
//nolint:gocognit // This AAA test keeps arrangement, operation and boundary assertions visible as one ordered scenario.
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
			stdout, stderr, err := Process(
				ctx,
				binary,
				[]string{"-test.run=^TestProcessHelper$", "--", "helper=" + mode},
				4096,
			)
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
	cs := Case{
		ID:              "case",
		ExpectedEffects: map[string]int{},
		Checks:          []Check{{Name: "required", Step: 0, Op: "success"}},
	}
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
