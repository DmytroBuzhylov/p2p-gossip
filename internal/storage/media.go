package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

type MediaService struct {
	basePath string
}

func NewMediaService(appDataPath string) (*MediaService, error) {
	mediaPath := filepath.Join(appDataPath, "media")
	subDirs := []string{"avatars", "files", "cache", "blocks"}

	for _, dir := range subDirs {
		err := os.MkdirAll(filepath.Join(mediaPath, dir), 0755)
		if err != nil {
			return nil, err
		}
	}
	return &MediaService{basePath: mediaPath}, nil
}

func (s *MediaService) StoreFile(reader io.Reader, category string) (string, error) {
	tempFile, err := os.CreateTemp(filepath.Join(s.basePath, "cache"), "upload_*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tempFile.Name())

	hash := sha256.New()

	multiWriter := io.MultiWriter(tempFile, hash)

	if _, err := io.Copy(multiWriter, reader); err != nil {
		return "", err
	}
	tempFile.Close()

	fileHash := hex.EncodeToString(hash.Sum(nil))

	finalPath := filepath.Join(s.basePath, category, fileHash)

	if _, err := os.Stat(finalPath); err == nil {
		return fileHash, nil
	}

	err = os.Rename(tempFile.Name(), finalPath)
	return fileHash, err
}

func (s *MediaService) ReadFile(name, category string) (io.ReadSeeker, error) {
	path := filepath.Join(s.basePath, category, name)
	return os.OpenFile(path, os.O_RDONLY, 0666)
}

func (s *MediaService) StoreBlock(hashHex string, data []byte) error {
	if len(hashHex) < 4 {
		return errors.New("invalid hash length")
	}
	shard1 := hashHex[:2]
	shard2 := hashHex[2:4]

	fileName := hashHex

	targetDir := filepath.Join(s.basePath, "blocks", shard1, shard2)
	finalPath := filepath.Join(targetDir, fileName)

	if _, err := os.Stat(finalPath); err == nil {
		return nil
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create block dir: %w", err)
	}

	tempFile, err := os.CreateTemp(filepath.Join(s.basePath, "cache"), "blk_*")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())

	if _, err := tempFile.Write(data); err != nil {
		tempFile.Close()
		return fmt.Errorf("failed to write data: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Rename(tempFile.Name(), finalPath); err != nil {
		return fmt.Errorf("failed to rename block: %w", err)
	}

	return nil
}

func (s *MediaService) ReadBlock(hashHex string) (io.ReadSeeker, error) {
	if len(hashHex) < 4 {
		return nil, errors.New("invalid hash length")
	}
	shard1 := hashHex[:2]
	shard2 := hashHex[2:4]
	fileName := hashHex

	targetDir := filepath.Join(s.basePath, "blocks", shard1, shard2)
	finalPath := filepath.Join(targetDir, fileName)

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create block dir: %w", err)
	}

	return os.OpenFile(finalPath, os.O_RDONLY, 0666)
}

func (s *MediaService) GetHandler() http.Handler {
	return http.FileServer(http.Dir(s.basePath))
}
