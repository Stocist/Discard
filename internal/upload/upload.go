package upload

import (
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Allowed MIME types.
var allowedTypes = map[string]string{
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
	"image/gif":       ".gif",
	"image/webp":      ".webp",
	"application/pdf": ".pdf",
	"text/plain":      ".txt",
	"application/zip": ".zip",
}

// convertibleImages are MIME types we convert to WebP via cwebp.
var convertibleImages = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
}

// IsImage returns true if the MIME type is an image type.
func IsImage(mimeType string) bool {
	return strings.HasPrefix(mimeType, "image/")
}

// Result holds metadata about a processed upload.
type Result struct {
	FilePath     string
	OriginalName string
	MimeType     string
	FileSize     int64
	Width        *int
	Height       *int
}

// MaxFileSize is the per-file upload limit.
const MaxFileSize = 10 << 20 // 10 MB

// ProcessFile saves an uploaded file to disk and optionally converts images to WebP.
// uploadDir is the root upload directory (e.g. "./uploads").
func ProcessFile(uploadDir string, fh *multipart.FileHeader) (*Result, error) {
	// Validate MIME type from the header.
	contentType := fh.Header.Get("Content-Type")
	// Normalize: take only the media type, strip parameters.
	if idx := strings.Index(contentType, ";"); idx != -1 {
		contentType = strings.TrimSpace(contentType[:idx])
	}

	ext, ok := allowedTypes[contentType]
	if !ok {
		return nil, fmt.Errorf("file type %q is not allowed", contentType)
	}

	if fh.Size > MaxFileSize {
		return nil, fmt.Errorf("file exceeds maximum size of %d bytes", MaxFileSize)
	}

	now := time.Now()
	subDir := filepath.Join("attachments", fmt.Sprintf("%d", now.Year()), fmt.Sprintf("%02d", now.Month()))
	fullDir := filepath.Join(uploadDir, subDir)
	if err := os.MkdirAll(fullDir, 0o755); err != nil {
		return nil, fmt.Errorf("create upload dir: %w", err)
	}

	fileID := uuid.New().String()
	diskName := fileID + ext
	diskPath := filepath.Join(fullDir, diskName)

	src, err := fh.Open()
	if err != nil {
		return nil, fmt.Errorf("open uploaded file: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(diskPath)
	if err != nil {
		return nil, fmt.Errorf("create dest file: %w", err)
	}
	defer dst.Close()

	written, err := io.Copy(dst, src)
	if err != nil {
		os.Remove(diskPath)
		return nil, fmt.Errorf("write file: %w", err)
	}
	dst.Close()

	result := &Result{
		OriginalName: fh.Filename,
		MimeType:     contentType,
		FileSize:     written,
	}

	// Convert to WebP if applicable.
	if convertibleImages[contentType] {
		webpName := fileID + ".webp"
		webpPath := filepath.Join(fullDir, webpName)

		cmd := exec.Command("cwebp", diskPath, "-o", webpPath)
		if err := cmd.Run(); err == nil {
			// Conversion succeeded — remove original, use WebP.
			os.Remove(diskPath)
			result.FilePath = filepath.Join(subDir, webpName)
			result.MimeType = "image/webp"
			if info, err := os.Stat(webpPath); err == nil {
				result.FileSize = info.Size()
			}
		} else {
			// cwebp not available or failed — keep original.
			result.FilePath = filepath.Join(subDir, diskName)
		}
	} else {
		result.FilePath = filepath.Join(subDir, diskName)
	}

	return result, nil
}
