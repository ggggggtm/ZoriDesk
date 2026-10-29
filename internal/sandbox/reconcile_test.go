package sandbox

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"loom-pi-rebuild/internal/journal"

	_ "modernc.org/sqlite"
)

const testCorrelation = "22222222-2222-4222-8222-222222222222"

func openSandboxJournal(t testing.TB) *journal.Store {
	t.Helper()
	values := url.Values{}
	values.Add("_pragma", "foreign_keys(1)")
	values.Add("_pragma", "busy_timeout(5000)")
	values.Add("_pragma", "journal_mode(WAL)")
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s/sandbox.db?%s", t.TempDir(), values.Encode()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := journal.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return journal.NewStore(db)
}

type failingBackend struct {
	matchingBackend
	failExec     bool
	destroyError bool
}

// matchingBackend returns the deterministic controller instance ID so the
// strict Create identity check can pass (the generic recordingBackend returns
// its own invented ID and is only used for backend-interface tests).
type matchingBackend struct {
	recordingBackend
}

func (backend *matchingBackend) Create(ctx context.Context, request CreateRequest) (Instance, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	backend.created = append(backend.created, request)
	return Instance{InstanceID: deterministicInstanceID(request.JobID, request.RunID, request.Generation)}, nil
}

func (backend *failingBackend) Exec(ctx context.Context, request ExecRequest) (ExecResult, error) {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	backend.execs = append(backend.execs, request)
	if backend.failExec {
		return ExecResult{}, errors.New("backend exec exploded")
	}
	return ExecResult{ExitCode: 0, OutputDigest: "sha256:out"}, nil
}

func (backend *failingBackend) Destroy(ctx context.Context, instanceID string) error {
	backend.mu.Lock()
	defer backend.mu.Unlock()
	backend.destroys = append(backend.destroys, instanceID)
	if backend.destroyError {
		return errors.New("backend destroy exploded")
	}
	return nil
}

func fixedNow() time.Time { return time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC) }

func newTestController(t testing.TB, backend SandboxBackend) (*Controller, *journal.Store) {
	t.Helper()
	store := openSandboxJournal(t)
	return NewController(store, backend, fixedNow), store
}

func streamFacts(t testing.TB, store *journal.Store, instanceID string) []journal.Event {
	t.Helper()
	events, err := store.ReadStream(context.Background(), streamPrefix+instanceID)
	if err != nil {
		t.Fatal(err)
	}
	return events
}

// createFailureBackend models resources left behind when Create fails. Cleanup
// success, not the Create return value, is the evidence that releases ownership.
type createFailureBackend struct {
	matchingBackend
	createErr    error
	cleanupErr   error
	returnedID   string
	remaining    bool
	cancelCreate context.CancelFunc
}

func (b *createFailureBackend) Create(ctx context.Context, request CreateRequest) (Instance, error) {
	b.created = append(b.created, request)
	b.remaining = true
	if b.cancelCreate != nil {
		b.cancelCreate()
	}
	id := b.returnedID
	if id == "" {
		id = deterministicInstanceID(request.JobID, request.RunID, request.Generation)
	}
	return Instance{InstanceID: id}, b.createErr
}

func (b *createFailureBackend) Destroy(ctx context.Context, id string) error {
	b.destroys = append(b.destroys, id)
	if b.cleanupErr != nil {
		return b.cleanupErr
	}
	b.remaining = false
	return nil
}

func TestCreateFailureRequiresConfirmedCleanup(t *testing.T) {
	for _, failCleanup := range []bool{false, true} {
		t.Run(fmt.Sprintf("cleanup-fails-%t", failCleanup), func(t *testing.T) {
			createErr := errors.New("injected create failure")
			cleanupErr := errors.New("injected cleanup failure")
			backend := &createFailureBackend{createErr: createErr}
			if failCleanup {
				backend.cleanupErr = cleanupErr
			}
			controller, store := newTestController(t, backend)
			request := CreateRequest{JobID: "partial", RunID: "run", Generation: 1}
			id := deterministicInstanceID(request.JobID, request.RunID, request.Generation)
			_, err := controller.Create(context.Background(), request, "create", testCorrelation)
			if !errors.Is(err, createErr) {
				t.Fatalf("lost create cause: %v", err)
			}
			if failCleanup && !errors.Is(err, cleanupErr) {
				t.Errorf("lost cleanup cause: %v", err)
			}
			if len(backend.destroys) != 1 || backend.destroys[0] != id {
				t.Errorf("cleanup did not target requested ID: %v", backend.destroys)
			}
			states, err := controller.Rebuild(context.Background())
			if err != nil || len(states) != 1 {
				t.Fatalf("rebuild: %+v %v", states, err)
			}
			if states[0].Destroyed == failCleanup || backend.remaining != failCleanup {
				t.Errorf("cleanup and journal disagree: %+v remaining=%t", states[0], backend.remaining)
			}
			// A fresh controller must retain the failed-creation fence from facts.
			restarted := NewController(store, backend, fixedNow)
			if _, err := restarted.Exec(context.Background(), ExecRequest{InstanceID: id, Command: "must-not-run"}, 1, "exec", testCorrelation); err == nil {
				t.Error("failed create allowed Exec")
			}
			if len(backend.execs) != 0 {
				t.Fatal("failed create reached backend Exec")
			}
			if _, err := restarted.Create(context.Background(), request, "create-again", testCorrelation); err == nil {
				t.Error("failed/destroyed generation was recreated")
			}
			if len(backend.created) != 1 {
				t.Errorf("Create replay reached backend: %d", len(backend.created))
			}
			backend.cleanupErr = nil
			results, err := restarted.Reconcile(context.Background())
			if err != nil || len(results) != 1 || !results[0].Destroyed || results[0].Err != nil || backend.remaining {
				t.Fatalf("cleanup recovery: %+v %v remaining=%t", results, err, backend.remaining)
			}
			calls := len(backend.destroys)
			if _, err := restarted.Reconcile(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(backend.destroys) != calls {
				t.Fatal("completed cleanup repeated backend work")
			}
			count := 0
			for _, event := range streamFacts(t, store, id) {
				if event.Type == EventDestroyed {
					count++
				}
			}
			if count != 1 {
				t.Fatalf("destroyed facts=%d", count)
			}
		})
	}
}

func TestCreateIdentityMismatchNeverCleansReturnedID(t *testing.T) {
	backend := &createFailureBackend{returnedID: "belongs-to-another-instance", cleanupErr: errors.New("retry later")}
	controller, _ := newTestController(t, backend)
	request := CreateRequest{JobID: "mismatch", RunID: "run", Generation: 1}
	id := deterministicInstanceID(request.JobID, request.RunID, request.Generation)
	if _, err := controller.Create(context.Background(), request, "create", testCorrelation); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected identity error: %v", err)
	}
	if len(backend.destroys) != 1 || backend.destroys[0] != id {
		t.Fatalf("unsafe cleanup IDs: %v", backend.destroys)
	}
	states, err := controller.Rebuild(context.Background())
	if err != nil || len(states) != 1 || states[0].Destroyed {
		t.Fatalf("unconfirmed cleanup: %+v %v", states, err)
	}
}

func TestCreateFailureJournalErrorIsReturned(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cause := errors.New("injected create failure")
	backend := &createFailureBackend{createErr: cause, cancelCreate: cancel}
	controller, store := newTestController(t, backend)
	request := CreateRequest{JobID: "journal-fail", RunID: "run", Generation: 1}
	id := deterministicInstanceID(request.JobID, request.RunID, request.Generation)
	_, err := controller.Create(ctx, request, "create", testCorrelation)
	if !errors.Is(err, cause) || !errors.Is(err, context.Canceled) {
		t.Fatalf("lost create/journal error: %v", err)
	}
	// This fake permits cleanup despite cancellation. That is not evidence that
	// the canceled journal write succeeded; reconciliation must retry the fact.
	if backend.remaining {
		t.Fatal("cleanup was not attempted")
	}
	for _, event := range streamFacts(t, store, id) {
		if event.Type == EventDestroyed {
			t.Fatal("canceled write marked destroyed")
		}
	}
	restarted := NewController(store, backend, fixedNow)
	results, err := restarted.Reconcile(context.Background())
	if err != nil || len(results) != 1 || !results[0].Destroyed || results[0].Err != nil {
		t.Fatalf("fact recovery: %+v %v", results, err)
	}
}

func TestCreateFailureFenceIsSticky(t *testing.T) {
	backend := &createFailureBackend{createErr: errors.New("partial create"), cleanupErr: errors.New("retry cleanup")}
	controller, store := newTestController(t, backend)
	request := CreateRequest{JobID: "sticky", RunID: "run", Generation: 1}
	id := deterministicInstanceID(request.JobID, request.RunID, request.Generation)
	if _, err := controller.Create(context.Background(), request, "create", testCorrelation); err == nil {
		t.Fatal("expected failure")
	}
	for _, op := range []func(context.Context, string, string, string) error{controller.Pause, controller.Resume, controller.Cancel} {
		if err := op(context.Background(), id, "must-fail", testCorrelation); !errors.Is(err, ErrInstanceNotReady) {
			t.Fatalf("failed create allowed lifecycle op: %v", err)
		}
	}
	// Even an older/in-flight lifecycle fact must not erase the failure fence.
	if err := controller.recordInstanceEvent(context.Background(), EventPaused, id, "late-fact", "late", testCorrelation); err != nil {
		t.Fatal(err)
	}
	restarted := NewController(store, backend, fixedNow)
	states, err := restarted.Rebuild(context.Background())
	if err != nil || len(states) != 1 || !states[0].CreationFailed {
		t.Fatalf("Rebuild lost sticky fence: %+v %v", states, err)
	}
	if _, err := restarted.Exec(context.Background(), ExecRequest{InstanceID: id, Command: "must-not-run"}, 1, "exec", testCorrelation); !errors.Is(err, ErrInstanceNotReady) {
		t.Fatalf("single-instance projection lost fence: %v", err)
	}
	if len(backend.execs) != 0 || len(backend.pauses) != 0 || len(backend.resumes) != 0 || len(backend.cancels) != 0 {
		t.Fatal("failed create reached backend operation")
	}
}

// TestRedSB2_AllFactsAtomicallyLanded covers RED 2: Create/Exec/Cancel/Pause/
// Resume/Destroy each produce their state fact on sandbox/<instance_id>.
func TestRedSB2_AllFactsAtomicallyLanded(t *testing.T) {
	backend := &matchingBackend{}
	controller, store := newTestController(t, backend)
	ctx := context.Background()

	instance, err := controller.Create(ctx, CreateRequest{
		JobID: "j1", RunID: "r1", Generation: 1, WorkspaceDigest: "sha256:w1",
	}, "op-create-1", testCorrelation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Exec(ctx, ExecRequest{
		InstanceID: instance.InstanceID, Command: "go test ./...",
		Timeout: time.Second, EnvDigest: EnvDigest(map[string]string{"LANG": "C"}),
	}, 1, "op-exec-1", testCorrelation); err != nil {
		t.Fatal(err)
	}
	if err := controller.Pause(ctx, instance.InstanceID, "op-pause-1", testCorrelation); err != nil {
		t.Fatal(err)
	}
	if err := controller.Resume(ctx, instance.InstanceID, "op-resume-1", testCorrelation); err != nil {
		t.Fatal(err)
	}
	if err := controller.Cancel(ctx, instance.InstanceID, "op-cancel-1", testCorrelation); err != nil {
		t.Fatal(err)
	}
	if err := controller.Destroy(ctx, instance.InstanceID, "op-destroy-1", testCorrelation); err != nil {
		t.Fatal(err)
	}

	facts := streamFacts(t, store, instance.InstanceID)
	types := make([]string, 0, len(facts))
	for _, fact := range facts {
		types = append(types, fact.Type)
	}
	want := []string{
		EventCreated, EventExecReq, EventExecDone,
		EventPaused, EventResumed, EventCancelled, EventDestroyed,
	}
	if strings.Join(types, ",") != strings.Join(want, ",") {
		t.Fatalf("facts = %v want %v", types, want)
	}
	if len(backend.created) != 1 || len(backend.execs) != 1 ||
		len(backend.pauses) != 1 || len(backend.resumes) != 1 ||
		len(backend.cancels) != 1 || len(backend.destroys) != 1 {
		t.Fatalf("backend ops not recorded: %+v", backend)
	}
}

// TestRedSB3_ExecFailureAndCancelLandsFacts covers RED 3: failed and cancelled
// executions record their terminal fact and never re-execute on replay.
func TestRedSB3_ExecFailureAndCancelLandsFacts(t *testing.T) {
	backend := &failingBackend{}
	controller, store := newTestController(t, backend)
	ctx := context.Background()
	instance, err := controller.Create(ctx, CreateRequest{
		JobID: "j1", RunID: "r1", Generation: 2, WorkspaceDigest: "sha256:w1",
	}, "op-create-2", testCorrelation)
	if err != nil {
		t.Fatal(err)
	}
	backend.failExec = true
	if _, err := controller.Exec(ctx, ExecRequest{
		InstanceID: instance.InstanceID, Command: "boom", Timeout: time.Second,
	}, 2, "op-exec-fail", testCorrelation); err == nil {
		t.Fatal("expected exec failure")
	}
	backend.failExec = false
	if _, err := controller.Exec(ctx, ExecRequest{
		InstanceID: instance.InstanceID, Command: "boom", Timeout: time.Second,
	}, 2, "op-exec-fail", testCorrelation); err == nil || !strings.Contains(err.Error(), "exploded") {
		t.Fatalf("idempotent replay must return the same failure without re-executing: %v", err)
	}
	if len(backend.execs) != 1 {
		t.Fatalf("failed exec re-ran on replay: execs=%d", len(backend.execs))
	}
	facts := streamFacts(t, store, instance.InstanceID)
	var types []string
	for _, fact := range facts {
		types = append(types, fact.Type)
	}
	if strings.Join(types, ",") != strings.Join(
		[]string{EventCreated, EventExecReq, EventExecFail}, ",") {
		t.Fatalf("failure facts = %v", types)
	}

	if err := controller.Cancel(ctx, instance.InstanceID, "op-cancel-2", testCorrelation); err != nil {
		t.Fatal(err)
	}
	types = nil
	for _, fact := range streamFacts(t, store, instance.InstanceID) {
		types = append(types, fact.Type)
	}
	if !containsType(types, EventCancelled) {
		t.Fatalf("cancel fact missing: %v", types)
	}
}

// TestRedSB4_StaleGenerationRejected covers RED 4: operating on an instance
// from an older generation is rejected and never touches the backend.
func TestRedSB4_StaleGenerationRejected(t *testing.T) {
	backend := &matchingBackend{}
	controller, _ := newTestController(t, backend)
	ctx := context.Background()
	instance, err := controller.Create(ctx, CreateRequest{
		JobID: "j1", RunID: "r1", Generation: 5, WorkspaceDigest: "sha256:w1",
	}, "op-create-3", testCorrelation)
	if err != nil {
		t.Fatal(err)
	}
	before := len(backend.execs)
	if _, err := controller.Exec(ctx, ExecRequest{
		InstanceID: instance.InstanceID, Command: "stale", Timeout: time.Second,
	}, 4, "op-exec-stale", testCorrelation); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("expected stale generation error, got %v", err)
	}
	if len(backend.execs) != before {
		t.Fatalf("stale op must not touch backend: %+v", backend)
	}
}

// TestRedSB5_JournalRebuildMatchesBackend covers RED 5: the Journal-derived
// projection is the authority and reconstructs instance state exactly.
func TestRedSB5_JournalRebuildMatchesBackend(t *testing.T) {
	backend := &matchingBackend{}
	controller, _ := newTestController(t, backend)
	ctx := context.Background()
	instance, err := controller.Create(ctx, CreateRequest{
		JobID: "j1", RunID: "r1", Generation: 3, WorkspaceDigest: "sha256:w1",
	}, "op-create-4", testCorrelation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Exec(ctx, ExecRequest{
		InstanceID: instance.InstanceID, Command: "test", Timeout: time.Second,
	}, 3, "op-exec-4", testCorrelation); err != nil {
		t.Fatal(err)
	}
	// A fresh controller over the same Journal rebuilds without re-executing.
	rebuilt, err := NewController(controller.store, &recordingBackend{}, fixedNow).Rebuild(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rebuilt) != 1 {
		t.Fatalf("rebuild instances = %+v", rebuilt)
	}
	state := rebuilt[0]
	if state.InstanceID != instance.InstanceID || state.JobID != "j1" ||
		state.RunID != "r1" || state.Generation != 3 ||
		state.WorkspaceDigest != "sha256:w1" || state.State != EventExecDone ||
		state.ExecCount != 1 || state.Destroyed {
		t.Fatalf("rebuilt state mismatch: %+v", state)
	}
}

// TestRedSB6_ReconcileCleansUndestroyedWithoutReexec covers RED 6: after
// restart/replay, undestroyed instances are marked and cleaned, never re-run.
func TestRedSB6_ReconcileCleansUndestroyedWithoutReexec(t *testing.T) {
	backend := &failingBackend{}
	controller, _ := newTestController(t, backend)
	ctx := context.Background()
	instance, err := controller.Create(ctx, CreateRequest{
		JobID: "j1", RunID: "r1", Generation: 1, WorkspaceDigest: "sha256:w1",
	}, "op-create-5", testCorrelation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Exec(ctx, ExecRequest{
		InstanceID: instance.InstanceID, Command: "test", Timeout: time.Second,
	}, 1, "op-exec-5", testCorrelation); err != nil {
		t.Fatal(err)
	}
	execsBefore := len(backend.execs)

	// Simulate a backend that still hosts the instance; reconcile destroys it.
	results, err := controller.Reconcile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || !results[0].Destroyed || results[0].Err != nil {
		t.Fatalf("reconcile results = %+v", results)
	}
	if len(backend.destroys) != 1 {
		t.Fatalf("reconcile did not destroy backend instance: %+v", backend)
	}
	if len(backend.execs) != execsBefore {
		t.Fatalf("reconcile must never re-execute: execs=%d", len(backend.execs))
	}
	state, err := controller.Rebuild(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !state[0].Destroyed {
		t.Fatalf("reconcile did not mark instance destroyed: %+v", state[0])
	}

	// A second reconcile is a no-op.
	results, err = controller.Reconcile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || !results[0].Destroyed {
		t.Fatalf("second reconcile should be no-op: %+v", results)
	}
	if len(backend.destroys) != 1 {
		t.Fatalf("reconcile re-destroyed instance: %+v", backend)
	}
}

// TestRedSB7_EnvDigestLeaksNoRawSecrets covers RED 7: the digest carries no
// raw environment values and is order-independent.
func TestRedSB7_EnvDigestLeaksNoRawSecrets(t *testing.T) {
	secret := "super-secret-token-42"
	env := map[string]string{"API_TOKEN": secret, "LANG": "C"}
	digest := EnvDigest(env)
	if strings.Contains(digest, secret) {
		t.Fatalf("digest leaks raw secret: %s", digest)
	}
	if digest == "" || !strings.HasPrefix(digest, "sha256:") {
		t.Fatalf("digest malformed: %s", digest)
	}
	if EnvDigest(map[string]string{"LANG": "C", "API_TOKEN": secret}) != digest {
		t.Fatal("digest must be order-independent")
	}
	if EnvDigest(map[string]string{"API_TOKEN": secret, "LANG": "en"}) == digest {
		t.Fatal("digest must change with values")
	}
}

// TestRedSB8_TwoJobsIsolated covers RED 8: instances bind to their own
// generation/workspace and never share state.
func TestRedSB8_TwoJobsIsolated(t *testing.T) {
	backend := &matchingBackend{}
	controller, store := newTestController(t, backend)
	ctx := context.Background()
	first, err := controller.Create(ctx, CreateRequest{
		JobID: "job-a", RunID: "run-a", Generation: 1, WorkspaceDigest: "sha256:wa",
	}, "op-create-a", testCorrelation)
	if err != nil {
		t.Fatal(err)
	}
	second, err := controller.Create(ctx, CreateRequest{
		JobID: "job-b", RunID: "run-b", Generation: 9, WorkspaceDigest: "sha256:wb",
	}, "op-create-b", testCorrelation)
	if err != nil {
		t.Fatal(err)
	}
	if first.InstanceID == second.InstanceID {
		t.Fatalf("instances must be distinct: %s", first.InstanceID)
	}
	if _, err := controller.Exec(ctx, ExecRequest{
		InstanceID: first.InstanceID, Command: "a", Timeout: time.Second,
	}, 1, "op-exec-a", testCorrelation); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Exec(ctx, ExecRequest{
		InstanceID: second.InstanceID, Command: "b", Timeout: time.Second,
	}, 9, "op-exec-b", testCorrelation); err != nil {
		t.Fatal(err)
	}
	states, err := controller.Rebuild(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 2 {
		t.Fatalf("rebuild states = %+v", states)
	}
	if states[0].JobID == states[1].JobID || states[0].WorkspaceDigest == states[1].WorkspaceDigest {
		t.Fatalf("instances not isolated: %+v", states)
	}
	factsA := len(streamFacts(t, store, first.InstanceID))
	factsB := len(streamFacts(t, store, second.InstanceID))
	if factsA != 3 || factsB != 3 {
		t.Fatalf("facts per stream: a=%d b=%d", factsA, factsB)
	}
}

func containsType(types []string, want string) bool {
	for _, value := range types {
		if value == want {
			return true
		}
	}
	return false
}

var _ sync.Locker = (*sync.Mutex)(nil) // keep sync import for recordingBackend composition
