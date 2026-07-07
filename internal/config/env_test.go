// SPDX-License-Identifier: MIT

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeEnvFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("writing env file: %v", err)
	}
	return path
}

func TestLoadEnvFileMissingFileIsNotAnError(t *testing.T) {
	if err := LoadEnvFile(filepath.Join(t.TempDir(), "does-not-exist.env")); err != nil {
		t.Fatalf("LoadEnvFile() error = %v, want nil for a missing file", err)
	}
}

func TestLoadEnvFileParsesVariousLines(t *testing.T) {
	// An existing-but-empty value must not block the file (see
	// TestLoadEnvFileOverridesExistingEmptyValue); t.Setenv("", ...) also
	// restores the pre-test state (unset) once this test finishes.
	t.Setenv("ENV_TEST_A", "")
	t.Setenv("ENV_TEST_B", "")
	t.Setenv("ENV_TEST_C", "")
	t.Setenv("ENV_TEST_D", "")

	path := writeEnvFile(t, `
# a comment

ENV_TEST_A=plain
export ENV_TEST_B="double quoted"
ENV_TEST_C='single quoted'
ENV_TEST_D = spaced out
not-a-valid-line
=orphan-value
`)

	if err := LoadEnvFile(path); err != nil {
		t.Fatalf("LoadEnvFile() error = %v", err)
	}
	cases := map[string]string{
		"ENV_TEST_A": "plain",
		"ENV_TEST_B": "double quoted",
		"ENV_TEST_C": "single quoted",
		"ENV_TEST_D": "spaced out",
	}
	for k, want := range cases {
		if got := os.Getenv(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

func TestLoadEnvFileDoesNotOverrideExistingNonEmptyValue(t *testing.T) {
	t.Setenv("ENV_TEST_EXISTING", "from-shell")
	path := writeEnvFile(t, "ENV_TEST_EXISTING=from-file\n")

	if err := LoadEnvFile(path); err != nil {
		t.Fatalf("LoadEnvFile() error = %v", err)
	}
	if got := os.Getenv("ENV_TEST_EXISTING"); got != "from-shell" {
		t.Errorf("ENV_TEST_EXISTING = %q, want from-shell (existing value must win)", got)
	}
}

func TestLoadEnvFileOverridesExistingEmptyValue(t *testing.T) {
	t.Setenv("ENV_TEST_EMPTY", "")
	path := writeEnvFile(t, "ENV_TEST_EMPTY=filled-in\n")

	if err := LoadEnvFile(path); err != nil {
		t.Fatalf("LoadEnvFile() error = %v", err)
	}
	if got := os.Getenv("ENV_TEST_EMPTY"); got != "filled-in" {
		t.Errorf("ENV_TEST_EMPTY = %q, want filled-in (an existing but empty value should not block the file)", got)
	}
}

// TestLoadEnvFilePropagatesNonNotExistOpenError covers the os.Open error
// branch other than "does not exist": treating a regular file as a directory
// component yields ENOTDIR, which LoadEnvFile must surface rather than
// silently swallow like a missing file.
func TestLoadEnvFilePropagatesNonNotExistOpenError(t *testing.T) {
	regularFile := writeEnvFile(t, "FOO=bar\n")
	pathThroughFile := filepath.Join(regularFile, "env")

	if err := LoadEnvFile(pathThroughFile); err == nil {
		t.Fatal("LoadEnvFile() expected error when a path component is a regular file, got nil")
	}
}

// TestLoadEnvFilePropagatesSetenvError covers the os.Setenv error branch: a
// key containing a NUL byte is accepted by the line scanner (NUL isn't a
// line delimiter) but rejected by os.Setenv.
func TestLoadEnvFilePropagatesSetenvError(t *testing.T) {
	path := writeEnvFile(t, "FOO\x00BAR=value\n")

	if err := LoadEnvFile(path); err == nil {
		t.Fatal("LoadEnvFile() expected error for a NUL byte in a variable name, got nil")
	}
}

func TestUnquote(t *testing.T) {
	cases := []struct{ in, want string }{
		{`"double"`, "double"},
		{`'single'`, "single"},
		{"unquoted", "unquoted"},
		{`"mismatched'`, `"mismatched'`},
		{`"`, `"`},
		{"", ""},
	}
	for _, c := range cases {
		if got := unquote(c.in); got != c.want {
			t.Errorf("unquote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
