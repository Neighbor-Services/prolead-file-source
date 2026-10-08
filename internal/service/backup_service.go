package service

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gostore/internal/repository/sqlite"
)

type BackupService struct {
	dbPath          string
	storageBasePath string
	db              *sqlite.DB
}

func NewBackupService(dbPath string, storageBasePath string, db *sqlite.DB) *BackupService {
	return &BackupService{
		dbPath:          dbPath,
		storageBasePath: storageBasePath,
		db:              db,
	}
}

// StreamBackup streams a complete gzipped tarball archive containing SQLite snapshot and storage metadata
func (s *BackupService) StreamBackup(w io.Writer) error {
	gw := gzip.NewWriter(w)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	// 1. Create a safe online SQLite snapshot via VACUUM INTO in temp file
	tempSnapshot := filepath.Join(os.TempDir(), fmt.Sprintf("gostore-snap-%d.db", time.Now().UnixNano()))
	defer os.Remove(tempSnapshot)

	if err := s.db.Exec(fmt.Sprintf("VACUUM INTO '%s'", tempSnapshot)).Error; err != nil {
		// Fallback: If VACUUM INTO fails, read source db directly with WAL checkpoint
		_ = s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error
		if err := copyFile(s.dbPath, tempSnapshot); err != nil {
			return fmt.Errorf("failed to create database snapshot: %w", err)
		}
	}

	// 2. Add sqlite snapshot to tar
	snapFile, err := os.Open(tempSnapshot)
	if err != nil {
		return fmt.Errorf("failed to open snapshot: %w", err)
	}
	defer snapFile.Close()

	snapInfo, err := snapFile.Stat()
	if err != nil {
		return err
	}

	snapHdr := &tar.Header{
		Name:    "gostore.db",
		Size:    snapInfo.Size(),
		Mode:    0644,
		ModTime: snapInfo.ModTime(),
	}
	if err := tw.WriteHeader(snapHdr); err != nil {
		return err
	}
	if _, err := io.Copy(tw, snapFile); err != nil {
		return err
	}

	// 3. Add storage blobs to tar archive
	blobDir := filepath.Join(s.storageBasePath, ".blobs")
	_ = filepath.WalkDir(blobDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(s.storageBasePath, path)
		if err != nil {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer file.Close()

		hdr := &tar.Header{
			Name:    filepath.ToSlash(relPath),
			Size:    info.Size(),
			Mode:    0644,
			ModTime: info.ModTime(),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		_, _ = io.Copy(tw, file)
		return nil
	})

	return nil
}

// RestoreBackup unpacks a backup tar.gz archive and restores metadata & CAS blobs
func (s *BackupService) RestoreBackup(r io.Reader) error {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("invalid gzip backup archive: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	restoredDBPath := ""

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read error: %w", err)
		}

		cleanName := filepath.Clean(hdr.Name)
		if strings.HasPrefix(cleanName, "..") {
			return errors.New("invalid path traversal in backup archive")
		}

		if cleanName == "gostore.db" {
			tempDB := filepath.Join(os.TempDir(), fmt.Sprintf("restore-%d.db", time.Now().UnixNano()))
			out, err := os.Create(tempDB)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
			restoredDBPath = tempDB
			continue
		}

		if strings.HasPrefix(cleanName, ".blobs") {
			targetPath := filepath.Join(s.storageBasePath, cleanName)
			_ = os.MkdirAll(filepath.Dir(targetPath), 0755)
			out, err := os.Create(targetPath)
			if err != nil {
				return err
			}
			_, _ = io.Copy(out, tr)
			out.Close()
		}
	}

	if restoredDBPath != "" {
		defer os.Remove(restoredDBPath)
		// Checkpoint active DB and replace with restored DB
		_ = s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error
		if err := copyFile(restoredDBPath, s.dbPath); err != nil {
			return fmt.Errorf("failed to restore database file: %w", err)
		}
	}

	return nil
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
