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

// Save 保存文件到本地磁盘
// fileName 既可以是简单文件名（如 "invoice.jpg"，此时自动按日期分目录），
// 也可以是完整相对路径（如 "EMP001/2026/07/07/uuid.jpg"，此时直接使用该路径）
func (s *LocalFileStorage) Save(_ context.Context, fileName string, _ string, reader io.Reader) (*UploadedFile, error) {
	now := time.Now()

	// 解析路径：如果 fileName 包含路径分隔符，直接作为相对路径使用
	// 否则按日期自动分目录
	var relativePath string
	if filepath.Dir(fileName) != "." {
		// fileName 已是完整相对路径（含用户ID前缀），直接使用
		relativePath = fileName
	} else {
		// 简单文件名：按日期自动分目录
		fileID := uuid.New().String()
		ext := filepath.Ext(fileName)
		if ext == "" {
			ext = ".bin"
		}
		relativePath = filepath.Join(now.Format("2006/01/02"), fileID+ext)
	}

	fullPath := filepath.Join(s.baseDir, relativePath)

	// 确保父目录存在
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return nil, fmt.Errorf("创建存储目录失败: %w", err)
	}

	f, err := os.Create(fullPath)
	if err != nil {
		return nil, fmt.Errorf("创建文件失败: %w", err)
	}
	defer f.Close()

	written, err := io.Copy(f, reader)
	if err != nil {
		os.Remove(fullPath)
		return nil, fmt.Errorf("写入文件失败: %w", err)
	}

	return &UploadedFile{
		FileID:    uuid.New().String(),
		FileName:  filepath.Base(fileName),
		MimeType:  "", // 从调用方传入，不在此处存储
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
