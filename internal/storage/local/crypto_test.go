package local

import (
	"bytes"
	"crypto/rand"
	"io"
	"os"
	"testing"
)

func TestCipherEngine_EncryptAndSeek(t *testing.T) {
	engine, err := NewCipherEngine("test-secret-key-12345")
	if err != nil {
		t.Fatalf("Failed to create cipher engine: %v", err)
	}

	// 1. Generate 100 KB random plaintext
	plaintext := make([]byte, 100*1024)
	if _, err := io.ReadFull(rand.Reader, plaintext); err != nil {
		t.Fatal(err)
	}

	// 2. Encrypt to a temporary file
	tmpFile, err := os.CreateTemp("", "gostore-enc-test-*.bin")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	written, err := engine.EncryptStream(tmpFile, bytes.NewReader(plaintext))
	if err != nil {
		t.Fatalf("EncryptStream failed: %v", err)
	}
	if written != int64(len(plaintext)) {
		t.Fatalf("Expected %d bytes encrypted, got %d", len(plaintext), written)
	}

	// Re-open for decrypted reading & seeking
	file, err := os.Open(tmpFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	reader, plaintextSize, err := NewDecryptedReadSeekCloser(file, engine)
	if err != nil {
		t.Fatalf("NewDecryptedReadSeekCloser failed: %v", err)
	}
	defer reader.Close()

	if plaintextSize != int64(len(plaintext)) {
		t.Fatalf("Expected plaintextSize %d, got %d", len(plaintext), plaintextSize)
	}

	// 3. Test Full Sequential Read
	decryptedFull, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if !bytes.Equal(plaintext, decryptedFull) {
		t.Fatal("Decrypted full content does not match original plaintext!")
	}

	// 4. Test Random Seeks (e.g. non-16-byte boundaries)
	seekOffsets := []int64{0, 1, 15, 16, 17, 31, 32, 100, 1023, 1024, 1025, 50000, 99990, 102400 - 10}
	for _, offset := range seekOffsets {
		newPos, err := reader.Seek(offset, io.SeekStart)
		if err != nil {
			t.Fatalf("Seek to %d failed: %v", offset, err)
		}
		if newPos != offset {
			t.Fatalf("Expected seek position %d, got %d", offset, newPos)
		}

		// Read 256 bytes from this position
		buf := make([]byte, 256)
		n, err := reader.Read(buf)
		if err != nil && err != io.EOF {
			t.Fatalf("Read at offset %d failed: %v", offset, err)
		}

		expected := plaintext[offset : offset+int64(n)]
		if !bytes.Equal(buf[:n], expected) {
			t.Fatalf("Decrypted slice at offset %d (length %d) mismatch!", offset, n)
		}
	}

	// 5. Test SeekEnd
	newPos, err := reader.Seek(-100, io.SeekEnd)
	if err != nil {
		t.Fatalf("Seek from end failed: %v", err)
	}
	if newPos != int64(len(plaintext)-100) {
		t.Fatalf("Expected pos %d, got %d", len(plaintext)-100, newPos)
	}

	tailBuf := make([]byte, 100)
	n, err := io.ReadFull(reader, tailBuf)
	if err != nil || n != 100 {
		t.Fatalf("Read tail failed: n=%d, err=%v", n, err)
	}
	if !bytes.Equal(tailBuf, plaintext[len(plaintext)-100:]) {
		t.Fatal("Tail bytes mismatch!")
	}
}

func TestCipherEngine_InPlaceEncryption(t *testing.T) {
	engine, err := NewCipherEngine("master-test-key")
	if err != nil {
		t.Fatal(err)
	}

	testData := []byte("Sensitive data that must be encrypted on disk!")
	tmpFile, err := os.CreateTemp("", "gostore-inplace-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.Write(testData); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	// Encrypt in place
	if err := EncryptFileInPlace(tmpPath, engine); err != nil {
		t.Fatalf("EncryptFileInPlace failed: %v", err)
	}

	// Raw bytes on disk should be ciphertext with header
	rawDiskBytes, err := os.ReadFile(tmpPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(rawDiskBytes, testData) {
		t.Fatal("File on disk was NOT encrypted!")
	}
	if !bytes.HasPrefix(rawDiskBytes, []byte(EncryptedHeaderMagic)) {
		t.Fatal("File on disk is missing encrypted magic header!")
	}

	// Open transparently via DecryptedReadSeekCloser
	f, err := os.Open(tmpPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	reader, size, err := NewDecryptedReadSeekCloser(f, engine)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()

	if size != int64(len(testData)) {
		t.Fatalf("Expected size %d, got %d", len(testData), size)
	}

	decrypted, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decrypted, testData) {
		t.Fatalf("Decrypted content mismatch: got %q, expected %q", string(decrypted), string(testData))
	}
}
