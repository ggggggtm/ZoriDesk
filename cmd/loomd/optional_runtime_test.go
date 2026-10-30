package main

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"loom-pi-rebuild/internal/api"
	"loom-pi-rebuild/internal/app"
	"loom-pi-rebuild/internal/journal"
	"loom-pi-rebuild/internal/localipc"
	"loom-pi-rebuild/internal/projection"
	loomruntime "loom-pi-rebuild/internal/runtime"
	"loom-pi-rebuild/internal/runtime/piadapter"
	"loom-pi-rebuild/internal/supervisor"
)

func TestQ11LocalAppOptionalRuntimeMatrix(t *testing.T) {
	for _, tc := range []struct {
		name             string
		pi, node, broken bool
		wantDisabled     bool
	}{
		{"neither installed", false, false, false, true},
		{"node only", false, true, false, true},
		{"pi without node", true, false, false, true},
		{"invalid pi executable", true, true, true, true},
		{"both installed", true, true, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := q11CanonicalTempDir(t)
			bin := filepath.Join(home, "tools")
			if err := os.Mkdir(bin, 0o700); err != nil {
				t.Fatal(err)
			}
			codex := filepath.Join(bin, "codex")
			if err := os.WriteFile(codex, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			if tc.node {
				if err := os.WriteFile(filepath.Join(bin, "node"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if tc.pi {
				piDir := filepath.Join(home, "Library/Application Support/Loom/runtimes/pi/0.82.1/node_modules/.bin")
				if err := os.MkdirAll(piDir, 0o700); err != nil {
					t.Fatal(err)
				}
				mode := os.FileMode(0o700)
				if tc.broken {
					mode = 0o600
				}
				if err := os.WriteFile(filepath.Join(piDir, "pi"), []byte("#!/bin/sh\nexit 0\n"), mode); err != nil {
					t.Fatal(err)
				}
			}
			lookup := func(name string) (string, error) {
				candidate := filepath.Join(bin, name)
				if _, err := os.Stat(candidate); err != nil {
					return "", exec.ErrNotFound
				}
				return candidate, nil
			}
			var warnings bytes.Buffer
			args, err := localAppServiceArgsForHome(home, lookup, &warnings)
			if err != nil {
				t.Fatal(err)
			}
			if slices.Contains(args, "--disable-pi") != tc.wantDisabled {
				t.Fatalf("args=%q", args)
			}
			if !slices.Contains(args, codex) {
				t.Fatalf("independent Codex lost: %q", args)
			}
			if tc.wantDisabled && (slices.Contains(args, "--runtime-dir") || slices.Contains(args, "--instance-id")) {
				t.Fatalf("fabricated Pi binding: %q", args)
			}
			if (warnings.Len() > 0) != tc.wantDisabled || strings.Contains(warnings.String(), home) {
				t.Fatalf("warning absent or leaked local path: %q", warnings.String())
			}
			var out, diagnostic bytes.Buffer
			if code := run(context.Background(), args, &out, &diagnostic, func(config daemonBuildConfig) (daemonRunner, error) {
				if config.Observer.DisablePi != tc.wantDisabled || config.CodexExecutable != codex {
					t.Fatalf("config=%#v", config)
				}
				return &fakeDaemonRunner{}, nil
			}); code != 0 {
				t.Fatalf("generated arguments rejected: %d %s", code, &diagnostic)
			}
		})
	}
}

func TestQ11OptionalPiDoesNotBypassSharedStorageValidation(t *testing.T) {
	home := q11CanonicalTempDir(t)
	appSupport := filepath.Join(home, "Library/Application Support/Loom")
	if err := os.MkdirAll(appSupport, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appSupport, "state"), []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := localAppServiceArgsForHome(home, func(string) (string, error) { return "", exec.ErrNotFound }, &bytes.Buffer{})
	if err == nil {
		t.Fatal("unsafe shared state accepted")
	}
	if content, err := os.ReadFile(filepath.Join(appSupport, "state")); err != nil || string(content) != "preserve" {
		t.Fatal("existing shared state file changed")
	}
}

func TestQ11DisabledPiRejectsConflictingExplicitFlags(t *testing.T) {
	base := []string{"--state", "/tmp/q11/state.db", "--isolation-root", "/tmp/q11/isolation", "--disable-pi", "--interval", "1s", "--process-timeout", "1s"}
	for _, extra := range [][]string{
		{"--runtime-dir", "/tmp/pi"}, {"--probe-id", "p"}, {"--instance-id", "i"},
		{"--device-id", "d"}, {"--display-name", "Pi"},
		{"--local-model-private-root", "/tmp/model", "--local-model-executable", "/tmp/model/bin", "--local-model-path", "/tmp/model/data"},
	} {
		var out, diagnostic bytes.Buffer
		code := run(context.Background(), append(slices.Clone(base), extra...), &out, &diagnostic, func(daemonBuildConfig) (daemonRunner, error) {
			t.Fatal("conflicting configuration reached builder")
			return nil, nil
		})
		if code != exitInvalidInput {
			t.Fatalf("accepted conflicting flags %q: %d", extra, code)
		}
	}
}

func TestQ11DisabledPiKeepsNativeExecutionConfig(t *testing.T) {
	t.Setenv("LOOM_ENABLE_WEB_TOOLS", "")
	config := missionExecutionConfigFromDaemonBuild(daemonBuildConfig{
		SocketPath: "/tmp/q11/service.sock", Observer: app.LocalRuntimeObservationDaemonConfig{DisablePi: true},
	})
	if config == nil || config.LocalModelCatalog != nil || config.RuntimeInstanceID != "" || len(config.RuntimeSearchPaths) != 0 {
		t.Fatalf("native execution removed or fabricated Pi: %#v", config)
	}
	if config.RemoteToolBroker == nil || !config.RemoteToolBroker.EnrollmentOnly {
		t.Fatal("disabled Pi removed the policy-gated native/MCP transport")
	}
}

func TestQ11ProductionDaemonWithoutPiServesSetup(t *testing.T) {
	t.Setenv("LOOM_ENABLE_WEB_TOOLS", "")
	t.Setenv("LOOM_MCP_STDIO_CONFIG", "")
	root, statePath := productDaemonFailureState(t)
	isolation := filepath.Join(root, "isolation")
	if err := os.Mkdir(isolation, 0o700); err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(root, "loomd.sock")
	runner, err := productionDaemonBuilder(daemonBuildConfig{
		Observer: app.LocalRuntimeObservationDaemonConfig{
			DisablePi: true, StatePath: statePath, IsolationRoot: isolation,
			ObservationInterval: time.Hour, ProcessTimeout: time.Second,
		}, SocketPath: socket,
	})
	if err != nil {
		t.Fatalf("production builder: %v, cause: %v, leaf: %v", err, errors.Unwrap(err), errors.Unwrap(errors.Unwrap(err)))
	}
	delegate := runner
	if wrapped, ok := runner.(*productMCPStdioDaemonRunner); ok {
		delegate = wrapped.delegate
	}
	product, ok := delegate.(*productDaemonRunner)
	if !ok {
		_ = runner.Close()
		t.Fatalf("runner=%T", runner)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := runner.Run(ctx); done <- err }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("daemon did not stop")
		}
		if err := runner.Close(); err != nil {
			t.Error(err)
		}
	})
	waitForProductSocket(t, product, socket)
	client, err := localipc.NewClient(localipc.ClientConfig{SocketPath: socket, Timeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	var snapshot app.SetupSnapshot
	if err := client.Call(context.Background(), "setup_snapshot", struct{}{}, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Runtimes == nil {
		t.Fatal("setup snapshot unavailable without Pi")
	}
}

func TestQ11DisabledPiCatalogFiltersHistoryWithoutChangingFacts(t *testing.T) {
	_, statePath := productDaemonFailureState(t)
	db, err := sql.Open("sqlite", statePath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	appendProductExecutionRuntimeFixture(t, db)
	view := projection.New(db)
	store := journal.NewStore(db)
	if err := ensureProductVerifiedNativeAgentRuntimes(context.Background(), store, view, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	before, err := store.ReadAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range []productSetupCatalogSource{{}, {disablePi: true}} {
		catalog, err := source.CatalogForView(context.Background(), view.GlobalReadView())
		if err != nil {
			t.Fatal(err)
		}
		var piFound, nativeFound bool
		for _, observation := range catalog.RuntimeDiscovery.Observations() {
			piFound = piFound || observation.Instance.AdapterType == "pi-cli"
			nativeFound = nativeFound || observation.Instance.ID == productNativeAgentRuntimeInstanceID
		}
		if piFound == source.disablePi || !nativeFound {
			t.Fatalf("catalog pi=%v native=%v disabled=%v", piFound, nativeFound, source.disablePi)
		}
	}
	after, err := store.ReadAll(context.Background())
	if err != nil || len(before) != len(after) {
		t.Fatal("availability filter wrote journal")
	}
	pi, ok := view.GlobalReadView().RuntimeInstance("runtime-1")
	if !ok || pi.Status != "online" {
		t.Fatal("historical Pi fact changed")
	}
	// Selection of a stale saved Pi binding cannot fall back to a different
	// backend. There is deliberately no executable adapter for Pi in this mode.
	executor := &productMissionExecutor{supervisors: map[string]*supervisor.Supervisor{
		productMissionAdapterKey("loom-native", "runtime.native"): {},
	}}
	if _, err := executor.Execute(context.Background(), supervisor.ExecuteInput{
		Profile:  loomruntime.RuntimeProfile{AdapterType: "pi-cli"},
		Instance: loomruntime.RuntimeInstance{ID: "runtime-1"},
	}); !errors.Is(err, app.ErrInvalidMissionExecution) {
		t.Fatalf("stale Pi selected fallback: %v", err)
	}
}

func TestQ11LocalAppRejectsSymlinkParent(t *testing.T) {
	home := q11CanonicalTempDir(t)
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(home, "Library")); err != nil {
		t.Fatal(err)
	}
	if _, err := localAppServiceArgsForHome(home, func(string) (string, error) { return "", exec.ErrNotFound }, &bytes.Buffer{}); err == nil {
		t.Fatal("symlinked shared parent accepted")
	}
	entries, err := os.ReadDir(target)
	if err != nil || len(entries) != 0 {
		t.Fatal("unsafe parent target changed")
	}
}

func q11CanonicalTempDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestQ11MalformedPiMetadataIsScopedAndMixedFailuresRemainFatal(t *testing.T) {
	if containableProductObserverTimeout(testPiMetadataFailure{command: loomruntime.PiMetadataVersion, cause: loomruntime.ErrDuplicatePiRuntimeModel}) {
		t.Error("version probe cannot return duplicate models")
	}
	for _, cause := range []error{loomruntime.ErrInvalidPiMetadataOutput, loomruntime.ErrPiMetadataStderr, loomruntime.ErrDuplicatePiRuntimeModel} {
		failure := testPiMetadataFailure{command: loomruntime.PiMetadataListModels, cause: cause}
		if !containableProductObserverTimeout(failure) {
			t.Errorf("optional malformed Pi metadata stopped entire product: %v", cause)
		}
		for _, other := range []error{journal.ErrPartialEventBatchConflict, piadapter.ErrPiMetadataBindingChanged, errors.New("unknown failure"), context.Canceled} {
			if containableProductObserverTimeout(errors.Join(failure, other)) {
				t.Errorf("contained unsafe mixed failure: %v", other)
			}
		}
		if containableProductObserverTimeout(cause) {
			t.Error("untyped error was contained")
		}
	}
}

func TestQ11MalformedPiMetadataKeepsProductIPCServing(t *testing.T) {
	t.Setenv("LOOM_ENABLE_WEB_TOOLS", "")
	root, statePath := productDaemonFailureState(t)
	isolation := filepath.Join(root, "isolation")
	runtimeDir := filepath.Join(root, "runtime")
	for _, directory := range []string{isolation, runtimeDir} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	// A real process -> real metadata parser, without Node, accounts or models.
	if err := os.WriteFile(filepath.Join(runtimeDir, "pi"), []byte("#!/bin/sh\nprintf 'not a valid version with spaces\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	socket := filepath.Join(root, "loomd.sock")
	runner, err := productionDaemonBuilder(daemonBuildConfig{
		Observer: app.LocalRuntimeObservationDaemonConfig{
			StatePath: statePath, IsolationRoot: isolation, RuntimeSearchPaths: []string{runtimeDir},
			ProbeID: "q11.pi", RuntimeInstanceID: "runtime.pi.q11", DeviceID: "device.q11", DisplayName: "Malformed fixture",
			ObservationInterval: time.Second, ProcessTimeout: time.Second,
		}, SocketPath: socket,
	})
	if err != nil {
		t.Fatalf("build reason=%s cause=%v", daemonBuildFailureReason(err), errors.Unwrap(err))
	}
	product, ok := runner.(*productDaemonRunner)
	if !ok {
		_ = runner.Close()
		t.Fatalf("runner=%T", runner)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := runner.Run(ctx); done <- err }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("daemon did not stop")
		}
		if err := runner.Close(); err != nil {
			t.Error(err)
		}
	})
	waitForProductSocket(t, product, socket)
	client, err := localipc.NewClient(localipc.ClientConfig{SocketPath: socket, Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		var snapshot api.LocalProductSnapshot
		if err := client.Call(context.Background(), "snapshot", api.LocalProductSnapshotRequest{Limit: 64}, &snapshot); err != nil {
			t.Fatal(err)
		}
		if snapshot.Partial && snapshot.Reason == "observer_version_output" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("missing partial diagnostic: %#v", snapshot)
		}
		time.Sleep(10 * time.Millisecond)
	}
	var setup app.SetupSnapshot
	if err := client.Call(context.Background(), "setup_snapshot", struct{}{}, &setup); err != nil {
		t.Fatal(err)
	}
	if setup.SavedTeams == nil {
		t.Fatal("setup unavailable after malformed Pi metadata")
	}
}
