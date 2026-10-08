package service

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"path/filepath"

	"gostore/internal/core/ports"
	"gostore/internal/repository/sqlite"
)

type ZipStreamer struct {
	storage  ports.StorageDriver
	fileRepo *sqlite.FileRepository
}

func NewZipStreamer(storage ports.StorageDriver, fileRepo *sqlite.FileRepository) *ZipStreamer {
	return &ZipStreamer{
		storage:  storage,
		fileRepo: fileRepo,
	}
}

// StreamZip streams a dynamic ZIP archive directly to the client
func (z *ZipStreamer) StreamZip(ctx context.Context, bucket string, filePaths []string, targetWriter io.Writer) error {
	zipWriter := zip.NewWriter(targetWriter)
	defer zipWriter.Close()

	for _, path := range filePaths {
		fileObj, err := z.fileRepo.GetByPath(ctx, bucket, path)
		if err != nil || fileObj == nil {
			continue // skip missing files
		}

		reader, _, err := z.storage.Open(ctx, bucket, path)
		if err != nil {
			continue
		}

		header := &zip.FileHeader{
			Name:     filepath.ToSlash(fileObj.Path),
			Method:   zip.Deflate,
			Modified: fileObj.UpdatedAt,
		}

		entryWriter, err := zipWriter.CreateHeader(header)
		if err != nil {
			reader.Close()
			return fmt.Errorf("failed to create zip entry for %s: %w", path, err)
		}

		_, copyErr := io.Copy(entryWriter, reader)
		reader.Close()
		if copyErr != nil {
			return fmt.Errorf("failed to write zip file %s: %w", path, copyErr)
		}
	}

	return zipWriter.Close()
}
