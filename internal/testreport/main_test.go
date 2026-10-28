package main

import (
	"strings"
	"testing"
)

func TestValidate(t *testing.T) {
	const success = `{"Action":"run","Package":"pkg/a","Test":"TestOne"}
{"Action":"output","Package":"pkg/a","Output":"--- SKIP: text in a log is not a skip event"}
{"Action":"pass","Package":"pkg/a","Test":"TestOne"}
{"Action":"pass","Package":"pkg/a"}
`
	tests := []struct {
		name, data string
		packages   []string
		wantError  bool
	}{
		{"success", success, []string{"pkg/a"}, false},
		{"skip", `{"Action":"skip","Package":"pkg/a","Test":"TestOne"}`, []string{"pkg/a"}, true},
		{"package skip", `{"Action":"skip","Package":"pkg/a"}`, []string{"pkg/a"}, true},
		{"test failure", `{"Action":"fail","Package":"pkg/a","Test":"TestOne"}`, []string{"pkg/a"}, true},
		{"package failure", success + `{"Action":"fail","Package":"pkg/a"}`, []string{"pkg/a"}, true},
		{"empty", "", []string{"pkg/a"}, true},
		{"no tests", `{"Action":"pass","Package":"pkg/a"}`, []string{"pkg/a"}, true},
		{"missing package", success, []string{"pkg/a", "pkg/b"}, true},
		{"unexpected package", success, []string{"pkg/b"}, true},
		{"malformed", success + `{`, []string{"pkg/a"}, true},
		{"missing package end", strings.ReplaceAll(success, `{"Action":"pass","Package":"pkg/a"}`, ""), []string{"pkg/a"}, true},
		{"unfinished test", success + `{"Action":"run","Package":"pkg/a","Test":"TestTwo"}`, []string{"pkg/a"}, true},
		{"no expected packages", success, nil, true},
		{"empty expected package", success, []string{""}, true},
		{"duplicate expected package", success, []string{"pkg/a", "pkg/a"}, true},
		{"duplicate run", strings.ReplaceAll(success, `{"Action":"pass","Package":"pkg/a","Test":"TestOne"}`, `{"Action":"run","Package":"pkg/a","Test":"TestOne"}`), []string{"pkg/a"}, true},
		{"pass without run", strings.ReplaceAll(success, `{"Action":"run","Package":"pkg/a","Test":"TestOne"}`, ""), []string{"pkg/a"}, true},
		{"duplicate package end", success + `{"Action":"pass","Package":"pkg/a"}`, []string{"pkg/a"}, true},
		{"unknown action", success + `{"Action":"unknown","Package":"pkg/a"}`, []string{"pkg/a"}, true},
		{"null record", success + `null`, []string{"pkg/a"}, true},
		{"parallel scheduling", strings.ReplaceAll(success, `{"Action":"pass","Package":"pkg/a","Test":"TestOne"}`, "{\"Action\":\"pause\",\"Package\":\"pkg/a\",\"Test\":\"TestOne\"}\n{\"Action\":\"cont\",\"Package\":\"pkg/a\",\"Test\":\"TestOne\"}\n{\"Action\":\"pass\",\"Package\":\"pkg/a\",\"Test\":\"TestOne\"}"), []string{"pkg/a"}, false},
		{"multiple packages", success + strings.ReplaceAll(success, "pkg/a", "pkg/b"), []string{"pkg/a", "pkg/b"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, err := validate(strings.NewReader(tt.data), tt.packages)
			if (err != nil) != tt.wantError {
				t.Fatalf("validate = %d, %v; wantError=%v", count, err, tt.wantError)
			}
			if !tt.wantError && count != len(tt.packages) {
				t.Fatalf("got %d passing tests, want %d", count, len(tt.packages))
			}
		})
	}
}
