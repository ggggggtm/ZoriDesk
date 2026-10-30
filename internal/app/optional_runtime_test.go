package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"loom-pi-rebuild/internal/journal"
	loomruntime "loom-pi-rebuild/internal/runtime"
	"loom-pi-rebuild/internal/runtime/piadapter"
	"loom-pi-rebuild/internal/state"
)

func q11DisabledObserverConfig(t *testing.T) LocalRuntimeObservationDaemonConfig {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	isolation := filepath.Join(root, "isolation")
	if err := os.Mkdir(isolation, 0o700); err != nil {
		t.Fatal(err)
	}
	return LocalRuntimeObservationDaemonConfig{
		DisablePi: true, StatePath: filepath.Join(root, "state.db"), IsolationRoot: isolation,
		ObservationInterval: time.Second, ProcessTimeout: time.Second, MaxCycles: 1,
	}
}

func TestQ11DisabledObserverPreservesIndependentRuntime(t *testing.T) {
	config := q11DisabledObserverConfig(t)
	daemon, err := NewLocalRuntimeObservationDaemon(config, newDaemonTestClock(), &daemonTestIdentitySource{})
	if err != nil {
		t.Fatal(err)
	}
	defer daemon.Close()
	instance := appDiscoveryInstance("runtime.native", "Independent native backend")
	instance.AdapterType = "loom-native-agent"
	snapshot, err := loomruntime.DiscoverRuntime(context.Background(), []loomruntime.RuntimeProbe{&appDiscoveryProbe{
		id: "probe.native", observations: []loomruntime.RuntimeObservation{{Instance: instance}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.CommitRuntimeDiscoverySnapshot(context.Background(), journal.NewStore(daemon.db), snapshot, appDiscoveryCommitInput(snapshot, "q11")); err != nil {
		t.Fatal(err)
	}
	result, err := daemon.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.CompletedCycles != 1 || result.NoWriteCycles != 1 || result.DiscoveryEvents != 0 || result.StatusEvents != 0 {
		t.Fatalf("disabled observer wrote facts: %#v", result)
	}
	if len(result.RuntimeFacts) != 1 || result.RuntimeFacts[0].RuntimeInstanceID != "runtime.native" || result.RuntimeFacts[0].Status != "online" {
		t.Fatalf("independent runtime changed: %#v", result.RuntimeFacts)
	}
	if err := daemon.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewLocalRuntimeObservationDaemon(config, newDaemonTestClock(), &daemonTestIdentitySource{})
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	result, err = restarted.Run(context.Background())
	if err != nil || result.NoWriteCycles != 1 || len(result.RuntimeFacts) != 1 || result.RuntimeFacts[0].Status != "online" {
		t.Fatalf("restart result=%#v err=%v", result, err)
	}
}

func TestQ11DisabledObserverRejectsMixedOrUnsafeConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*LocalRuntimeObservationDaemonConfig)
	}{
		{"probe", func(c *LocalRuntimeObservationDaemonConfig) { c.ProbeID = "p" }},
		{"instance", func(c *LocalRuntimeObservationDaemonConfig) { c.RuntimeInstanceID = "i" }},
		{"device", func(c *LocalRuntimeObservationDaemonConfig) { c.DeviceID = "d" }},
		{"display", func(c *LocalRuntimeObservationDaemonConfig) { c.DisplayName = "Pi" }},
		{"search", func(c *LocalRuntimeObservationDaemonConfig) { c.RuntimeSearchPaths = []string{c.IsolationRoot} }},
		{"catalog", func(c *LocalRuntimeObservationDaemonConfig) {
			c.LocalModelCatalog = &piadapter.PiLocalModelCatalogConfig{}
		}},
		{"relative state", func(c *LocalRuntimeObservationDaemonConfig) { c.StatePath = "relative.db" }},
		{"missing isolation", func(c *LocalRuntimeObservationDaemonConfig) { c.IsolationRoot += "-absent" }},
		{"invalid interval", func(c *LocalRuntimeObservationDaemonConfig) { c.ObservationInterval = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := q11DisabledObserverConfig(t)
			tc.mutate(&config)
			daemon, err := NewLocalRuntimeObservationDaemon(config, newDaemonTestClock(), &daemonTestIdentitySource{})
			if err == nil {
				if daemon != nil {
					_ = daemon.Close()
				}
				t.Fatal("invalid configuration accepted")
			}
		})
	}
}
