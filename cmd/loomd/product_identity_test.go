package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"loom-pi-rebuild/internal/app"
	"loom-pi-rebuild/internal/localipc"
	"loom-pi-rebuild/internal/productidentity"
)

func productIdentityHome(t *testing.T) string {
	t.Helper()
	home, err := os.MkdirTemp("/tmp", "zd-")
	if err != nil {
		t.Fatal(err)
	}
	home, err = filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	return home
}

func TestZoriDeskInvocationReexecsIsolatedCanonicalArgs(t *testing.T) {
	home := productIdentityHome(t)
	root := filepath.Join(home, "Library/Application Support/ZoriDesk")
	args := []string{"--state", filepath.Join(root, "state/loom.db"),
		"--isolation-root", filepath.Join(root, "isolation"),
		"--socket", filepath.Join(root, "run/loomd.sock"), "--disable-pi",
		"--interval", "10s", "--process-timeout", "10s", "--product-id", "zoridesk"}
	plan, err := prepareLocalAppInvocation(
		[]string{"--zoridesk-app-service", "--parent-pid", "4242"},
		func() ([]string, error) { return args, nil },
	)
	if err != nil || !plan.Reexec {
		t.Fatalf("isolated app invocation was not expanded: reexec=%v err=%v", plan.Reexec, err)
	}
	if !slices.Equal(plan.Args, append(args, managedLocalAppParentFlag, "4242")) {
		t.Fatal("isolated canonical args or managed parent changed")
	}
}

func TestZoriDeskExpansionDoesNotDiscoverLegacyCredentials(t *testing.T) {
	home := productIdentityHome(t)
	legacy := filepath.Join(home, ".cc-switch")
	if err := os.Mkdir(legacy, 0700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(legacy, "cc-switch.db")
	if err := os.WriteFile(sentinel, []byte("fixture-not-a-credential"), 0600); err != nil {
		t.Fatal(err)
	}
	args, err := expandLocalAppServiceArgs([]string{"--zoridesk-app-service"}, home, nil, "", "")
	if err != nil {
		t.Fatalf("isolated expansion unavailable: %v", err)
	}
	for _, expected := range []string{
		filepath.Join(home, "Library/Application Support/ZoriDesk/state/loom.db"),
		filepath.Join(home, "Library/Application Support/ZoriDesk/run/loomd.sock"),
		"--product-id", "zoridesk",
	} {
		if !slices.Contains(args, expected) {
			t.Fatalf("isolated argument absent: %q", expected)
		}
	}
	if slices.Contains(args, "--credential-import-source") || strings.Contains(strings.Join(args, "\n"), "/Loom/") {
		t.Fatal("isolated app discovered a legacy source")
	}
	data, err := os.ReadFile(sentinel)
	if err != nil || string(data) != "fixture-not-a-credential" {
		t.Fatal("legacy sentinel changed")
	}
}

func TestZoriDeskInstallerRejectsLegacyTargetBeforeAccess(t *testing.T) {
	home := productIdentityHome(t)
	legacy := filepath.Join(home, "Loom.app")
	if err := os.Mkdir(legacy, 0700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(legacy, "keep")
	if err := os.WriteFile(sentinel, []byte("legacy"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("/bin/sh", "../../scripts/install-loom-local-app.sh",
		"--rollback", "--destination", legacy)
	output, err := cmd.CombinedOutput()
	if err == nil || !bytes.Contains(output, []byte("usage:")) {
		t.Fatalf("legacy target was not refused before validation: %q", output)
	}
	data, err := os.ReadFile(sentinel)
	if err != nil || string(data) != "legacy" {
		t.Fatal("legacy installation changed")
	}
	for _, entry := range []struct{ script, option string }{
		{"install-loom-local-app.sh", "--destination"},
		{"build-loom-local-app.sh", "--output"},
	} {
		t.Run(entry.script, func(t *testing.T) {
			args := []string{"../../scripts/" + entry.script, entry.option, filepath.Join(legacy, "ZoriDesk.app")}
			output, err := exec.Command("/bin/sh", args...).CombinedOutput()
			if err == nil || !bytes.Contains(output, []byte("inside an application bundle")) {
				t.Fatalf("nested legacy bundle target accepted: %q", output)
			}
		})
	}
	inside := filepath.Join(legacy, "Contents")
	if err := os.Mkdir(inside, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(home, "alias")
	if err := os.Symlink(legacy, alias); err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct{ script, option string }{
		{"install-loom-local-app.sh", "--destination"},
		{"build-loom-local-app.sh", "--output"},
	} {
		output, err := exec.Command("/bin/sh", "../../scripts/"+entry.script, entry.option,
			filepath.Join(alias, "Contents/ZoriDesk.app")).CombinedOutput()
		if err == nil || !bytes.Contains(output, []byte("non-canonical")) {
			t.Fatalf("aliased application target accepted: %q", output)
		}
	}
}

func TestZoriDeskRunRejectsMixedPathsAndImports(t *testing.T) {
	home := productIdentityHome(t)
	args, err := expandLocalAppServiceArgs([]string{zorideskAppServiceFlag}, home, nil, "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range [][2]string{
		{"--state", filepath.Join(home, "Library/Application Support/Loom/state/loom.db")},
		{"--socket", filepath.Join(home, "Library/Application Support/Loom/run/loomd.sock")},
		{"--isolation-root", filepath.Join(home, "Library/Application Support/Loom/isolation")},
		{"--product-id", "loom"},
		{"--credential-import-source", filepath.Join(home, "old.db")},
	} {
		t.Run(change[0], func(t *testing.T) {
			modified := slices.Clone(args)
			i := slices.Index(modified, change[0])
			if i < 0 {
				modified = append(modified, change[0], change[1])
			} else {
				modified[i+1] = change[1]
			}
			code := run(context.Background(), modified, &bytes.Buffer{}, &bytes.Buffer{}, func(daemonBuildConfig) (daemonRunner, error) {
				t.Fatal("mixed product arguments reached the builder")
				return nil, nil
			})
			if code != exitInvalidInput {
				t.Fatalf("mixed path accepted: %d", code)
			}
		})
	}
}

func TestZoriDeskDaemonKeepsLegacyFilesAndServesNewIdentity(t *testing.T) {
	t.Setenv("LOOM_ENABLE_WEB_TOOLS", "")
	t.Setenv("LOOM_MCP_STDIO_CONFIG", "")
	home := productIdentityHome(t)
	legacyRoot := filepath.Join(home, "Library/Application Support/Loom")
	if err := os.MkdirAll(legacyRoot, 0700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(legacyRoot, "keep")
	if err := os.WriteFile(sentinel, []byte("legacy-unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	// Any accidental legacy activation is confined to this fixture and detectable.
	activation := filepath.Join(home, "activation")
	t.Setenv("LOOM_PRODUCTION_SANDBOX_ROOT", activation)
	args, err := localAppServiceArgsForHomeAndProduct(home,
		func(string) (string, error) { return "", exec.ErrNotFound }, &bytes.Buffer{}, zorideskAppServiceFlag)
	if err != nil {
		t.Fatal(err)
	}
	var config daemonBuildConfig
	code := run(context.Background(), args, &bytes.Buffer{}, &bytes.Buffer{}, func(value daemonBuildConfig) (daemonRunner, error) {
		config = value
		return &fakeDaemonRunner{}, nil
	})
	if code != exitSuccess || config.ProductID != productidentity.ProductID {
		t.Fatal("product identity not carried to builder")
	}
	config.Observer.ObservationInterval = time.Hour
	runner, err := productionDaemonBuilder(config)
	if err != nil {
		t.Fatalf("isolated production builder: %v", err)
	}
	delegate := runner
	if wrapped, ok := runner.(*productMCPStdioDaemonRunner); ok {
		delegate = wrapped.delegate
	}
	product, ok := delegate.(*productDaemonRunner)
	if !ok {
		_ = runner.Close()
		t.Fatal("product runner unavailable")
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
			t.Error("isolated daemon did not stop")
		}
		if err := runner.Close(); err != nil {
			t.Error(err)
		}
	})
	waitForProductSocket(t, product, config.SocketPath)
	client, err := localipc.NewClient(localipc.ClientConfig{SocketPath: config.SocketPath, Timeout: 3 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	ping, err := client.Ping(context.Background())
	if err != nil || !ping.Available || ping.BuildID != productidentity.BuildID {
		t.Fatalf("wrong product ping: %#v %v", ping, err)
	}
	var setup app.SetupSnapshot
	if err := client.Call(context.Background(), "setup_snapshot", struct{}{}, &setup); err != nil {
		t.Fatal(err)
	}
	if setup.Runtimes == nil {
		t.Fatal("native setup unavailable")
	}
	paths, err := productidentity.ForHome(home)
	if err != nil {
		t.Fatal(err)
	}
	vault, key, _, err := productCredentialVaultPaths(paths.State)
	if err != nil || vault != paths.Vault || key != paths.Key {
		t.Fatal("Vault escaped isolated root")
	}
	if _, err := os.Stat(activation); !os.IsNotExist(err) {
		t.Fatal("legacy activation paths were touched")
	}
	data, err := os.ReadFile(sentinel)
	if err != nil || string(data) != "legacy-unchanged" {
		t.Fatal("legacy data changed")
	}
}

func TestZoriDeskManagedProductRefusesLegacyActivation(t *testing.T) {
	route := isolatedProductProductionRoute{}
	if _, err := route.ProductionSnapshot(context.Background(), app.ProductionSnapshotRequest{}); !errors.Is(err, app.ErrProductionUnavailable) {
		t.Fatal("legacy activation snapshot exposed")
	}
	if _, err := route.ProductionCommand(context.Background(), app.ProductionCommandRequest{Action: "confirm", OperationID: "fixture"}); !errors.Is(err, app.ErrProductionUnavailable) {
		t.Fatal("legacy activation command exposed")
	}
	if route.Degraded(context.Background()) {
		t.Fatal("disabled legacy activation degraded unrelated routes")
	}
}

func TestZoriDeskBootstrapRefusesLegacyDirectoryAlias(t *testing.T) {
	home := productIdentityHome(t)
	legacy := filepath.Join(home, "Library/Application Support/Loom")
	if err := os.MkdirAll(legacy, 0700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(legacy, "keep")
	if err := os.WriteFile(sentinel, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(filepath.Dir(legacy), "ZoriDesk")
	if err := os.Symlink(legacy, alias); err != nil {
		t.Fatal(err)
	}
	_, err := localAppServiceArgsForHomeAndProduct(home,
		func(string) (string, error) { return "", exec.ErrNotFound }, &bytes.Buffer{}, zorideskAppServiceFlag)
	if err == nil {
		t.Fatal("legacy root alias accepted")
	}
	data, err := os.ReadFile(sentinel)
	if err != nil || string(data) != "keep" {
		t.Fatal("legacy root changed")
	}
	entries, err := os.ReadDir(legacy)
	if err != nil || len(entries) != 1 {
		t.Fatal("bootstrap created files in the legacy root")
	}
}
