package local

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gostore/internal/core/ports"
)

type DiskStorage struct {
	basePath string
	blobPath string
	cipher   *CipherEngine
	cache    *MemoryCache
}

func NewDiskStorage(basePath string, key ...string) (*DiskStorage, error) {
	absPath, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve storage path: %w", err)
	}

	blobDir := filepath.Join(absPath, ".blobs")
	if err := os.MkdirAll(blobDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create blobs directory: %w", err)
	}

	var secretKey string
	if len(key) > 0 && key[0] != "" {
		secretKey = key[0]
	} else {
		secretKey = "gostore-default-at-rest-master-key-256"
	}

	cipherEngine, err := NewCipherEngine(secretKey)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize encryption cipher engine: %w", err)
	}

	ds := &DiskStorage{
		basePath: absPath,
		blobPath: blobDir,
		cipher:   cipherEngine,
		cache:    NewMemoryCache(64), // 64 MB LRU Hot File Cache (Plaintext)
	}

	// Encrypt any existing legacy plaintext files found in storage on startup
	_ = ds.EnsureStorageEncrypted()

	return ds, nil
}

// GetCipherEngine returns the underlying encryption cipher engine
func (d *DiskStorage) GetCipherEngine() *CipherEngine {
	return d.cipher
}

// EnsureStorageEncrypted scans existing files on disk and encrypts any unencrypted files in place
func (d *DiskStorage) EnsureStorageEncrypted() error {
	return filepath.WalkDir(d.basePath, func(path string, de fs.DirEntry, err error) error {
		if err != nil || de.IsDir() {
			return nil
		}
		name := de.Name()
		if strings.HasPrefix(name, ".") || strings.HasSuffix(name, ".tmp") {
			return nil
		}
		// Encrypt in place if unencrypted
		_ = EncryptFileInPlace(path, d.cipher)
		return nil
	})
}

func (d *DiskStorage) resolvePath(bucket, objectPath string) (string, error) {
	cleanBucket := filepath.Clean(bucket)
	if strings.Contains(cleanBucket, "..") || cleanBucket == "." || cleanBucket == "/" {
		return "", errors.New("invalid bucket name")
	}

	cleanObj := filepath.Clean("/" + strings.TrimPrefix(objectPath, "/"))
	fullPath := filepath.Join(d.basePath, cleanBucket, cleanObj)

	expectedPrefix := filepath.Join(d.basePath, cleanBucket)
	if !strings.HasPrefix(fullPath, expectedPrefix) {
		return "", errors.New("path traversal attempt detected")
	}

	return fullPath, nil
}

func SniffContentType(reader io.Reader) (string, io.Reader, error) {
	buf := make([]byte, 512)
	n, err := io.ReadFull(reader, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "application/octet-stream", reader, err
	}

	detected := http.DetectContentType(buf[:n])
	combinedReader := io.MultiReader(bytes.NewReader(buf[:n]), reader)
	return detected, combinedReader, nil
}

func (d *DiskStorage) Save(ctx context.Context, bucket, objectPath string, reader io.Reader) (int64, string, string, error) {
	finalPath, err := d.resolvePath(bucket, objectPath)
	if err != nil {
		return 0, "", "", err
	}

	dir := filepath.Dir(finalPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return 0, "", "", fmt.Errorf("failed to create directory: %w", err)
	}

	tmpFile, err := os.CreateTemp(d.blobPath, ".upload-*.tmp")
	if err != nil {
		return 0, "", "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	var successful bool
	defer func() {
		tmpFile.Close()
		if !successful {
			os.Remove(tmpPath)
		}
	}()

	md5Hasher := md5.New()
	sha256Hasher := sha256.New()
	// MultiWriter computes hashes on the PLAINTEXT stream
	hashWriter := io.MultiWriter(md5Hasher, sha256Hasher)
	teeReader := io.TeeReader(reader, hashWriter)

	// Stream plaintext through TeeReader while writing AES-256-CTR ciphertext to disk
	written, err := d.cipher.EncryptStream(tmpFile, teeReader)
	if err != nil {
		return 0, "", "", fmt.Errorf("failed to stream encrypted file to disk: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return 0, "", "", fmt.Errorf("failed to close temp file: %w", err)
	}

	sha256Str := hex.EncodeToString(sha256Hasher.Sum(nil))
	md5Str := hex.EncodeToString(md5Hasher.Sum(nil))

	blobSubDir := filepath.Join(d.blobPath, sha256Str[:2], sha256Str[2:4])
	if err := os.MkdirAll(blobSubDir, 0755); err != nil {
		return 0, "", "", err
	}
	canonicalBlobPath := filepath.Join(blobSubDir, sha256Str)

	if _, err := os.Stat(canonicalBlobPath); os.IsNotExist(err) {
		if err := os.Rename(tmpPath, canonicalBlobPath); err != nil {
			return 0, "", "", fmt.Errorf("failed to store blob: %w", err)
		}
		_ = os.Chmod(canonicalBlobPath, 0444)
	} else {
		os.Remove(tmpPath)
	}

	_ = os.Remove(finalPath)
	if err := os.Link(canonicalBlobPath, finalPath); err != nil {
		if err := copyFile(canonicalBlobPath, finalPath); err != nil {
			return 0, "", "", fmt.Errorf("failed to link blob: %w", err)
		}
	}

	// Invalidate memory cache key
	cacheKey := bucket + "/" + objectPath
	d.cache.Invalidate(cacheKey)

	successful = true
	return written, md5Str, sha256Str, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func (d *DiskStorage) Open(ctx context.Context, bucket, objectPath string) (ports.ReadSeekCloser, int64, error) {
	cacheKey := bucket + "/" + objectPath
	if data, found := d.cache.Get(cacheKey); found {
		return NewMemoryReadSeekCloser(data), int64(len(data)), nil
	}

	filePath, err := d.resolvePath(bucket, objectPath)
	if err != nil {
		return nil, 0, err
	}

	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, errors.New("file not found")
		}
		return nil, 0, err
	}

	stat, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, 0, err
	}

	if stat.IsDir() {
		file.Close()
		return nil, 0, errors.New("target is a directory, not a file")
	}

	// Wrap in DecryptedReadSeekCloser for transparent on-the-fly decryption and seekability
	reader, plaintextSize, err := NewDecryptedReadSeekCloser(file, d.cipher)
	if err != nil {
		file.Close()
		return nil, 0, err
	}

	// Populate plaintext cache if file is small (< 1MB)
	if plaintextSize <= 1024*1024 {
		data, err := io.ReadAll(reader)
		if err == nil {
			d.cache.Put(cacheKey, data)
			_ = reader.Close()
			return NewMemoryReadSeekCloser(data), plaintextSize, nil
		}
		_, _ = reader.Seek(0, io.SeekStart)
	}

	return reader, plaintextSize, nil
}

func (d *DiskStorage) Delete(ctx context.Context, bucket, objectPath string) error {
	cacheKey := bucket + "/" + objectPath
	d.cache.Invalidate(cacheKey)

	filePath, err := d.resolvePath(bucket, objectPath)
	if err != nil {
		return err
	}

	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	dir := filepath.Dir(filePath)
	bucketDir := filepath.Join(d.basePath, filepath.Clean(bucket))
	for dir != bucketDir && dir != d.basePath && dir != "/" && dir != "." {
		if err := os.Remove(dir); err != nil {
			break
		}
		dir = filepath.Dir(dir)
	}

	return nil
}

func (d *DiskStorage) Exists(ctx context.Context, bucket, objectPath string) (bool, error) {
	filePath, err := d.resolvePath(bucket, objectPath)
	if err != nil {
		return false, err
	}

	stat, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	return !stat.IsDir(), nil
}

func (d *DiskStorage) DeleteBucket(ctx context.Context, bucket string) error {
	bucketDir := filepath.Join(d.basePath, filepath.Clean(bucket))
	if !strings.HasPrefix(bucketDir, d.basePath) {
		return errors.New("invalid bucket path")
	}
	return os.RemoveAll(bucketDir)
}
