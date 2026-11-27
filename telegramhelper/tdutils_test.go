package telegramhelper

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"github.com/researchaccelerator-hub/telegram-scraper/common"
	"github.com/researchaccelerator-hub/telegram-scraper/crawler"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/zelenin/go-tdlib/client"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// Example demonstrates that downloadAndExtractTarball refuses insecure URLs:
// pre-seeded TDLib session archives must come from an https host, and when an
// operator allow-list (TDLIB_ARCHIVE_ALLOWED_HOSTS) is configured, from a host
// on that list.
func Example_downloadAndExtractTarball() {
	// A plain http URL is refused before any request is made.
	err := downloadAndExtractTarball("http://insecure.example.com/tdlib.tar.gz", os.TempDir())
	if err != nil {
		fmt.Printf("Refused: %v\n", err)
	}

	// Output:
	// Refused: TDLib archive URL must use https, got "http"
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
	// Use a TLS test server: session archives must be served over https.
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

	// Production downloadAndExtractTarball validates the URL (https only) and
	// refuses loopback dials, so the fetch-path tests exercise fetchAndExtract
	// with a client that trusts the test server's self-signed certificate.
	testClient := server.Client()

	tempDir, err := os.MkdirTemp("", "tarball-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	err = fetchAndExtract(testClient, server.URL, tempDir)
	if err != nil {
		t.Fatalf("fetchAndExtract failed: %v", err)
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
	testClient := server.Client()

	tempDir, err := os.MkdirTemp("", "tarball-error-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	err = fetchAndExtract(testClient, server.URL, tempDir)
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
	testClient := server.Client()

	tempDir, err := os.MkdirTemp("", "tarball-invalid-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	err = fetchAndExtract(testClient, server.URL, tempDir)
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
	testClient := server.Client()

	tempDir, err := os.MkdirTemp("", "tarball-corrupted-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	err = fetchAndExtract(testClient, server.URL, tempDir)
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
