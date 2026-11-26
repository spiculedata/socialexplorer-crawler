package state

import (
	"os"
	"path/filepath"
	"testing"
)

// The local storage provider persists collected posts, media and crawl state.
// Its directories and files must be owner-only so that other local users (or
// other containers sharing the volume) cannot read the collected data.

func TestLocalStorageProviderWriteFileCreatesOwnerOnlyPermissions(t *testing.T) {
	p := NewLocalStorageProvider(t.TempDir())
	path := filepath.Join(p.BasePath, "nested", "dir", "posts.jsonl")

	if err := p.WriteFile(path, []byte("{}")); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if info, err := os.Stat(path); err != nil {
		t.Fatalf("failed to stat %s: %v", path, err)
	} else if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("file has permissions %o, want 0600", got)
	}

	dir := filepath.Dir(path)
	if info, err := os.Stat(dir); err != nil {
		t.Fatalf("failed to stat %s: %v", dir, err)
	} else if got := info.Mode().Perm(); got != 0700 {
		t.Errorf("directory has permissions %o, want 0700", got)
	}
}

func TestLocalStorageProviderAppendToFileCreatesOwnerOnlyPermissions(t *testing.T) {
	p := NewLocalStorageProvider(t.TempDir())
	path := filepath.Join(p.BasePath, "nested", "dir", "messages.jsonl")

	if err := p.AppendToFile(path, []byte("first\n")); err != nil {
		t.Fatalf("AppendToFile failed: %v", err)
	}
	if err := p.AppendToFile(path, []byte("second\n")); err != nil {
		t.Fatalf("AppendToFile failed: %v", err)
	}

	if info, err := os.Stat(path); err != nil {
		t.Fatalf("failed to stat %s: %v", path, err)
	} else if got := info.Mode().Perm(); got != 0600 {
		t.Errorf("file has permissions %o, want 0600", got)
	}

	dir := filepath.Dir(path)
	if info, err := os.Stat(dir); err != nil {
		t.Fatalf("failed to stat %s: %v", dir, err)
	} else if got := info.Mode().Perm(); got != 0700 {
		t.Errorf("directory has permissions %o, want 0700", got)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read appended file: %v", err)
	}
	if string(data) != "first\nsecond\n" {
		t.Errorf("unexpected appended content: %q", data)
	}
}

func TestLocalStorageProviderCreateDirCreatesOwnerOnlyPermissions(t *testing.T) {
	p := NewLocalStorageProvider(t.TempDir())
	dir := filepath.Join(p.BasePath, "nested", "dir")

	if err := p.CreateDir(dir); err != nil {
		t.Fatalf("CreateDir failed: %v", err)
	}

	if info, err := os.Stat(dir); err != nil {
		t.Fatalf("failed to stat %s: %v", dir, err)
	} else if got := info.Mode().Perm(); got != 0700 {
		t.Errorf("directory has permissions %o, want 0700", got)
	}
}
