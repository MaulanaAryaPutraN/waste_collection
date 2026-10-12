package storage

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

type FileStore interface {
	Save(reader io.Reader, contentType string) (string, error)
	Remove(urlPath string) error
}

type Local struct{ dir string }

func NewLocal(dir string) (*Local, error) {
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("create upload directory: %w", err)
	}
	return &Local{dir: dir}, nil
}

func (s *Local) Save(reader io.Reader, contentType string) (string, error) {
	ext := ""
	switch contentType {
	case "image/jpeg":
		ext = ".jpg"
	case "image/png":
		ext = ".png"
	case "application/pdf":
		ext = ".pdf"
	default:
		return "", fmt.Errorf("unsupported proof content type")
	}
	name := uuid.NewString() + ext
	file, err := os.OpenFile(filepath.Join(s.dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0640)
	if err != nil {
		return "", fmt.Errorf("create proof file: %w", err)
	}
	if _, err := io.Copy(file, reader); err != nil {
		_ = file.Close()
		_ = os.Remove(filepath.Join(s.dir, name))
		return "", fmt.Errorf("write proof file: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(filepath.Join(s.dir, name))
		return "", fmt.Errorf("close proof file: %w", err)
	}
	return "/uploads/" + name, nil
}

func (s *Local) Remove(urlPath string) error {
	name := filepath.Base(urlPath)
	if name == "." || strings.Contains(name, "..") {
		return fmt.Errorf("invalid stored file path")
	}
	err := os.Remove(filepath.Join(s.dir, name))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove proof file: %w", err)
	}
	return nil
}

func DetectType(data []byte) string { return http.DetectContentType(data) }
