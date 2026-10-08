package local

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiskStorage_SaveAndRead(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gostore-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	storage, err := NewDiskStorage(tempDir, "custom-encryption-key-for-test")
	if err != nil {
		t.Fatalf("Failed to init disk storage: %v", err)
	}

	ctx := context.Background()
	bucket := "test-bucket"
	objectPath := "photos/test.txt"
	testContent := "Hello, GoStore Object Engine with Full Transparent Disk Encryption!"

	// 1. Save
	written, md5Hash, sha256Hash, err := storage.Save(ctx, bucket, objectPath, strings.NewReader(testContent))
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if written != int64(len(testContent)) {
		t.Errorf("Expected %d bytes written, got %d", len(testContent), written)
	}
	if md5Hash == "" || sha256Hash == "" {
		t.Errorf("Expected valid hashes, got md5: %s, sha256: %s", md5Hash, sha256Hash)
	}

	// 2. Verify PHYSICAL BYTES ON DISK ARE ENCRYPTED
	rawDiskPath := filepath.Join(tempDir, bucket, "photos/test.txt")
	rawBytes, err := os.ReadFile(rawDiskPath)
	if err != nil {
		t.Fatalf("Failed to read raw disk file: %v", err)
	}
	if bytes.Equal(rawBytes, []byte(testContent)) {
		t.Fatal("SECURITY ERROR: Physical file on disk is NOT encrypted!")
	}
	if !bytes.HasPrefix(rawBytes, []byte(EncryptedHeaderMagic)) {
		t.Fatal("SECURITY ERROR: Physical file on disk lacks EncryptedHeaderMagic header!")
	}
	if int64(len(rawBytes)) != int64(len(testContent))+int64(HeaderSize) {
		t.Errorf("Expected physical encrypted size %d, got %d", len(testContent)+HeaderSize, len(rawBytes))
	}

	// 3. Exists
	exists, err := storage.Exists(ctx, bucket, objectPath)
	if err != nil || !exists {
		t.Fatalf("Expected file to exist, exists=%v, err=%v", exists, err)
	}

	// 4. Open and Read (Decrypted plaintext stream)
	reader, size, err := storage.Open(ctx, bucket, objectPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer reader.Close()

	if size != int64(len(testContent)) {
		t.Errorf("Expected size %d, got %d", len(testContent), size)
	}

	buf, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if string(buf) != testContent {
		t.Errorf("Expected content %q, got %q", testContent, string(buf))
	}

	// 5. Delete
	if err := storage.Delete(ctx, bucket, objectPath); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	existsAfter, err := storage.Exists(ctx, bucket, objectPath)
	if err != nil || existsAfter {
		t.Fatalf("Expected file to not exist after delete, exists=%v", existsAfter)
	}
}

func TestDiskStorage_PathTraversalProtection(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "gostore-test-traversal-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	storage, _ := NewDiskStorage(tempDir)
	ctx := context.Background()

	// Try escaping via path traversal
	_, _, _, err = storage.Save(ctx, "test-bucket", "../../evil.sh", strings.NewReader("malicious"))
	if err == nil {
		outsidePath := filepath.Join(tempDir, "..", "evil.sh")
		if _, err := os.Stat(outsidePath); !os.IsNotExist(err) {
			t.Errorf("Security vulnerability: File escaped base storage path!")
		}
	}
}
