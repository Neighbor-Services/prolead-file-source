package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/disintegration/imaging"
)

type ImageTransformOptions struct {
	Width   int
	Height  int
	Fit     string // "cover", "contain", "fill", "scale"
	Format  string // "jpeg", "png", "webp", "gif"
	Quality int    // 1 to 100
}

type ImageProcessor struct {
	cacheDir string
}

func NewImageProcessor(storageBasePath string) *ImageProcessor {
	cacheDir := filepath.Join(storageBasePath, ".cache", "images")
	_ = os.MkdirAll(cacheDir, 0755)
	return &ImageProcessor{cacheDir: cacheDir}
}

// GenerateLQIP generates an ultra-compact Base64 Data URI thumbnail for instant UI blur-up
func (p *ImageProcessor) GenerateLQIP(src io.Reader) (string, error) {
	img, _, err := image.Decode(src)
	if err != nil {
		return "", err
	}

	// Downscale to 16x16 low quality placeholder
	lqipImg := imaging.Resize(img, 16, 16, imaging.Lanczos)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, lqipImg, &jpeg.Options{Quality: 30}); err != nil {
		return "", err
	}

	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
	return "data:image/jpeg;base64," + encoded, nil
}

// Process transforms an image stream with caching
func (p *ImageProcessor) Process(src io.Reader, fileKey string, opts ImageTransformOptions) ([]byte, string, error) {
	if opts.Width <= 0 && opts.Height <= 0 && opts.Format == "" {
		// No transform requested
		data, err := io.ReadAll(src)
		return data, "", err
	}

	// Calculate cache key
	cacheKey := fmt.Sprintf("%s_w%d_h%d_f%s_fmt%s_q%d", fileKey, opts.Width, opts.Height, opts.Fit, opts.Format, opts.Quality)
	hash := sha256.Sum256([]byte(cacheKey))
	hashStr := hex.EncodeToString(hash[:])
	cachedFile := filepath.Join(p.cacheDir, hashStr)

	// Check if cached variant exists
	if cachedData, err := os.ReadFile(cachedFile); err == nil {
		mimeType := detectImageMime(opts.Format)
		return cachedData, mimeType, nil
	}

	// Decode source image
	img, originalFormat, err := image.Decode(src)
	if err != nil {
		return nil, "", fmt.Errorf("failed to decode image: %w", err)
	}

	// Apply Transformations
	var transformed image.Image = img
	w := opts.Width
	h := opts.Height

	if w > 0 || h > 0 {
		switch strings.ToLower(opts.Fit) {
		case "cover", "crop":
			if w > 0 && h > 0 {
				transformed = imaging.Fill(img, w, h, imaging.Center, imaging.Lanczos)
			} else {
				transformed = imaging.Resize(img, w, h, imaging.Lanczos)
			}
		case "contain", "fit":
			transformed = imaging.Fit(img, w, h, imaging.Lanczos)
		default: // "scale" or proportional resize
			if w > 0 && h > 0 {
				transformed = imaging.Resize(img, w, h, imaging.Lanczos)
			} else if w > 0 {
				transformed = imaging.Resize(img, w, 0, imaging.Lanczos)
			} else {
				transformed = imaging.Resize(img, 0, h, imaging.Lanczos)
			}
		}
	}

	targetFormat := opts.Format
	if targetFormat == "" {
		targetFormat = originalFormat
	}
	targetFormat = strings.ToLower(targetFormat)

	outBuf := &bytes.Buffer{}
	var outMime string

	quality := opts.Quality
	if quality <= 0 || quality > 100 {
		quality = 85
	}

	switch targetFormat {
	case "jpeg", "jpg":
		outMime = "image/jpeg"
		err = jpeg.Encode(outBuf, transformed, &jpeg.Options{Quality: quality})
	case "png":
		outMime = "image/png"
		err = png.Encode(outBuf, transformed)
	case "gif":
		outMime = "image/gif"
		err = gif.Encode(outBuf, transformed, nil)
	default: // fallback to jpeg
		outMime = "image/jpeg"
		err = jpeg.Encode(outBuf, transformed, &jpeg.Options{Quality: quality})
	}

	if err != nil {
		return nil, "", fmt.Errorf("failed to encode transformed image: %w", err)
	}

	data := outBuf.Bytes()
	// Write to disk cache
	_ = os.WriteFile(cachedFile, data, 0644)

	return data, outMime, nil
}

func detectImageMime(format string) string {
	switch strings.ToLower(format) {
	case "jpeg", "jpg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	default:
		return "image/jpeg"
	}
}
