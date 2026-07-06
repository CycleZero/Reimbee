package infra

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// MinIOFileStorage MinIO（S3 兼容）文件存储实现
// 演示阶段为骨架，实际接入需引入 github.com/minio/minio-go
type MinIOFileStorage struct {
	endpoint   string
	bucketName string
	useSSL     bool
	baseURL    string // MinIO 对外访问地址
}

// NewMinIOFileStorage 创建 MinIO 文件存储实例
func NewMinIOFileStorage(endpoint, bucketName string, useSSL bool, baseURL string) *MinIOFileStorage {
	return &MinIOFileStorage{
		endpoint:   endpoint,
		bucketName: bucketName,
		useSSL:     useSSL,
		baseURL:    baseURL,
	}
}

// Save 保存文件到 MinIO
func (s *MinIOFileStorage) Save(ctx context.Context, fileName string, mimeType string, reader io.Reader) (*UploadedFile, error) {
	fileID := uuid.New().String()
	now := time.Now()
	ext := filepath.Ext(fileName)
	if ext == "" {
		ext = mimeToExt(mimeType)
	}

	// MinIO 对象路径: 2026/07/04/uuid.jpg
	objectPath := filepath.Join(now.Format("2006/01/02"), fileID+ext)

	// TODO: 接入 minio-go SDK
	// _, err := s.client.PutObject(ctx, s.bucketName, objectPath, reader, -1,
	//     minio.PutObjectOptions{ContentType: mimeType})

	_ = objectPath
	return &UploadedFile{
		FileID:    fileID,
		FileName:  fileName,
		MimeType:  mimeType,
		Size:      0, // TODO: 从 PutObject 返回值获取
		URL:       fmt.Sprintf("%s/%s/%s", s.baseURL, s.bucketName, objectPath),
		Path:      objectPath,
		CreatedAt: now.Format(time.RFC3339),
	}, fmt.Errorf("MinIO 存储暂未实现（需引入 minio-go SDK）")
}

// Get 从 MinIO 读取文件
func (s *MinIOFileStorage) Get(ctx context.Context, fileID string) (io.ReadCloser, error) {
	// TODO: s.client.GetObject(ctx, s.bucketName, fileID, minio.GetObjectOptions{})
	_ = fileID
	return nil, fmt.Errorf("MinIO 存储暂未实现")
}

// Delete 从 MinIO 删除文件
func (s *MinIOFileStorage) Delete(ctx context.Context, fileID string) error {
	// TODO: s.client.RemoveObject(ctx, s.bucketName, fileID, minio.RemoveObjectOptions{})
	_ = fileID
	return fmt.Errorf("MinIO 存储暂未实现")
}

// URL 返回 MinIO 文件访问 URL
func (s *MinIOFileStorage) URL(_ context.Context, fileID string) string {
	return fmt.Sprintf("%s/%s/%s", s.baseURL, s.bucketName, fileID)
}
