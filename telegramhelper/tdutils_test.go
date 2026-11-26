package telegramhelper

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"fmt"
	"github.com/researchaccelerator-hub/telegram-scraper/common"
	"github.com/researchaccelerator-hub/telegram-scraper/crawler"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/zelenin/go-tdlib/client"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// tarEntry describes one entry in a test-built tar.gz archive.
type tarEntry struct {
	name string
	data []byte
	dir  bool
}

// buildTarGz builds a gzipped tar archive in memory from the given entries.
func buildTarGz(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Mode: 0644}
		if e.dir {
			h.Typeflag = tar.TypeDir
			if err := tw.WriteHeader(h); err != nil {
				t.Fatal(err)
			}
			continue
		}
		h.Typeflag = tar.TypeReg
		h.Size = int64(len(e.data))
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(e.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// newTestDownloadClient returns an HTTP client suitable for talking to a
// local httptest TLS server (which uses a self-signed certificate).
func newTestDownloadClient() *http.Client {
	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}, //nolint:gosec // test-only client for local httptest servers
	}
}

// useTestDownloadClient swaps the hardened download client for one that can
// reach the local httptest server, restoring the hardened client afterwards.
func useTestDownloadClient(t *testing.T) {
	t.Helper()
	old := downloadHTTPClient
	downloadHTTPClient = newTestDownloadClient()
	t.Cleanup(func() { downloadHTTPClient = old })
}

// Example demonstrates how to use the downloadAndExtractTarball function
// to download and extract a tarball from a URL to a target directory.
func Example_downloadAndExtractTarball() {
	// Step 1: Create a temporary directory for extraction
	targetDir, err := os.MkdirTemp("", "example-extract")
	if err != nil {
		fmt.Printf("Failed to create temp dir: %v\n", err)
		return
	}
	defer os.RemoveAll(targetDir) // Clean up after the example runs

	// Step 2: Create a mock HTTPS server serving a known tarball
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Create an in-memory tar.gz archive
		var buf bytes.Buffer
		gzWriter := gzip.NewWriter(&buf)
		tarWriter := tar.NewWriter(gzWriter)

		// Add a test file
		fileContent := []byte("test content")
		fileHeader := &tar.Header{
			Name:     "testfile.txt",
			Mode:     0644,
			Size:     int64(len(fileContent)),
			Typeflag: tar.TypeReg,
		}
		if err := tarWriter.WriteHeader(fileHeader); err != nil {
			fmt.Printf("Failed to write tar header: %v\n", err)
			return
		}
		if _, err := tarWriter.Write(fileContent); err != nil {
			fmt.Printf("Failed to write file content: %v\n", err)
			return
		}

		// Close tar and gzip writers
		tarWriter.Close()
		gzWriter.Close()

		// Write tar.gz content to response
		w.Header().Set("Content-Type", "application/x-gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(buf.Bytes())
	}))
	defer server.Close()

	// The hardened download client refuses loopback addresses, so swap in a
	// test client that accepts the local server's self-signed certificate.
	oldClient := downloadHTTPClient
	downloadHTTPClient = newTestDownloadClient()
	defer func() { downloadHTTPClient = oldClient }()

	// Step 3: Call function to download and extract
	err = downloadAndExtractTarball(server.URL, targetDir)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Step 4: Verify extracted file exists
	extractedFilePath := filepath.Join(targetDir, "testfile.txt")
	_, err = os.Stat(extractedFilePath)
	if os.IsNotExist(err) {
		fmt.Printf("Error: Expected file not found at %s\n", extractedFilePath)
		return
	}

	fmt.Println("Successfully downloaded and extracted tarball")

	// Output:
	// Successfully downloaded and extracted tarball
}

func TestDownloadAndExtractTarballFromReader(t *testing.T) {
	// Create a mock tar.gz archive in memory
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzWriter)

	// Add a directory
	dirHeader := &tar.Header{
		Name:     "testdir",
		Mode:     0755,
		Typeflag: tar.TypeDir,
	}
	if err := tarWriter.WriteHeader(dirHeader); err != nil {
		t.Fatal(err)
	}

	// Add a file
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

	// Close writers
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzWriter.Close(); err != nil {
		t.Fatal(err)
	}

	// Create a temporary directory for extraction
	tempDir, err := os.MkdirTemp("", "tarball-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Use the new function to extract from reader
	err = downloadAndExtractTarballFromReader(&buf, tempDir)
	if err != nil {
		t.Fatalf("downloadAndExtractTarballFromReader failed: %v", err)
	}

	// Verify that the file was extracted correctly
	extractedFilePath := filepath.Join(tempDir, "testdir/testfile.txt")
	content, err := os.ReadFile(extractedFilePath)
	if err != nil {
		t.Fatalf("Failed to read extracted file: %v", err)
	}

	if string(content) != "test file content" {
		t.Errorf("Unexpected file content: %s", content)
	}
}

func TestDownloadAndExtractTarball(t *testing.T) {
	// Create a test server that serves a mock tarball
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check headers
		userAgent := r.Header.Get("User-Agent")
		if userAgent != "Mozilla/5.0 (Windows NT 10.0; Win64; x64)" {
			t.Errorf("Expected User-Agent header, got: %s", userAgent)
		}

		// Create and serve a tarball
		var buf bytes.Buffer
		gzWriter := gzip.NewWriter(&buf)
		tarWriter := tar.NewWriter(gzWriter)

		// Add a directory
		dirHeader := &tar.Header{
			Name:     "testdir",
			Mode:     0755,
			Typeflag: tar.TypeDir,
		}
		if err := tarWriter.WriteHeader(dirHeader); err != nil {
			t.Fatal(err)
		}

		// Add a file
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

		// Close writers
		if err := tarWriter.Close(); err != nil {
			t.Fatal(err)
		}
		if err := gzWriter.Close(); err != nil {
			t.Fatal(err)
		}

		// Send response
		w.Header().Set("Content-Type", "application/x-gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(buf.Bytes())
	}))
	defer server.Close()
	useTestDownloadClient(t)

	tempDir, err := os.MkdirTemp("", "tarball-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	err = downloadAndExtractTarball(server.URL, tempDir)
	if err != nil {
		t.Fatalf("downloadAndExtractTarball failed: %v", err)
	}

	extractedFilePath := filepath.Join(tempDir, "testdir/testfile.txt")
	content, err := os.ReadFile(extractedFilePath)
	if err != nil {
		t.Fatalf("Failed to read extracted file: %v", err)
	}

	if string(content) != "test file content" {
		t.Errorf("Unexpected file content: %s", content)
	}
}

func TestDownloadError(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	useTestDownloadClient(t)

	tempDir, err := os.MkdirTemp("", "tarball-error-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	err = downloadAndExtractTarball(server.URL, tempDir)
	if err == nil {
		t.Error("Expected error for 404 response, got nil")
	}
}

func TestInvalidGzipFormat(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-gzip")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("This is not a valid gzip file"))
	}))
	defer server.Close()
	useTestDownloadClient(t)

	tempDir, err := os.MkdirTemp("", "tarball-invalid-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	err = downloadAndExtractTarball(server.URL, tempDir)
	if err == nil {
		t.Error("Expected error for invalid gzip data, got nil")
	}
}

func TestCorruptedTarFile(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var buf bytes.Buffer
		gzWriter := gzip.NewWriter(&buf)
		gzWriter.Write([]byte("Not a valid tar file"))
		gzWriter.Close()

		w.Header().Set("Content-Type", "application/x-gzip")
		w.WriteHeader(http.StatusOK)
		w.Write(buf.Bytes())
	}))
	defer server.Close()
	useTestDownloadClient(t)

	tempDir, err := os.MkdirTemp("", "tarball-corrupted-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	err = downloadAndExtractTarball(server.URL, tempDir)
	if err == nil {
		t.Error("Expected error for corrupted tar data, got nil")
	}
}

// MockTelegramService is a mock implementation for testing
type MockTelegramService struct{}

func (m *MockTelegramService) InitializeClientWithConfig(storagePrefix string, cfg common.CrawlerConfig) (crawler.TDLibClient, error) {
	//TODO implement me
	panic("implement me")
}

// InitializeClient simulates a successful TDLib connection
func (m *MockTelegramService) InitializeClient(storagePrefix string) (crawler.TDLibClient, error) {
	log.Info().Msg("MockTelegramService: Simulating client initialization")
	return nil, nil
}

// GetMe simulates retrieving a fake user
func (m *MockTelegramService) GetMe(tdlibClient crawler.TDLibClient) (*client.User, error) {
	return &client.User{
		FirstName: "Mock",
		LastName:  "User",
	}, nil
}

// TestMockTelegramService_InitializeClient tests mock client initialization
func TestMockTelegramService_InitializeClient(t *testing.T) {
	service := &MockTelegramService{}
	tdlibClient, err := service.InitializeClient("/tmp")

	assert.NoError(t, err, "MockTelegramService should not return an error")
	assert.Nil(t, tdlibClient, "MockTelegramService should return nil")
}

// TestMockTelegramService_GetMe tests mock retrieval of a user
func TestMockTelegramService_GetMe(t *testing.T) {
	service := &MockTelegramService{}
	user, err := service.GetMe(nil)

	assert.NoError(t, err, "MockTelegramService GetMe should not return an error")
	assert.NotNil(t, user, "MockTelegramService GetMe should return a user")
	assert.Equal(t, "Mock", user.FirstName, "User first name should be Mock")
	assert.Equal(t, "User", user.LastName, "User last name should be User")
}

// TestGenCode_MockService tests GenCode using a mock service
func TestGenCode_MockService(t *testing.T) {
	service := &MockTelegramService{}
	assert.NotPanics(t, func() { GenCode(service, "/tmp") }, "GenCode should not panic")
}

// ExampleGenCode demonstrates how GenCode is used
func ExampleGenCode() {
	// Store the original logger configuration
	originalLogger := log.Logger

	// Configure zerolog to use the console writer with minimal formatting
	consoleWriter := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		NoColor:    true,
		TimeFormat: "",
		FormatLevel: func(i interface{}) string {
			return ""
		},
		FormatTimestamp: func(i interface{}) string {
			return ""
		},
		FormatCaller: func(i interface{}) string {
			return ""
		},
		FormatMessage: func(i interface{}) string {
			return i.(string)
		},
	}

	// Replace the global logger
	log.Logger = zerolog.New(consoleWriter).With().Logger()

	// Run the function
	service := &MockTelegramService{}
	GenCode(service, "/tmp")

	// Restore the original logger
	log.Logger = originalLogger

	// Output:
	// MockTelegramService: Simulating client initialization
	// Authenticated as: Mock User
}

// serveTarGz starts a local HTTPS test server that always serves the given
// archive payload.
func serveTarGz(t *testing.T, payload []byte) *httptest.Server {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-gzip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	}))
	t.Cleanup(server.Close)
	return server
}

// TestExtractRefusesPathTraversal verifies that an archive entry whose name
// escapes the extraction directory is refused and writes nothing outside it.
func TestExtractRefusesPathTraversal(t *testing.T) {
	payload := buildTarGz(t, []tarEntry{{name: "../../evil.txt", data: []byte("pwned")}})
	tempDir := t.TempDir()

	err := downloadAndExtractTarballFromReader(bytes.NewReader(payload), tempDir)
	if err == nil {
		t.Fatal("expected traversal entry to be refused, got nil error")
	}
	if !strings.Contains(err.Error(), "escapes") {
		t.Errorf("expected escape error, got: %v", err)
	}

	// The entry would land next to tempDir if extraction were vulnerable.
	escaped := filepath.Join(filepath.Dir(tempDir), "evil.txt")
	if _, statErr := os.Stat(escaped); statErr == nil {
		os.Remove(escaped)
		t.Errorf("traversal entry was written outside the extraction directory: %s", escaped)
	}
}

// TestExtractRefusesAbsolutePath verifies that an archive entry with an
// absolute path is refused instead of being written at the absolute location.
func TestExtractRefusesAbsolutePath(t *testing.T) {
	const marker = "/tmp/tdlib-evil-abs-marker"
	_ = os.Remove(marker)
	payload := buildTarGz(t, []tarEntry{{name: marker, data: []byte("pwned")}})
	defer os.Remove(marker)

	err := downloadAndExtractTarballFromReader(bytes.NewReader(payload), t.TempDir())
	if err == nil {
		t.Fatal("expected absolute-path entry to be refused, got nil error")
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Errorf("absolute-path entry was written to %s", marker)
	}
}

// TestExtractRefusesOversizedEntry verifies that a single archive entry over
// the per-file size cap is refused and no truncated file is left behind.
func TestExtractRefusesOversizedEntry(t *testing.T) {
	oldCap := maxArchiveFileBytes
	maxArchiveFileBytes = 64
	defer func() { maxArchiveFileBytes = oldCap }()

	payload := buildTarGz(t, []tarEntry{{name: "big.bin", data: bytes.Repeat([]byte("a"), 1024)}})
	tempDir := t.TempDir()

	err := downloadAndExtractTarballFromReader(bytes.NewReader(payload), tempDir)
	if err == nil {
		t.Fatal("expected oversized entry to be refused, got nil error")
	}
	if _, statErr := os.Stat(filepath.Join(tempDir, "big.bin")); statErr == nil {
		t.Error("oversized entry was left on disk")
	}
}

// TestExtractRefusesTotalOversize verifies that an archive whose total
// uncompressed size exceeds the cap is refused.
func TestExtractRefusesTotalOversize(t *testing.T) {
	oldCap := maxArchiveTotalBytes
	maxArchiveTotalBytes = 1024
	defer func() { maxArchiveTotalBytes = oldCap }()

	payload := buildTarGz(t, []tarEntry{
		{name: "a.bin", data: make([]byte, 800)},
		{name: "b.bin", data: make([]byte, 800)},
	})

	err := downloadAndExtractTarballFromReader(bytes.NewReader(payload), t.TempDir())
	if err == nil {
		t.Fatal("expected archive over total size cap to be refused, got nil error")
	}
}

// TestDownloadRefusesPrivateAddress verifies that a download whose target
// host resolves to a loopback (or other internal) address is refused by the
// hardened client's dialer. The hardened client is deliberately NOT swapped
// out for the test client here.
func TestDownloadRefusesPrivateAddress(t *testing.T) {
	server := serveTarGz(t, buildTarGz(t, []tarEntry{{name: "ok.txt", data: []byte("ok")}}))

	err := downloadAndExtractTarball(server.URL, t.TempDir())
	if err == nil {
		t.Fatal("expected download from a loopback address to be refused, got nil error")
	}
	if !strings.Contains(err.Error(), "disallowed") {
		t.Errorf("expected SSRF dialer refusal, got: %v", err)
	}
}

// TestDownloadRefusesNonHTTPS verifies that a non-https archive URL is
// refused before any request is made.
func TestDownloadRefusesNonHTTPS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("request should never reach the server")
	}))
	defer server.Close()

	err := downloadAndExtractTarball(server.URL, t.TempDir())
	if err == nil {
		t.Fatal("expected non-https URL to be refused, got nil error")
	}
	if !strings.Contains(err.Error(), "https") {
		t.Errorf("expected https-only error, got: %v", err)
	}
}

// TestValidateArchiveURLAllowList verifies the operator host allow-list.
func TestValidateArchiveURLAllowList(t *testing.T) {
	t.Setenv(archiveURLAllowedHostsEnv, "archives.example.com")

	offList, err := neturl.Parse("https://evil.example.com/db.tgz")
	if err != nil {
		t.Fatal(err)
	}
	if err := validateArchiveURL(offList); err == nil {
		t.Error("expected host outside the allow-list to be refused")
	}

	onList, err := neturl.Parse("https://archives.example.com/db.tgz")
	if err != nil {
		t.Fatal(err)
	}
	if err := validateArchiveURL(onList); err != nil {
		t.Errorf("expected host on the allow-list to be accepted, got: %v", err)
	}

	httpURL, err := neturl.Parse("https://archives.example.com/db.tgz")
	if err != nil {
		t.Fatal(err)
	}
	httpURL.Scheme = "http"
	if err := validateArchiveURL(httpURL); err == nil {
		t.Error("expected http scheme to be refused")
	}
}
