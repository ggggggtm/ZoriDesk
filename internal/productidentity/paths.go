// Package productidentity binds desktop installation and private storage names.
// It does not create directories, migrate data or authorize an installation.
package productidentity

import (
	"errors"
	"path/filepath"
	"strings"
)

const (
	ProductID    = "zoridesk"
	BundleID     = "io.github.ggggggtm.zoridesk.local"
	ServiceLabel = BundleID + ".daemon"
	BuildID      = "zoridesk-local-v1"
)

var ErrInvalidHome = errors.New("invalid product home")

type Paths struct {
	Root, State, Socket, Isolation, Runtime, Diagnostics, Vault, Key string
}

func ForHome(home string) (Paths, error) {
	if !filepath.IsAbs(home) || home == string(filepath.Separator) ||
		filepath.Clean(home) != home || strings.ContainsRune(home, 0) {
		return Paths{}, ErrInvalidHome
	}
	root := filepath.Join(home, "Library", "Application Support", "ZoriDesk")
	paths := Paths{
		Root: root, State: filepath.Join(root, "state", "loom.db"),
		Socket:      filepath.Join(root, "run", "loomd.sock"),
		Isolation:   filepath.Join(root, "isolation"),
		Runtime:     filepath.Join(root, "runtimes", "pi"),
		Diagnostics: filepath.Join(root, "diagnostics"),
		Vault:       filepath.Join(root, "state", "credential-vault.db"),
		Key:         filepath.Join(root, "private", "vault.key"),
	}
	// Keep the existing cross-platform IPC bound; never fall back to another root.
	if len([]byte(paths.Socket)) > 96 {
		return Paths{}, ErrInvalidHome
	}
	return paths, nil
}
