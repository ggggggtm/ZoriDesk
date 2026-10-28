// Command testreport checks a bounded go test -json result. It does not run tests.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

type event struct {
	Action  string
	Package string
	Test    string
}

type result struct {
	passed bool
	tests  map[string]bool
}

func validate(reader io.Reader, expected []string) (int, error) {
	if len(expected) == 0 {
		return 0, fmt.Errorf("at least one expected package is required")
	}
	packages := make(map[string]*result, len(expected))
	for _, name := range expected {
		if name == "" || packages[name] != nil {
			return 0, fmt.Errorf("empty or duplicate expected package")
		}
		packages[name] = &result{tests: make(map[string]bool)}
	}
	decoder := json.NewDecoder(reader)
	for {
		var entry event
		if err := decoder.Decode(&entry); err != nil {
			if err == io.EOF {
				break
			}
			return 0, fmt.Errorf("invalid test JSON: %w", err)
		}
		pkg := packages[entry.Package]
		if pkg == nil {
			return 0, fmt.Errorf("unexpected package %q", entry.Package)
		}
		switch entry.Action {
		case "skip", "fail":
			return 0, fmt.Errorf("%s: %s %s", entry.Action, entry.Package, entry.Test)
		case "run":
			if entry.Test == "" || pkg.passed {
				return 0, fmt.Errorf("invalid test start in %s", entry.Package)
			}
			if _, exists := pkg.tests[entry.Test]; exists {
				return 0, fmt.Errorf("duplicate test run: %s", entry.Test)
			}
			pkg.tests[entry.Test] = false
		case "pass":
			if entry.Test == "" {
				if pkg.passed {
					return 0, fmt.Errorf("duplicate package completion: %s", entry.Package)
				}
				pkg.passed = true
			} else {
				passed, started := pkg.tests[entry.Test]
				if !started || passed || pkg.passed {
					return 0, fmt.Errorf("test completion without active run: %s", entry.Test)
				}
				pkg.tests[entry.Test] = true
			}
		case "start", "output", "pause", "cont":
			// Logs and parallel-test scheduling are not pass/skip evidence.
		default:
			return 0, fmt.Errorf("unsupported test action %q", entry.Action)
		}
	}
	count := 0
	for name, pkg := range packages {
		if !pkg.passed || len(pkg.tests) == 0 {
			return 0, fmt.Errorf("package %s has no completed passing test suite", name)
		}
		for test, passed := range pkg.tests {
			if !passed {
				return 0, fmt.Errorf("test %s/%s did not finish", name, test)
			}
			count++
		}
	}
	return count, nil
}

func main() {
	count, err := validate(os.Stdin, os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "verification failed:", err)
		os.Exit(1)
	}
	fmt.Printf("Verified %d passing test/subtest records across %d packages; no skips.\n", count, len(os.Args[1:]))
}
