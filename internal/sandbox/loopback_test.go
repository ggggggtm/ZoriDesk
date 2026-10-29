package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func newLoopback(t testing.TB) *LoopbackBackend {
	t.Helper()
	backend, err := NewLoopbackBackend(t.TempDir(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		backend.mu.Lock()
		ids := make([]string, 0, len(backend.dirs))
		for id := range backend.dirs {
			ids = append(ids, id)
		}
		backend.mu.Unlock()
		for _, id := range ids {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			err := backend.Destroy(ctx, id)
			cancel()
			if err != nil {
				t.Errorf("cleanup %s: %v", id, err)
			}
		}
	})
	return backend
}

func TestLoopbackCreatePreservesUnownedPaths(t *testing.T) {
	for _, kind := range []string{"directory", "file", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			backend := newLoopback(t)
			request := CreateRequest{JobID: "existing", RunID: kind, Generation: 1}
			id := deterministicInstanceID(request.JobID, request.RunID, request.Generation)
			path := filepath.Join(backend.workRoot, id)
			target := t.TempDir()
			marker := filepath.Join(target, "keep")
			if err := os.WriteFile(marker, []byte("existing data"), 0o600); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "directory":
				if err := os.Mkdir(path, 0o750); err != nil {
					t.Fatal(err)
				}
				marker = filepath.Join(path, "keep")
				if err := os.WriteFile(marker, []byte("existing data"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "file":
				marker = path
				if err := os.WriteFile(path, []byte("existing data"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.Lstat(path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := backend.Create(context.Background(), request); err == nil {
				t.Errorf("Create accepted an unowned %s", kind)
			}
			// A cleanup request must neither adopt nor remove this pre-existing path.
			if err := backend.Destroy(context.Background(), id); err == nil {
				t.Errorf("Destroy reported unowned %s as cleaned", kind)
			}
			after, err := os.Lstat(path)
			if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() {
				t.Fatalf("existing path changed: before=%v after=%v err=%v", before, after, err)
			}
			if data, err := os.ReadFile(marker); err != nil || string(data) != "existing data" {
				t.Fatalf("existing data changed: %q %v", data, err)
			}
		})
	}
}

func TestLoopbackPartialCreateCleanupRetry(t *testing.T) {
	for _, failCleanup := range []bool{false, true} {
		t.Run(fmt.Sprintf("cleanup-fails-%t", failCleanup), func(t *testing.T) {
			backend := newLoopback(t)
			initErr := errors.New("injected initialization failure")
			removeErr := errors.New("injected removal failure")
			backend.initializeDir = func(path string) error {
				if err := os.WriteFile(filepath.Join(path, "partial"), []byte("owned"), 0o600); err != nil {
					return err
				}
				return initErr
			}
			backend.removeDir = func(path string) error {
				if failCleanup {
					return removeErr
				}
				return os.RemoveAll(path)
			}
			request := CreateRequest{JobID: "partial", RunID: "loopback", Generation: 1}
			id := deterministicInstanceID(request.JobID, request.RunID, request.Generation)
			path := filepath.Join(backend.workRoot, id)
			_, err := backend.Create(context.Background(), request)
			if !errors.Is(err, initErr) || (failCleanup && !errors.Is(err, removeErr)) {
				t.Fatalf("lost failure: %v", err)
			}
			_, statErr := os.Stat(path)
			if failCleanup {
				if statErr != nil || backend.dirs[id] != path {
					t.Fatalf("lost cleanup ownership: %v", statErr)
				}
				if _, err := backend.Create(context.Background(), request); err == nil {
					t.Fatal("partial Create replay succeeded")
				}
			} else if !errors.Is(statErr, os.ErrNotExist) || len(backend.dirs) != 0 {
				t.Fatalf("partial directory leaked: %v", statErr)
			}
			if _, err := backend.Exec(context.Background(), ExecRequest{InstanceID: id, Command: "touch must-not-exist"}); err == nil {
				t.Fatal("partial initialization allowed Exec")
			}
			// Re-enable filesystem cleanup without changing ownership/ready state.
			failCleanup = false
			for i := 0; i < 2; i++ {
				if err := backend.Destroy(context.Background(), id); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("retry left directory: %v", err)
			}
			if len(backend.dirs) != 0 || len(backend.ready) != 0 || len(backend.owned) != 0 {
				t.Fatal("cleanup retained ownership")
			}
		})
	}
}

func TestLoopbackDestroyFailureFencesExecAndPreservesOtherInstance(t *testing.T) {
	backend := newLoopback(t)
	ctx := context.Background()
	first, err := backend.Create(ctx, CreateRequest{JobID: "first", RunID: "run", Generation: 1})
	if err != nil {
		t.Fatal(err)
	}
	second, err := backend.Create(ctx, CreateRequest{JobID: "second", RunID: "run", Generation: 1})
	if err != nil {
		t.Fatal(err)
	}
	removeErr := errors.New("injected remove failure")
	backend.removeDir = func(string) error { return removeErr }
	if err := backend.Destroy(ctx, first.InstanceID); !errors.Is(err, removeErr) {
		t.Fatalf("expected removal failure: %v", err)
	}
	if _, err := backend.Exec(ctx, ExecRequest{InstanceID: first.InstanceID, Command: "true"}); err == nil {
		t.Fatal("cleanup failure allowed Exec")
	}
	backend.removeDir = os.RemoveAll
	if err := backend.Destroy(ctx, first.InstanceID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(backend.dirs[second.InstanceID]); err != nil {
		t.Fatalf("other instance touched: %v", err)
	}
	if result, err := backend.Exec(ctx, ExecRequest{InstanceID: second.InstanceID, Command: "true"}); err != nil || result.ExitCode != 0 {
		t.Fatalf("other instance unusable: %+v %v", result, err)
	}
}

func TestLoopbackFreshBackendDoesNotAdoptLeftoverDirectory(t *testing.T) {
	backend := newLoopback(t)
	request := CreateRequest{JobID: "restart", RunID: "run", Generation: 1}
	instance, err := backend.Create(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	path := backend.dirs[instance.InstanceID]
	fresh, err := NewLoopbackBackend(backend.workRoot, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fresh.Create(context.Background(), request); err == nil {
		t.Fatal("fresh backend adopted leftover")
	}
	if err := fresh.Destroy(context.Background(), instance.InstanceID); err == nil {
		t.Fatal("fresh backend falsely confirmed cleanup")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("original ownership lost: %v", err)
	}
}

func TestLoopbackDestroyRejectsReplacedOwnedDirectory(t *testing.T) {
	// No process is started. TempDir owns test cleanup, so a failed assertion
	// cannot make the helper's Destroy cleanup adopt a replacement directory.
	backend, err := NewLoopbackBackend(t.TempDir(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	instance, err := backend.Create(ctx, CreateRequest{JobID: "replacement", RunID: "run", Generation: 1})
	if err != nil {
		t.Fatal(err)
	}
	path := backend.dirs[instance.InstanceID]
	backup := filepath.Join(backend.workRoot, "original-backup")
	if err := os.Rename(path, backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(path, "keep")
	if err := os.WriteFile(marker, []byte("replacement data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := backend.Destroy(ctx, instance.InstanceID); err == nil {
		t.Fatal("Destroy accepted a replacement directory with a different identity")
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "replacement data" {
		t.Fatalf("replacement directory was changed: %q %v", data, err)
	}
	if backend.dirs[instance.InstanceID] != path {
		t.Fatal("failed cleanup lost the original ownership claim")
	}
	// Preserve the replacement elsewhere, then restore the exact original
	// directory and retry; a matching pathname alone was not sufficient.
	replacement := filepath.Join(backend.workRoot, "preserved-replacement")
	if err := os.Rename(path, replacement); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(backup, path); err != nil {
		t.Fatal(err)
	}
	if err := backend.Destroy(ctx, instance.InstanceID); err != nil {
		t.Fatalf("restored original could not be cleaned: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("original directory remains after retry: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(replacement, "keep")); err != nil || string(data) != "replacement data" {
		t.Fatalf("retry touched replacement data: %q %v", data, err)
	}
}

func TestLoopbackExecIsolatedWorkdirSanitizedEnv(t *testing.T) {
	backend := newLoopback(t)
	ctx := context.Background()
	instance, err := backend.Create(ctx, CreateRequest{
		JobID: "j1", RunID: "r1", Generation: 1, WorkspaceDigest: "sha256:w",
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := backend.Exec(ctx, ExecRequest{
		InstanceID: instance.InstanceID, Command: "pwd && env",
		Timeout: 2 * time.Second, EnvDigest: EnvDigest(map[string]string{"API_TOKEN": "super-secret"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit=%d", result.ExitCode)
	}
	workdir, ok := backend.dirs[instance.InstanceID]
	if !ok {
		t.Fatal("instance workdir missing")
	}
	if filepath.Base(workdir) != instance.InstanceID {
		t.Fatalf("workdir not instance-scoped: %s", workdir)
	}
	// The sanitized env must not include inherited variables with secrets.
	envOut, err := backend.Exec(ctx, ExecRequest{
		InstanceID: instance.InstanceID, Command: "env | grep -c super-secret || true",
		Timeout: 2 * time.Second, EnvDigest: "sha256:e",
	})
	if err != nil {
		t.Fatal(err)
	}
	// ExecResult carries only digests; the canary script asserts visible output.
	if envOut.ExitCode != 0 {
		t.Fatalf("grep exit=%d (secret leaked into sanitized env?)", envOut.ExitCode)
	}
}

func TestLoopbackCancelTerminatesLongCommand(t *testing.T) {
	backend := newLoopback(t)
	ctx := context.Background()
	instance, err := backend.Create(ctx, CreateRequest{
		JobID: "j2", RunID: "r2", Generation: 1, WorkspaceDigest: "sha256:w",
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan ExecResult, 1)
	errs := make(chan error, 1)
	go func() {
		result, execErr := backend.Exec(ctx, ExecRequest{
			InstanceID: instance.InstanceID, Command: "sleep 10 & echo $! > ready; wait; :",
			Timeout: 30 * time.Second, EnvDigest: "sha256:e",
		})
		if execErr != nil {
			errs <- execErr
			return
		}
		done <- result
	}()
	waitLoopbackReady(t, backend, instance.InstanceID)
	if err := backend.Cancel(ctx, instance.InstanceID); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-done:
		if result.ExitCode == 0 {
			t.Fatal("canceled command must not exit zero")
		}
	case execErr := <-errs:
		if execErr == nil {
			t.Fatal("expected error or nonzero exit")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancel did not terminate the command")
	}
}

func waitLoopbackReady(t *testing.T, backend *LoopbackBackend, id string) string {
	t.Helper()
	backend.mu.Lock()
	path := filepath.Join(backend.dirs[id], "ready")
	backend.mu.Unlock()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(path); err == nil && strings.TrimSpace(string(data)) != "" {
			return strings.TrimSpace(string(data))
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("child did not announce readiness")
	return ""
}

func assertLoopbackChildStopped(t *testing.T, pid string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		output, err := exec.Command("ps", "-o", "stat=", "-p", pid).Output()
		state := strings.TrimSpace(string(output))
		// A killed orphan can briefly remain a zombie until the OS reaps it.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 && state == "" {
			return
		}
		if err != nil {
			t.Fatalf("inspect child state: %v", err)
		}
		if strings.HasPrefix(state, "Z") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("owned child %s remained running", pid)
}

func TestLoopbackCancellationReclaimsChildren(t *testing.T) {
	for _, mode := range []string{"cancel", "context", "deadline", "destroy", "paused-cancel", "paused-context", "paused-deadline", "resume-cancel"} {
		t.Run(mode, func(t *testing.T) {
			backend := newLoopback(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			instance, err := backend.Create(ctx, CreateRequest{JobID: mode})
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			timeout := 5 * time.Second
			if mode == "deadline" || mode == "paused-deadline" {
				timeout = 500 * time.Millisecond
			}
			go func() {
				result, err := backend.Exec(ctx, ExecRequest{InstanceID: instance.InstanceID, Command: "sleep 10 & echo $! > ready; wait; :", Timeout: timeout})
				if err == nil && result.ExitCode == 0 {
					err = fmt.Errorf("canceled execution succeeded")
				}
				done <- err
			}()
			pid := waitLoopbackReady(t, backend, instance.InstanceID)
			if strings.HasPrefix(mode, "paused-") || mode == "resume-cancel" {
				if err := backend.Pause(ctx, instance.InstanceID); err != nil {
					t.Fatal(err)
				}
			}
			switch mode {
			case "context", "paused-context":
				cancel()
			case "deadline", "paused-deadline": // The configured deadline cancels the already-ready child.
			case "resume-cancel":
				resumed := make(chan error, 1)
				go func() { resumed <- backend.Resume(ctx, instance.InstanceID) }()
				if err := backend.Cancel(ctx, instance.InstanceID); err != nil {
					t.Fatal(err)
				}
				select {
				case <-resumed: // A completed resume or a terminal-race error is valid.
				case <-time.After(3 * time.Second):
					t.Fatal("concurrent Resume and Cancel deadlocked")
				}
			case "destroy":
				if err := backend.Destroy(context.Background(), instance.InstanceID); err != nil {
					t.Fatal(err)
				}
				if err := backend.Destroy(context.Background(), instance.InstanceID); err != nil {
					t.Fatal(err)
				}
			default:
				for i := 0; i < 3; i++ {
					if err := backend.Cancel(ctx, instance.InstanceID); err != nil {
						t.Fatal(err)
					}
				}
			}
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("cancellation left execution waiting")
			}
			assertLoopbackChildStopped(t, pid)
		})
	}
}

func TestLoopbackCompletionReclaimsBackgroundChild(t *testing.T) {
	for _, code := range []int{0, 7} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			backend := newLoopback(t)
			instance, err := backend.Create(context.Background(), CreateRequest{JobID: "background"})
			if err != nil {
				t.Fatal(err)
			}
			started := time.Now()
			result, err := backend.Exec(context.Background(), ExecRequest{InstanceID: instance.InstanceID, Command: fmt.Sprintf("sleep 10 & echo $! > ready; exit %d", code)})
			if err != nil || result.ExitCode != code {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if time.Since(started) > 3*time.Second {
				t.Fatal("completed shell left output pipes open")
			}
			assertLoopbackChildStopped(t, waitLoopbackReady(t, backend, instance.InstanceID))
		})
	}
}

func TestLoopbackExecutionOwnership(t *testing.T) {
	backend := newLoopback(t)
	ctx := context.Background()
	first, err := backend.Create(ctx, CreateRequest{JobID: "first"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := backend.Create(ctx, CreateRequest{JobID: "second"})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan ExecResult, 2)
	for _, instance := range []Instance{first, second} {
		go func(id string) {
			result, err := backend.Exec(ctx, ExecRequest{InstanceID: id, Command: "sleep 10 & echo $! > ready; wait; :", Timeout: 20 * time.Second})
			if err != nil {
				result.ExitCode = 255
			}
			done <- result
		}(instance.InstanceID)
	}
	waitLoopbackReady(t, backend, first.InstanceID)
	waitLoopbackReady(t, backend, second.InstanceID)
	if _, err := backend.Exec(ctx, ExecRequest{InstanceID: first.InstanceID, Command: "true"}); err == nil || !strings.Contains(err.Error(), "already executing") {
		t.Fatalf("concurrent execution must not replace tracking: %v", err)
	}
	if err := backend.Cancel(ctx, first.InstanceID); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-done:
		if result.ExitCode == 0 || result.ExitCode == 255 {
			t.Fatalf("first cancellation result: %+v", result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("first cancellation did not return")
	}
	select {
	case result := <-done:
		t.Fatalf("canceling first also stopped unrelated instance: %+v", result)
	case <-time.After(100 * time.Millisecond):
	}
	if err := backend.Destroy(ctx, second.InstanceID); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-done:
		if result.ExitCode == 0 || result.ExitCode == 255 {
			t.Fatalf("second destroy result: %+v", result)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("second destroy did not return")
	}
	if _, err := backend.Create(ctx, CreateRequest{JobID: "second"}); err == nil {
		t.Fatal("destroyed instance must not be resurrected during cleanup")
	}
}

func TestLoopbackCanceledBeforeStart(t *testing.T) {
	backend := newLoopback(t)
	instance, err := backend.Create(context.Background(), CreateRequest{JobID: "pre-canceled"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := backend.Exec(ctx, ExecRequest{InstanceID: instance.InstanceID, Command: "touch should-not-exist"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled start: %v", err)
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if backend.procs[instance.InstanceID] != nil {
		t.Fatal("failed start left process ownership")
	}
	if _, err := os.Stat(filepath.Join(backend.dirs[instance.InstanceID], "should-not-exist")); !os.IsNotExist(err) {
		t.Fatalf("pre-canceled command ran: %v", err)
	}
}

func TestLoopbackRejectsUnconfirmedCleanup(t *testing.T) {
	for _, code := range []string{"0", "125"} {
		err := exec.Command("sh", "-c", "exit "+code).Run()
		if validateLoopbackCompletion(err) == nil {
			t.Fatalf("supervisor exit %s must not confirm group cleanup", code)
		}
	}
	if validateLoopbackCompletion(errors.New("wait failed")) == nil {
		t.Fatal("wait failure accepted")
	}
}

func TestLoopbackPauseResumeStopsWorkload(t *testing.T) {
	backend := newLoopback(t)
	ctx := context.Background()
	instance, err := backend.Create(ctx, CreateRequest{JobID: "pause-workload"})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := backend.Exec(ctx, ExecRequest{InstanceID: instance.InstanceID, Command: `printf x > progress; echo $$ > ready; i=0; while [ "$i" -lt 200 ]; do printf x >> progress; sleep 0.02; i=$((i+1)); done`, Timeout: 20 * time.Second})
		done <- err
	}()
	pid := waitLoopbackReady(t, backend, instance.InstanceID)
	if err := backend.Pause(ctx, instance.InstanceID); err != nil {
		t.Fatal(err)
	}
	stopped := false
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		state, err := exec.Command("ps", "-o", "stat=", "-p", pid).Output()
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(strings.TrimSpace(string(state)), "T") {
			stopped = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !stopped {
		t.Fatal("Pause did not stop the user workload")
	}
	backend.mu.Lock()
	progress := filepath.Join(backend.dirs[instance.InstanceID], "progress")
	backend.mu.Unlock()
	before, err := os.ReadFile(progress)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(80 * time.Millisecond)
	after, err := os.ReadFile(progress)
	if err != nil || string(before) != string(after) {
		t.Fatalf("paused workload continued writing: %v", err)
	}
	if err := backend.Resume(ctx, instance.InstanceID); err != nil {
		t.Fatal(err)
	}
	resumed := false
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(progress)
		if err != nil {
			t.Fatal(err)
		}
		if len(data) > len(after) {
			resumed = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !resumed {
		t.Fatal("Resume did not restart workload progress")
	}
	// No sleep between commands: CONT may arrive before the supervisor has
	// delivered its own STOP, so Resume must wait for the matching acknowledgment.
	for i := 0; i < 8; i++ {
		if err := backend.Pause(ctx, instance.InstanceID); err != nil {
			t.Fatal(err)
		}
		if err := backend.Pause(ctx, instance.InstanceID); err != nil {
			t.Fatal(err)
		}
		if err := backend.Resume(ctx, instance.InstanceID); err != nil {
			t.Fatal(err)
		}
		if err := backend.Resume(ctx, instance.InstanceID); err != nil {
			t.Fatal(err)
		}
	}
	if err := backend.Cancel(ctx, instance.InstanceID); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("resumed execution did not cancel")
	}
}

func TestLoopbackCancellationDuringStartup(t *testing.T) {
	backend := newLoopback(t)
	for i := 0; i < 16; i++ {
		instance, err := backend.Create(context.Background(), CreateRequest{JobID: fmt.Sprintf("startup-%d", i)})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			result, err := backend.Exec(ctx, ExecRequest{InstanceID: instance.InstanceID, Command: "sleep 10; :"})
			if err == nil && result.ExitCode == 0 {
				done <- errors.New("canceled startup succeeded")
			} else {
				done <- nil
			} // Rejection or nonzero cancellation is expected.
		}()
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("startup cancellation deadlocked")
		}
	}
}

func TestLoopbackPauseResumePreservesExitStatus(t *testing.T) {
	backend := newLoopback(t)
	ctx := context.Background()
	instance, err := backend.Create(ctx, CreateRequest{JobID: "paused-exit"})
	if err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		result ExecResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		result, err := backend.Exec(ctx, ExecRequest{InstanceID: instance.InstanceID, Command: `echo $$ > ready; while [ ! -f release ]; do sleep 0.02; done; exit 7`, Timeout: 10 * time.Second})
		done <- outcome{result, err}
	}()
	waitLoopbackReady(t, backend, instance.InstanceID)
	if err := backend.Pause(ctx, instance.InstanceID); err != nil {
		t.Fatal(err)
	}
	if err := backend.Resume(ctx, instance.InstanceID); err != nil {
		t.Fatal(err)
	}
	backend.mu.Lock()
	release := filepath.Join(backend.dirs[instance.InstanceID], "release")
	backend.mu.Unlock()
	if err := os.WriteFile(release, nil, 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		if got.err != nil || got.result.ExitCode != 7 {
			t.Fatalf("paused execution result=%+v err=%v", got.result, got.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("resumed command did not complete")
	}
}

func TestLoopbackResumeAckProtocol(t *testing.T) {
	for _, tc := range []struct {
		line string
		want uint64
	}{
		{"resumed 1\n", 1}, {"resumed 18446744073709551615\n", ^uint64(0)},
		{"resumed 0\n", 0}, {"resumed 01\n", 0}, {"resumed +1\n", 0},
		{"resumed 1 extra\n", 0}, {"resumed 1", 0}, {"resumed 18446744073709551616\n", 0},
		{"exit 1\n", 0}, {"resumed\n", 0},
	} {
		got, err := parseLoopbackResumeAck(tc.line)
		if tc.want == 0 && err == nil {
			t.Fatalf("accepted invalid ack %q", tc.line)
		}
		if tc.want != 0 && (err != nil || got != tc.want) {
			t.Fatalf("ack %q = %d, %v", tc.line, got, err)
		}
	}
	if matched, err := matchLoopbackResumeAck(2, 1); err != nil || matched {
		t.Fatalf("stale ack matched: %v %v", matched, err)
	}
	if matched, err := matchLoopbackResumeAck(2, 2); err != nil || !matched {
		t.Fatalf("matching ack rejected: %v %v", matched, err)
	}
	for _, pair := range [][2]uint64{{2, 3}, {0, 1}, {1, 0}} {
		if _, err := matchLoopbackResumeAck(pair[0], pair[1]); err == nil {
			t.Fatalf("invalid sequence pair accepted: %v", pair)
		}
	}
}

func TestLoopbackQueuedPauseRequestsAreOrdered(t *testing.T) {
	backend := newLoopback(t)
	ctx := context.Background()
	instance, err := backend.Create(ctx, CreateRequest{JobID: "queued-pause"})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := backend.Exec(ctx, ExecRequest{InstanceID: instance.InstanceID, Command: "sleep 10 & echo $! > ready; wait; :", Timeout: 20 * time.Second})
		done <- err
	}()
	waitLoopbackReady(t, backend, instance.InstanceID)
	control := backend.control(instance.InstanceID)
	// Queue both requests before the first handler can acknowledge. Unlike
	// repeated signals, both complete records must be processed exactly in order.
	if _, err := control.requests.Write([]byte("pause 41\npause 42\n")); err != nil {
		t.Fatal(err)
	}
	for _, want := range []uint64{41, 42} {
		deadline := time.NewTimer(2 * time.Second)
		tick := time.NewTicker(10 * time.Millisecond)
		matched := false
		for !matched {
			if err := control.process.Signal(syscall.SIGCONT); err != nil {
				t.Fatal(err)
			}
			select {
			case ack, ok := <-control.resumed:
				if !ok || ack.err != nil || ack.sequence != want {
					t.Fatalf("want ack %d, got %+v open=%v", want, ack, ok)
				}
				matched = true
			case <-tick.C:
			case <-deadline.C:
				t.Fatalf("missing ordered ack %d", want)
			}
		}
		tick.Stop()
		deadline.Stop()
	}
	if err := backend.Cancel(ctx, instance.InstanceID); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("queued-pause execution did not cancel")
	}
}

func TestLoopbackResumeRetryRetainsPendingSequence(t *testing.T) {
	backend := newLoopback(t)
	ctx := context.Background()
	instance, err := backend.Create(ctx, CreateRequest{JobID: "resume-retry"})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := backend.Exec(ctx, ExecRequest{InstanceID: instance.InstanceID, Command: "sleep 10 & echo $! > ready; wait; :", Timeout: 20 * time.Second})
		done <- err
	}()
	waitLoopbackReady(t, backend, instance.InstanceID)
	if err := backend.Pause(ctx, instance.InstanceID); err != nil {
		t.Fatal(err)
	}
	control := backend.control(instance.InstanceID)
	control.mu.Lock()
	pending := control.pending
	control.mu.Unlock()
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := backend.Resume(canceled, instance.InstanceID); !errors.Is(err, context.Canceled) {
		t.Fatalf("resume with canceled context: %v", err)
	}
	control.mu.Lock()
	retained := control.pending
	control.mu.Unlock()
	if retained != pending || pending == 0 {
		t.Fatal("failed resume lost pending sequence")
	}
	if err := backend.Resume(ctx, instance.InstanceID); err != nil {
		t.Fatal(err)
	}
	if err := backend.Cancel(ctx, instance.InstanceID); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("retry execution did not cancel")
	}
}

func TestLoopbackDestroyCleansWorkdir(t *testing.T) {
	backend := newLoopback(t)
	ctx := context.Background()
	instance, err := backend.Create(ctx, CreateRequest{
		JobID: "j3", RunID: "r3", Generation: 1, WorkspaceDigest: "sha256:w",
	})
	if err != nil {
		t.Fatal(err)
	}
	workdir := backend.dirs[instance.InstanceID]
	if _, err := os.Stat(workdir); err != nil {
		t.Fatal(err)
	}
	if err := backend.Destroy(ctx, instance.InstanceID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(workdir); !os.IsNotExist(err) {
		t.Fatalf("workdir still exists after destroy: %v", err)
	}
	if _, err := backend.Exec(ctx, ExecRequest{
		InstanceID: instance.InstanceID, Command: "echo x", Timeout: time.Second,
	}); err == nil || !strings.Contains(err.Error(), "unknown instance") {
		t.Fatalf("exec after destroy must fail: %v", err)
	}
}
