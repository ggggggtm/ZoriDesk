package sandbox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func expectedTailDigest(data []byte) string {
	if len(data) > loopbackOutputLimit {
		data = data[len(data)-loopbackOutputLimit:]
	}
	return fmt.Sprintf("sha256:%x", sha256.Sum256(data))
}

func TestOutputTailRetainsSuffix(t *testing.T) {
	for _, size := range []int{0, 1, loopbackOutputLimit - 1, loopbackOutputLimit, loopbackOutputLimit + 1, 3*loopbackOutputLimit + 17} {
		for _, chunk := range []int{1, 997, loopbackOutputLimit, 2*loopbackOutputLimit + 3} {
			t.Run(fmt.Sprintf("bytes_%d/chunk_%d", size, chunk), func(t *testing.T) {
				data := make([]byte, size)
				for i := range data {
					data[i] = byte(i * 31 % 251)
				}
				var output outputTail
				for offset := 0; offset < size; offset += chunk {
					end := min(offset+chunk, size)
					n, err := output.Write(data[offset:end])
					if n != end-offset || err != nil {
						t.Fatalf("write=(%d,%v), want %d,nil", n, err, end-offset)
					}
					if chunk > 1 || end == size || end%loopbackOutputLimit == 0 {
						if got, want := output.Digest(), expectedTailDigest(data[:end]); got != want {
							t.Fatalf("prefix %d: digest=%s want=%s", end, got, want)
						}
					}
				}
				if n, err := output.Write(nil); n != 0 || err != nil {
					t.Fatalf("empty write=(%d,%v)", n, err)
				}
				if got := output.Digest(); got != expectedTailDigest(data) {
					t.Fatalf("final digest=%s", got)
				}
			})
		}
	}
}

func TestOutputTailWritesDoNotAllocate(t *testing.T) {
	var output outputTail
	chunk := bytes.Repeat([]byte("x"), 997)
	// The steady-state writer must not allocate or append an output-sized buffer.
	allocs := testing.AllocsPerRun(1000, func() { _, _ = output.Write(chunk) })
	if allocs != 0 {
		t.Fatalf("allocations per write=%v, want zero", allocs)
	}
}

func TestOutputTailConcurrentWritesAndDigest(t *testing.T) {
	var output outputTail
	chunk := bytes.Repeat([]byte("x"), 997)
	var workers sync.WaitGroup
	for i := 0; i < 4; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for j := 0; j < 100; j++ {
				_, _ = output.Write(chunk)
				_ = output.Digest()
			}
		}()
	}
	workers.Wait()
	if got, want := output.Digest(), expectedTailDigest(bytes.Repeat([]byte("x"), loopbackOutputLimit)); got != want {
		t.Fatalf("digest=%s want=%s", got, want)
	}
}

func TestLoopbackOutputTailCompatibility(t *testing.T) {
	zeroTail := make([]byte, loopbackOutputLimit)
	for _, tc := range []struct {
		name, command string
		exit          int
		want          []byte
	}{
		{"empty", ":", 0, nil},
		{"small_mixed", "printf stdout; printf stderr >&2", 0, []byte("stdoutstderr")},
		{"large_stdout", "printf prefix; dd if=/dev/zero bs=65536 count=32 2>/dev/null", 0, zeroTail},
		{"large_stderr", "(dd if=/dev/zero bs=65536 count=32 2>/dev/null) >&2", 0, zeroTail},
		{"concurrent_streams", "dd if=/dev/zero bs=65536 count=16 2>/dev/null & (dd if=/dev/zero bs=65536 count=16 2>/dev/null) >&2 & wait", 0, zeroTail},
		{"failure_after_large_output", "dd if=/dev/zero bs=65536 count=32 2>/dev/null; printf failure >&2; exit 7", 7, append(append([]byte(nil), zeroTail[7:]...), []byte("failure")...)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			backend := newLoopback(t)
			instance, err := backend.Create(context.Background(), CreateRequest{JobID: tc.name, RunID: "output", Generation: 1})
			if err != nil {
				t.Fatal(err)
			}
			result, err := backend.Exec(context.Background(), ExecRequest{InstanceID: instance.InstanceID, Command: tc.command, Timeout: 5 * time.Second})
			if err != nil {
				t.Fatal(err)
			}
			if result.ExitCode != tc.exit || result.OutputDigest != expectedTailDigest(tc.want) {
				t.Fatalf("result=%+v want exit=%d digest=%s", result, tc.exit, expectedTailDigest(tc.want))
			}
		})
	}
}

func TestLoopbackLargeOutputThenCancel(t *testing.T) {
	backend := newLoopback(t)
	ctx := context.Background()
	instance, err := backend.Create(ctx, CreateRequest{JobID: "output-cancel", RunID: "output", Generation: 1})
	if err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		result ExecResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, execErr := backend.Exec(ctx, ExecRequest{
			InstanceID: instance.InstanceID,
			Command:    "dd if=/dev/zero bs=65536 count=32 2>/dev/null; sleep 10 & echo $! > ready; wait; :",
			Timeout:    30 * time.Second,
		})
		done <- outcome{result, execErr}
	}()
	pid := waitLoopbackReady(t, backend, instance.InstanceID)
	if err := backend.Cancel(ctx, instance.InstanceID); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		if got.err != nil || got.result.ExitCode == 0 || got.result.OutputDigest != expectedTailDigest(make([]byte, loopbackOutputLimit)) {
			t.Fatalf("cancel result=%+v error=%v", got.result, got.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("large-output cancellation did not finish")
	}
	assertLoopbackChildStopped(t, pid)
}

func TestLoopbackOutputDigestJournalReplay(t *testing.T) {
	backend := newLoopback(t)
	controller, store := newTestController(t, backend)
	ctx := context.Background()
	instance, err := controller.Create(ctx, CreateRequest{JobID: "output-journal", RunID: "output", Generation: 1, WorkspaceDigest: "sha256:w"}, "create-output", testCorrelation)
	if err != nil {
		t.Fatal(err)
	}
	request := ExecRequest{InstanceID: instance.InstanceID, Command: "printf x >> executions; dd if=/dev/zero bs=65536 count=32 2>/dev/null; exit 7", Timeout: 5 * time.Second}
	first, err := controller.Exec(ctx, request, 1, "exec-output", testCorrelation)
	if err != nil {
		t.Fatal(err)
	}
	if first.ExitCode != 7 || first.OutputDigest != expectedTailDigest(make([]byte, loopbackOutputLimit)) {
		t.Fatalf("first=%+v", first)
	}
	// A new Controller reads the terminal journal fact, not another backend Exec.
	replayed, err := NewController(store, backend, fixedNow).Exec(ctx, request, 1, "exec-output", testCorrelation)
	if err != nil || replayed != first {
		t.Fatalf("replayed=%+v error=%v want=%+v", replayed, err, first)
	}
	backend.mu.Lock()
	path := filepath.Join(backend.dirs[instance.InstanceID], "executions")
	backend.mu.Unlock()
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "x" {
		t.Fatalf("execution marker=%q error=%v", data, err)
	}
}
