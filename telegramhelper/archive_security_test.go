package telegramhelper

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// buildTarGz produces a gzipped tarball in memory with the given entries.
// Entries whose size is negative mean "pad the file with size bytes of data".
func buildTarGz(t *testing.T, entries map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)
	for name, content := range entries {
		hdr := &tar.Header{
			Name:     name,
			Mode:     0o644,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}
		if err := tarWriter.WriteHeader(hdr); err != nil {
			t.Fatalf("write header %q: %v", name, err)
		}
		if _, err := tarWriter.Write(content); err != nil {
			t.Fatalf("write content %q: %v", name, err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func extractToTemp(t *testing.T, archive []byte, targetDir string) error {
	t.Helper()
	return downloadAndExtractTarballFromReader(bytes.NewReader(archive), targetDir)
}

// A tar entry using ../ must be refused, and nothing may be written outside the
// target directory.
func TestExtractRefusesPathTraversal(t *testing.T) {
	targetDir := t.TempDir()

	archive := buildTarGz(t, map[string][]byte{
		"../evil.txt": []byte("pwned"),
	})

	if err := extractToTemp(t, archive, targetDir); err == nil {
		t.Fatal("expected traversal entry to be refused, got nil error")
	}

	// The file must not have landed outside the target directory.
	if _, err := os.Stat(filepath.Join(filepath.Dir(targetDir), "evil.txt")); err == nil {
		t.Fatal("traversal entry escaped the target directory")
	}
}

// Nested traversal like a/../../evil must be refused too.
func TestExtractRefusesNestedTraversal(t *testing.T) {
	targetDir := t.TempDir()

	archive := buildTarGz(t, map[string][]byte{
		"a/../../evil.txt": []byte("pwned"),
	})

	if err := extractToTemp(t, archive, targetDir); err == nil {
		t.Fatal("expected nested traversal entry to be refused, got nil error")
	}
}

// Absolute archive paths must be refused.
func TestExtractRefusesAbsolutePath(t *testing.T) {
	targetDir := t.TempDir()

	archive := buildTarGz(t, map[string][]byte{
		"/tmp/tdlib-evil-test/evil.txt": []byte("pwned"),
	})

	if err := extractToTemp(t, archive, targetDir); err == nil {
		t.Fatal("expected absolute-path entry to be refused, got nil error")
	}
	if _, err := os.Stat("/tmp/tdlib-evil-test"); err == nil {
		t.Fatal("absolute-path entry was created outside the target directory")
	}
}

// Windows-style backslash names must not sneak a traversal past the check.
func TestExtractRefusesBackslashTraversal(t *testing.T) {
	targetDir := t.TempDir()

	archive := buildTarGz(t, map[string][]byte{
		"..\\evil.txt": []byte("pwned"),
	})

	if err := extractToTemp(t, archive, targetDir); err == nil {
		t.Fatal("expected backslash traversal entry to be refused, got nil error")
	}
}

// A well-formed archive still extracts into the target directory.
func TestExtractAcceptsWellFormedArchive(t *testing.T) {
	targetDir := t.TempDir()

	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)
	if err := tarWriter.WriteHeader(&tar.Header{Name: "testdir", Mode: 0o755, Typeflag: tar.TypeDir}); err != nil {
		t.Fatal(err)
	}
	for _, e := range []struct {
		name    string
		content []byte
	}{
		{"topfile.txt", []byte("top level")},
		{"testdir/nested.db", []byte("tdlib data")},
	} {
		if err := tarWriter.WriteHeader(&tar.Header{Name: e.name, Mode: 0o644, Size: int64(len(e.content)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(e.content); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzWriter.Close(); err != nil {
		t.Fatal(err)
	}

	if err := downloadAndExtractTarballFromReader(bytes.NewReader(buf.Bytes()), targetDir); err != nil {
		t.Fatalf("well-formed archive refused: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(targetDir, "testdir/nested.db"))
	if err != nil {
		t.Fatalf("nested file not extracted: %v", err)
	}
	if string(got) != "tdlib data" {
		t.Fatalf("unexpected content: %q", got)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "topfile.txt")); err != nil {
		t.Fatalf("top-level file not extracted: %v", err)
	}
}

// A single entry larger than the per-file cap must be refused.
func TestExtractEnforcesPerFileCap(t *testing.T) {
	origCap := maxArchiveFileBytes
	maxArchiveFileBytes = 16
	defer func() { maxArchiveFileBytes = origCap }()

	targetDir := t.TempDir()
	archive := buildTarGz(t, map[string][]byte{
		"big.bin": bytes.Repeat([]byte("A"), 64),
	})

	if err := extractToTemp(t, archive, targetDir); err == nil {
		t.Fatal("expected oversized entry to be refused, got nil error")
	}
}

// Several small entries whose combined size exceeds the total cap must be
// refused, even though each is under the per-file cap.
func TestExtractEnforcesTotalCap(t *testing.T) {
	origFileCap := maxArchiveFileBytes
	origTotalCap := maxArchiveTotalBytes
	maxArchiveFileBytes = 16
	maxArchiveTotalBytes = 24
	defer func() {
		maxArchiveFileBytes = origFileCap
		maxArchiveTotalBytes = origTotalCap
	}()

	targetDir := t.TempDir()
	archive := buildTarGz(t, map[string][]byte{
		"one.bin": bytes.Repeat([]byte("A"), 16),
		"two.bin": bytes.Repeat([]byte("B"), 16),
	})

	if err := extractToTemp(t, archive, targetDir); err == nil {
		t.Fatal("expected archive over total cap to be refused, got nil error")
	}
}

func TestValidateArchiveURL(t *testing.T) {
	origAllowlist := archiveHostAllowlist
	defer func() { archiveHostAllowlist = origAllowlist }()

	t.Run("https is required", func(t *testing.T) {
		archiveHostAllowlist = map[string]bool{}
		if _, err := validateArchiveURL("http://example.com/db.tar.gz"); err == nil {
			t.Fatal("expected http URL to be refused")
		}
	})
	t.Run("plain https url accepted when no allow-list", func(t *testing.T) {
		archiveHostAllowlist = map[string]bool{}
		if _, err := validateArchiveURL("https://archives.example.com/db.tar.gz"); err != nil {
			t.Fatalf("expected https URL to be accepted: %v", err)
		}
	})
	t.Run("host not in allow-list refused", func(t *testing.T) {
		archiveHostAllowlist = parseArchiveAllowlist("archives.example.com")
		if _, err := validateArchiveURL("https://evil.example.com/db.tar.gz"); err == nil {
			t.Fatal("expected non-allow-listed host to be refused")
		}
	})
	t.Run("host in allow-list accepted", func(t *testing.T) {
		archiveHostAllowlist = parseArchiveAllowlist("Archives.Example.com")
		if _, err := validateArchiveURL("https://archives.example.com/db.tar.gz"); err != nil {
			t.Fatalf("expected allow-listed host to be accepted: %v", err)
		}
	})
	t.Run("unparseable url refused", func(t *testing.T) {
		archiveHostAllowlist = map[string]bool{}
		if _, err := validateArchiveURL("ht tp://bad"); err == nil {
			t.Fatal("expected unparseable URL to be refused")
		}
	})
	t.Run("hostless url refused", func(t *testing.T) {
		archiveHostAllowlist = map[string]bool{}
		if _, err := validateArchiveURL("https:///db.tar.gz"); err == nil {
			t.Fatal("expected hostless URL to be refused")
		}
	})
}

// The hardened HTTP client refuses loopback, private, link-local and multicast
// addresses, so an archive URL cannot reach cloud metadata or in-cluster
// services.
func TestDownloadRefusesPrivateAddresses(t *testing.T) {
	t.Run("loopback https server refused", func(t *testing.T) {
		// httptest.NewTLSServer listens on 127.0.0.1: the URL passes scheme
		// validation but the dialer must refuse the loopback address.
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		err := downloadAndExtractTarball(server.URL, t.TempDir())
		if err == nil {
			t.Fatal("expected loopback address to be refused")
		}
	})
	t.Run("cloud metadata address refused", func(t *testing.T) {
		err := downloadAndExtractTarball("https://169.254.169.254/latest/meta-data/", t.TempDir())
		if err == nil {
			t.Fatal("expected link-local metadata address to be refused")
		}
	})
	t.Run("private rfc1918 address refused", func(t *testing.T) {
		err := downloadAndExtractTarball("https://10.1.2.3/db.tar.gz", t.TempDir())
		if err == nil {
			t.Fatal("expected private address to be refused")
		}
	})
	t.Run("plain http to loopback refused before dial", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		err := downloadAndExtractTarball(server.URL, t.TempDir())
		if err == nil {
			t.Fatal("expected http URL to be refused")
		}
	})
}

func TestIsAllowedArchiveAddr(t *testing.T) {
	blocked := []string{
		"127.0.0.1",       // loopback
		"10.0.0.5",        // RFC1918
		"172.16.0.9",      // RFC1918
		"192.168.1.1",     // RFC1918
		"169.254.169.254", // link-local (cloud metadata)
		"224.0.0.1",       // multicast
		"0.0.0.0",         // unspecified
		"::1",             // IPv6 loopback
		"fe80::1",         // IPv6 link-local
		"fc00::1",         // IPv6 unique-local
		"::ffff:10.0.0.5", // IPv4-mapped private
	}
	for _, s := range blocked {
		ip := net.ParseIP(s)
		if ip == nil {
			t.Fatalf("bad test IP %q", s)
		}
		if isAllowedArchiveAddr(ip) {
			t.Errorf("expected %q to be blocked", s)
		}
	}

	allowed := []string{
		"93.184.216.34",
		"8.8.8.8",
		"2606:2800:220:1:248:1893:25c8:1946",
	}
	for _, s := range allowed {
		ip := net.ParseIP(s)
		if ip == nil {
			t.Fatalf("bad test IP %q", s)
		}
		if !isAllowedArchiveAddr(ip) {
			t.Errorf("expected %q to be allowed", s)
		}
	}
}

// safeDialContext itself refuses a blocked address rather than opening the
// connection.
func TestSafeDialContextRefusesLoopback(t *testing.T) {
	_, err := safeDialContext(context.Background(), "tcp", "127.0.0.1:1")
	if err == nil {
		t.Fatal("expected dial to loopback to be refused")
	}
}
