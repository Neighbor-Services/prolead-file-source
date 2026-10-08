package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"gostore/internal/core/domain"
)

type ChunkSession struct {
	ID          string            `json:"uploadId"`
	Bucket      string            `json:"bucket"`
	Path        string            `json:"path"`
	TotalSize   int64             `json:"totalSize"`
	ContentType string            `json:"contentType"`
	IsPublic    bool              `json:"isPublic"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	BytesSoFar  int64             `json:"bytesReceived"`
	TempPath    string            `json:"-"`
	CreatedAt   time.Time         `json:"createdAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
}

type ChunkService struct {
	mu             sync.RWMutex
	sessions       map[string]*ChunkSession
	tempDir        string
	storageService *StorageService
}

func NewChunkService(storageBasePath string, storageService *StorageService) *ChunkService {
	tempDir := filepath.Join(storageBasePath, ".tmp-chunks")
	_ = os.MkdirAll(tempDir, 0755)

	cs := &ChunkService{
		sessions:       make(map[string]*ChunkSession),
		tempDir:        tempDir,
		storageService: storageService,
	}

	cs.StartPruneWorker(30*time.Minute, 24*time.Hour)
	return cs
}

func (c *ChunkService) StartPruneWorker(interval time.Duration, maxAge time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			c.PruneStaleUploads(maxAge)
		}
	}()
}

func (c *ChunkService) PruneStaleUploads(maxAge time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	cutoff := time.Now().UTC().Add(-maxAge)
	for id, session := range c.sessions {
		if session.UpdatedAt.Before(cutoff) {
			_ = os.Remove(session.TempPath)
			delete(c.sessions, id)
			log.Printf("🧹 Pruned stale chunk upload session: %s", id)
		}
	}
}

func (c *ChunkService) InitUpload(bucket, path, contentType string, totalSize int64, isPublic bool, metadata map[string]string) (*ChunkSession, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	uploadID := "upl-" + uuid.New().String()
	tempFile := filepath.Join(c.tempDir, uploadID+".part")

	file, err := os.Create(tempFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create upload session: %w", err)
	}
	file.Close()

	session := &ChunkSession{
		ID:          uploadID,
		Bucket:      bucket,
		Path:        path,
		TotalSize:   totalSize,
		ContentType: contentType,
		IsPublic:    isPublic,
		Metadata:    metadata,
		BytesSoFar:  0,
		TempPath:    tempFile,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	c.sessions[uploadID] = session
	return session, nil
}

func (c *ChunkService) GetSession(uploadID string) (*ChunkSession, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	session, exists := c.sessions[uploadID]
	if !exists {
		return nil, false
	}
	cp := *session
	return &cp, true
}

func (c *ChunkService) AppendChunk(uploadID string, offset int64, reader io.Reader) (int64, error) {
	c.mu.Lock()
	session, exists := c.sessions[uploadID]
	c.mu.Unlock()

	if !exists {
		return 0, errors.New("invalid or expired upload session")
	}

	file, err := os.OpenFile(session.TempPath, os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return 0, fmt.Errorf("failed to seek chunk offset: %w", err)
	}

	written, err := io.Copy(file, reader)
	if err != nil {
		return 0, fmt.Errorf("failed to write chunk: %w", err)
	}

	c.mu.Lock()
	session.BytesSoFar += written
	session.UpdatedAt = time.Now().UTC()
	currentTotal := session.BytesSoFar
	c.mu.Unlock()

	return currentTotal, nil
}

func (c *ChunkService) AbortUpload(uploadID string) error {
	c.mu.Lock()
	session, exists := c.sessions[uploadID]
	if exists {
		delete(c.sessions, uploadID)
	}
	c.mu.Unlock()

	if !exists {
		return errors.New("upload session not found")
	}

	_ = os.Remove(session.TempPath)
	return nil
}

func (c *ChunkService) CompleteUpload(ctx context.Context, uploadID string) (*domain.FileObject, error) {
	c.mu.Lock()
	session, exists := c.sessions[uploadID]
	if exists {
		delete(c.sessions, uploadID)
	}
	c.mu.Unlock()

	if !exists {
		return nil, errors.New("invalid or expired upload session")
	}

	file, err := os.Open(session.TempPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open finalized chunk file: %w", err)
	}
	defer func() {
		file.Close()
		os.Remove(session.TempPath)
	}()

	// Feed to StorageService
	uploadInput := UploadInput{
		Bucket:      session.Bucket,
		Path:        session.Path,
		ContentType: session.ContentType,
		IsPublic:    &session.IsPublic,
		Metadata:    session.Metadata,
		Reader:      file,
	}

	return c.storageService.Upload(ctx, uploadInput)
}
