package infra

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// LocalFileStorage 本地磁盘文件存储实现
type LocalFileStorage struct {
	baseDir  string // 文件存储根目录，如 ./uploads
	baseURL  string // 外部访问前缀，如 /uploads
}

// NewLocalFileStorage 创建本地文件存储实例
func NewLocalFileStorage(baseDir, baseURL string) *LocalFileStorage {
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		os.MkdirAll(baseDir, 0755)
	}
	return &LocalFileStorage{baseDir: baseDir, baseURL: baseURL}
}

// Save 保存文件到本地磁盘，按日期分目录
func (s *LocalFileStorage) Save(ctx context.Context, fileName string, mimeType string, reader io.Reader) (*UploadedFile, error) {
	// 生成唯一文件名
	fileID := uuid.New().String()
	now := time.Now()
	ext := filepath.Ext(fileName)
	if ext == "" {
		ext = mimeToExt(mimeType)
	}
	savedName := fileID + ext

	// 按日期分目录: uploads/2026/07/04/
	dateDir := filepath.Join(s.baseDir, now.Format("2006/01/02"))
	if err := os.MkdirAll(dateDir, 0755); err != nil {
		return nil, fmt.Errorf("创建存储目录失败: %w", err)
	}

	fullPath := filepath.Join(dateDir, savedName)
	f, err := os.Create(fullPath)
	if err != nil {
		return nil, fmt.Errorf("创建文件失败: %w", err)
	}
	defer f.Close()

	written, err := io.Copy(f, reader)
	if err != nil {
		os.Remove(fullPath) // 写入失败清理半截文件
		return nil, fmt.Errorf("写入文件失败: %w", err)
	}

	relativePath := filepath.Join(now.Format("2006/01/02"), savedName)

	return &UploadedFile{
		FileID:    fileID,
		FileName:  fileName,
		MimeType:  mimeType,
		Size:      written,
		URL:       s.baseURL + "/" + relativePath,
		Path:      relativePath,
		CreatedAt: now.Format(time.RFC3339),
	}, nil
}

// Get 根据文件 ID 读取文件内容
func (s *LocalFileStorage) Get(_ context.Context, fileID string) (io.ReadCloser, error) {
	path := filepath.Join(s.baseDir, fileID)
	return os.Open(path)
}

// Delete 删除本地文件
func (s *LocalFileStorage) Delete(_ context.Context, fileID string) error {
	path := filepath.Join(s.baseDir, fileID)
	return os.Remove(path)
}

// URL 返回文件访问 URL
func (s *LocalFileStorage) URL(_ context.Context, fileID string) string {
	return s.baseURL + "/" + fileID
}

// mimeToExt 根据 MIME 类型推断文件扩展名
func mimeToExt(mimeType string) string {
	switch mimeType {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/bmp":
		return ".bmp"
	case "application/pdf":
		return ".pdf"
	default:
		return ".bin"
	}
}
