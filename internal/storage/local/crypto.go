package local

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"gostore/internal/core/ports"
)

const (
	EncryptedHeaderMagic = "GSENC01\x00" // 8 bytes magic & version identifier
	HeaderSize           = 24           // 8 bytes magic + 16 bytes IV
)

type CipherEngine struct {
	key   []byte
	block cipher.Block
}

func NewCipherEngine(secretKey string) (*CipherEngine, error) {
	if secretKey == "" {
		secretKey = "gostore-default-at-rest-master-key-256"
	}
	hash := sha256.Sum256([]byte(secretKey))
	block, err := aes.NewCipher(hash[:])
	if err != nil {
		return nil, fmt.Errorf("failed to initialize AES block cipher: %w", err)
	}

	return &CipherEngine{
		key:   hash[:],
		block: block,
	}, nil
}

// EncryptStream writes the encryption header (magic + random IV) to dst and streams AES-CTR encrypted ciphertext
func (c *CipherEngine) EncryptStream(dst io.Writer, src io.Reader) (int64, error) {
	if c == nil || c.block == nil {
		return io.Copy(dst, src)
	}

	// Generate 16-byte random cryptographic IV
	iv := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return 0, fmt.Errorf("failed to generate encryption IV: %w", err)
	}

	// Write magic header + IV (24 bytes)
	if _, err := dst.Write([]byte(EncryptedHeaderMagic)); err != nil {
		return 0, fmt.Errorf("failed to write encrypted magic header: %w", err)
	}
	if _, err := dst.Write(iv); err != nil {
		return 0, fmt.Errorf("failed to write encryption IV: %w", err)
	}

	stream := cipher.NewCTR(c.block, iv)
	writer := &cipher.StreamWriter{S: stream, W: dst}

	return io.Copy(writer, src)
}

// EncryptBytes encrypts a byte slice in memory
func (c *CipherEngine) EncryptBytes(plaintext []byte) ([]byte, error) {
	if c == nil || c.block == nil {
		return plaintext, nil
	}

	buf := bytes.NewBuffer(make([]byte, 0, HeaderSize+len(plaintext)))
	_, err := c.EncryptStream(buf, bytes.NewReader(plaintext))
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// DecryptBytes decrypts a byte slice in memory
func (c *CipherEngine) DecryptBytes(data []byte) ([]byte, error) {
	if c == nil || c.block == nil || len(data) < HeaderSize {
		return data, nil
	}

	if !bytes.HasPrefix(data, []byte(EncryptedHeaderMagic)) {
		return data, nil // unencrypted data
	}

	iv := data[len(EncryptedHeaderMagic):HeaderSize]
	ciphertext := data[HeaderSize:]

	stream := cipher.NewCTR(c.block, iv)
	plaintext := make([]byte, len(ciphertext))
	stream.XORKeyStream(plaintext, ciphertext)

	return plaintext, nil
}

// DecryptedReadSeekCloser wraps an encrypted file on disk and exposes transparent plaintext Read/Seek/Close
type DecryptedReadSeekCloser struct {
	file          *os.File
	block         cipher.Block
	baseIV        []byte
	plaintextSize int64
	currentOffset int64
	stream        cipher.Stream
	isEncrypted   bool
	mu            sync.Mutex
}

// NewDecryptedReadSeekCloser inspects the file header and returns a transparent ReadSeekCloser with plaintext size
func NewDecryptedReadSeekCloser(file *os.File, engine *CipherEngine) (ports.ReadSeekCloser, int64, error) {
	stat, err := file.Stat()
	if err != nil {
		return nil, 0, err
	}

	fileSize := stat.Size()
	if fileSize < HeaderSize {
		// Too short to have encryption header; treat as unencrypted
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return nil, 0, err
		}
		return file, fileSize, nil
	}

	header := make([]byte, HeaderSize)
	if _, err := io.ReadFull(file, header); err != nil {
		_, _ = file.Seek(0, io.SeekStart)
		return file, fileSize, nil
	}

	if !bytes.HasPrefix(header, []byte(EncryptedHeaderMagic)) || engine == nil || engine.block == nil {
		// Not encrypted with GoStore header; rewind and return standard file
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return nil, 0, err
		}
		return file, fileSize, nil
	}

	iv := header[len(EncryptedHeaderMagic):HeaderSize]
	baseIV := make([]byte, 16)
	copy(baseIV, iv)

	plaintextSize := fileSize - int64(HeaderSize)
	if plaintextSize < 0 {
		plaintextSize = 0
	}

	r := &DecryptedReadSeekCloser{
		file:          file,
		block:         engine.block,
		baseIV:        baseIV,
		plaintextSize: plaintextSize,
		currentOffset: 0,
		isEncrypted:   true,
	}

	if err := r.resetStream(0); err != nil {
		return nil, 0, err
	}

	return r, plaintextSize, nil
}

func (r *DecryptedReadSeekCloser) resetStream(targetOffset int64) error {
	if !r.isEncrypted {
		_, err := r.file.Seek(targetOffset, io.SeekStart)
		return err
	}

	if targetOffset > r.plaintextSize {
		targetOffset = r.plaintextSize
	}

	blockIndex := uint64(targetOffset / 16)
	subOffset := int(targetOffset % 16)

	iv := addCounter(r.baseIV, blockIndex)
	r.stream = cipher.NewCTR(r.block, iv)

	if subOffset > 0 {
		var dummy [16]byte
		r.stream.XORKeyStream(dummy[:subOffset], dummy[:subOffset])
	}

	_, err := r.file.Seek(int64(HeaderSize)+targetOffset, io.SeekStart)
	return err
}

func (r *DecryptedReadSeekCloser) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.isEncrypted {
		return r.file.Read(p)
	}

	if r.currentOffset >= r.plaintextSize {
		return 0, io.EOF
	}

	remaining := r.plaintextSize - r.currentOffset
	toRead := len(p)
	if int64(toRead) > remaining {
		toRead = int(remaining)
	}

	n, err := r.file.Read(p[:toRead])
	if n > 0 {
		r.stream.XORKeyStream(p[:n], p[:n])
		r.currentOffset += int64(n)
	}
	if err == io.EOF && r.currentOffset < r.plaintextSize {
		return n, io.ErrUnexpectedEOF
	}
	return n, err
}

func (r *DecryptedReadSeekCloser) Seek(offset int64, whence int) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var target int64
	switch whence {
	case io.SeekStart:
		target = offset
	case io.SeekCurrent:
		target = r.currentOffset + offset
	case io.SeekEnd:
		target = r.plaintextSize + offset
	default:
		return 0, errors.New("invalid whence")
	}

	if target < 0 {
		return 0, errors.New("negative seek offset")
	}

	if err := r.resetStream(target); err != nil {
		return 0, err
	}
	r.currentOffset = target
	return target, nil
}

func (r *DecryptedReadSeekCloser) Close() error {
	return r.file.Close()
}

// addCounter adds a 64-bit block count to the 128-bit big-endian IV for AES-CTR seeking
func addCounter(baseIV []byte, blocks uint64) []byte {
	iv := make([]byte, 16)
	copy(iv, baseIV)
	carry := blocks
	for i := 15; i >= 0; i-- {
		sum := uint64(iv[i]) + (carry & 0xFF)
		iv[i] = byte(sum)
		carry = (carry >> 8) + (sum >> 8)
		if carry == 0 && i < 8 {
			break
		}
	}
	return iv
}

// EncryptFileInPlace encrypts a plaintext file in place on disk atomically
func EncryptFileInPlace(filePath string, engine *CipherEngine) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return nil
	}

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}

	// Check if already encrypted
	header := make([]byte, len(EncryptedHeaderMagic))
	n, _ := io.ReadFull(file, header)
	if n == len(EncryptedHeaderMagic) && string(header) == EncryptedHeaderMagic {
		file.Close()
		return nil // already encrypted
	}

	// Rewind to start
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		file.Close()
		return err
	}

	dir := filepath.Dir(filePath)
	tmpFile, err := os.CreateTemp(dir, ".encrypt-*.tmp")
	if err != nil {
		file.Close()
		return err
	}
	tmpPath := tmpFile.Name()

	if _, err := engine.EncryptStream(tmpFile, file); err != nil {
		file.Close()
		tmpFile.Close()
		os.Remove(tmpPath)
		return err
	}

	file.Close()
	tmpFile.Close()

	return os.Rename(tmpPath, filePath)
}
