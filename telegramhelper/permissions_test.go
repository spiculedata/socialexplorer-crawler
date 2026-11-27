package telegramhelper

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// assertPerm checks that the path exists and its permission bits match want.
func assertPerm(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat %s: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("%s has permissions %o, want %o", path, got, want)
	}
}

// TestPrepareSessionDirsCreatesOwnerOnlyPermissions verifies that the TDLib
// session directories (which hold the Telegram session database) are created
// with owner-only permissions, so other local users cannot read the session.
func TestPrepareSessionDirsCreatesOwnerOnlyPermissions(t *testing.T) {
	storageRoot := t.TempDir()

	dbDir, filesDir, err := prepareSessionDirs(storageRoot, "conn_test")
	if err != nil {
		t.Fatalf("prepareSessionDirs failed: %v", err)
	}

	// Leaf directories and the intermediate .tdlib directories that MkdirAll
	// created must all be 0700.
	assertPerm(t, dbDir, 0700)
	assertPerm(t, filesDir, 0700)
	assertPerm(t, filepath.Dir(dbDir), 0700)
	assertPerm(t, filepath.Dir(filesDir), 0700)
}

// TestDownloadAndExtractTarballFromReaderCreatesOwnerOnlyPermissions verifies
// that extracted session files are created 0600 and extracted directories 0700,
// even when the archive header carries more permissive modes.
func TestDownloadAndExtractTarballFromReaderCreatesOwnerOnlyPermissions(t *testing.T) {
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	// Directory and file headers use permissive modes on purpose: the
	// extraction must clamp them to owner-only permissions.
	dirHeader := &tar.Header{
		Name:     "testdir",
		Mode:     0755,
		Typeflag: tar.TypeDir,
	}
	if err := tarWriter.WriteHeader(dirHeader); err != nil {
		t.Fatal(err)
	}

	fileContent := []byte("test file content")
	fileHeader := &tar.Header{
		Name:     "testdir/testfile.txt",
		Mode:     0644,
		Size:     int64(len(fileContent)),
		Typeflag: tar.TypeReg,
	}
	if err := tarWriter.WriteHeader(fileHeader); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(fileContent); err != nil {
		t.Fatal(err)
	}

	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzWriter.Close(); err != nil {
		t.Fatal(err)
	}

	tempDir := t.TempDir()

	if err := downloadAndExtractTarballFromReader(&buf, tempDir); err != nil {
		t.Fatalf("downloadAndExtractTarballFromReader failed: %v", err)
	}

	assertPerm(t, filepath.Join(tempDir, "testdir"), 0700)
	assertPerm(t, filepath.Join(tempDir, "testdir", "testfile.txt"), 0600)

	data, err := os.ReadFile(filepath.Join(tempDir, "testdir", "testfile.txt"))
	if err != nil {
		t.Fatalf("failed to read extracted file: %v", err)
	}
	if string(data) != string(fileContent) {
		t.Errorf("extracted file content mismatch: got %q", data)
	}
}

// TestDownloadAndExtractTarballFromReaderReaderIsFullyConsumed is a guard that
// the permission changes did not break the read loop.
func TestDownloadAndExtractTarballFromReaderCompletesOnMultipleFiles(t *testing.T) {
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	for _, name := range []string{"a.txt", "b.txt", "sub/c.txt"} {
		content := []byte("content of " + name)
		header := &tar.Header{
			Name:     name,
			Mode:     0644,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(tarWriter, string(content)); err != nil {
			t.Fatal(err)
		}
	}

	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzWriter.Close(); err != nil {
		t.Fatal(err)
	}

	tempDir := t.TempDir()
	if err := downloadAndExtractTarballFromReader(&buf, tempDir); err != nil {
		t.Fatalf("downloadAndExtractTarballFromReader failed: %v", err)
	}

	for _, name := range []string{"a.txt", "b.txt", "sub/c.txt"} {
		assertPerm(t, filepath.Join(tempDir, name), 0600)
	}
	assertPerm(t, filepath.Join(tempDir, "sub"), 0700)
}
