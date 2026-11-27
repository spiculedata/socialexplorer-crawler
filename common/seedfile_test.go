package common

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolveSeedFile verifies that seed-file names supplied in a job payload
// can only ever resolve inside the fixed seed directory. A payload that names
// an absolute path or a path containing ".." must be rejected, otherwise the
// crawler would read an arbitrary local file (session credentials, service
// account tokens, /proc/self/environ) and treat its contents as seed URLs
// (issue: "[Security][Crawler] urlFile in the job payload reads any local
// file into logs and state").
func TestResolveSeedFile(t *testing.T) {
	t.Run("relative name resolves inside the seed directory", func(t *testing.T) {
		resolved, err := ResolveSeedFile("urls.txt")
		if err != nil {
			t.Fatalf("expected relative name to resolve, got error: %v", err)
		}
		want := filepath.Join(SeedFileDir(), "urls.txt")
		if resolved != want {
			t.Fatalf("expected %q, got %q", want, resolved)
		}
	})

	t.Run("nested relative name resolves inside the seed directory", func(t *testing.T) {
		resolved, err := ResolveSeedFile("telegram/urls.txt")
		if err != nil {
			t.Fatalf("expected nested relative name to resolve, got error: %v", err)
		}
		if !strings.HasPrefix(resolved, SeedFileDir()+string(os.PathSeparator)) {
			t.Fatalf("expected %q to be inside %q", resolved, SeedFileDir())
		}
	})

	t.Run("absolute path is rejected", func(t *testing.T) {
		for _, name := range []string{
			"/proc/self/environ",
			"/app/credentials.json",
			"/etc/passwd",
		} {
			if _, err := ResolveSeedFile(name); err == nil {
				t.Fatalf("expected absolute path %q to be rejected, got no error", name)
			}
		}
	})

	t.Run("path with dot-dot is rejected", func(t *testing.T) {
		for _, name := range []string{
			"../credentials.json",
			"../../credentials.json",
			"sub/../../credentials.json",
			"..",
		} {
			if _, err := ResolveSeedFile(name); err == nil {
				t.Fatalf("expected dot-dot path %q to be rejected, got no error", name)
			}
		}
	})

	t.Run("empty name is rejected", func(t *testing.T) {
		if _, err := ResolveSeedFile(""); err == nil {
			t.Fatal("expected empty name to be rejected, got no error")
		}
	})

	t.Run("seed directory override via environment", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("CRAWLER_SEED_DIR", dir)
		resolved, err := ResolveSeedFile("urls.txt")
		if err != nil {
			t.Fatalf("expected relative name to resolve, got error: %v", err)
		}
		if resolved != filepath.Join(dir, "urls.txt") {
			t.Fatalf("expected %q, got %q", filepath.Join(dir, "urls.txt"), resolved)
		}
		// An escape attempt must still be rejected against the overridden root.
		if _, err := ResolveSeedFile("../outside.txt"); err == nil {
			t.Fatal("expected dot-dot path to be rejected against the overridden seed directory")
		}
	})
}
