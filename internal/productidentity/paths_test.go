package productidentity

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPathsMatchSharedDesktopFixture(t *testing.T) {
	body, err := os.ReadFile("../../testdata/product-identity.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture map[string]string
	if err := json.Unmarshal(body, &fixture); err != nil {
		t.Fatal(err)
	}
	paths, err := ForHome(fixture["home"])
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{"product_id": ProductID, "bundle_id": BundleID,
		"service_label": ServiceLabel, "build_id": BuildID,
		"root": paths.Root, "socket": paths.Socket, "state": paths.State,
		"vault": paths.Vault, "key": paths.Key, "diagnostics": paths.Diagnostics}
	for name, value := range got {
		if value != fixture[name] {
			t.Errorf("%s: got %q want %q", name, value, fixture[name])
		}
	}
}

func TestPathsRejectUnsafeAndOverlongHomes(t *testing.T) {
	for _, home := range []string{"", "/", ".", "/Users/x/../y", "/Users/x/", "/Users/\x00x", "/" + strings.Repeat("界", 40)} {
		t.Run(home, func(t *testing.T) {
			if _, err := ForHome(home); err == nil {
				t.Fatal("unsafe home accepted")
			}
		})
	}
	for _, home := range []string{"/Users/fixture", "/Users/space user", "/Users/用户"} {
		if _, err := ForHome(home); err != nil {
			t.Fatalf("valid home rejected: %v", err)
		}
	}
}

func TestPathsDoNotCreateOrInspectLegacyRoot(t *testing.T) {
	home, err := os.MkdirTemp("/tmp", "zd-path-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(home)
	home, err = filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatal(err)
	}
	paths, err := ForHome(home)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(paths.Root); !os.IsNotExist(err) {
		t.Fatal("path resolution wrote to disk")
	}
	if !strings.HasPrefix(paths.Socket, paths.Root+string(filepath.Separator)) {
		t.Fatal("socket escaped root")
	}
}
