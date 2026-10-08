package proleadfile

import (
	"io"
	"sync/atomic"
)

// ProgressCallback receives the number of bytes transferred, total expected bytes, and percentage.
type ProgressCallback func(transferred int64, total int64, percent float64)

// ProgressReader wraps an io.Reader and reports transfer progress.
type ProgressReader struct {
	reader      io.Reader
	totalBytes  int64
	transferred int64
	callback    ProgressCallback
}

// NewProgressReader constructs a new streaming ProgressReader.
func NewProgressReader(reader io.Reader, totalBytes int64, callback ProgressCallback) *ProgressReader {
	return &ProgressReader{
		reader:     reader,
		totalBytes: totalBytes,
		callback:   callback,
	}
}

func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.reader.Read(p)
	if n > 0 {
		curr := atomic.AddInt64(&pr.transferred, int64(n))
		if pr.callback != nil {
			var percent float64
			if pr.totalBytes > 0 {
				percent = float64(curr) / float64(pr.totalBytes) * 100.0
				if percent > 100.0 {
					percent = 100.0
				}
			}
			pr.callback(curr, pr.totalBytes, percent)
		}
	}
	return n, err
}

// ProgressWriter wraps an io.Writer and reports transfer progress.
type ProgressWriter struct {
	writer      io.Writer
	totalBytes  int64
	transferred int64
	callback    ProgressCallback
}

// NewProgressWriter constructs a new ProgressWriter.
func NewProgressWriter(writer io.Writer, totalBytes int64, callback ProgressCallback) *ProgressWriter {
	return &ProgressWriter{
		writer:     writer,
		totalBytes: totalBytes,
		callback:   callback,
	}
}

func (pw *ProgressWriter) Write(p []byte) (int, error) {
	n, err := pw.writer.Write(p)
	if n > 0 {
		curr := atomic.AddInt64(&pw.transferred, int64(n))
		if pw.callback != nil {
			var percent float64
			if pw.totalBytes > 0 {
				percent = float64(curr) / float64(pw.totalBytes) * 100.0
				if percent > 100.0 {
					percent = 100.0
				}
			}
			pw.callback(curr, pw.totalBytes, percent)
		}
	}
	return n, err
}
