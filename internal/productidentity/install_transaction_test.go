package productidentity

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestInstallerTransactionRetainsBackupWhenRestoreIsBlocked(t *testing.T) {
	root := t.TempDir()
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	library, err := filepath.Abs("../../scripts/lib/desktop-install-transaction.sh")
	if err != nil {
		t.Fatal(err)
	}
	// The actual shared transaction runs on fake directories; no app is signed or launched.
	script := `set -eu
. "$1"
destination="$2/ZoriDesk.app"
previous="$destination.previous"
transaction="$2/.ZoriDesk.app.transaction"
incoming="$transaction/incoming"
mkdir -m 700 "$transaction" "$transaction/current"
printf 'original-v1' > "$transaction/current/fixture-version"
chmod 600 "$transaction/current/fixture-version"
printf 'unrelated-replacement' > "$destination"
chmod 600 "$destination"
if [ "$(uname -s)" = Darwin ]; then
  identity=$(/usr/bin/stat -f '%d:%i' "$transaction/current")
else
  identity=$(/usr/bin/stat -c '%d:%i' "$transaction/current")
fi
printf 'zoridesk-app-transaction-v1 install 1 %s absent absent active\n' "$identity" > "$transaction/receipt"
chmod 600 "$transaction/receipt"
validate_bundle() { [ -d "$1" ] && [ -f "$1/fixture-version" ]; }
transaction_mode=install
had_current=1
had_previous=0
new_installed=1
transaction_owned=1
cleanup
`
	command := exec.Command("/bin/sh", "-c", script, "fixture", library, root)
	output, runErr := command.CombinedOutput()
	saved, readErr := os.ReadFile(filepath.Join(root, ".ZoriDesk.app.transaction/current/fixture-version"))
	if runErr == nil || readErr != nil || string(saved) != "original-v1" {
		t.Fatalf("failed restoration discarded its backup: status=%v read=%v saved=%q output=%q", runErr, readErr, saved, output)
	}
	foreign, err := os.ReadFile(filepath.Join(root, "ZoriDesk.app"))
	if err != nil || string(foreign) != "unrelated-replacement" {
		t.Fatal("blocked restoration overwrote an unrelated replacement")
	}
	if strings.Contains(string(output), root) {
		t.Fatal("recovery error disclosed fixture paths")
	}
}

// The signature verifier is deliberately a fixture. File transactions, identities,
// traps, locks and recovery below are the exact library used by the real installer.
const installerFixtureRunner = `set -eu
. "$1"
destination="$2/Applications/ZoriDesk.app"
source_app="$2/source/ZoriDesk.app"
operation=$3
fault=${4:-}
validate_bundle() {
  [ -d "$1" ] && [ ! -L "$1" ] && [ -f "$1/fixture-version" ] || return 1
  [ -z "$(find "$1" -type l -print -quit)" ] || return 1
  if [ "${1##*/}" = incoming ] && [ "$fault" = copy_validation ]; then return 1; fi
  version=$(cat "$1/fixture-version")
  if [ "$1" = "$destination" ] && [ "$version" = v3 ]; then
    case "$fault" in
      kill_install) kill -KILL "$$" ;;
      block_restore)
        /bin/rm -rf "$destination"
        printf unrelated > "$destination"
        return 1
        ;;
    esac
  fi
  if [ "$operation" = rollback ] && [ "$fault" = kill_rollback ] &&
    [ "$1" = "$destination" ] && [ "$version" = v0 ]; then kill -KILL "$$"; fi
}
desktop_configure
case "$operation" in
  install) desktop_install ;;
  rollback) desktop_rollback ;;
  recover) desktop_recover ;;
  committed_crash) desktop_install; kill -KILL "$$" ;;
  hold)
    desktop_begin install
    printf 'ready\n'
    read -r release
    exit 9
    ;;
  *) exit 2 ;;
esac
`

type installFixture struct {
	root, library, destination, previous, transaction string
}

func newInstallFixture(t *testing.T, existing bool) installFixture {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	library, err := filepath.Abs("../../scripts/lib/desktop-install-transaction.sh")
	if err != nil {
		t.Fatal(err)
	}
	f := installFixture{root: root, library: library,
		destination: filepath.Join(root, "Applications/ZoriDesk.app"),
		previous:    filepath.Join(root, "Applications/ZoriDesk.app.previous"),
		transaction: filepath.Join(root, "Applications/.ZoriDesk.app.transaction")}
	for _, path := range []string{"Applications/Loom.app", "Library/Application Support/Loom",
		"Library/Application Support/ZoriDesk", "source/ZoriDesk.app"} {
		writeInstallFixture(t, filepath.Join(root, path), "keep", "unchanged")
	}
	writeInstallFixture(t, filepath.Join(root, "source/ZoriDesk.app"), "fixture-version", "v3")
	if existing {
		writeInstallFixture(t, f.destination, "fixture-version", "v1")
		writeInstallFixture(t, f.previous, "fixture-version", "v0")
	}
	return f
}

func writeInstallFixture(t *testing.T, directory, name, body string) {
	t.Helper()
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, name), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func (f installFixture) command(operation, fault string) *exec.Cmd {
	command := exec.Command("/bin/sh", "-c", installerFixtureRunner, "fixture", f.library, f.root, operation, fault)
	command.Env = append(os.Environ(), "LOOM_APP_INSTALL_TEST_FAIL_STEP=", "LOOM_APP_INSTALL_TEST_SIGNAL_STEP=")
	return command
}

func (f installFixture) run(t *testing.T, operation, fault string, success bool) {
	t.Helper()
	output, err := f.command(operation, fault).CombinedOutput()
	if (err == nil) != success {
		t.Fatalf("%s/%s success=%v error=%v output=%q", operation, fault, success, err, output)
	}
}

func assertInstallVersion(t *testing.T, directory, want string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(directory, "fixture-version"))
	if err != nil || string(data) != want {
		t.Fatalf("version: got %q err=%v want %q", data, err, want)
	}
}

func installTreeSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	result := make(map[string]string)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		value := fmt.Sprintf("%o", info.Mode())
		if info.Mode().IsRegular() {
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value += fmt.Sprintf(":%x", sha256.Sum256(body))
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			value += ":" + target
		}
		result[relative] = value
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func (f installFixture) assertUntouched(t *testing.T) {
	t.Helper()
	for _, path := range []string{"Applications/Loom.app", "Library/Application Support/Loom",
		"Library/Application Support/ZoriDesk"} {
		body, err := os.ReadFile(filepath.Join(f.root, path, "keep"))
		entries, listErr := os.ReadDir(filepath.Join(f.root, path))
		if err != nil || listErr != nil || string(body) != "unchanged" || len(entries) != 1 {
			t.Fatalf("unrelated app/data changed: %s", path)
		}
		directoryInfo, directoryErr := os.Stat(filepath.Join(f.root, path))
		fileInfo, fileErr := os.Stat(filepath.Join(f.root, path, "keep"))
		if directoryErr != nil || fileErr != nil || directoryInfo.Mode().Perm() != 0700 || fileInfo.Mode().Perm() != 0600 {
			t.Fatal("unrelated app/data permissions changed")
		}
	}
}

func TestInstallerTransactionCleanUpgradeRepeatAndRollback(t *testing.T) {
	f := newInstallFixture(t, false)
	f.run(t, "install", "", true)
	assertInstallVersion(t, f.destination, "v3")
	if _, err := os.Lstat(f.previous); !os.IsNotExist(err) {
		t.Fatal("clean installation invented a previous version")
	}
	writeInstallFixture(t, filepath.Join(f.root, "source/ZoriDesk.app"), "fixture-version", "v4")
	f.run(t, "install", "", true)
	assertInstallVersion(t, f.destination, "v4")
	assertInstallVersion(t, f.previous, "v3")
	f.run(t, "install", "", true)
	assertInstallVersion(t, f.destination, "v4")
	f.run(t, "rollback", "", true)
	assertInstallVersion(t, f.destination, "v4")
	f.run(t, "install", "", true)
	writeInstallFixture(t, filepath.Join(f.root, "source/ZoriDesk.app"), "fixture-version", "v5")
	f.run(t, "install", "", true)
	f.run(t, "rollback", "", true)
	assertInstallVersion(t, f.destination, "v4")
	assertInstallVersion(t, f.previous, "v5")
	f.assertUntouched(t)
	if _, err := os.Lstat(f.transaction); !os.IsNotExist(err) {
		t.Fatal("successful transaction was not removed")
	}
}

func TestInstallerTransactionFailuresRestoreExactOriginals(t *testing.T) {
	for _, fault := range []string{"copy_validation", "after_swap", "signal_after_swap"} {
		t.Run(fault, func(t *testing.T) {
			f := newInstallFixture(t, true)
			before := installTreeSnapshot(t, f.root)
			command := f.command("install", fault)
			if fault == "after_swap" {
				command.Env = append(command.Env, "LOOM_APP_INSTALL_TEST_FAIL_STEP=after_swap")
			}
			if fault == "signal_after_swap" {
				command.Env = append(command.Env, "LOOM_APP_INSTALL_TEST_SIGNAL_STEP=after_swap")
			}
			if output, err := command.CombinedOutput(); err == nil {
				t.Fatalf("injected failure succeeded: %q", output)
			}
			if !reflect.DeepEqual(before, installTreeSnapshot(t, f.root)) {
				t.Fatal("failed transaction did not restore exact bytes and modes")
			}
			f.assertUntouched(t)
		})
	}
}

func TestInstallerTransactionKilledOwnerCanBeExplicitlyRecovered(t *testing.T) {
	for _, operation := range []string{"install", "rollback", "committed_crash"} {
		t.Run(operation, func(t *testing.T) {
			f := newInstallFixture(t, true)
			before := installTreeSnapshot(t, f.root)
			fault := "kill_" + operation
			f.run(t, operation, fault, false)
			info, err := os.Stat(filepath.Join(f.transaction, "receipt"))
			if err != nil || info.Mode().Perm() != 0600 {
				t.Fatal("missing private recovery receipt")
			}
			crashed := installTreeSnapshot(t, f.root)
			f.run(t, "install", "", false)
			if !reflect.DeepEqual(crashed, installTreeSnapshot(t, f.root)) {
				t.Fatal("another installation changed an incomplete transaction")
			}
			f.run(t, "recover", "", true)
			if operation == "committed_crash" {
				assertInstallVersion(t, f.destination, "v3")
				assertInstallVersion(t, f.previous, "v1")
			} else if !reflect.DeepEqual(before, installTreeSnapshot(t, f.root)) {
				t.Fatal("recovery did not restore exact originals")
			}
			f.assertUntouched(t)
			f.run(t, "recover", "", false)
		})
	}
}

func TestInstallerTransactionBlockedRestoreCanBeRetried(t *testing.T) {
	f := newInstallFixture(t, true)
	before := installTreeSnapshot(t, f.root)
	f.run(t, "install", "block_restore", false)
	assertInstallVersion(t, filepath.Join(f.transaction, "current"), "v1")
	assertInstallVersion(t, filepath.Join(f.transaction, "previous"), "v0")
	blocked := installTreeSnapshot(t, f.root)
	f.run(t, "recover", "", false)
	if !reflect.DeepEqual(blocked, installTreeSnapshot(t, f.root)) {
		t.Fatal("recovery overwrote an unrelated replacement or discarded originals")
	}
	if err := os.Remove(f.destination); err != nil {
		t.Fatal(err)
	}
	f.run(t, "recover", "", true)
	if !reflect.DeepEqual(before, installTreeSnapshot(t, f.root)) {
		t.Fatal("retry did not restore original app and previous app")
	}
	f.assertUntouched(t)
}

func TestInstallerTransactionRejectsConcurrentOwnerAndRecovery(t *testing.T) {
	f := newInstallFixture(t, true)
	before := installTreeSnapshot(t, f.root)
	owner := f.command("hold", "")
	stdout, err := owner.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := owner.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	owner.Stderr = &stderr
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if owner.ProcessState == nil {
			_ = owner.Process.Kill()
			_ = owner.Wait()
		}
		_ = stdin.Close()
	})
	ready := make(chan bool, 1)
	go func() { scanner := bufio.NewScanner(stdout); ready <- scanner.Scan() && scanner.Text() == "ready" }()
	select {
	case ok := <-ready:
		if !ok {
			t.Fatal("transaction owner not ready")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("transaction owner did not start")
	}
	locked := installTreeSnapshot(t, f.root)
	f.run(t, "install", "", false)
	f.run(t, "recover", "", false)
	if !reflect.DeepEqual(locked, installTreeSnapshot(t, f.root)) {
		t.Fatal("competing installer/recovery changed a live transaction")
	}
	_, _ = stdin.Write([]byte("release\n"))
	_ = stdin.Close()
	if err := owner.Wait(); err == nil {
		t.Fatal("fixture owner exit was lost")
	}
	if !reflect.DeepEqual(before, installTreeSnapshot(t, f.root)) {
		t.Fatal("owner cleanup did not restore its inputs")
	}
	f.run(t, "install", "", true)
	f.assertUntouched(t)
}

func TestInstallerTransactionRejectsInvalidRecoveryWithoutWrites(t *testing.T) {
	for _, mutation := range []string{"version", "phase", "pid", "extra_line", "unknown_identity",
		"public_replacement", "missing_backup", "receipt_permissions", "receipt_symlink", "receipt_hardlink", "unknown_entry", "stale_recovery"} {
		t.Run(mutation, func(t *testing.T) {
			f := newInstallFixture(t, true)
			f.run(t, "install", "kill_install", false)
			receipt := filepath.Join(f.transaction, "receipt")
			body, err := os.ReadFile(receipt)
			if err != nil {
				t.Fatal(err)
			}
			fields := strings.Fields(string(body))
			switch mutation {
			case "version":
				fields[0] = "unknown-v2"
			case "phase":
				fields[6] = "anything"
			case "pid":
				fields[2] = "999999999999999999999"
			case "unknown_identity":
				fields[3] = "../../not-an-identity"
			case "extra_line":
				fields = append(fields, "\nextra")
			case "receipt_permissions":
				if err := os.Chmod(receipt, 0644); err != nil {
					t.Fatal(err)
				}
			case "receipt_symlink":
				outside := filepath.Join(f.root, "outside-receipt")
				if err := os.Rename(receipt, outside); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, receipt); err != nil {
					t.Fatal(err)
				}
			case "receipt_hardlink":
				if err := os.Link(receipt, filepath.Join(f.root, "outside-receipt")); err != nil {
					t.Fatal(err)
				}
			case "unknown_entry":
				writeInstallFixture(t, f.transaction, "foreign-file", "preserve")
			case "stale_recovery":
				if err := os.Mkdir(filepath.Join(f.transaction, "recovering"), 0700); err != nil {
					t.Fatal(err)
				}
			case "public_replacement":
				if err := os.Rename(f.destination, filepath.Join(f.root, "saved-new-app")); err != nil {
					t.Fatal(err)
				}
				writeInstallFixture(t, f.destination, "fixture-version", "foreign-but-valid")
			case "missing_backup":
				if err := os.Rename(filepath.Join(f.transaction, "current"), filepath.Join(f.root, "preserved-outside")); err != nil {
					t.Fatal(err)
				}
			}
			if mutation == "version" || mutation == "phase" || mutation == "pid" || mutation == "unknown_identity" || mutation == "extra_line" {
				if err := os.WriteFile(receipt, []byte(strings.Join(fields, " ")+"\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			before := installTreeSnapshot(t, f.root)
			f.run(t, "recover", "", false)
			if !reflect.DeepEqual(before, installTreeSnapshot(t, f.root)) {
				t.Fatal("invalid recovery input caused writes")
			}
			if mutation == "missing_backup" {
				assertInstallVersion(t, filepath.Join(f.root, "preserved-outside"), "v1")
			} else {
				assertInstallVersion(t, filepath.Join(f.transaction, "current"), "v1")
			}
			assertInstallVersion(t, filepath.Join(f.transaction, "previous"), "v0")
			f.assertUntouched(t)
		})
	}
}

func TestInstallerTransactionRejectsUnsafeTargetsAndLegacyResidue(t *testing.T) {
	for _, mutation := range []string{"destination_symlink", "previous_symlink", "group_writable_parent", "legacy_transaction", "legacy_incoming"} {
		t.Run(mutation, func(t *testing.T) {
			f := newInstallFixture(t, false)
			switch mutation {
			case "destination_symlink":
				if err := os.Symlink(filepath.Join(f.root, "Applications/Loom.app"), f.destination); err != nil {
					t.Fatal(err)
				}
			case "previous_symlink":
				if err := os.Symlink(filepath.Join(f.root, "Applications/Loom.app"), f.previous); err != nil {
					t.Fatal(err)
				}
			case "group_writable_parent":
				if err := os.Chmod(filepath.Dir(f.destination), 0770); err != nil {
					t.Fatal(err)
				}
			case "legacy_transaction":
				writeInstallFixture(t, f.transaction+".1234", "keep", "legacy")
			case "legacy_incoming":
				writeInstallFixture(t, filepath.Join(filepath.Dir(f.destination), ".ZoriDesk.app.incoming.1234"), "keep", "legacy")
			}
			before := installTreeSnapshot(t, f.root)
			f.run(t, "install", "", false)
			if !reflect.DeepEqual(before, installTreeSnapshot(t, f.root)) {
				t.Fatal("unsafe installation target changed files")
			}
			f.assertUntouched(t)
		})
	}
}
